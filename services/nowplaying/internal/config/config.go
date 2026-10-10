// Package config reads the now-playing programs' settings from the
// environment.
//
// Environment rather than a file, for lightd's reasons: the broker
// credentials must never land on disk in a repository, and one binary has to
// run under systemd, in a container, or from a shell on a Mac without knowing
// which. Every broker setting falls back to the platform's names - the ones
// in the tenant's kit.env - and an NP_* variable, when set, wins.
//
// A value that cannot be used stops the program. A service that starts with a
// setting it quietly ignored is one that does the wrong thing for weeks.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cdeever/eds/services/nowplaying/internal/bus"
)

// Common is what both programs need: the broker and the topic prefix.
type Common struct {
	Broker bus.Config
	Prefix string
}

// LoadCommon reads the broker settings. clientID is the program's default,
// used when NP_MQTT_CLIENT_ID is not set.
func LoadCommon(clientID string) (Common, error) {
	insecure, err := Bool("NP_MQTT_INSECURE", false)
	if err != nil {
		return Common{}, err
	}

	c := Common{
		Broker: bus.Config{
			URL:      Str("NP_MQTT_URL", kitBrokerURL()),
			ClientID: Str("NP_MQTT_CLIENT_ID", clientID),
			Username: Str("NP_MQTT_USERNAME", Str("MQTT_USERNAME", "")),
			Password: Str("NP_MQTT_PASSWORD", Str("MQTT_PASSWORD", "")),
			CAFile:   Str("NP_MQTT_CA_FILE", Str("MQTT_CA_FILE", "")),
			// Bring-up escape hatch only.
			InsecureSkipVerify: insecure,
			Timeout:            10 * time.Second,
		},
		// The broker confines a tenant to topics under its own name.
		Prefix: Str("NP_TOPIC_PREFIX", Str("DEEVNET_TENANT", "eds")),
	}

	switch {
	case c.Broker.ClientID == "":
		return c, fmt.Errorf("config: NP_MQTT_CLIENT_ID must not be empty")
	case c.Prefix == "" || strings.ContainsAny(c.Prefix, "/#+$ "):
		return c, fmt.Errorf("config: NP_TOPIC_PREFIX must be one topic level, got %q", c.Prefix)
	}
	return c, nil
}

// kitBrokerURL is the broker kit.env names: MQTT_HOST over TLS, on MQTT_PORT
// or 8883. Without MQTT_HOST it is the development broker.
func kitBrokerURL() string {
	host, ok := os.LookupEnv("MQTT_HOST")
	if !ok || host == "" {
		return "tcp://127.0.0.1:1883"
	}
	return "tls://" + host + ":" + Str("MQTT_PORT", "8883")
}

// Str is an environment variable, or fallback when it is not set. A variable
// set to nothing is nothing, not the fallback: that is how a default is
// switched off.
func Str(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

// Bool parses a boolean setting. Unlike lightd's, an unparseable value is an
// error rather than the fallback: NP_MQTT_INSECURE=yes meaning "no" is the
// kind of surprise this package exists to prevent.
func Bool(key string, fallback bool) (bool, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback, fmt.Errorf("config: %s=%q is not a boolean", key, v)
	}
	return parsed, nil
}

// Duration parses a setting such as "10s", and refuses one outside its range.
func Duration(key string, fallback, min, max time.Duration) (time.Duration, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback, fmt.Errorf("config: %s=%q is not a duration such as 10s", key, v)
	}
	if parsed < min || parsed > max {
		return fallback, fmt.Errorf("config: %s must be between %s and %s, got %s", key, min, max, parsed)
	}
	return parsed, nil
}

// List splits a comma-separated setting, dropping empty items.
func List(key, fallback string) []string {
	var out []string
	for _, item := range strings.Split(Str(key, fallback), ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Nowplayd is everything cmd/nowplayd needs.
type Nowplayd struct {
	Common
	// Catalogues are where a missing album or cover is looked up, in order.
	// Empty means never: current is then only as complete as its source.
	Catalogues []string
	// UserAgent is what catalogues are told. They ask for a way to reach
	// whoever is calling, and one refuses callers that give none.
	UserAgent     string
	LookupTimeout time.Duration
}

// LoadNowplayd builds nowplayd's configuration.
func LoadNowplayd(version string) (Nowplayd, error) {
	common, err := LoadCommon("eds-nowplayd")
	if err != nil {
		return Nowplayd{}, err
	}

	cfg := Nowplayd{Common: common, Catalogues: List("NP_CATALOGUES", "deezer,itunes,musicbrainz")}

	contact := Str("NP_CONTACT", "https://github.com/cdeever/eds")
	if strings.TrimSpace(contact) == "" {
		return cfg, fmt.Errorf("config: NP_CONTACT must not be empty; catalogues require a way to reach the caller")
	}
	cfg.UserAgent = fmt.Sprintf("eds-nowplaying/%s ( %s )", version, contact)

	if cfg.LookupTimeout, err = Duration("NP_LOOKUP_TIMEOUT", 10*time.Second, time.Second, time.Minute); err != nil {
		return cfg, err
	}
	return cfg, nil
}
