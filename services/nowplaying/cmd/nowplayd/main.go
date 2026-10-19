// Command nowplayd decides what is playing.
//
// Drivers each report one player to the bus. nowplayd reads all of them,
// picks the one current track - most recent wins - fills in the album and the
// cover where the source had none, and publishes the result, retained, for
// whatever wants to react to it. It knows nothing about lights or stands
// (ADR-0006, ADR-0008).
//
// It also carries the one thing that travels the other way: a controller's
// command, passed to the source that is current (ADR-0020).
//
// And it hears, for a source whose equipment has a beacon on its switch,
// whether that equipment is on (ADR-0023).
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/calls"
	"github.com/cdeever/eds/services/nowplaying/internal/command"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
	"github.com/cdeever/eds/services/nowplaying/internal/discogs"
	"github.com/cdeever/eds/services/nowplaying/internal/hub"
	"github.com/cdeever/eds/services/nowplaying/internal/logstore"
	"github.com/cdeever/eds/services/nowplaying/internal/looks"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
	"github.com/cdeever/eds/services/nowplaying/internal/smallcover"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("nowplayd exited", "error", err)
		os.Exit(1)
	}
}

// decision is what the publisher is asked to put on the bus: the winner, and
// the cover its own source supplied, if any.
type decision struct {
	winner *contract.State
	cover  *current.Image
	// idle: nothing is playing as far as a room is concerned (ADR-0024).
	idle bool
	// on: some source's equipment is switched on (ADR-0028).
	on bool
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadNowplayd(version)
	if err != nil {
		return err
	}

	// Everything asked of somebody else's service goes through a client
	// that counts it (ADR-0027).
	asked := calls.New()

	var resolver current.Resolver
	if len(cfg.Catalogs) > 0 {
		catalogs, err := resolve.Named(cfg.Catalogs, asked.Client(cfg.LookupTimeout), cfg.UserAgent)
		if err != nil {
			return err
		}
		r := resolve.New(catalogs, cfg.UserAgent, logger)
		r.HTTP = asked.Client(r.HTTP.Timeout)
		resolver = r
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	counted, err := reportCalls(ctx, asked, cfg, logger)
	if err != nil {
		return err
	}
	// After stop, so that the last minute's counts are sent on the way out.
	defer func() {
		stop()
		<-counted
	}()

	// Deciding is quick and publishing is not: completing a track can mean a
	// catalog lookup of several seconds. So the two are separate, joined by
	// a slot that holds one decision. A decision that arrives while another
	// is being published replaces any that is waiting, because only the
	// newest matters - working through a queue would publish, late and in
	// order, tracks that have already ended.
	decisions := make(chan decision, 1)
	submit := func(d decision) {
		select {
		case decisions <- d:
		default:
			select {
			case <-decisions:
			default:
			}
			select {
			case decisions <- d:
			default:
			}
		}
	}

	sources := hub.New(cfg.Prefix, func(d hub.Decision) { submit(decision{winner: d.Winner, cover: d.Cover, idle: d.Idle, on: d.On}) }, logger)
	sources.IdleAfter = cfg.IdleAfter
	sources.StoppedIdleAfter = cfg.StoppedIdleAfter
	sources.PresenceLapse = 3 * bus.DefaultPresenceEvery

	// Commands for whatever is current (ADR-0020). The router is given its
	// bus once there is one, and drops what it is handed before that.
	commands := &command.Router{Prefix: cfg.Prefix, Logger: logger}
	commandTopic := contract.CommandTopic(cfg.Prefix)

	// Whether a source's equipment is switched on, for the sources that have
	// a beacon to say so (ADR-0023). Anything a beacon says that is not
	// "online" is off, as with a source's own presence.
	switches := make(map[string][]string, len(cfg.Power)) // beacon topic to its sources
	optional := []string{commandTopic}
	for source, beacon := range cfg.Power {
		topic := contract.BeaconStatusTopic(cfg.Prefix, beacon)
		if len(switches[topic]) == 0 {
			optional = append(optional, topic)
		}
		switches[topic] = append(switches[topic], source)
	}

	switched := make(map[string]bool, len(switches)) // what each beacon last said; only handle touches it
	handle := func(msg bus.Message) {
		if msg.Topic == commandTopic {
			commands.Handle(msg)
			return
		}
		if held, ok := switches[msg.Topic]; ok {
			on := string(msg.Payload) == contract.Online
			// A beacon says it again every thirty seconds (ADR-0034). The
			// hub hears each repeat, which keeps it from lapsing; the log
			// says only a change.
			was, heard := switched[msg.Topic]
			switched[msg.Topic] = on
			for _, source := range held {
				if !heard || was != on {
					logger.Info("a source's equipment was switched", "source", source, "on", on, "topic", msg.Topic)
				}
				sources.Power(source, on)
			}
			return
		}
		// Everything else is a source's. The filter for those also matches
		// the command topics this program publishes to, which the hub takes
		// for what they are: not a source's report, and nothing to it.
		sources.Handle(msg)
	}

	client, err := bus.Connect(bus.Config{
		URL:                cfg.Broker.URL,
		ClientID:           cfg.Broker.ClientID,
		Username:           cfg.Broker.Username,
		Password:           cfg.Broker.Password,
		CAFile:             cfg.Broker.CAFile,
		InsecureSkipVerify: cfg.Broker.InsecureSkipVerify,
		Timeout:            cfg.Broker.Timeout,
		StatusTopics:       []string{contract.StatusTopic(cfg.Prefix)},
		// Passing commands on is something nowplayd can do without, and
		// saying what is playing is not. An account that has not been
		// granted the command topic yet must not stop the rest. The same
		// goes for a beacon: unheard, its source is arbitrated as it was
		// before there was one.
		OptionalTopics: optional,
		OnRefused: func(topic string, err error) {
			if topic == commandTopic {
				logger.Warn("commands will not be passed on until the broker account is granted the topic", "topic", topic, "error", err)
				return
			}
			logger.Warn("a source's switch will not be heard until the broker account is granted the topic", "sources", switches[topic], "topic", topic, "error", err)
		},
	}, []string{contract.AllSourcesTopic(cfg.Prefix)}, handle)
	if err != nil {
		return err
	}
	defer client.Close()
	commands.SetBus(client)
	sources.Settle()

	logger.Info("connected to broker",
		"url", cfg.Broker.URL,
		"sources", contract.AllSourcesTopic(cfg.Prefix),
		"current", contract.CurrentTopic(cfg.Prefix),
		"commands", commandTopic,
		"catalogs", cfg.Catalogs,
	)
	if len(cfg.Power) > 0 {
		logger.Info("these sources are held while their equipment is on, and unheard while it is off", "switches", cfg.Power)
	}

	coverless := make(map[string]bool, len(cfg.Coverless))
	for _, source := range cfg.Coverless {
		coverless[source] = true
	}
	if len(coverless) > 0 {
		logger.Info("reporting these sources without a cover", "sources", cfg.Coverless)
	}

	noControls := make(map[string]bool, len(cfg.NoControls))
	for _, source := range cfg.NoControls {
		noControls[source] = true
	}
	if len(noControls) > 0 {
		logger.Info("telling controllers to offer nothing for these sources", "sources", cfg.NoControls)
	}
	dark := make(map[string]bool, len(cfg.Dark))
	for _, source := range cfg.Dark {
		dark[source] = true
	}
	if len(dark) > 0 {
		logger.Info("publishing these sources' covers with the look that lights nothing", "sources", cfg.Dark)
	}
	if cfg.IdleAfter > 0 {
		logger.Info("a track paused this long is no longer said to be playing, and its cover is taken away", "after", cfg.IdleAfter)
	}
	if cfg.StoppedIdleAfter > 0 {
		logger.Info("a source held with nothing playing on it this long has its cover taken away", "after", cfg.StoppedIdleAfter)
	}

	publisher := &current.Publisher{
		Bus:        client,
		Resolver:   resolver,
		Coverless:  coverless,
		NoControls: noControls,
		Dark:       dark,
		AnySource:  looks.AnySource,
		Looks:      looks.For,
		Small:      smallcover.Make,
		Prefix:     cfg.Prefix,
		Timeout:    cfg.LookupTimeout,
		Logger:     logger,
	}
	// The owner's records, for the sources that play them (ADR-0022). The
	// token is never logged; whether there is one is.
	if len(cfg.Owned) > 0 {
		publisher.Owned = make(map[string]bool, len(cfg.Owned))
		for _, source := range cfg.Owned {
			publisher.Owned[source] = true
		}
		collection := &discogs.Collection{
			User:      cfg.DiscogsUser,
			Token:     cfg.DiscogsToken,
			HTTP:      asked.Client(cfg.LookupTimeout),
			UserAgent: cfg.UserAgent,
			Logger:    logger,
		}
		publisher.Collection = collection
		// For a track the collection does not have: the record already
		// playing if it has the track, or the original album the track is on
		// (ADR-0038). Deezer finds, MusicBrainz says what each album is.
		publisher.Albums = &resolve.Originals{
			Deezer:      "https://api.deezer.com",
			MusicBrainz: "https://musicbrainz.org",
			HTTP:        asked.Client(cfg.LookupTimeout),
			UserAgent:   cfg.UserAgent,
			Logger:      logger,
		}
		logger.Info("looking these sources up in the owner's collection first",
			"sources", cfg.Owned, "discogs_user", cfg.DiscogsUser, "token", cfg.DiscogsToken != "")
		// Listed now, and not when the first record is played. Not waited
		// for, and not fatal: a collection that cannot be read is said so,
		// and tracks are completed the ordinary way until it can.
		go func() {
			listing, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			if err := collection.Warm(listing); err != nil {
				logger.Warn("the owner's collection could not be read", "error", err)
			}
		}()
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil

		case d := <-decisions:
			var (
				published contract.State
				sent      bool
				err       error
			)
			if d.idle {
				published, sent, err = publisher.PublishIdle(ctx, d.winner, d.on)
			} else {
				published, sent, err = publisher.Publish(ctx, d.winner, d.cover)
			}
			if err != nil {
				// Not fatal: the bus reconnects by itself, and the next
				// decision publishes in full.
				logger.Warn("could not publish the current track", "error", err)
				continue
			}
			// Whoever is current is who a command is for, from now.
			commands.SetCurrent(published.Source)
			if sent {
				if d.idle {
					logger.Info("idle: nothing has played for a while, and the cover is taken away", "source", published.Source, "switched_on", d.on)
				}
				logCurrent(logger, published)
			}
		}
	}
}

// logCurrent says what subscribers were just told, as they were told it.
func logCurrent(logger *slog.Logger, st contract.State) {
	attrs := []any{"state", st.State, "source", st.Source}
	if st.Track != nil {
		attrs = append(attrs, "artist", st.Track.Artist, "album", st.Track.Album, "title", st.Track.Title)
	}
	if st.Art != nil {
		attrs = append(attrs, "art_from", st.Art.From)
	}
	logger.Info("current", attrs...)
}

// reportCalls says once a minute what was asked of each outside service: in
// the journal, and in the tenant's log store when the service has been given
// the way there, which is where a dashboard can add it up (ADR-0027). The
// channel it returns closes when the last minute has been said.
func reportCalls(ctx context.Context, asked *calls.Meter, cfg config.Nowplayd, logger *slog.Logger) (<-chan struct{}, error) {
	var store *logstore.Writer
	if cfg.LogEndpoint != "" && cfg.LogToken != "" {
		var err error
		if store, err = logstore.New(cfg.LogEndpoint, cfg.LogToken, "nowplayd", cfg.Broker.CAFile); err != nil {
			return nil, err
		}
		logger.Info("counting what is asked of outside services, for the log store", "endpoint", cfg.LogEndpoint, "every", calls.Every)
	} else {
		logger.Info("counting what is asked of outside services, in this log only: no log store was named", "every", calls.Every)
	}

	failing := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		asked.Run(ctx, calls.Every, func(s calls.Summary) {
			msg := fmt.Sprintf("%d calls to %s", s.Calls, s.API)
			logger.Info(msg, append([]any{"event", calls.Event}, s.Attrs()...)...)
			if store == nil {
				return
			}
			fields := map[string]any{"event": calls.Event, "level": "info"}
			attrs := s.Attrs()
			for i := 0; i+1 < len(attrs); i += 2 {
				fields[attrs[i].(string)] = attrs[i+1]
			}
			// Not ctx: the last of these is sent after it has ended.
			sending, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			err := store.Write(sending, msg, fields)
			cancel()
			// Said once for a run of failures, and once when it mends.
			switch {
			case err != nil && !failing:
				failing = true
				logger.Warn("the counts could not be written to the log store", "reason", err)
			case err == nil && failing:
				failing = false
				logger.Info("the counts are reaching the log store again")
			}
		})
	}()
	return done, nil
}
