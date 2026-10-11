package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/itunes"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/spotify"
	"github.com/cdeever/eds/services/nowplaying/internal/events"
	"github.com/cdeever/eds/services/nowplaying/internal/spotifyauth"
)

// errNothingRunning means every driver has stopped for a reason that starting
// again would not cure: nothing paired, nothing authorized.
var errNothingRunning = errors.New("no driver is running: each needs something a person has to do (see above)")

// exitNotConfigured is what the process exits with for errNothingRunning. It
// is sysexits' EX_CONFIG, and the unit is told not to restart on it: a
// restart would find the same missing pairing thirty seconds later, for ever.
const exitNotConfigured = 78

// A job is one driver, ready to run or not.
type job struct {
	name   string // the driver: itunes, spotify
	source string // what it is called on the bus
	// run follows the player until ctx ends, or until something a person has
	// to fix. Nil when the driver could not be set up at all; err says why.
	run func(ctx context.Context, sink driver.Sink) error
	err error
}

// drivers reads NP_DRIVERS and sets each one up as far as it will go. A
// driver that is not ready is not an error here: it is reported, and the
// others run without it.
func drivers(logger *slog.Logger) ([]job, error) {
	names := config.List("NP_DRIVERS", "itunes")
	if len(names) == 0 {
		return nil, errors.New("NP_DRIVERS names no driver: itunes, spotify, or both")
	}

	var jobs []job
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			return nil, fmt.Errorf("NP_DRIVERS names %q twice", name)
		}
		seen[name] = true

		var (
			j   job
			err error
		)
		switch name {
		case "itunes":
			j, err = itunesJob(logger)
		case "spotify":
			j, err = spotifyJob(logger)
		default:
			return nil, fmt.Errorf("NP_DRIVERS: there is no driver called %q (known: itunes, spotify)", name)
		}
		if err != nil {
			// A setting that cannot be read is a mistake to stop for. A
			// driver that is merely not ready comes back as a job with err.
			return nil, err
		}
		for _, other := range jobs {
			if other.source == j.source {
				return nil, fmt.Errorf("the %s and %s drivers would both report as %q", other.name, j.name, j.source)
			}
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func itunesJob(logger *slog.Logger) (job, error) {
	j := job{name: "itunes", source: config.Str("NP_SOURCE_ID", "itunes")}
	if !contract.ValidSourceID(j.source) {
		return j, fmt.Errorf("NP_SOURCE_ID=%q cannot name a source: lowercase letters, digits and hyphens", j.source)
	}
	kind := config.Str("NP_PLAYER", "itunes")
	if kind != "itunes" && kind != "music" {
		return j, fmt.Errorf("NP_PLAYER=%q: itunes or music", kind)
	}

	p, err := player()
	if err != nil {
		j.err = err
		return j, nil
	}
	j.run = func(ctx context.Context, sink driver.Sink) error {
		logger.Info("following the player", "driver", j.name, "player", fmt.Sprintf("%s:%d", p.Host, p.Port))
		d := &itunes.Driver{
			Source: j.source,
			Kind:   kind,
			GUID:   p.GUID,
			Player: dacp.NewClient(p.Host, p.Port),
			Sink:   sink,
			Logger: logger.With("driver", j.name),
		}
		return d.Run(ctx)
	}
	return j, nil
}

func spotifyTokenPath() (string, error) {
	dir, err := stateDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "spotify-token.json"), nil
}

// spotifyHTTP is the client for Spotify's API and its token endpoint. The
// timeout is the backstop; each request sets a tighter one of its own.
func spotifyHTTP() *http.Client { return &http.Client{Timeout: 30 * time.Second} }

func spotifyJob(logger *slog.Logger) (job, error) {
	j := job{name: "spotify", source: "spotify"}

	// Spotify publishes no limit, only that there is one. Five seconds is as
	// often as this will ask whatever it is told.
	interval, err := config.Duration("NP_SPOTIFY_INTERVAL", 10*time.Second, 5*time.Second, 5*time.Minute)
	if err != nil {
		return j, err
	}
	devices := config.List("NP_SPOTIFY_DEVICES", "")

	path, err := spotifyTokenPath()
	if err != nil {
		return j, err
	}
	tokens, err := spotifyauth.NewSource(path, spotifyHTTP())
	switch {
	case os.IsNotExist(err):
		j.err = errors.New("not authorized yet: run `npagent spotify-auth` first")
		return j, nil
	case err != nil:
		j.err = err
		return j, nil
	case len(devices) == 0:
		j.err = errors.New("NP_SPOTIFY_DEVICES names no device, so no playback would count: " +
			"`npagent spotify-devices` lists them")
		return j, nil
	}

	j.run = func(ctx context.Context, sink driver.Sink) error {
		logger.Info("following the account", "driver", j.name, "devices", devices, "every", interval,
			"authorized_until", tokens.Token().Expires().Format("2006-01-02"))
		d := &spotify.Driver{
			Source:   j.source,
			HTTP:     spotifyHTTP(),
			Tokens:   tokens,
			Sink:     sink,
			Logger:   logger.With("driver", j.name),
			Devices:  devices,
			Interval: interval,
		}
		return d.Run(ctx)
	}
	return j, nil
}

// supervise runs every job that can run and waits. One driver stopping does
// not stop another: an iTunes that was never paired is no reason for Spotify
// to go unreported. It returns nil when ctx ends, and errNothingRunning when
// the last driver has stopped by itself.
func supervise(ctx context.Context, logger *slog.Logger, jobs []job, sinks map[string]driver.Sink) error {
	var wg sync.WaitGroup
	for _, j := range jobs {
		sink := sinks[j.name]
		if j.run == nil {
			logger.Warn("driver is not running", events.Key, "driver.not_set_up", "driver", j.name, "reason", j.err)
			sink.Online(false)
			continue
		}
		wg.Add(1)
		go func(j job) {
			defer wg.Done()
			err := j.run(ctx, sink)
			if ctx.Err() == nil {
				// It gave up while the agent is still wanted: something only
				// a person can fix. Say so where it will be seen, both here
				// and on the bus.
				logger.Error("driver stopped", events.Key, "driver.stopped", "driver", j.name, "reason", err)
				sink.Online(false)
			}
		}(j)
	}
	wg.Wait()

	if ctx.Err() != nil {
		return nil
	}
	return errNothingRunning
}

// --- run ---------------------------------------------------------------------

// report sets the agent up to report its events to the tenant's log store,
// if it has been told what it is called there. It returns the logger to use
// either way, and a function that sends what is still waiting.
//
// A connection of its own, so that an event about a driver's connection does
// not depend on that connection.
func report(ctx context.Context, common config.Common, journal slog.Handler) (*slog.Logger, func(), error) {
	device := config.Str("NP_LOG_DEVICE", "")
	if device == "" {
		return slog.New(journal), func() {}, nil
	}
	if !contract.ValidSourceID(device) {
		return nil, nil, fmt.Errorf("NP_LOG_DEVICE=%q cannot name a device: lowercase letters, digits and hyphens", device)
	}

	cfg := common.Broker
	cfg.ClientID = common.Broker.ClientID + "-log"
	client, err := bus.Connect(cfg, nil, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("events: %w", err)
	}

	topic := events.Topic(common.Prefix, device)
	reporter := events.New(func(payload []byte) error { return client.Publish(topic, payload, false) })

	// It outlives ctx by one pass over its queue, so that the lines about
	// stopping are sent before the connection is closed.
	done := make(chan struct{})
	go func() {
		reporter.Run(ctx)
		close(done)
	}()
	logger := slog.New(reporter.Handler(journal))
	logger.Info("reporting events", "topic", topic)
	return logger, func() {
		<-done
		client.Close()
	}, nil
}

func run(ctx context.Context) error {
	journal := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})

	common, err := config.LoadCommon("eds-npagent")
	if err != nil {
		return err
	}
	// Stopping is two steps: the drivers are told to stop, and only once they
	// have is the reporter, so that what they say on the way out is sent.
	reporting, stopReporting := context.WithCancel(context.Background())
	logger, flush, err := report(reporting, common, journal)
	if err != nil {
		stopReporting()
		return err
	}
	defer func() {
		stopReporting()
		flush()
	}()

	jobs, err := drivers(logger)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(jobs))
	for _, j := range jobs {
		names = append(names, j.name)
	}
	logger.Info("agent started", events.Key, "agent.started", "drivers", strings.Join(names, ","))

	// A connection each. MQTT allows one last will per connection, and each
	// driver's presence is its own: the broker has to be able to say "spotify
	// is offline" without saying anything about iTunes.
	sinks := map[string]driver.Sink{}
	for _, j := range jobs {
		sink := &busSink{prefix: common.Prefix, source: j.source, logger: logger.With("driver", j.name)}

		cfg := common.Broker
		cfg.ClientID = common.Broker.ClientID + "-" + j.name
		// Presence is the driver's to announce, so nothing is announced for
		// it automatically; the will covers the case where it cannot speak.
		cfg.WillTopic = contract.SourceStatusTopic(common.Prefix, j.source)
		cfg.OnConnect = sink.announce

		client, err := bus.Connect(cfg, nil, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", j.name, err)
		}
		defer client.Close()
		sink.bus = client
		sinks[j.name] = sink

		logger.Info("connected to broker", "driver", j.name, "url", cfg.URL, "client_id", cfg.ClientID,
			"state", contract.SourceStateTopic(common.Prefix, j.source))
	}

	err = supervise(ctx, logger, jobs, sinks)
	if err == nil {
		logger.Info("shutting down", events.Key, "agent.stopping")
	}
	return err
}

// --- watch -------------------------------------------------------------------

// printSink shows what a driver would have published.
type printSink struct{ name string }

func (p printSink) Online(online bool) {
	word := contract.Offline
	if online {
		word = contract.Online
	}
	fmt.Printf("%s  %-8s %s\n", stamp(), p.name, word)
}

func (p printSink) Publish(u driver.Update) {
	line := string(u.State.State)
	if t := u.State.Track; t != nil {
		line = fmt.Sprintf("%-8s %s - %s  [%s]", u.State.State, t.Artist, t.Title, t.Album)
		if u.State.PositionMS != nil {
			line += fmt.Sprintf("  %s / %s", clock(uint32(*u.State.PositionMS)), clock(uint32(t.DurationMS)))
		}
	}
	fmt.Printf("%s  %-8s %s\n", stamp(), p.name, line)
}

// watch follows the same drivers `run` would and prints instead of
// publishing. The iTunes driver's watch is its own, older and more talkative:
// it shows the player's revisions and saves the cover, which is what is
// wanted when a pairing is being tried out.
func watch(ctx context.Context) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	jobs, err := drivers(logger)
	if err != nil {
		return err
	}

	sinks := map[string]driver.Sink{}
	for i := range jobs {
		sinks[jobs[i].name] = printSink{name: jobs[i].name}
		if jobs[i].name == "itunes" && jobs[i].run != nil {
			jobs[i].run = func(ctx context.Context, _ driver.Sink) error { return watchITunes(ctx) }
		}
	}
	fmt.Println("Watching. Nothing is published. Ctrl-C to stop.")
	fmt.Println()
	return supervise(ctx, logger, jobs, sinks)
}
