package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
	"github.com/cdeever/eds/services/nowplaying/internal/driver/turntable"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type recorder struct {
	mu      sync.Mutex
	online  []bool
	updates []driver.Update
}

func (r *recorder) Online(online bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.online = append(r.online, online)
}

func (r *recorder) Publish(u driver.Update) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, u)
}

func (r *recorder) seen() ([]bool, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]bool(nil), r.online...), len(r.updates)
}

// reporting is a driver that publishes once and then follows until told to stop.
func reporting(published chan<- struct{}) func(context.Context, driver.Sink) error {
	return func(ctx context.Context, sink driver.Sink) error {
		sink.Online(true)
		sink.Publish(driver.Update{State: contract.State{V: contract.Version, State: contract.Stopped}})
		close(published)
		<-ctx.Done()
		return ctx.Err()
	}
}

func TestADriverThatIsNotReadyDoesNotStopAnother(t *testing.T) {
	itunes, spotify := &recorder{}, &recorder{}
	published := make(chan struct{})
	jobs := []job{
		{name: "itunes", source: "itunes", err: errors.New("not paired yet")},
		{name: "spotify", source: "spotify", run: reporting(published)},
	}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() {
		finished <- supervise(ctx, quiet, jobs, map[string]driver.Sink{"itunes": itunes, "spotify": spotify})
	}()

	select {
	case <-published:
	case <-time.After(2 * time.Second):
		t.Fatal("the driver that could run did not")
	}
	select {
	case err := <-finished:
		t.Fatalf("the agent stopped with one driver still running: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// The one that could not start says so on the bus rather than saying nothing.
	if online, updates := itunes.seen(); len(online) != 1 || online[0] || updates != 0 {
		t.Fatalf("itunes: presence %v, %d updates", online, updates)
	}
	if online, updates := spotify.seen(); len(online) != 1 || !online[0] || updates != 1 {
		t.Fatalf("spotify: presence %v, %d updates", online, updates)
	}

	cancel()
	if err := <-finished; err != nil {
		t.Fatalf("being told to stop is not a failure: %v", err)
	}
}

func TestADriverThatGivesUpDoesNotStopAnother(t *testing.T) {
	itunes, spotify := &recorder{}, &recorder{}
	published := make(chan struct{})
	gaveUp := make(chan struct{})
	jobs := []job{
		{name: "itunes", source: "itunes", run: reporting(published)},
		{name: "spotify", source: "spotify", run: func(_ context.Context, sink driver.Sink) error {
			sink.Online(true)
			defer close(gaveUp)
			return errors.New("the authorization has ended")
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		finished <- supervise(ctx, quiet, jobs, map[string]driver.Sink{"itunes": itunes, "spotify": spotify})
	}()

	<-gaveUp
	<-published
	select {
	case err := <-finished:
		t.Fatalf("the agent stopped with one driver still running: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if online, _ := spotify.seen(); len(online) != 2 || online[1] {
		t.Fatalf("spotify: presence %v, want online then offline", online)
	}
}

func TestWithNothingLeftRunningTheAgentSaysSo(t *testing.T) {
	jobs := []job{
		{name: "itunes", source: "itunes", err: errors.New("not paired yet")},
		{name: "spotify", source: "spotify", run: func(context.Context, driver.Sink) error {
			return errors.New("the authorization has ended")
		}},
	}
	sinks := map[string]driver.Sink{"itunes": &recorder{}, "spotify": &recorder{}}

	err := supervise(context.Background(), quiet, jobs, sinks)
	if !errors.Is(err, errNothingRunning) {
		t.Fatalf("got %v, want errNothingRunning", err)
	}
}

func TestDriversAreChosenByName(t *testing.T) {
	t.Setenv("NP_STATE_DIR", t.TempDir())

	t.Setenv("NP_DRIVERS", "itunes, spotify")
	jobs, err := drivers(quiet)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 || jobs[0].name != "itunes" || jobs[1].source != "spotify" {
		t.Fatalf("got %+v", jobs)
	}
	// Nothing is paired or authorized in an empty directory. That is each
	// driver's own trouble, not a reason to refuse the list.
	for _, j := range jobs {
		if j.run != nil || j.err == nil {
			t.Fatalf("%s: run=%v err=%v", j.name, j.run != nil, j.err)
		}
	}

	// The turntable is one more, and no more ready than the others: nobody
	// has said which input to listen to.
	t.Setenv("NP_DRIVERS", "turntable")
	jobs, err = drivers(quiet)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].source != "turntable" || jobs[0].run != nil || jobs[0].err == nil || jobs[0].command != nil {
		t.Fatalf("got %+v", jobs)
	}
	for key, bad := range map[string]string{
		"NP_TURNTABLE_INTERVAL":   "1s",
		"NP_TURNTABLE_STOP_AFTER": "soon",
		"NP_TURNTABLE_SILENCE_DB": "55",
	} {
		t.Setenv(key, bad)
		if _, err := drivers(quiet); err == nil {
			t.Fatalf("%s=%q was accepted", key, bad)
		}
		os.Unsetenv(key)
	}

	for _, bad := range []string{"", "itunes,tidal", "spotify,spotify"} {
		t.Setenv("NP_DRIVERS", bad)
		if _, err := drivers(quiet); err == nil {
			t.Fatalf("NP_DRIVERS=%q was accepted", bad)
		}
	}

	// Two drivers cannot report under one name.
	t.Setenv("NP_DRIVERS", "itunes,spotify")
	t.Setenv("NP_SOURCE_ID", "spotify")
	if _, err := drivers(quiet); err == nil {
		t.Fatal("two drivers were allowed the same source id")
	}
}

func TestAPlayEventSaysWhereItPlayed(t *testing.T) {
	track := &contract.Track{Title: "Velcro Fly", Artist: "ZZ Top", Album: "Afterburner"}
	cases := map[string]contract.State{
		"playing on Spotify: ZZ Top - Velcro Fly [Afterburner]": {Source: "spotify", Player: "spotify", State: contract.Playing, Track: track},
		"paused on iTunes: ZZ Top - Velcro Fly [Afterburner]":   {Source: "itunes", Player: "itunes", State: contract.Paused, Track: track},
		"stopped on Spotify":             {Source: "spotify", Player: "spotify", State: contract.Stopped},
		"playing on kitchen: Velcro Fly": {Source: "kitchen", State: contract.Playing, Track: &contract.Track{Title: "Velcro Fly"}},
	}
	for want, state := range cases {
		if got := sentence(state); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

// --- commands (ADR-0020) -----------------------------------------------------

func TestACommandOnTheBusIsCarriedOut(t *testing.T) {
	var done []contract.Command
	handle := commander(context.Background(), quiet, "itunes", func(_ context.Context, c contract.Command) error {
		done = append(done, c)
		return nil
	})

	handle(bus.Message{Topic: "eds/nowplaying/source/itunes/command",
		Payload: []byte(`{"v":1,"command":"next","from":"cover-screen-01"}`)})

	if len(done) != 1 || done[0].Command != contract.Next || done[0].From != "cover-screen-01" {
		t.Errorf("carried out %+v", done)
	}
}

// What must never reach the player: a command the broker kept, and anything
// that is not one.
func TestWhatTheAgentWillNotCarryOut(t *testing.T) {
	for name, msg := range map[string]bus.Message{
		"a retained command": {Payload: []byte(`{"v":1,"command":"next"}`), Retained: true},
		"an unknown command": {Payload: []byte(`{"v":1,"command":"shuffle"}`)},
		"another version":    {Payload: []byte(`{"v":2,"command":"next"}`)},
		"not a command":      {Payload: []byte("online")},
		"nothing":            {},
	} {
		t.Run(name, func(t *testing.T) {
			handle := commander(context.Background(), quiet, "itunes", func(context.Context, contract.Command) error {
				t.Error("carried out")
				return nil
			})
			handle(msg)
		})
	}
}

// A command is given a short while and no longer, and one that fails does
// not take the agent with it.
func TestACommandIsGivenADeadline(t *testing.T) {
	handle := commander(context.Background(), quiet, "itunes", func(ctx context.Context, _ contract.Command) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > commandTimeout {
			t.Errorf("deadline = %v, %v", deadline, ok)
		}
		return errors.New("the player is asleep")
	})
	handle(bus.Message{Payload: []byte(`{"v":1,"command":"play_pause"}`)})
}

// Only a driver that can do something about a command listens for one.
func TestOnlyTheITunesDriverTakesCommands(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NP_ITUNES_HOST", "")

	j, err := spotifyJob(quiet)
	if err != nil {
		t.Fatal(err)
	}
	if j.command != nil {
		t.Error("the Spotify driver takes commands: its authorization only reads (ADR-0016)")
	}
}

type fixedRecognizer struct {
	match turntable.Match
	err   error
}

func (f fixedRecognizer) Recognize(context.Context, []int16) (turntable.Match, error) {
	return f.match, f.err
}

func TestARecognizerIsCountedUnderTheServiceItNames(t *testing.T) {
	asked.Take()
	defer asked.Take()

	// Before it has answered, nobody knows whose it is.
	failing := &countedRecognizer{next: fixedRecognizer{err: errors.New("no network")}}
	failing.Recognize(context.Background(), nil)

	r := &countedRecognizer{next: fixedRecognizer{match: turntable.Match{Title: "x", Service: "somebody"}}}
	r.Recognize(context.Background(), nil)
	// Not knowing is an answer; and the name is remembered for a failure
	// that cannot give it.
	r.next = fixedRecognizer{match: turntable.Match{Service: "somebody"}, err: turntable.ErrNoMatch}
	r.Recognize(context.Background(), nil)
	r.next = fixedRecognizer{err: errors.New("no network")}
	r.Recognize(context.Background(), nil)

	got := asked.Take()
	if len(got) != 2 || got[0].API != "recognizer" || got[0].Calls != 1 || got[0].Failed != 1 {
		t.Fatalf("summaries = %+v", got)
	}
	if got[1].API != "somebody" || got[1].Calls != 3 || got[1].Failed != 1 {
		t.Fatalf("summaries = %+v", got)
	}
}
