package bus

import (
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/paho.mqtt.golang/packets"
)

// A broker of this test's own, for the one thing the development broker will
// not do: refuse a subscription. It answers a subscription to any topic with
// "refused" in it the way a broker answers an account that was not granted
// it, and otherwise says yes to everything.
type refusingBroker struct {
	listener net.Listener

	mu         sync.Mutex
	subscribed []string
	published  []string
}

func newRefusingBroker(t *testing.T) *refusingBroker {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	b := &refusingBroker{listener: l}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go b.serve(conn)
		}
	}()
	return b
}

func (b *refusingBroker) url() string { return "tcp://" + b.listener.Addr().String() }

func (b *refusingBroker) serve(conn net.Conn) {
	defer conn.Close()
	for {
		packet, err := packets.ReadPacket(conn)
		if err != nil {
			return
		}
		switch p := packet.(type) {
		case *packets.ConnectPacket:
			_ = packets.NewControlPacket(packets.Connack).Write(conn)

		case *packets.SubscribePacket:
			ack := packets.NewControlPacket(packets.Suback).(*packets.SubackPacket)
			ack.MessageID = p.MessageID
			b.mu.Lock()
			for _, topic := range p.Topics {
				if strings.Contains(topic, "refused") {
					ack.ReturnCodes = append(ack.ReturnCodes, subackFailure)
					continue
				}
				ack.ReturnCodes = append(ack.ReturnCodes, 1)
				b.subscribed = append(b.subscribed, topic)
			}
			b.mu.Unlock()
			_ = ack.Write(conn)

		case *packets.PublishPacket:
			b.mu.Lock()
			b.published = append(b.published, p.TopicName)
			b.mu.Unlock()
			if p.Qos > 0 {
				ack := packets.NewControlPacket(packets.Puback).(*packets.PubackPacket)
				ack.MessageID = p.MessageID
				_ = ack.Write(conn)
			}

		case *packets.PingreqPacket:
			_ = packets.NewControlPacket(packets.Pingresp).Write(conn)

		case *packets.DisconnectPacket:
			return
		}
	}
}

func (b *refusingBroker) seen() (subscribed, published []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.subscribed...), append([]string(nil), b.published...)
}

// The reason optional subscriptions exist. A grant is made in one place and
// the program that needs it deployed from another; a program refused one
// topic must carry on with everything else it does.
func TestARefusedOptionalSubscriptionDoesNotStopTheProgram(t *testing.T) {
	broker := newRefusingBroker(t)

	refused := make(chan string, 4)
	connected := make(chan struct{}, 4)
	b, err := Connect(Config{
		URL: broker.url(), ClientID: "optional", Timeout: 5 * time.Second,
		StatusTopics:   []string{"t/status"},
		OptionalTopics: []string{"t/refused/command", "t/granted/command"},
		OnRefused:      func(topic string, _ error) { refused <- topic },
		OnConnect:      func() { connected <- struct{}{} },
	}, []string{"t/source/+/+"}, func(Message) {})
	if err != nil {
		t.Fatalf("a refused optional subscription stopped the program: %v", err)
	}
	defer b.Close()

	select {
	case topic := <-refused:
		if topic != "t/refused/command" {
			t.Errorf("told that %q was refused", topic)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nothing was said about the refusal: a program that cannot hear commands must say so")
	}
	select {
	case topic := <-refused:
		t.Errorf("told that %q was refused, which was granted", topic)
	default:
	}
	select {
	case <-connected:
	case <-time.After(2 * time.Second):
		t.Error("OnConnect was not called")
	}

	// And it can still do what it is for.
	if err := b.Publish("t/current", []byte("x"), true); err != nil {
		t.Errorf("could not publish after the refusal: %v", err)
	}
	subscribed, published := broker.seen()
	if strings.Join(subscribed, " ") != "t/source/+/+ t/granted/command" &&
		strings.Join(subscribed, " ") != "t/granted/command t/source/+/+" {
		t.Errorf("subscribed to %v", subscribed)
	}
	if !contains(published, "t/status") || !contains(published, "t/current") {
		t.Errorf("published %v", published)
	}
}

// A required subscription is still required: that is the difference.
func TestOnlyOptionalSubscriptionsAreOptional(t *testing.T) {
	broker := newRefusingBroker(t)
	b, err := Connect(Config{URL: broker.url(), ClientID: "only-optional", Timeout: 5 * time.Second,
		OptionalTopics: []string{"t/refused/command"},
	}, nil, func(Message) {})
	if err != nil {
		t.Fatalf("a program with nothing but a refused optional subscription did not start: %v", err)
	}
	b.Close()

	if _, err := Connect(Config{URL: broker.url(), OptionalTopics: []string{"a"}}, nil, nil); err == nil {
		t.Error("accepted optional subscriptions with nothing to deliver them to")
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// Against the real broker: an optional subscription is a subscription, and
// is made again after a reconnect like any other.
func TestAnOptionalSubscriptionDelivers(t *testing.T) {
	url := testBroker(t)
	topic := "nptest-optional/command"

	got := make(chan Message, 4)
	sub, err := Connect(Config{URL: url, ClientID: id("optional"), OptionalTopics: []string{topic},
		OnRefused: func(topic string, err error) { t.Errorf("%s refused: %v", topic, err) },
	}, nil, func(m Message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Close()

	pub, err := Connect(Config{URL: url, ClientID: id("pub")}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer pub.Close()
	if err := pub.Publish(topic, []byte("next"), false); err != nil {
		t.Fatal(err)
	}

	msg := await(t, got, "a message on an optional subscription")
	if string(msg.Payload) != "next" || msg.Retained {
		t.Errorf("got %q retained=%v", msg.Payload, msg.Retained)
	}
}
