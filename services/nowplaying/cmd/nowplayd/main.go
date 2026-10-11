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
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/command"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
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

	sources := hub.New(cfg.Prefix, func(d hub.Decision) { submit(decision{winner: d.Winner, cover: d.Cover}) }, logger)

	// Commands for whatever is current (ADR-0020). The router is given its
	// bus once there is one, and drops what it is handed before that.
	commands := &command.Router{Prefix: cfg.Prefix, Logger: logger}
	commandTopic := contract.CommandTopic(cfg.Prefix)
	handle := func(msg bus.Message) {
		if msg.Topic == commandTopic {
			commands.Handle(msg)
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
		// granted the command topic yet must not stop the rest.
		OptionalTopics: []string{commandTopic},
		OnRefused: func(topic string, err error) {
			logger.Warn("commands will not be passed on until the broker account is granted the topic", "topic", topic, "error", err)
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

	coverless := make(map[string]bool, len(cfg.Coverless))
	for _, source := range cfg.Coverless {
		coverless[source] = true
	}
	if len(coverless) > 0 {
		logger.Info("reporting these sources without a cover", "sources", cfg.Coverless)
	}

	publisher := &current.Publisher{
		Bus:       client,
		Resolver:  resolver,
		Coverless: coverless,
		Looks:     looks.For,
		Small:     smallcover.Make,
		Prefix:    cfg.Prefix,
		Timeout:   cfg.LookupTimeout,
		Logger:    logger,
	}

	for {
		select {
		case <-ctx.Done():
			logger.Info("shutting down")
			return nil

		case d := <-decisions:
			published, sent, err := publisher.Publish(ctx, d.winner, d.cover)
			if err != nil {
				// Not fatal: the bus reconnects by itself, and the next
				// decision publishes in full.
				logger.Warn("could not publish the current track", "error", err)
				continue
			}
			// Whoever is current is who a command is for, from now.
			commands.SetCurrent(published.Source)
			if sent {
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
