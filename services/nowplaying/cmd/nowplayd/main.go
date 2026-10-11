// Command nowplayd decides what is playing.
//
// Drivers each report one player to the bus. nowplayd reads all of them,
// picks the one current track - most recent wins - fills in the album and the
// cover where the source had none, and publishes the result, retained, for
// whatever wants to react to it. It knows nothing about lights or stands
// (ADR-0006, ADR-0008).
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/current"
	"github.com/cdeever/eds/services/nowplaying/internal/hub"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
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

	client, err := bus.Connect(bus.Config{
		URL:                cfg.Broker.URL,
		ClientID:           cfg.Broker.ClientID,
		Username:           cfg.Broker.Username,
		Password:           cfg.Broker.Password,
		CAFile:             cfg.Broker.CAFile,
		InsecureSkipVerify: cfg.Broker.InsecureSkipVerify,
		Timeout:            cfg.Broker.Timeout,
		StatusTopics:       []string{contract.StatusTopic(cfg.Prefix)},
	}, []string{contract.AllSourcesTopic(cfg.Prefix)}, sources.Handle)
	if err != nil {
		return err
	}
	defer client.Close()

	logger.Info("connected to broker",
		"url", cfg.Broker.URL,
		"sources", contract.AllSourcesTopic(cfg.Prefix),
		"current", contract.CurrentTopic(cfg.Prefix),
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
