package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
	"github.com/cdeever/eds/services/nowplaying/internal/driver"
)

// asking is a driver that asks its service every few milliseconds for as
// long as it runs, and counts both its runs and what it asked.
type asking struct {
	runs, asked atomic.Int32
}

func (a *asking) run(ctx context.Context, sink driver.Sink) error {
	a.runs.Add(1)
	sink.Online(true)
	defer sink.Online(false)
	for {
		a.asked.Add(1)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func switchTo(t *testing.T, sw *driverSwitch, word string) {
	t.Helper()
	payload, err := contract.EncodeSwitch(word)
	if err != nil {
		t.Fatal(err)
	}
	sw.hear(quiet, "spotify", bus.Message{Topic: "eds/nowplaying/source/spotify/switch", Payload: payload, Retained: true})
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("never: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// still holds for a while, to show something did not happen.
func still(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for end := time.Now().Add(50 * time.Millisecond); time.Now().Before(end); time.Sleep(time.Millisecond) {
		if !ok() {
			t.Fatalf("not so: %s", what)
		}
	}
}

// ADR-0051: switched off, which is how it starts, the driver does not run
// and asks nothing, and the agent stays up to be switched on.
func TestASwitchedDriverIsOffUntilSwitchedOn(t *testing.T) {
	spotify := &asking{}
	sw := newSwitch()
	jobs := []job{{name: "spotify", source: "spotify", run: spotify.run, switched: sw}}

	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { finished <- supervise(ctx, quiet, jobs, map[string]driver.Sink{"spotify": &recorder{}}) }()

	still(t, "nothing asked while off", func() bool { return spotify.asked.Load() == 0 })
	select {
	case err := <-finished:
		t.Fatalf("the agent stopped with its one driver switched off: %v", err)
	default:
	}

	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "it runs once switched on", func() bool { return spotify.asked.Load() > 0 })

	cancel()
	if err := <-finished; err != nil {
		t.Fatalf("being told to stop is not a failure: %v", err)
	}
}

// Off ends the run, so the asking stops; on starts it again; the driver
// beside it is never touched.
func TestSwitchingOffStopsTheAskingAndLeavesTheOthersAlone(t *testing.T) {
	spotify, itunes := &asking{}, &asking{}
	sw := newSwitch()
	sink := &recorder{}
	jobs := []job{
		{name: "itunes", source: "itunes", run: itunes.run},
		{name: "spotify", source: "spotify", run: spotify.run, switched: sw},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go supervise(ctx, quiet, jobs, map[string]driver.Sink{"itunes": &recorder{}, "spotify": sink})

	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "running", func() bool { return spotify.asked.Load() > 0 })

	switchTo(t, sw, contract.SwitchOff)
	eventually(t, "said offline", func() bool {
		online, _ := sink.seen()
		return len(online) > 0 && !online[len(online)-1]
	})
	asked := spotify.asked.Load()
	still(t, "nothing asked once off", func() bool { return spotify.asked.Load() == asked })

	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "running again", func() bool { return spotify.runs.Load() == 2 && spotify.asked.Load() > asked })

	if n := itunes.runs.Load(); n != 1 {
		t.Errorf("iTunes was started %d times; switching Spotify is not iTunes' business", n)
	}
}

// The broker sends the retained setting on every reconnect. The same word
// again is no change, and does not restart a driver that is running.
func TestTheSameWordAgainChangesNothing(t *testing.T) {
	spotify := &asking{}
	sw := newSwitch()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go supervise(ctx, quiet, []job{{name: "spotify", source: "spotify", run: spotify.run, switched: sw}},
		map[string]driver.Sink{"spotify": &recorder{}})

	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "running", func() bool { return spotify.runs.Load() == 1 })
	switchTo(t, sw, contract.SwitchOn)
	still(t, "one run", func() bool { return spotify.runs.Load() == 1 })
}

// Anything that is not a setting, and a setting taken away, is off.
func TestAnUnreadableSwitchIsOff(t *testing.T) {
	for _, payload := range []string{`{"v":1,"driver":"yes"}`, `{"v":2,"driver":"on"}`, `on`, ``} {
		sw := newSwitch()
		switchTo(t, sw, contract.SwitchOn)
		sw.hear(quiet, "spotify", bus.Message{Payload: []byte(payload)})
		if sw.On() {
			t.Errorf("%q left it on", payload)
		}
	}
}

// A driver that gives up while on needs a person. It is not started again
// until it has been switched off and on.
func TestADriverThatGivesUpWaitsToBeSwitchedAgain(t *testing.T) {
	var runs atomic.Int32
	sw := newSwitch()
	sink := &recorder{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go supervise(ctx, quiet, []job{{name: "spotify", source: "spotify", switched: sw,
		run: func(context.Context, driver.Sink) error {
			runs.Add(1)
			return errors.New("the authorization has ended")
		}}}, map[string]driver.Sink{"spotify": sink})

	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "tried once", func() bool { return runs.Load() == 1 })
	still(t, "not tried again", func() bool { return runs.Load() == 1 })
	if online, _ := sink.seen(); len(online) == 0 || online[len(online)-1] {
		t.Errorf("presence %v, want offline after giving up", online)
	}

	switchTo(t, sw, contract.SwitchOff)
	switchTo(t, sw, contract.SwitchOn)
	eventually(t, "tried again after off and on", func() bool { return runs.Load() == 2 })
}

// A command while off fails, and is not kept for later.
func TestACommandWhileOffFails(t *testing.T) {
	var carried atomic.Int32
	sw := newSwitch()
	do := sw.gate(func(context.Context, contract.Command) error { carried.Add(1); return nil })

	next := contract.Command{Command: contract.Next}
	if err := do(context.Background(), next); !errors.Is(err, errSwitchedOff) {
		t.Fatalf("off: %v, want errSwitchedOff", err)
	}
	switchTo(t, sw, contract.SwitchOn)
	if err := do(context.Background(), next); err != nil {
		t.Fatalf("on: %v", err)
	}
	if n := carried.Load(); n != 1 {
		t.Errorf("carried out %d commands, want the one sent while on", n)
	}
}

// The connection hears the switch on its own topic and commands on theirs,
// and a command while off is refused there, before the driver.
func TestTheConnectionRoutesTheSwitchAndTheCommands(t *testing.T) {
	var carried atomic.Int32
	sw := newSwitch()
	j := job{name: "spotify", source: "spotify", switched: sw,
		command: func(context.Context, contract.Command) error { carried.Add(1); return nil }}

	topics, refused, handle := listens(context.Background(), quiet, "eds", j)
	if len(topics) != 2 || topics[0] != "eds/nowplaying/source/spotify/command" || topics[1] != "eds/nowplaying/source/spotify/switch" {
		t.Fatalf("topics %v", topics)
	}
	if refused == nil || handle == nil {
		t.Fatal("no handler")
	}

	next, _ := contract.EncodeCommand(contract.Command{Command: contract.Next})
	handle(bus.Message{Topic: topics[0], Payload: next})
	if carried.Load() != 0 {
		t.Fatal("a command was carried out while off")
	}
	on, _ := contract.EncodeSwitch(contract.SwitchOn)
	handle(bus.Message{Topic: topics[1], Payload: on, Retained: true})
	if !sw.On() {
		t.Fatal("the switch was not heard")
	}
	handle(bus.Message{Topic: topics[0], Payload: next})
	if carried.Load() != 1 {
		t.Fatalf("carried %d once on", carried.Load())
	}

	// iTunes has no switch, and hears only its commands.
	topics, _, _ = listens(context.Background(), quiet, "eds", job{name: "itunes", source: "itunes",
		command: func(context.Context, contract.Command) error { return nil }})
	if len(topics) != 1 || topics[0] != "eds/nowplaying/source/itunes/command" {
		t.Errorf("iTunes topics %v", topics)
	}
	// The turntable has neither, and subscribes to nothing.
	if topics, _, handle := listens(context.Background(), quiet, "eds", job{name: "turntable", source: "turntable"}); topics != nil || handle != nil {
		t.Errorf("the turntable listens to %v", topics)
	}
}
