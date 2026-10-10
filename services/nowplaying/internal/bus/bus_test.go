package bus

import (
	"os"
	"testing"
	"time"
)

// Broker tests are gated the way lightd's are: they need a real broker,
// because retention and the last will are the broker's behaviour and a fake
// would only confirm what the fake was written to do.
func testBroker(t *testing.T) string {
	t.Helper()
	url := os.Getenv("NP_TEST_BROKER")
	if url == "" {
		t.Skip("set NP_TEST_BROKER to run broker integration tests")
	}
	return url
}

func id(name string) string { return "np-test-" + name + "-" + time.Now().Format("150405.000000") }

func await(t *testing.T, ch <-chan Message, what string) Message {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return Message{}
	}
}

func clear(t *testing.T, url string, topics ...string) {
	t.Helper()
	b, err := Connect(Config{URL: url, ClientID: id("clear")}, nil, nil)
	if err != nil {
		return
	}
	defer b.Close()
	for _, topic := range topics {
		_ = b.Publish(topic, nil, true)
	}
}

func TestConnectRequiresAURL(t *testing.T) {
	if _, err := Connect(Config{}, nil, nil); err == nil {
		t.Error("connected to nowhere")
	}
}

func TestSubscriptionsNeedAHandler(t *testing.T) {
	if _, err := Connect(Config{URL: "tcp://127.0.0.1:1"}, []string{"a"}, nil); err == nil {
		t.Error("accepted subscriptions with nothing to deliver them to")
	}
}

// The load-bearing property, again: state published before a program starts
// must be waiting for it when it does.
func TestRetainedStateReachesALateSubscriber(t *testing.T) {
	url := testBroker(t)
	topic := "nptest-retain/nowplaying/current"
	t.Cleanup(func() { clear(t, url, topic) })

	pub, err := Connect(Config{URL: url, ClientID: id("pub")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	if err := pub.Publish(topic, []byte(`{"v":1,"state":"playing"}`), true); err != nil {
		t.Fatal(err)
	}

	got := make(chan Message, 4)
	sub, err := Connect(Config{URL: url, ClientID: id("sub")}, []string{topic}, func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	msg := await(t, got, "retained state")
	if string(msg.Payload) != `{"v":1,"state":"playing"}` || !msg.Retained {
		t.Errorf("got %q retained=%v", msg.Payload, msg.Retained)
	}
}

func TestPresenceIsAnnouncedAndWithdrawnOnACleanClose(t *testing.T) {
	url := testBroker(t)
	one, two := "nptest-presence/a/status", "nptest-presence/b/status"
	t.Cleanup(func() { clear(t, url, one, two) })

	got := make(chan Message, 8)
	watch, err := Connect(Config{URL: url, ClientID: id("watch")}, []string{"nptest-presence/+/status"}, func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	b, err := Connect(Config{URL: url, ClientID: id("agent"), StatusTopics: []string{one, two}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]string{}
	for len(seen) < 2 {
		m := await(t, got, "online")
		seen[m.Topic] = string(m.Payload)
	}
	if seen[one] != "online" || seen[two] != "online" {
		t.Fatalf("presence = %v", seen)
	}

	b.Close()

	seen = map[string]string{}
	for len(seen) < 2 {
		m := await(t, got, "offline")
		seen[m.Topic] = string(m.Payload)
	}
	if seen[one] != "offline" || seen[two] != "offline" {
		t.Errorf("after close, presence = %v", seen)
	}
}

// A handler that publishes and waits must not deadlock: it runs off the
// client's goroutine precisely so that it can.
func TestAHandlerMayPublish(t *testing.T) {
	url := testBroker(t)
	in, out := "nptest-relay/in", "nptest-relay/out"

	got := make(chan Message, 4)
	watch, err := Connect(Config{URL: url, ClientID: id("watch")}, []string{out}, func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	var relay *Bus
	ready := make(chan struct{})
	relay, err = Connect(Config{URL: url, ClientID: id("relay")}, []string{in}, func(m Message) {
		<-ready
		if err := relay.Publish(out, m.Payload, false); err != nil {
			t.Errorf("publish from a handler: %v", err)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()
	close(ready)

	if err := watch.Publish(in, []byte("through"), false); err != nil {
		t.Fatal(err)
	}
	if msg := await(t, got, "the relayed message"); string(msg.Payload) != "through" {
		t.Errorf("relayed %q", msg.Payload)
	}
}

func TestMessagesArriveInOrder(t *testing.T) {
	url := testBroker(t)
	topic := "nptest-order/t"

	got := make(chan Message, 64)
	sub, err := Connect(Config{URL: url, ClientID: id("sub")}, []string{topic}, func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	for i := 0; i < 20; i++ {
		if err := sub.Publish(topic, []byte{byte('a' + i)}, false); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 20; i++ {
		if msg := await(t, got, "message"); msg.Payload[0] != byte('a'+i) {
			t.Fatalf("message %d was %q", i, msg.Payload)
		}
	}
}
