// Command voiced hears spoken requests on the bus and asks for the album
// each one names (ADR-0042).
//
// A device sends what followed its wake word, as a WAV, on
// voice/<device>/utterance. voiced has scribe write it down, reads the words
// as a request, counts the artist's albums in MusicBrainz when the request
// names one by its place (ADR-0041), and sends play_album on
// nowplaying/command (ADR-0040). What it made of the request is said back on
// voice/<device>/heard.
//
// Settings, from the environment:
//
//	NP_MQTT_*, NP_TOPIC_PREFIX   the broker, as for nowplayd; client id eds-voiced
//	NP_SCRIBE_URL                scribe (default http://127.0.0.1:8733)
//	NP_CONTACT                   what MusicBrainz is told, as for nowplayd
//	NP_STATE_DIR                 where discographies are kept across restarts
//	                             (default: none; they are asked for again)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/calls"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/resolve"
	"github.com/cdeever/eds/services/nowplaying/internal/voice"
)

// version is stamped at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("voiced exited", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	common, err := config.LoadCommon("eds-voiced")
	if err != nil {
		return err
	}
	contact := config.Str("NP_CONTACT", "https://github.com/cdeever/eds")
	if contact == "" {
		return fmt.Errorf("config: NP_CONTACT must not be empty; MusicBrainz requires a way to reach the caller")
	}
	scribe := config.Str("NP_SCRIBE_URL", "http://127.0.0.1:8733")
	stateDir := config.Str("NP_STATE_DIR", "")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// MusicBrainz is somebody else's service, and counted (ADR-0027).
	asked := calls.New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		asked.Run(ctx, calls.Every, func(s calls.Summary) {
			logger.Info(fmt.Sprintf("%d calls to %s", s.Calls, s.API), append([]any{"event", calls.Event}, s.Attrs()...)...)
		})
	}()

	// The connection that delivers a request is the one that answers it, and
	// it can deliver before Connect has handed it over.
	var connected atomic.Pointer[bus.Bus]
	listener := &voice.Listener{
		Prefix: common.Prefix,
		Publish: func(topic string, payload []byte, retained bool) error {
			if c := connected.Load(); c != nil {
				return c.Publish(topic, payload, retained)
			}
			return errors.New("not connected yet")
		},
		// Writing down a few seconds of speech on two cores takes about
		// one; the model's first load, several.
		Scribe: voice.Scribe{URL: scribe, HTTP: &http.Client{Timeout: 60 * time.Second}},
		Albums: albums(stateDir, &resolve.Originals{
			MusicBrainz: "https://musicbrainz.org",
			HTTP:        asked.Client(10 * time.Second),
			UserAgent:   fmt.Sprintf("eds-voiced/%s ( %s )", version, contact),
			Logger:      logger,
		}, logger),
		Logger:  logger,
		Timeout: 90 * time.Second,
	}

	b := common.Broker
	b.StatusTopics = []string{voice.StatusTopic(common.Prefix)}
	// The woken filter is optional: an account not yet granted it hears
	// requests as before, and the music is not turned down for them.
	b.OptionalTopics = []string{voice.WokenFilter(common.Prefix)}
	b.OnRefused = func(topic string, err error) {
		logger.Warn("the music will not be turned down for a request until the broker account is granted the topic",
			"topic", topic, "error", err)
	}
	client, err := bus.Connect(b, []string{voice.UtteranceFilter(common.Prefix)}, listener.Handle)
	if err != nil {
		return err
	}
	connected.Store(client)
	logger.Info("voiced listening", "version", version, "broker", b.URL, "prefix", common.Prefix, "scribe", scribe,
		"state", stateDir)

	<-ctx.Done()
	client.Close()
	<-done
	return nil
}

// albums counts in MusicBrainz, keeping what it finds in dir when there is
// one, so that a restart does not forget it (CHG-0037).
func albums(dir string, musicbrainz *resolve.Originals, logger *slog.Logger) voice.Counter {
	if dir == "" {
		return musicbrainz
	}
	return &voice.Remembered{Next: musicbrainz, Path: filepath.Join(dir, "discographies.json"), Logger: logger}
}
