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
	"github.com/cdeever/eds/services/nowplaying/internal/contract"
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
	// Coverless names sources that are reported without a cover (ADR-0016).
	Coverless []string
	// Owned names sources that play the owner's own records, and so are
	// looked up in the owner's collection first (ADR-0022).
	Owned []string
	// DiscogsUser is whose collection that is. DiscogsToken is a personal
	// access token for it, needed when the collection is private.
	DiscogsUser, DiscogsToken string
	// Power names, for a source whose equipment has a beacon on its switch,
	// that beacon: source id to beacon id (ADR-0023).
	Power map[string]string
	// IdleAfter is how long a track may sit paused, or nothing be reporting,
	// before nothing is said to be playing and the cover is taken away
	// (ADR-0024). Zero is never.
	IdleAfter time.Duration
	// NoControls names sources a controller should offer nothing for
	// (ADR-0025).
	NoControls []string
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

	// A name here that could not be a source is a typo, and a typo here is a
	// source that lights the room when it was meant not to.
	cfg.Coverless = List("NP_COVERLESS_SOURCES", "")
	for _, source := range cfg.Coverless {
		if !contract.ValidSourceID(source) {
			return cfg, fmt.Errorf("config: NP_COVERLESS_SOURCES names %q, which cannot be a source", source)
		}
	}

	cfg.Owned = List("NP_COLLECTION_SOURCES", "")
	for _, source := range cfg.Owned {
		if !contract.ValidSourceID(source) {
			return cfg, fmt.Errorf("config: NP_COLLECTION_SOURCES names %q, which cannot be a source", source)
		}
	}
	cfg.DiscogsUser = strings.TrimSpace(Str("NP_DISCOGS_USER", ""))
	cfg.DiscogsToken = strings.TrimSpace(Str("NP_DISCOGS_TOKEN", ""))
	// A source said to play the owner's records, and no owner: the setting
	// would do nothing, and nothing would say so.
	if len(cfg.Owned) > 0 && cfg.DiscogsUser == "" {
		return cfg, fmt.Errorf("config: NP_COLLECTION_SOURCES is set and NP_DISCOGS_USER is not: whose collection?")
	}

	if cfg.IdleAfter, err = Duration("NP_IDLE_AFTER", 15*time.Second, 0, time.Hour); err != nil {
		return cfg, err
	}

	cfg.NoControls = List("NP_NO_CONTROLS_SOURCES", "")
	for _, source := range cfg.NoControls {
		if !contract.ValidSourceID(source) {
			return cfg, fmt.Errorf("config: NP_NO_CONTROLS_SOURCES names %q, which cannot be a source", source)
		}
	}

	// Each entry is source=beacon. Both become topic levels, and a typo in
	// either is a source that is never held, or never let go, with nothing
	// to say why.
	for _, entry := range List("NP_SOURCE_POWER", "") {
		source, beacon, ok := strings.Cut(entry, "=")
		source, beacon = strings.TrimSpace(source), strings.TrimSpace(beacon)
		switch {
		case !ok:
			return cfg, fmt.Errorf("config: NP_SOURCE_POWER has %q, which is not source=beacon", entry)
		case !contract.ValidSourceID(source):
			return cfg, fmt.Errorf("config: NP_SOURCE_POWER names %q, which cannot be a source", source)
		case !contract.ValidSourceID(beacon):
			return cfg, fmt.Errorf("config: NP_SOURCE_POWER names %q, which cannot be a beacon", beacon)
		case cfg.Power[source] != "":
			return cfg, fmt.Errorf("config: NP_SOURCE_POWER names %q twice: a source has one switch", source)
		}
		if cfg.Power == nil {
			cfg.Power = make(map[string]string)
		}
		cfg.Power[source] = beacon
	}
	return cfg, nil
}
