package link

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"github.com/cdeever/eds/tools/ota/internal/image"
	"github.com/cdeever/eds/tools/ota/internal/send"
)

// A build through a real broker: eight-kilobyte messages at QoS 1, and the
// answers on a topic subscribed before the offer. The device here is a
// stand-in that takes pieces as the firmware does.
//
//	OTA_TEST_BROKER=tcp://127.0.0.1:21883 go test ./internal/link/
func TestABuildCrossesARealBroker(t *testing.T) {
	url := os.Getenv("OTA_TEST_BROKER")
	if url == "" {
		t.Skip("set OTA_TEST_BROKER to run broker integration tests")
	}
	stamp := time.Now().Format("150405.000000")
	topics := send.TopicsFor("otatest-"+stamp, "lightstand", "lp-stand-01")

	img := make([]byte, 5*send.Chunk+123)
	for i := range img {
		img[i] = byte(i*7 + i>>9)
	}
	desc := image.Desc{Project: "lp-stand", Version: "test", ELF: "0123456789abcdef"}

	written := stand(t, url, topics, stamp)

	conn, err := Dial(Config{URL: url, ClientID: "ota-test-sender-" + stamp}, topics.State)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	w := send.Waits{Ready: 5 * time.Second, Piece: 5 * time.Second, Tries: 3, Restart: 5 * time.Second}
	res, err := send.Send(context.Background(), conn, topics, img, desc, w, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Kept {
		t.Errorf("result %+v, want kept", res)
	}
	if got := <-written; !bytes.Equal(got, img) {
		t.Errorf("the stand-in wrote %d bytes, not the image", len(got))
	}
}

func stand(t *testing.T, url string, topics send.Topics, stamp string) <-chan []byte {
	t.Helper()
	written := make(chan []byte, 1)
	var (
		mu   sync.Mutex
		buf  []byte
		next int
		desc send.Offer
	)
	opts := mqtt.NewClientOptions().AddBroker(url).SetClientID("ota-test-stand-" + stamp).SetCleanSession(true)
	c := mqtt.NewClient(opts)
	if tok := c.Connect(); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
		t.Fatalf("stand-in connect: %v", tok.Error())
	}
	t.Cleanup(func() { c.Disconnect(100) })

	say := func(v any) {
		b, _ := json.Marshal(v)
		c.Publish(topics.State, 1, false, b)
	}
	handle := func(_ mqtt.Client, m mqtt.Message) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case strings.HasSuffix(m.Topic(), "/chunk"):
			p := m.Payload()
			if buf == nil || len(p) <= 4 {
				return
			}
			if off := int(binary.BigEndian.Uint32(p)); off == next && off+len(p)-4 <= len(buf) {
				copy(buf[off:], p[4:])
				next += len(p) - 4
			}
			if next == len(buf) {
				say(map[string]any{"v": 1, "state": "rebooting", "version": desc.Version, "elf": desc.ELF})
				say(map[string]any{"v": 1, "state": "running", "version": desc.Version, "elf": desc.ELF})
				written <- buf
				buf = nil
				return
			}
			say(map[string]any{"v": 1, "state": "receiving", "next": next})
		default:
			if err := json.Unmarshal(m.Payload(), &desc); err != nil {
				say(map[string]any{"v": 1, "state": "refused", "why": fmt.Sprint(err)})
				return
			}
			buf, next = make([]byte, desc.Size), 0
			say(map[string]any{"v": 1, "state": "ready", "next": 0})
		}
	}
	for _, topic := range []string{topics.Offer, topics.Chunk} {
		if tok := c.Subscribe(topic, 1, handle); !tok.WaitTimeout(5*time.Second) || tok.Error() != nil {
			t.Fatalf("stand-in subscribe: %v", tok.Error())
		}
	}
	return written
}
