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
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/command"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
	"github.com/cdeever/eds/services/nowplaying/internal/discogs"
	"github.com/cdeever/eds/services/nowplaying/internal/hub"
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
}

func run(logger *slog.Logger) error {
	cfg, err := config.LoadNowplayd(version)
	if err != nil {
		return err
	}

	var resolver current.Resolver
	if len(cfg.Catalogues) > 0 {
		catalogues, err := resolve.Named(cfg.Catalogues, &http.Client{Timeout: cfg.LookupTimeout}, cfg.UserAgent)
		if err != nil {
			return err
		}
		resolver = resolve.New(catalogues, cfg.UserAgent, logger)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Deciding is quick and publishing is not: completing a track can mean a
	// catalogue lookup of several seconds. So the two are separate, joined by
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

	sources := hub.New(cfg.Prefix, func(d hub.Decision) { submit(decision{winner: d.Winner, cover: d.Cover, idle: d.Idle}) }, logger)
	sources.IdleAfter = cfg.IdleAfter

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

	handle := func(msg bus.Message) {
		if msg.Topic == commandTopic {
			commands.Handle(msg)
			return
		}
		if held, ok := switches[msg.Topic]; ok {
			on := string(msg.Payload) == contract.Online
			for _, source := range held {
				logger.Info("a source's equipment was switched", "source", source, "on", on, "topic", msg.Topic)
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

	logger.Info("connected to broker",
		"url", cfg.Broker.URL,
		"sources", contract.AllSourcesTopic(cfg.Prefix),
		"current", contract.CurrentTopic(cfg.Prefix),
		"commands", commandTopic,
		"catalogues", cfg.Catalogues,
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
	if cfg.IdleAfter > 0 {
		logger.Info("a track paused this long is no longer said to be playing, and its cover is taken away", "after", cfg.IdleAfter)
	}

	publisher := &current.Publisher{
		Bus:        client,
		Resolver:   resolver,
		Coverless:  coverless,
		NoControls: noControls,
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
			HTTP:      &http.Client{Timeout: cfg.LookupTimeout},
			UserAgent: cfg.UserAgent,
			Logger:    logger,
		}
		publisher.Collection = collection
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
				published, sent, err = publisher.PublishIdle(ctx, d.winner)
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
					logger.Info("idle: nothing has played for a while, and the cover is taken away", "source", published.Source)
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
