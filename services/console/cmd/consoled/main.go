// Command consoled serves the EdS console: one page that shows what is
// playing, what each stand is showing, and whether the parts are up
// (ADR-0018).
//
// It subscribes to the bus and serves what it hears. It publishes its own
// presence, and the one setting the page can change: which sources the stand
// is lit for (ADR-0039).
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
	"strconv"
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

	// Read before anything is connected: a certificate that cannot be served
	// with is a reason to stop at once, not after the broker has answered.
	var identity *web.Identity
	certFile, keyFile := env("CONSOLE_TLS_CERT_FILE", ""), env("CONSOLE_TLS_KEY_FILE", "")
	switch {
	case (certFile == "") != (keyFile == ""):
		return errors.New("CONSOLE_TLS_CERT_FILE and CONSOLE_TLS_KEY_FILE go together: one is set and the other is not")
	case certFile != "":
		id, err := web.LoadIdentity(certFile, keyFile, time.Now())
		if err != nil {
			return err
		}
		identity = &id
	}

	leds := web.DefaultStripLEDs
	if raw, ok := os.LookupEnv("CONSOLE_STRIP_LEDS"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 300 {
			return fmt.Errorf("CONSOLE_STRIP_LEDS=%q is not a number of LEDs, 1 to 300", raw)
		}
		leds = n
	}

	picture := view.New(prefix)
	server := &web.Server{
		View:      picture,
		StripLEDs: leds,
		Version:   version,
		Logger:    logger,
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
		apply := picture.Apply
		if m.Retained {
			apply = picture.ApplyStored
		}
		if apply(m.Topic, m.Payload) {
			server.Changed()
		}
	})
	if err != nil {
		return err
	}
	defer client.Close()
	stand := view.StandTopic(prefix)
	server.Switch = func(sources string) error {
		payload, err := view.EncodeStand(sources)
		if err != nil {
			return err
		}
		// Retained: it is a setting, and outlives everyone who reads it.
		return client.Publish(stand, payload, true)
	}

	// A part that goes without a will says nothing; its silence is noticed
	// here (ADR-0034).
	go func() {
		for range time.Tick(5 * time.Second) {
			if picture.Expire() {
				server.Changed()
			}
		}
	}()
	logger.Info("connected to broker", "url", cfg.URL, "topics", len(view.Topics(prefix)), "status", status)

	addr := env("CONSOLE_ADDR", ":8080")
	// One server in the clear, or two: with a certificate the page is served
	// over TLS, and the plain port only sends people there.
	servers := []*http.Server{httpServer(addr, server.Handler())}
	serve := []func(*http.Server) error{(*http.Server).ListenAndServe}

	if identity != nil {
		id := *identity
		tlsAddr := env("CONSOLE_TLS_ADDR", ":443")
		server.Secure = true
		secure := httpServer(tlsAddr, server.Handler())
		secure.TLSConfig = id.TLSConfig()
		servers = []*http.Server{secure, httpServer(addr, web.ToHTTPS(id.Name))}
		serve = []func(*http.Server) error{
			func(s *http.Server) error { return s.ListenAndServeTLS("", "") },
			(*http.Server).ListenAndServe,
		}

		left := time.Until(id.NotAfter)
		logger.Info("serving over TLS", "addr", tlsAddr, "name", id.Name, "names", id.Names,
			"chain", id.Chain, "expires", id.NotAfter.UTC().Format("2006-01-02"), "days_left", int(left.Hours()/24))
		// The certificate is renewed by a person applying and deploying.
		// Nothing does it for them, so this is said every time it starts.
		if left < 30*24*time.Hour {
			logger.Warn("the certificate is close to expiring; apply the tenant's configuration and deploy again",
				"days_left", int(left.Hours()/24))
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	failed := make(chan error, len(servers))
	for i, s := range servers {
		go func() { failed <- serve[i](s) }()
		logger.Info("listening", "addr", s.Addr, "version", version)
	}

	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	logger.Info("shutting down")
	// Viewers hold requests open for ever, so they are not waited for.
	closing, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for _, s := range servers {
		if err := s.Shutdown(closing); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		_ = s.Close()
	}
	return nil
}

func httpServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// No write timeout: a viewer's request is meant never to end.
		MaxHeaderBytes: 16 << 10,
	}
}
