package broker

import (
	"os"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// These tests need a real broker, because the property they check - that a
// scene is still there for a client which was not connected when it was
// published - is a broker behaviour, not a client one. A fake would prove
// nothing.
//
//	LIGHTD_TEST_BROKER=tcp://127.0.0.1:21883 go test ./internal/broker/
func testBroker(t *testing.T) string {
	t.Helper()
	url := os.Getenv("LIGHTD_TEST_BROKER")
	if url == "" {
		t.Skip("set LIGHTD_TEST_BROKER to run broker integration tests")
	}
	return url
}

func TestTopics(t *testing.T) {
	if got := SceneTopic("eds", "lp-stand-01"); got != "eds/lightstand/lp-stand-01/scene" {
		t.Errorf("scene topic = %q", got)
	}
	if got := StatusTopic("eds"); got != "eds/lightd/status" {
		t.Errorf("status topic = %q", got)
	}
}

func TestConnectRequiresURL(t *testing.T) {
	if _, err := Connect(Config{}); err == nil {
		t.Error("expected an error with no broker url")
	}
}

// subscribe returns a channel of payloads for topic, from a client that
// connects fresh - which is the point.
func subscribe(t *testing.T, url, topic string) <-chan []byte {
	t.Helper()

	received := make(chan []byte, 8)
	opts := mqtt.NewClientOptions().
		AddBroker(url).
		SetClientID("test-sub-" + topic + "-" + time.Now().Format("150405.000000")).
		SetCleanSession(true)

	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("subscriber connect: %v", token.Error())
	}
	t.Cleanup(func() { client.Disconnect(100) })

	token := client.Subscribe(topic, 1, func(_ mqtt.Client, m mqtt.Message) {
		payload := make([]byte, len(m.Payload()))
		copy(payload, m.Payload())
		received <- payload
	})
	if !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("subscribe: %v", token.Error())
	}
	return received
}

func await(t *testing.T, ch <-chan []byte, what string) []byte {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		return nil
	}
}

// The load-bearing property. A stand that reboots - power blip, reflash, WiFi
// drop - must pull the current scene down on reconnect instead of sitting dark
// until the next record.
func TestSceneIsRetainedForLateSubscribers(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-retain"
	topic := SceneTopic(prefix, "lp-stand-01")

	pub, err := Connect(Config{URL: url, ClientID: "lightd-test-retain", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearRetained(t, url, topic)
		clearRetained(t, url, StatusTopic(prefix))
		pub.Close()
	})

	payload := []byte(`{"v":1,"effect":"breathe","palette":[{"rgb":[225,82,26],"weight":1}]}`)
	if err := pub.PublishScene("lp-stand-01", payload); err != nil {
		t.Fatal(err)
	}

	// Only now does the "stand" come online - after the scene was published.
	got := await(t, subscribe(t, url, topic), "retained scene")
	if string(got) != string(payload) {
		t.Errorf("retained payload = %s", got)
	}
}

func TestPublishReachesAConnectedSubscriber(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-live"
	topic := SceneTopic(prefix, "lp-stand-02")

	messages := subscribe(t, url, topic)

	pub, err := Connect(Config{URL: url, ClientID: "lightd-test-live", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearRetained(t, url, topic)
		clearRetained(t, url, StatusTopic(prefix))
		pub.Close()
	})

	if err := pub.PublishScene("lp-stand-02", []byte(`{"v":1,"effect":"solid"}`)); err != nil {
		t.Fatal(err)
	}
	if got := await(t, messages, "live scene"); string(got) != `{"v":1,"effect":"solid"}` {
		t.Errorf("payload = %s", got)
	}
}

// A dark stand and a dead publisher look identical from the room. The status
// topic is what tells them apart.
func TestPresenceIsAnnouncedAndWithdrawn(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-presence"
	status := StatusTopic(prefix)

	pub, err := Connect(Config{URL: url, ClientID: "lightd-test-presence", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { clearRetained(t, url, status) })

	if got := await(t, subscribe(t, url, status), "online status"); string(got) != "online" {
		t.Errorf("status = %s, want online", got)
	}

	pub.Close()

	if got := await(t, subscribe(t, url, status), "offline status"); string(got) != "offline" {
		t.Errorf("status after close = %s, want offline", got)
	}
}

// clearRetained removes a retained message so a rerun starts clean.
func clearRetained(t *testing.T, url, topic string) {
	t.Helper()
	opts := mqtt.NewClientOptions().AddBroker(url).
		SetClientID("test-clear-" + time.Now().Format("150405.000000"))
	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		return
	}
	defer client.Disconnect(100)
	client.Publish(topic, 1, true, []byte{}).WaitTimeout(2 * time.Second)
}
