// Command consoled serves the EdS console: one page that shows what is
// playing, what each stand is showing, and whether the parts are up
// (ADR-0018).
//
// It subscribes to the bus and serves what it hears. It publishes one thing,
// its own presence, and changes nothing.
//
// Settings are all environment. On a Deevnet workload the broker's come from
// kit.env; a CONSOLE_* variable, when set, wins.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/cdeever/eds/services/console/internal/bus"
	"github.com/cdeever/eds/services/console/internal/view"
	"github.com/cdeever/eds/services/console/internal/web"
)

var version = "dev"

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// brokerURL is the broker kit.env names, over TLS; without one, the
// development broker.
func brokerURL() string {
	host := os.Getenv("MQTT_HOST")
	if host == "" {
		return "tcp://127.0.0.1:1883"
	}
	return "tls://" + host + ":" + env("MQTT_PORT", "8883")
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("stopping", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	prefix := env("CONSOLE_TOPIC_PREFIX", env("DEEVNET_TENANT", "eds"))
	if prefix == "" || strings.ContainsAny(prefix, "/#+$ ") {
		return fmt.Errorf("CONSOLE_TOPIC_PREFIX must be one topic level, got %q", prefix)
	}
	insecure := false
	if raw, ok := os.LookupEnv("CONSOLE_MQTT_INSECURE"); ok {
		switch raw {
		case "true", "1":
			insecure = true
		case "false", "0":
		default:
			return fmt.Errorf("CONSOLE_MQTT_INSECURE=%q is not a boolean", raw)
		}
	}

	picture := view.New(prefix)
	server := &web.Server{
		View:    picture,
		Version: version,
		Logger:  logger,
		Links: web.Links{
			Dashboard: env("CONSOLE_DASHBOARD_URL", ""),
			Docs:      env("CONSOLE_DOCS_URL", "https://cdeever.github.io/eds/"),
		},
	}

	status := prefix + "/console/status"
	cfg := bus.Config{
		URL:                env("CONSOLE_MQTT_URL", brokerURL()),
		ClientID:           env("CONSOLE_MQTT_CLIENT_ID", "eds-console"),
		Username:           env("CONSOLE_MQTT_USERNAME", env("MQTT_USERNAME", "")),
		Password:           env("CONSOLE_MQTT_PASSWORD", env("MQTT_PASSWORD", "")),
		CAFile:             env("CONSOLE_MQTT_CA_FILE", env("MQTT_CA_FILE", "")),
		InsecureSkipVerify: insecure,
		StatusTopics:       []string{status},
		Timeout:            10 * time.Second,
	}
	client, err := bus.Connect(cfg, view.Topics(prefix), func(m bus.Message) {
		if picture.Apply(m.Topic, m.Payload) {
			server.Changed()
		}
	})
	if err != nil {
		return err
	}
	defer client.Close()
	logger.Info("connected to broker", "url", cfg.URL, "topics", len(view.Topics(prefix)), "status", status)

	addr := env("CONSOLE_ADDR", ":8080")
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No write timeout: a viewer's request is meant never to end.
		MaxHeaderBytes: 16 << 10,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failed := make(chan error, 1)
	go func() { failed <- httpServer.ListenAndServe() }()
	logger.Info("listening", "addr", addr, "version", version)

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	// Viewers hold requests open for ever, so they are not waited for.
	closing, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(closing); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return httpServer.Close()
}
