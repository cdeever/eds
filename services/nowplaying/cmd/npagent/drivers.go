package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/calls"
	"github.com/cdeever/eds/services/nowplaying/internal/config"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/dacp"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/itunes"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/spotify"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/turntable"
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
	name   string // the driver: itunes, spotify, turntable
	source string // what it is called on the bus
	// run follows the player until ctx ends, or until something a person has
	// to fix. Nil when the driver could not be set up at all; err says why.
	run func(ctx context.Context, sink driver.Sink) error
	err error
	// command does what a controller asked of this driver's player
	// (ADR-0020). Nil for a driver that takes no commands, which then
	// subscribes to nothing at all.
	command func(ctx context.Context, c contract.Command) error
}

// drivers reads NP_DRIVERS and sets each one up as far as it will go. A
// driver that is not ready is not an error here: it is reported, and the
// others run without it.
func drivers(logger *slog.Logger) ([]job, error) {
	names := config.List("NP_DRIVERS", "itunes")
	if len(names) == 0 {
		return nil, errors.New("NP_DRIVERS names no driver: itunes, spotify, turntable")
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
		case "turntable":
			j, err = turntableJob(logger)
		default:
			return nil, fmt.Errorf("NP_DRIVERS: there is no driver called %q (known: itunes, spotify, turntable)", name)
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
	// Built here and not in run, because a command arrives from the bus on a
	// goroutine of its own and has to find the same driver.
	d := &itunes.Driver{
		Source: j.source,
		Kind:   kind,
		GUID:   p.GUID,
		Player: dacp.NewClient(p.Host, p.Port),
		Logger: logger.With("driver", j.name),
	}
	j.run = func(ctx context.Context, sink driver.Sink) error {
		logger.Info("following the player", "driver", j.name, "player", fmt.Sprintf("%s:%d", p.Host, p.Port))
		d.Sink = sink
		return d.Run(ctx)
	}
	j.command = d.Command
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
//
// It counts what it asks (ADR-0027), as everything here that asks somebody
// else's service does.
func spotifyHTTP() *http.Client { return asked.Client(30 * time.Second) }

// asked counts what the drivers ask of outside services. The agent says
// what it holds once a minute, as an event; a command that runs once and
// exits, such as spotify-auth, counts and tells nobody.
var asked = calls.New()

// countedRecognizer counts each clip a recognizer is asked about. Whose
// service that is, the recognizer says itself in its answer; until one has
// answered, it is "recognizer".
type countedRecognizer struct {
	next turntable.Recognizer
	mu   sync.Mutex
	name string
}

func (c *countedRecognizer) Recognize(ctx context.Context, clip []int16) (turntable.Match, error) {
	started := time.Now()
	m, err := c.next.Recognize(ctx, clip)
	took := time.Since(started)

	c.mu.Lock()
	if m.Service != "" {
		c.name = m.Service
	}
	name := c.name
	c.mu.Unlock()
	if name == "" {
		name = "recognizer"
	}

	// Not knowing the track is an answer. Not being able to ask is not.
	outcome := calls.OK
	if err != nil && !errors.Is(err, turntable.ErrNoMatch) {
		outcome = calls.Failed
	}
	asked.Record(name, outcome, took)
	return m, err
}

// The Spotify driver takes no commands, and so has none here: it reads the
// account with scopes that only read (ADR-0012, ADR-0016), and a command for
// it would need more than it was authorized for.
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

// The turntable driver takes no commands either: there is nothing on a
// turntable for one to reach.
func turntableJob(logger *slog.Logger) (job, error) {
	j := job{name: "turntable", source: "turntable"}

	// The recognizer publishes no limit. Ten seconds is as often as this
	// will ask it whatever it is told; twenty is three clips a minute.
	interval, err := config.Duration("NP_TURNTABLE_INTERVAL", 20*time.Second, 10*time.Second, 5*time.Minute)
	if err != nil {
		return j, err
	}
	// Five seconds is longer than the gap between two tracks. Less would
	// make every gap a stop.
	stopAfter, err := config.Duration("NP_TURNTABLE_STOP_AFTER", 20*time.Second, 5*time.Second, 5*time.Minute)
	if err != nil {
		return j, err
	}
	silence := turntable.DefaultSilenceDB
	if v, ok := os.LookupEnv("NP_TURNTABLE_SILENCE_DB"); ok {
		silence, err = strconv.ParseFloat(v, 64)
		if err != nil || silence > -10 || silence < -90 {
			return j, fmt.Errorf("NP_TURNTABLE_SILENCE_DB=%q: a level in dBFS between -90 and -10", v)
		}
	}

	device := config.Str("NP_TURNTABLE_DEVICE", "")
	recognizer := config.Str("NP_TURNTABLE_RECOGNIZER", "/usr/local/bin/eds-recognize")
	if device == "" {
		j.err = errors.New("NP_TURNTABLE_DEVICE names no input to listen to: `arecord -L` lists them")
		return j, nil
	}
	if _, err := exec.LookPath("arecord"); err != nil {
		j.err = errors.New("arecord is not installed, and it is what listens")
		return j, nil
	}
	if _, err := exec.LookPath(recognizer); err != nil {
		j.err = fmt.Errorf("the recognizer cannot be run: %w", err)
		return j, nil
	}

	j.run = func(ctx context.Context, sink driver.Sink) error {
		d := &turntable.Driver{
			Source:     j.source,
			Capture:    turntable.ALSA{Device: device},
			Recognizer: &countedRecognizer{next: turntable.Command{Path: recognizer}},
			Sink:       sink,
			Logger:     logger.With("driver", j.name),
			Interval:   interval,
			SilenceDB:  silence,
			StopAfter:  stopAfter,
		}
		logger.Info("listening to the turntable", "driver", j.name, "device", device, "recognizer", recognizer, "rules", d.String())
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

		// A driver that takes commands listens for its own, and for nothing
		// else. Optional, because reporting what is playing must not wait on
		// a grant that is made somewhere else, at some other time.
		var handle func(bus.Message)
		if j.command != nil {
			topic := contract.SourceCommandTopic(common.Prefix, j.source)
			cfg.OptionalTopics = []string{topic}
			cfg.OnRefused = func(topic string, err error) {
				logger.Warn("driver will not hear commands until the broker account is granted the topic",
					events.Key, "driver.commands_refused", "driver", j.name, "topic", topic, "reason", err)
			}
			handle = commander(ctx, logger.With("driver", j.name), j.source, j.command)
		}

		client, err := bus.Connect(cfg, nil, handle)
		if err != nil {
			return fmt.Errorf("%s: %w", j.name, err)
		}
		defer client.Close()
		sink.bus = client
		sinks[j.name] = sink

		logger.Info("connected to broker", "driver", j.name, "url", cfg.URL, "client_id", cfg.ClientID,
			"state", contract.SourceStateTopic(common.Prefix, j.source))
	}

	// What was asked of outside services, a minute at a time (ADR-0027). It
	// ends with the drivers, and its last minute is said before the
	// reporter is stopped.
	counting, stopCounting := context.WithCancel(context.Background())
	counted := make(chan struct{})
	go func() {
		defer close(counted)
		asked.Run(counting, calls.Every, func(s calls.Summary) {
			logger.Info(fmt.Sprintf("%d calls to %s", s.Calls, s.API), append([]any{events.Key, calls.Event}, s.Attrs()...)...)
		})
	}()

	err = supervise(ctx, logger, jobs, sinks)
	stopCounting()
	<-counted
	if err == nil {
		logger.Info("shutting down", events.Key, "agent.stopping")
	}
	return err
}

// commandTimeout is how long a command may take. A press that has not
// reached the player in this long is not wanted any more: the hand that
// pressed it has pressed again, or given up.
const commandTimeout = 5 * time.Second

// commander is what a driver's connection does with a message on its command
// topic: carry it out, and say so where it will be seen. The player reporting
// the change is the answer on the bus; this is the answer in the log store,
// which is also where a command that did nothing is found.
func commander(ctx context.Context, logger *slog.Logger, source string,
	do func(context.Context, contract.Command) error) func(bus.Message) {
	return func(msg bus.Message) {
		// Never retained by anyone who follows the contract, and never
		// carried out if it was: a "next" the broker kept would skip a track
		// every time this agent reconnected.
		if msg.Retained {
			logger.Warn("dropped a retained command", events.Key, "driver.command_failed",
				"source", source, "reason", "a command is never retained")
			return
		}
		c, err := contract.DecodeCommand(msg.Payload)
		if err != nil {
			logger.Warn("dropped a command", events.Key, "driver.command_failed", "source", source, "reason", err)
			return
		}

		short, cancel := context.WithTimeout(ctx, commandTimeout)
		defer cancel()
		if err := do(short, c); err != nil {
			logger.Warn("command failed: "+c.Command, events.Key, "driver.command_failed",
				"source", source, "command", c.Command, "from", c.From, "reason", err)
			return
		}
		logger.Info("command: "+c.Command, events.Key, "driver.command",
			"source", source, "command", c.Command, "from", c.From)
	}
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
