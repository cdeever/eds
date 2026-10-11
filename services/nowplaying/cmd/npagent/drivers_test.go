package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
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
