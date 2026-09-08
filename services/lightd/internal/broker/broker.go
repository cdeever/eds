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

// MQTT is a connected publisher.
type MQTT struct {
	client  mqtt.Client
	prefix  string
	timeout time.Duration
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

// Connect dials the broker and announces presence.
func Connect(cfg Config) (*MQTT, error) {
	if cfg.URL == "" {
		return nil, fmt.Errorf("broker: no url configured")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	status := StatusTopic(cfg.TopicPrefix)

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

	return &MQTT{client: client, prefix: cfg.TopicPrefix, timeout: cfg.Timeout}, nil
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
// Retention is load-bearing, not an optimisation. A stand that reboots - power
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
