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

func TestCoverTopicIsUnderThePrefix(t *testing.T) {
	if got := CoverTopic("eds", "nowplaying/current/art"); got != "eds/nowplaying/current/art" {
		t.Errorf("cover topic = %q", got)
	}
}

// publishRaw puts bytes on a topic from a client of its own, the way
// nowplayd would.
func publishRaw(t *testing.T, url, topic string, payload []byte, retained bool) {
	t.Helper()
	opts := mqtt.NewClientOptions().AddBroker(url).
		SetClientID("test-pub-" + time.Now().Format("150405.000000"))
	client := mqtt.NewClient(opts)
	if token := client.Connect(); !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("publisher connect: %v", token.Error())
	}
	defer client.Disconnect(100)
	if token := client.Publish(topic, 1, retained, payload); !token.WaitTimeout(5*time.Second) || token.Error() != nil {
		t.Fatalf("publish: %v", token.Error())
	}
}

// The cover is retained for the same reason the scene is. A lightd that
// starts, or restarts, after the cover was published must still light the
// room from it rather than wait for the next album.
func TestRetainedCoverReachesALateSubscriber(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-cover-retain"
	topic := CoverTopic(prefix, "nowplaying/current/art")

	publishRaw(t, url, topic, []byte("COVERBYTES"), true)

	sub, err := Connect(Config{URL: url, ClientID: "lightd-test-cover-retain", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearRetained(t, url, topic)
		clearRetained(t, url, StatusTopic(prefix))
		sub.Close()
	})

	got := make(chan []byte, 8)
	if err := sub.SubscribeCovers("nowplaying/current/art", func(image []byte) { got <- image }); err != nil {
		t.Fatal(err)
	}

	if cover := await(t, got, "retained cover"); string(cover) != "COVERBYTES" {
		t.Errorf("cover = %q", cover)
	}
}

func TestCoversKeepArrivingAndAnEmptyOneIsNotACover(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-cover-live"
	topic := CoverTopic(prefix, "nowplaying/current/art")

	sub, err := Connect(Config{URL: url, ClientID: "lightd-test-cover-live", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearRetained(t, url, StatusTopic(prefix))
		sub.Close()
	})

	got := make(chan []byte, 8)
	if err := sub.SubscribeCovers("nowplaying/current/art", func(image []byte) { got <- image }); err != nil {
		t.Fatal(err)
	}

	publishRaw(t, url, topic, []byte("FIRST"), false)
	if cover := await(t, got, "first cover"); string(cover) != "FIRST" {
		t.Errorf("cover = %q", cover)
	}

	// Clearing a retained topic publishes nothing-at-all, which must not be
	// handed on as an image. The cover after it shows the subscription lived.
	publishRaw(t, url, topic, []byte{}, false)
	publishRaw(t, url, topic, []byte("SECOND"), false)
	if cover := await(t, got, "second cover"); string(cover) != "SECOND" {
		t.Errorf("cover = %q, want SECOND - the empty message was handed on", cover)
	}
}

// When covers arrive faster than they can be handled, the newest must win and
// the handler must never work through a backlog of covers already gone.
func TestASlowHandlerSeesTheNewestCoverNotABacklog(t *testing.T) {
	url := testBroker(t)
	prefix := "edstest-cover-burst"
	topic := CoverTopic(prefix, "nowplaying/current/art")

	sub, err := Connect(Config{URL: url, ClientID: "lightd-test-cover-burst", TopicPrefix: prefix})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		clearRetained(t, url, StatusTopic(prefix))
		sub.Close()
	})

	release := make(chan struct{})
	got := make(chan []byte, 16)
	first := true
	if err := sub.SubscribeCovers("nowplaying/current/art", func(image []byte) {
		got <- image
		if first {
			first = false
			<-release // hold the first one while the burst arrives
		}
	}); err != nil {
		t.Fatal(err)
	}

	publishRaw(t, url, topic, []byte("A"), false)
	if cover := await(t, got, "the cover being handled"); string(cover) != "A" {
		t.Fatalf("cover = %q", cover)
	}
	for _, c := range []string{"B", "C", "D", "E"} {
		publishRaw(t, url, topic, []byte(c), false)
	}
	time.Sleep(300 * time.Millisecond) // let the burst reach the client
	close(release)

	if cover := await(t, got, "the newest cover"); string(cover) != "E" {
		t.Errorf("after the burst the handler got %q, want E", cover)
	}
	select {
	case extra := <-got:
		t.Errorf("the handler was also given %q: a backlog was kept", extra)
	case <-time.After(300 * time.Millisecond):
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
