// Package config reads lightd's settings from the environment.
//
// Environment rather than a file because the two secrets involved - the broker
// credentials - must never land on disk in a repository, and because the same
// binary has to work under a systemd unit or a container without knowing which
// it is.
//
// Every broker setting falls back to the platform's names - the ones in the
// tenant's kit.env (MQTT_HOST, MQTT_USERNAME, DEEVNET_TENANT, ...) - so the
// same image runs on a Deevnet workload or a Pi with nothing but that file.
// A LIGHTD_* variable, when set, still wins.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cdeever/eds/services/lightd/internal/broker"
	"github.com/cdeever/eds/services/lightd/internal/scene"
)

// Config is everything lightd needs to run.
type Config struct {
	Addr       string
	PaletteURL string
	Swatches   int
	StandID    string
	// CoverTopic, when set, is a topic of cover images to light the stand
	// from, relative to the topic prefix. Empty means HTTP is the only input.
	CoverTopic string
	Broker     broker.Config
	Scene      scene.Options
}

// Load builds a Config from the environment, applying defaults.
func Load() (Config, error) {
	defaults := scene.DefaultOptions()

	cfg := Config{
		Addr:       env("LIGHTD_ADDR", ":8732"),
		PaletteURL: env("LIGHTD_PALETTE_URL", "http://127.0.0.1:8731"),
		StandID:    env("LIGHTD_STAND_ID", "lp-stand-01"),
		CoverTopic: env("LIGHTD_COVER_TOPIC", ""),
		Broker: broker.Config{
			URL:      env("LIGHTD_MQTT_URL", kitBrokerURL()),
			ClientID: env("LIGHTD_MQTT_CLIENT_ID", "lightd"),
			Username: env("LIGHTD_MQTT_USERNAME", env("MQTT_USERNAME", "")),
			Password: env("LIGHTD_MQTT_PASSWORD", env("MQTT_PASSWORD", "")),
			CAFile:   env("LIGHTD_MQTT_CA_FILE", env("MQTT_CA_FILE", "")),
			// Bring-up escape hatch only; mqtt01 gets a real certificate.
			InsecureSkipVerify: envBool("LIGHTD_MQTT_INSECURE", false),
			// The broker confines a tenant to topics under its own name.
			TopicPrefix: env("LIGHTD_TOPIC_PREFIX", env("DEEVNET_TENANT", "eds")),
			Timeout:     10 * time.Second,
		},
		Scene: defaults,
	}

	var err error
	if cfg.Swatches, err = envInt("LIGHTD_SWATCHES", 6); err != nil {
		return cfg, err
	}
	if cfg.Scene.Effect = env("LIGHTD_EFFECT", defaults.Effect); cfg.Scene.Effect == "" {
		return cfg, fmt.Errorf("config: LIGHTD_EFFECT must not be empty")
	}
	if cfg.Scene.Speed, err = envFloat("LIGHTD_SPEED", defaults.Speed); err != nil {
		return cfg, err
	}
	if cfg.Scene.Brightness, err = envFloat("LIGHTD_BRIGHTNESS", defaults.Brightness); err != nil {
		return cfg, err
	}
	if cfg.Scene.MinChroma, err = envFloat("LIGHTD_MIN_CHROMA", defaults.MinChroma); err != nil {
		return cfg, err
	}
	if cfg.Scene.Saturation, err = envFloat("LIGHTD_SATURATION", defaults.Saturation); err != nil {
		return cfg, err
	}
	if cfg.Scene.MaxColors, err = envInt("LIGHTD_MAX_COLORS", defaults.MaxColors); err != nil {
		return cfg, err
	}

	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch {
	case c.StandID == "":
		return fmt.Errorf("config: LIGHTD_STAND_ID must not be empty")
	// The broker enforces these too, but silently: a subscription it refuses
	// looks exactly like a quiet topic. Better to stop here and say why.
	case strings.HasPrefix(c.CoverTopic, "/") || strings.ContainsAny(c.CoverTopic, "#+$"):
		return fmt.Errorf("config: LIGHTD_COVER_TOPIC must be one topic relative to the prefix, "+
			"with no leading / and no wildcards, got %q", c.CoverTopic)
	case c.Swatches < 1 || c.Swatches > 16:
		return fmt.Errorf("config: LIGHTD_SWATCHES must be 1-16, got %d", c.Swatches)
	case c.Scene.Brightness < 0 || c.Scene.Brightness > 1:
		return fmt.Errorf("config: LIGHTD_BRIGHTNESS must be 0-1, got %v", c.Scene.Brightness)
	case c.Scene.Speed < 0 || c.Scene.Speed > 1:
		return fmt.Errorf("config: LIGHTD_SPEED must be 0-1, got %v", c.Scene.Speed)
	case c.Scene.Saturation <= 0:
		return fmt.Errorf("config: LIGHTD_SATURATION must be positive, got %v", c.Scene.Saturation)
	case c.Scene.MaxColors < 1:
		return fmt.Errorf("config: LIGHTD_MAX_COLORS must be at least 1, got %d", c.Scene.MaxColors)
	}
	return nil
}

// kitBrokerURL is the broker kit.env names: MQTT_HOST over TLS, on MQTT_PORT
// or 8883. Without MQTT_HOST it is the development broker.
func kitBrokerURL() string {
	host, ok := os.LookupEnv("MQTT_HOST")
	if !ok || host == "" {
		return "tcp://127.0.0.1:1883"
	}
	return "tls://" + host + ":" + env("MQTT_PORT", "8883")
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback, fmt.Errorf("config: %s=%q is not an integer", key, v)
	}
	return parsed, nil
}

func envFloat(key string, fallback float64) (float64, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback, fmt.Errorf("config: %s=%q is not a number", key, v)
	}
	return parsed, nil
}

func envBool(key string, fallback bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
