package config

import (
	"testing"
	"time"
)

func TestDefaults(t *testing.T) {
	cfg, err := LoadNowplayd("0.1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Broker.URL != "tcp://127.0.0.1:1883" || cfg.Broker.ClientID != "eds-nowplayd" || cfg.Prefix != "eds" {
		t.Errorf("broker %q as %q under %q", cfg.Broker.URL, cfg.Broker.ClientID, cfg.Prefix)
	}
	if len(cfg.Catalogues) != 3 || cfg.Catalogues[0] != "deezer" {
		t.Errorf("catalogues = %v", cfg.Catalogues)
	}
	if cfg.UserAgent != "eds-nowplaying/0.1 ( https://github.com/cdeever/eds )" {
		t.Errorf("user agent = %q", cfg.UserAgent)
	}
	if cfg.LookupTimeout != 10*time.Second {
		t.Errorf("lookup timeout = %s", cfg.LookupTimeout)
	}
}

// The same image must run on a Deevnet workload with nothing but kit.env.
func TestKitEnv(t *testing.T) {
	t.Setenv("DEEVNET_TENANT", "bench1")
	t.Setenv("MQTT_HOST", "mqtt.mobile.deevnet.net")
	t.Setenv("MQTT_USERNAME", "bench1-nowplayd")
	t.Setenv("MQTT_PASSWORD", "secret")
	t.Setenv("MQTT_CA_FILE", "/kit/deevnet-root-ca.pem")

	cfg, err := LoadCommon("eds-nowplayd")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Broker.URL != "tls://mqtt.mobile.deevnet.net:8883" {
		t.Errorf("url = %q", cfg.Broker.URL)
	}
	if cfg.Prefix != "bench1" || cfg.Broker.Username != "bench1-nowplayd" || cfg.Broker.CAFile != "/kit/deevnet-root-ca.pem" {
		t.Errorf("kit names not honoured: %+v under %q", cfg.Broker, cfg.Prefix)
	}
}

// kit.env carries one login, lightd's. A second service on the same workload
// has to be able to name its own beside it.
func TestOwnSettingsWinOverKitEnv(t *testing.T) {
	t.Setenv("MQTT_HOST", "mqtt.mobile.deevnet.net")
	t.Setenv("MQTT_USERNAME", "eds-lightd")
	t.Setenv("MQTT_PASSWORD", "lightds")
	t.Setenv("NP_MQTT_USERNAME", "eds-nowplayd")
	t.Setenv("NP_MQTT_PASSWORD", "nowplayds")
	t.Setenv("NP_TOPIC_PREFIX", "eds")
	t.Setenv("DEEVNET_TENANT", "other")

	cfg, err := LoadCommon("eds-nowplayd")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Broker.Username != "eds-nowplayd" || cfg.Broker.Password != "nowplayds" || cfg.Prefix != "eds" {
		t.Errorf("own settings did not win: %q %q under %q", cfg.Broker.Username, cfg.Broker.Password, cfg.Prefix)
	}
	if cfg.Broker.URL != "tls://mqtt.mobile.deevnet.net:8883" {
		t.Errorf("the kit's broker was lost: %q", cfg.Broker.URL)
	}
}

func TestCataloguesCanBeChosenAndSwitchedOff(t *testing.T) {
	t.Setenv("NP_CATALOGUES", " itunes , ,musicbrainz")
	cfg, err := LoadNowplayd("0.1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Catalogues) != 2 || cfg.Catalogues[0] != "itunes" || cfg.Catalogues[1] != "musicbrainz" {
		t.Errorf("catalogues = %v", cfg.Catalogues)
	}

	t.Setenv("NP_CATALOGUES", "")
	cfg, err = LoadNowplayd("0.1")
	if err != nil || len(cfg.Catalogues) != 0 {
		t.Errorf("an empty list did not switch lookups off: %v, %v", cfg.Catalogues, err)
	}
}

func TestInvalidValuesStopTheProgram(t *testing.T) {
	cases := map[string]map[string]string{
		"insecure that is not a boolean": {"NP_MQTT_INSECURE": "yes please"},
		"an empty client id":             {"NP_MQTT_CLIENT_ID": ""},
		"an empty prefix":                {"NP_TOPIC_PREFIX": ""},
		"a prefix of several levels":     {"NP_TOPIC_PREFIX": "eds/nowplaying"},
		"a wildcard prefix":              {"NP_TOPIC_PREFIX": "+"},
		"no contact for catalogues":      {"NP_CONTACT": " "},
		"a timeout that is not one":      {"NP_LOOKUP_TIMEOUT": "ten"},
		"a timeout of nothing":           {"NP_LOOKUP_TIMEOUT": "0s"},
		"a timeout of an hour":           {"NP_LOOKUP_TIMEOUT": "1h"},
		"a coverless source misspelt":    {"NP_COVERLESS_SOURCES": "Spotify"},
		"a coverless wildcard":           {"NP_COVERLESS_SOURCES": "itunes,+"},
		"a collection source misspelt":   {"NP_COLLECTION_SOURCES": "Turntable", "NP_DISCOGS_USER": "someone"},
		"a collection with no owner":     {"NP_COLLECTION_SOURCES": "turntable"},
		"idle after a day":               {"NP_IDLE_AFTER": "24h"},
		"stopped idle after a week":      {"NP_IDLE_AFTER_STOPPED": "168h"},
		"idle after something else":      {"NP_IDLE_AFTER": "soon"},
		"a source without controls, +":   {"NP_NO_CONTROLS_SOURCES": "turntable,+"},
		"a switch with no beacon":        {"NP_SOURCE_POWER": "turntable"},
		"a switch for no source":         {"NP_SOURCE_POWER": "=turntable-power-01"},
		"a switch that is a wildcard":    {"NP_SOURCE_POWER": "turntable=+"},
		"a source with two switches":     {"NP_SOURCE_POWER": "turntable=a,turntable=b"},
	}
	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := LoadNowplayd("0.1"); err == nil {
				t.Error("expected an error")
			}
		})
	}
}

func TestCoverlessSources(t *testing.T) {
	cfg, err := LoadNowplayd("0.1")
	if err != nil || len(cfg.Coverless) != 0 {
		t.Fatalf("by default every source has its cover: %v, %v", cfg.Coverless, err)
	}

	t.Setenv("NP_COVERLESS_SOURCES", " spotify , tidal")
	cfg, err = LoadNowplayd("0.1")
	if err != nil || len(cfg.Coverless) != 2 || cfg.Coverless[0] != "spotify" || cfg.Coverless[1] != "tidal" {
		t.Errorf("coverless = %v, %v", cfg.Coverless, err)
	}
}

func TestCollectionSources(t *testing.T) {
	cfg, err := LoadNowplayd("0.1")
	if err != nil || len(cfg.Owned) != 0 {
		t.Fatalf("by default no source is looked up in a collection: %v, %v", cfg.Owned, err)
	}

	// An owner and no sources is nothing to stop for: nothing is asked.
	t.Setenv("NP_DISCOGS_USER", " someone ")
	t.Setenv("NP_DISCOGS_TOKEN", " secret\n")
	if _, err := LoadNowplayd("0.1"); err != nil {
		t.Fatal(err)
	}

	t.Setenv("NP_COLLECTION_SOURCES", "turntable")
	cfg, err = LoadNowplayd("0.1")
	if err != nil || len(cfg.Owned) != 1 || cfg.Owned[0] != "turntable" || cfg.DiscogsUser != "someone" || cfg.DiscogsToken != "secret" {
		t.Errorf("owned = %v, user = %q, err = %v", cfg.Owned, cfg.DiscogsUser, err)
	}
}

func TestSourcePower(t *testing.T) {
	cfg, err := LoadNowplayd("0.1")
	if err != nil || len(cfg.Power) != 0 {
		t.Fatalf("by default no source has a switch: %v, %v", cfg.Power, err)
	}

	t.Setenv("NP_SOURCE_POWER", " turntable = turntable-power-01 , tape=deck-power-01")
	cfg, err = LoadNowplayd("0.1")
	if err != nil || len(cfg.Power) != 2 || cfg.Power["turntable"] != "turntable-power-01" || cfg.Power["tape"] != "deck-power-01" {
		t.Errorf("power = %v, %v", cfg.Power, err)
	}
}

func TestIdleAfterAndNoControls(t *testing.T) {
	cfg, err := LoadNowplayd("0.1")
	if err != nil || cfg.IdleAfter != 15*time.Second || cfg.StoppedIdleAfter != 5*time.Minute || len(cfg.NoControls) != 0 {
		t.Fatalf("defaults: idle %v, no controls %v, %v", cfg.IdleAfter, cfg.NoControls, err)
	}

	t.Setenv("NP_IDLE_AFTER", "0s")
	t.Setenv("NP_NO_CONTROLS_SOURCES", "turntable")
	cfg, err = LoadNowplayd("0.1")
	if err != nil || cfg.IdleAfter != 0 || len(cfg.NoControls) != 1 || cfg.NoControls[0] != "turntable" {
		t.Errorf("idle %v, no controls %v, %v", cfg.IdleAfter, cfg.NoControls, err)
	}
}
