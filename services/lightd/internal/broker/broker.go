// Package broker publishes scenes to MQTT.
//
// The broker is mqtt01 on the substrate's IoT Backend segment, not a tenant
// VM. MQTT clients always dial the broker, so whichever segment holds it must
// accept inbound - and the tenant fabric has no inbound path by design. IoT
// Backend is the segment already defined to accept exactly this, so lightd
// connects outbound to it from the tenant.
package broker

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Publisher is the surface the HTTP layer depends on, so it can be tested
// without a broker.
type Publisher interface {
	PublishScene(standID string, payload []byte) error
	Close()
}

// Config describes how to reach the broker.
type Config struct {
	URL      string // tcp://host:1883, tls://host:8883
	ClientID string
	Username string
	Password string
	CAFile   string

	// InsecureSkipVerify exists for bring-up against a self-signed broker.
	// It should never be set once mqtt01 has a real certificate.
	InsecureSkipVerify bool

	TopicPrefix string
	Timeout     time.Duration
}

// MQTT is a connected publisher, and optionally a subscriber to a topic of
// cover images and to one that says whether a cover has a look of its own.
type MQTT struct {
	client  mqtt.Client
	prefix  string
	timeout time.Duration

	mu         sync.Mutex
	coverTopic string
	covers     chan []byte
	lookTopic  string
	looks      chan []byte
}

// SceneTopic is where a stand listens for what to render.
func SceneTopic(prefix, standID string) string {
	return fmt.Sprintf("%s/lightstand/%s/scene", prefix, standID)
}

// StatusTopic is lightd's own presence topic. The stands own
// <prefix>/lightstand/<id>/status; this one says whether the publisher is
// alive, so a dark stand can be told apart from a dead daemon.
func StatusTopic(prefix string) string {
	return prefix + "/lightd/status"
}

// CoverTopic is where covers arrive, when lightd is told to listen for them.
// The name is relative to the prefix, like every topic the tenant declares,
// because the broker confines an account to its tenant's own tree.
func CoverTopic(prefix, name string) string {
	return prefix + "/" + name
}

// Connect dials the broker and announces presence.
func Connect(cfg Config) (*MQTT, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("broker: no url configured")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	status := StatusTopic(cfg.TopicPrefix)
	m := &MQTT{prefix: cfg.TopicPrefix, timeout: cfg.Timeout}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		// Keep retrying rather than exiting: lightd may well start before the
		// broker, and a unit that dies on boot ordering is a unit that needs
		// babysitting.
		SetConnectRetry(true).
		SetConnectRetryInterval(5*time.Second).
		SetConnectTimeout(cfg.Timeout).
		SetMaxReconnectInterval(30*time.Second).
		// Last will: if this process dies, the broker says so on its behalf.
		SetWill(status, "offline", 1, true).
		SetOnConnectHandler(func(c mqtt.Client) {
			c.Publish(status, 1, true, "online")
			// A session does not outlive a connection here, so a
			// subscription made once is gone after the first reconnect.
			// Making it again on every connect is what keeps covers
			// arriving, and it brings the retained cover with it.
			m.resubscribe(c)
		})

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

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(cfg.Timeout) {
		return nil, fmt.Errorf("broker: timed out connecting to %s", cfg.URL)
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("broker: connect %s: %w", cfg.URL, err)
	}

	m.client = client
	return m, nil
}

// SubscribeCovers starts delivering images published to the named topic,
// retained one first, to handle. It stays subscribed across reconnects.
//
// An emptied topic is delivered as an image of no bytes, which means "there
// is no cover", not "here is one".
//
// Covers are handled one at a time, off the client's own goroutine, and a
// cover that arrives while another is being handled replaces any that is
// waiting. Only the newest one matters: the handler asks palette for colors
// and publishes a scene, which is slow next to a burst of track changes, and
// working through a queue would light the room with covers already gone.
func (m *MQTT) SubscribeCovers(name string, handle func(image []byte)) error {
	if name == "" || handle == nil {
		return fmt.Errorf("broker: a cover subscription needs a topic and a handler")
	}

	m.mu.Lock()
	if m.covers != nil {
		m.mu.Unlock()
		return fmt.Errorf("broker: already subscribed to covers on %s", m.coverTopic)
	}
	m.coverTopic = CoverTopic(m.prefix, name)
	m.covers = make(chan []byte, 1)
	covers, topic := m.covers, m.coverTopic
	m.mu.Unlock()

	go func() {
		for image := range covers {
			handle(image)
		}
	}()

	token := m.client.Subscribe(topic, 1, m.onCover)
	if !token.WaitTimeout(m.timeout) {
		return fmt.Errorf("broker: timed out subscribing to %s", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("broker: subscribe %s: %w", topic, err)
	}
	return nil
}

// SubscribeLooks delivers each message on a topic under the prefix that says
// whether the current cover has a look of its own (ADR-0019), the retained
// one first. It stays subscribed across reconnects.
//
// Like covers, looks are handled one at a time off the client's goroutine,
// and only the newest waits: an old look is about a cover that has gone.
func (m *MQTT) SubscribeLooks(name string, handle func(payload []byte)) error {
	if name == "" || handle == nil {
		return fmt.Errorf("broker: a look subscription needs a topic and a handler")
	}

	m.mu.Lock()
	if m.looks != nil {
		m.mu.Unlock()
		return fmt.Errorf("broker: already subscribed to looks on %s", m.lookTopic)
	}
	m.lookTopic = CoverTopic(m.prefix, name)
	m.looks = make(chan []byte, 1)
	looks, topic := m.looks, m.lookTopic
	m.mu.Unlock()

	go func() {
		for payload := range looks {
			handle(payload)
		}
	}()

	token := m.client.Subscribe(topic, 1, m.onLook)
	if !token.WaitTimeout(m.timeout) {
		return fmt.Errorf("broker: timed out subscribing to %s", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("broker: subscribe %s: %w", topic, err)
	}
	return nil
}

func (m *MQTT) resubscribe(c mqtt.Client) {
	m.mu.Lock()
	cover, look := m.coverTopic, m.lookTopic
	m.mu.Unlock()
	if cover != "" {
		c.Subscribe(cover, 1, m.onCover)
	}
	if look != "" {
		c.Subscribe(look, 1, m.onLook)
	}
}

// onLook runs on the client's goroutine, so it only hands the message on.
func (m *MQTT) onLook(_ mqtt.Client, msg mqtt.Message) {
	if len(msg.Payload()) == 0 {
		return
	}
	latest(m.looks, append([]byte(nil), msg.Payload()...))
}

// latest puts v in a slot of one, replacing whatever was waiting there.
func latest(slot chan []byte, v []byte) {
	select {
	case slot <- v:
	default:
		select {
		case <-slot:
		default:
		}
		select {
		case slot <- v:
		default:
		}
	}
}

// onCover runs on the client's goroutine, so it only hands the image on.
func (m *MQTT) onCover(_ mqtt.Client, msg mqtt.Message) {
	// An empty message is how the topic is cleared, and is handed on as no
	// image at all: it is how the publisher says there is no cover now
	// (ADR-0024). It is never a cover, and the handler must not take it for
	// one.
	image := append([]byte(nil), msg.Payload()...)

	select {
	case m.covers <- image:
	default:
		// One is already waiting. Drop it for this one.
		select {
		case <-m.covers:
		default:
		}
		select {
		case m.covers <- image:
		default:
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
			return nil, fmt.Errorf("broker: read ca %q: %w", cfg.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("broker: no certificates found in %q", cfg.CAFile)
		}
		conf.RootCAs = pool
	}
	return conf, nil
}

// PublishScene sends a scene to one stand, retained.
//
// Retention is load-bearing, not an optimization. A stand that reboots - power
// blip, reflash, WiFi drop - pulls the current scene down on reconnect instead
// of sitting dark until the next record. Without it the failure mode is a
// black stand in the middle of an album.
func (m *MQTT) PublishScene(standID string, payload []byte) error {
	topic := SceneTopic(m.prefix, standID)
	token := m.client.Publish(topic, 1, true, payload)
	if !token.WaitTimeout(m.timeout) {
		return fmt.Errorf("broker: timed out publishing to %s", topic)
	}
	if err := token.Error(); err != nil {
		return fmt.Errorf("broker: publish %s: %w", topic, err)
	}
	return nil
}

// Close says goodbye properly, so the will is not fired for an orderly exit.
func (m *MQTT) Close() {
	if m.client != nil && m.client.IsConnected() {
		token := m.client.Publish(StatusTopic(m.prefix), 1, true, "offline")
		token.WaitTimeout(m.timeout)
		m.client.Disconnect(250)
	}
}
