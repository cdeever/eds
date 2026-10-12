package command

import (
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

type sent struct {
	topic    string
	payload  string
	retained bool
}

type fakeBus struct{ sent []sent }

func (f *fakeBus) Publish(topic string, payload []byte, retained bool) error {
	f.sent = append(f.sent, sent{topic, string(payload), retained})
	return nil
}

func router(b Bus) *Router {
	r := &Router{Prefix: "eds", Logger: quiet}
	r.SetBus(b)
	return r
}

// The connection that delivers a command is the one that carries it on, and
// it starts delivering before the router has been given it.
func TestACommandBeforeThereIsABusIsDropped(t *testing.T) {
	r := &Router{Prefix: "eds", Logger: quiet}
	r.SetCurrent("itunes")
	r.Handle(command(`{"v":1,"command":"next"}`)) // must not panic
}

func command(payload string) bus.Message {
	return bus.Message{Topic: "eds/nowplaying/command", Payload: []byte(payload)}
}

func TestACommandGoesToTheSourceThatIsCurrent(t *testing.T) {
	b := &fakeBus{}
	r := router(b)
	r.SetCurrent("itunes")

	// As it arrived, with a field this build has never heard of.
	payload := `{"v":1,"command":"next","from":"cover-screen-01","new":true}`
	r.Handle(command(payload))

	if len(b.sent) != 1 {
		t.Fatalf("published %d messages", len(b.sent))
	}
	got := b.sent[0]
	if got.topic != "eds/nowplaying/source/itunes/command" || got.payload != payload {
		t.Errorf("passed on as %s %s", got.topic, got.payload)
	}
	if got.retained {
		t.Error("a command was retained")
	}
}

func TestACommandFollowsTheCurrentSourceWhenItChanges(t *testing.T) {
	b := &fakeBus{}
	r := router(b)
	r.SetCurrent("itunes")
	r.SetCurrent("spotify")
	r.Handle(command(`{"v":1,"command":"play_pause"}`))

	if len(b.sent) != 1 || b.sent[0].topic != "eds/nowplaying/source/spotify/command" {
		t.Errorf("published %+v", b.sent)
	}
}

func TestWhatIsDropped(t *testing.T) {
	for name, c := range map[string]struct {
		source string
		msg    bus.Message
	}{
		"nothing is current":  {"", command(`{"v":1,"command":"next"}`)},
		"an unknown command":  {"itunes", command(`{"v":1,"command":"shuffle"}`)},
		"another version":     {"itunes", command(`{"v":2,"command":"next"}`)},
		"not a command":       {"itunes", command(`next`)},
		"a retained command":  {"itunes", bus.Message{Topic: "eds/nowplaying/command", Payload: []byte(`{"v":1,"command":"next"}`), Retained: true}},
		"an emptied retained": {"itunes", bus.Message{Topic: "eds/nowplaying/command", Retained: true}},
	} {
		t.Run(name, func(t *testing.T) {
			b := &fakeBus{}
			r := router(b)
			r.SetCurrent(c.source)
			r.Handle(c.msg)
			if len(b.sent) != 0 {
				t.Errorf("passed on: %+v", b.sent)
			}
		})
	}
}

// --- against a real broker ---------------------------------------------------
//
// Whether a message is handed over as retained is the broker's doing, so the
// rule that matters most here - a command the broker kept is not carried out
// when a program reconnects - is only proven against one.

func testBroker(t *testing.T) string {
	t.Helper()
	url := os.Getenv("NP_TEST_BROKER")
	if url == "" {
		t.Skip("set NP_TEST_BROKER to run broker integration tests")
	}
	return url
}

func id(name string) string { return "np-test-" + name + "-" + time.Now().Format("150405.000000") }

func connect(t *testing.T, cfg bus.Config, topics []string, handle func(bus.Message)) *bus.Bus {
	t.Helper()
	b, err := bus.Connect(cfg, topics, handle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestOnABrokerACommandReachesTheCurrentSourceAndARetainedOneDoesNot(t *testing.T) {
	url := testBroker(t)
	prefix := "nptest-" + time.Now().Format("150405.000000")
	commands := contract.CommandTopic(prefix)

	controller := connect(t, bus.Config{URL: url, ClientID: id("controller")}, nil, nil)

	// Something that does not follow the contract leaves a "next" with the
	// broker, before the router is running.
	stale, _ := contract.EncodeCommand(contract.Command{Command: contract.Next, From: "stale"})
	if err := controller.Publish(commands, stale, true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Publish(commands, nil, true) })

	// The driver's side: what arrives on the current source's command topic.
	arrived := make(chan bus.Message, 8)
	connect(t, bus.Config{URL: url, ClientID: id("driver")},
		[]string{contract.SourceCommandTopic(prefix, "itunes")}, func(m bus.Message) { arrived <- m })
	// And another source, which must hear nothing.
	other := make(chan bus.Message, 8)
	connect(t, bus.Config{URL: url, ClientID: id("other")},
		[]string{contract.SourceCommandTopic(prefix, "spotify")}, func(m bus.Message) { other <- m })

	// nowplayd's side, subscribed the way nowplayd subscribes. It is given
	// somewhere to pass commands on to before it hears any, so that the
	// retained one is dropped for being retained and for no other reason.
	r := &Router{Prefix: prefix, Logger: quiet}
	r.SetCurrent("itunes")
	r.SetBus(connect(t, bus.Config{URL: url, ClientID: id("router-out")}, nil, nil))
	refused := make(chan error, 1)
	connect(t, bus.Config{URL: url, ClientID: id("router"),
		OptionalTopics: []string{commands},
		OnRefused:      func(_ string, err error) { refused <- err },
	}, nil, r.Handle)
	select {
	case err := <-refused:
		t.Fatalf("the development broker refused the command topic: %v", err)
	default:
	}

	fresh, _ := contract.EncodeCommand(contract.Command{Command: contract.PlayPause, From: "fresh"})
	if err := controller.Publish(commands, fresh, false); err != nil {
		t.Fatal(err)
	}

	// The retained one was handed to the router first, on subscribing. If it
	// had been passed on it would be the first to arrive here.
	select {
	case msg := <-arrived:
		c, err := contract.DecodeCommand(msg.Payload)
		if err != nil {
			t.Fatalf("what arrived is not a command: %v", err)
		}
		if c.From != "fresh" || c.Command != contract.PlayPause {
			t.Errorf("the driver was handed %+v: the retained command was carried out", c)
		}
		if msg.Retained {
			t.Error("the command was passed on retained")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the command never reached the current source")
	}

	select {
	case msg := <-arrived:
		t.Errorf("a second command arrived: %s", msg.Payload)
	case msg := <-other:
		t.Errorf("a source that is not current was handed %s", msg.Payload)
	case <-time.After(300 * time.Millisecond):
	}
}
