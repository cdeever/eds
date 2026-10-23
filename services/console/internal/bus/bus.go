// Package bus is the console's MQTT connection: presence with a last will,
// and subscriptions that survive a reconnect.
//
// It follows nowplaying's bus package, which follows lightd's broker package;
// neither can be imported, because each is internal to its own module. The
// console mostly reads: what is kept here is the subscribing, and the one
// setting it may write (ADR-0039).
package bus

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Config describes how to reach the broker and how to announce presence.
type Config struct {
	URL      string // tcp://host:1883, tls://host:8883
	ClientID string
	Username string
	Password string
	CAFile   string

	// InsecureSkipVerify exists for bring-up against a self-signed broker.
	InsecureSkipVerify bool

	// StatusTopics each get "online" on connect and "offline" on a clean
	// close. The first is also the last will, so the broker says "offline"
	// for a process that dies without closing.
	StatusTopics []string

	// OnConnect is called after every connection, the first and each
	// reconnect, once presence is announced and subscriptions are made. A
	// program whose last will has just fired uses it to say what is true now.
	OnConnect func()

	// PresenceEvery is how often the status topics are said again; zero
	// means DefaultPresenceEvery.
	PresenceEvery time.Duration

	Timeout time.Duration
}

// DefaultPresenceEvery is how often presence is said again (ADR-0034). A
// retained online can outlive its program when the broker is down as it
// goes, so a reader believes one only for ninety seconds after hearing it.
const DefaultPresenceEvery = 30 * time.Second

// Message is one delivery from a subscription.
type Message struct {
	Topic    string
	Payload  []byte
	Retained bool
}

// Bus is a connected client.
type Bus struct {
	client  mqtt.Client
	cfg     Config
	inbound chan Message
	done    chan struct{}
}

// Connect dials the broker, announces presence, and subscribes to topics.
// Each message is passed to handle, one at a time and in the order received.
//
// handle runs on a goroutine of its own rather than the client's, so it may
// publish and wait for the result. The client's own goroutine must never do
// that: it is the one that would deliver the acknowledgment.
func Connect(cfg Config, topics []string, handle func(Message)) (*Bus, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("bus: no broker url configured")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if len(topics) > 0 && handle == nil {
		return nil, fmt.Errorf("bus: subscriptions need a handler")
	}

	b := &Bus{cfg: cfg, inbound: make(chan Message, 256), done: make(chan struct{})}

	onMessage := func(_ mqtt.Client, m mqtt.Message) {
		// The payload belongs to the client once this returns.
		msg := Message{Topic: m.Topic(), Payload: append([]byte(nil), m.Payload()...), Retained: m.Retained()}
		select {
		case b.inbound <- msg:
		case <-b.done:
		}
	}

	// Connect does not return until the first subscriptions are acknowledged.
	// Without that, a message published the moment after Connect returns can
	// reach the broker before the subscription does, and is simply not
	// delivered - which for a retained topic is hidden, and for anything else
	// is a message lost at startup with nothing to show for it.
	subscribed := make(chan error, 1)
	var first sync.Once

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		// Keep retrying rather than exiting: a unit that dies on boot
		// ordering is a unit that needs babysitting.
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetConnectTimeout(cfg.Timeout).
		SetMaxReconnectInterval(30 * time.Second).
		SetOnConnectHandler(func(c mqtt.Client) {
			for _, topic := range cfg.StatusTopics {
				c.Publish(topic, 1, true, "online")
			}
			// A session does not outlive a connection, so a subscription
			// made once is gone after the first reconnect. Making them
			// again here also brings every retained message back, which is
			// how a restarted program learns the state of the world.
			if len(topics) == 0 {
				first.Do(func() { subscribed <- nil })
				if cfg.OnConnect != nil {
					go cfg.OnConnect()
				}
				return
			}
			filters := make(map[string]byte, len(topics))
			for _, topic := range topics {
				filters[topic] = 1
			}
			token := c.SubscribeMultiple(filters, onMessage)
			go func() {
				var err error
				if !token.WaitTimeout(cfg.Timeout) {
					err = fmt.Errorf("bus: timed out subscribing")
				} else if token.Error() != nil {
					err = fmt.Errorf("bus: subscribe: %w", token.Error())
				}
				first.Do(func() { subscribed <- err })
				if err == nil && cfg.OnConnect != nil {
					cfg.OnConnect()
				}
			}()
		})

	if len(cfg.StatusTopics) > 0 {
		opts.SetWill(cfg.StatusTopics[0], "offline", 1, true)
	}
	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}

	tlsConfig, err := buildTLS(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig != nil {
		opts.SetTLSConfig(tlsConfig)
	}

	if handle != nil {
		go func() {
			for {
				select {
				case msg := <-b.inbound:
					handle(msg)
				case <-b.done:
					return
				}
			}
		}()
	}

	b.client = mqtt.NewClient(opts)
	token := b.client.Connect()
	if !token.WaitTimeout(cfg.Timeout) {
		return nil, fmt.Errorf("bus: timed out connecting to %s", cfg.URL)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("bus: connect %s: %w", cfg.URL, err)
	}

	select {
	case err := <-subscribed:
		if err != nil {
			b.Close()
			return nil, err
		}
	case <-time.After(cfg.Timeout):
		b.Close()
		return nil, fmt.Errorf("bus: timed out subscribing on %s", cfg.URL)
	}
	go b.repeatPresence()
	return b, nil
}

// repeatPresence says every status topic's "online" again every
// PresenceEvery while connected, until Close. QoS 0: a lost repeat is
// followed by another.
func (b *Bus) repeatPresence() {
	every := b.cfg.PresenceEvery
	if every <= 0 {
		every = DefaultPresenceEvery
	}
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		select {
		case <-b.done:
			return
		case <-tick.C:
			if !b.client.IsConnected() {
				continue
			}
			for _, topic := range b.cfg.StatusTopics {
				b.client.Publish(topic, 0, true, "online")
			}
		}
	}
}

func buildTLS(cfg Config) (*tls.Config, error) {
	if cfg.CAFile == "" && !cfg.InsecureSkipVerify {
		return nil, nil
	}

	conf := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: cfg.InsecureSkipVerify}
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("bus: read ca %q: %w", cfg.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("bus: no certificates found in %q", cfg.CAFile)
		}
		conf.RootCAs = pool
	}
	return conf, nil
}

// Publish sends one message at QoS 1 and waits for the broker to take it.
// A broker that has not granted the topic takes it all the same and drops
// it, so a caller learns what happened by hearing it back, not from this.
func (b *Bus) Publish(topic string, payload []byte, retained bool) error {
	token := b.client.Publish(topic, 1, retained, payload)
	if !token.WaitTimeout(b.cfg.Timeout) {
		return fmt.Errorf("bus: timed out publishing to %s", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("bus: publish to %s: %w", topic, err)
	}
	return nil
}

// Close says goodbye properly, so the will is not fired for an orderly exit.
func (b *Bus) Close() {
	if b.client != nil && b.client.IsConnected() {
		for _, topic := range b.cfg.StatusTopics {
			b.client.Publish(topic, 1, true, "offline").WaitTimeout(b.cfg.Timeout)
		}
		b.client.Disconnect(250)
	}
	select {
	case <-b.done:
	default:
		close(b.done)
	}
}
