// Package link is the broker connection the sender uses: one client,
// subscribed to one device's answers before anything is offered.
package link

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Config is how to reach the broker.
type Config struct {
	URL      string // tls://host:8883, or tcp://127.0.0.1:21883 for a dev broker
	CAFile   string
	Username string
	Password string
	ClientID string
}

// MQTT is a Link (package send) over paho.
type MQTT struct {
	client  mqtt.Client
	answers chan []byte
}

// Dial connects and subscribes to state.
func Dial(cfg Config, state string) (*MQTT, error) {
	if cfg.URL == "" {
		return nil, errors.New("link: no broker url")
	}
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.URL).
		SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetCleanSession(true).
		SetConnectTimeout(10 * time.Second).
		// An update takes a minute or so; one that loses its connection is
		// started again by a person, not resumed by the client.
		SetAutoReconnect(false)
	if cfg.CAFile != "" {
		pem, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, fmt.Errorf("link: read ca %q: %w", cfg.CAFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("link: no certificates in %q", cfg.CAFile)
		}
		opts.SetTLSConfig(&tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12})
	}

	m := &MQTT{answers: make(chan []byte, 64)}
	m.client = mqtt.NewClient(opts)
	if t := m.client.Connect(); !t.WaitTimeout(15*time.Second) || t.Error() != nil {
		return nil, fmt.Errorf("link: connect to %s: %v", cfg.URL, t.Error())
	}
	t := m.client.Subscribe(state, 1, func(_ mqtt.Client, msg mqtt.Message) {
		// Never retained by the device; one that is was left by someone else.
		if msg.Retained() {
			return
		}
		b := append([]byte(nil), msg.Payload()...)
		select {
		case m.answers <- b:
		default: // a sender that is not reading has stopped; nothing waits on this
		}
	})
	if !t.WaitTimeout(10*time.Second) || t.Error() != nil {
		m.client.Disconnect(100)
		return nil, fmt.Errorf("link: subscribe to %s: %v", state, t.Error())
	}
	return m, nil
}

// Publish sends at QoS 1, never retained, and waits for the broker to take it.
func (m *MQTT) Publish(topic string, payload []byte) error {
	t := m.client.Publish(topic, 1, false, payload)
	if !t.WaitTimeout(15 * time.Second) {
		return errors.New("the broker did not acknowledge it")
	}
	return t.Error()
}

// Answers delivers the device's answers.
func (m *MQTT) Answers() <-chan []byte { return m.answers }

// Close disconnects.
func (m *MQTT) Close() { m.client.Disconnect(250) }
