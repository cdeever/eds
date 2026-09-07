// Command lightd turns album cover art into light on an LP jacket stand.
//
// It takes a cover image over HTTP, asks the palette service for the colours
// in it, maps those onto a scene descriptor, and publishes that scene retained
// to the stand's MQTT topic.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cdeever/eds/services/lightd/internal/broker"
	"github.com/cdeever/eds/services/lightd/internal/config"
	"github.com/cdeever/eds/services/lightd/internal/palette"
	"github.com/cdeever/eds/services/lightd/internal/server"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("lightd exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	publisher, err := broker.Connect(cfg.Broker)
	if err != nil {
		return err
	}
	defer publisher.Close()

	logger.Info("connected to broker",
		"url", cfg.Broker.URL,
		"scene_topic", broker.SceneTopic(cfg.Broker.TopicPrefix, cfg.StandID),
		"status_topic", broker.StatusTopic(cfg.Broker.TopicPrefix),
	)

	handler := server.New(server.Options{
		Extractor:    palette.New(cfg.PaletteURL),
		Publisher:    publisher,
		TopicPrefix:  cfg.Broker.TopicPrefix,
		DefaultStand: cfg.StandID,
		Swatches:     cfg.Swatches,
		Scene:        cfg.Scene,
		Logger:       logger,
	}).Routes()

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() {
		logger.Info("listening", "addr", cfg.Addr, "palette", cfg.PaletteURL)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
	}

	// Give in-flight requests a moment, then let the deferred Close say
	// goodbye to the broker so the will is not fired for an orderly exit.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpServer.Shutdown(shutdownCtx)
}
