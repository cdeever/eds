package config

import "testing"

func TestDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Addr != ":8732" {
		t.Errorf("addr = %q", cfg.Addr)
	}
	if cfg.Broker.TopicPrefix != "eds" {
		t.Errorf("prefix = %q", cfg.Broker.TopicPrefix)
	}
	if cfg.StandID != "lp-stand-01" {
		t.Errorf("stand = %q", cfg.StandID)
	}
	if cfg.Scene.Effect != "breathe" {
		t.Errorf("effect = %q", cfg.Scene.Effect)
	}
}

func TestOverrides(t *testing.T) {
	t.Setenv("LIGHTD_ADDR", ":9000")
	t.Setenv("LIGHTD_STAND_ID", "lp-stand-42")
	t.Setenv("LIGHTD_EFFECT", "sweep")
	t.Setenv("LIGHTD_SATURATION", "1.8")
	t.Setenv("LIGHTD_MAX_COLORS", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9000" || cfg.StandID != "lp-stand-42" {
		t.Errorf("overrides ignored: %+v", cfg)
	}
	if cfg.Scene.Effect != "sweep" || cfg.Scene.Saturation != 1.8 || cfg.Scene.MaxColors != 2 {
		t.Errorf("scene overrides ignored: %+v", cfg.Scene)
	}
}

// kit.env is the platform's settings file: with nothing but it, lightd must
// reach the tenant's broker over TLS as the tenant's account.
func TestKitEnv(t *testing.T) {
	t.Setenv("DEEVNET_TENANT", "tdemo")
	t.Setenv("MQTT_HOST", "mqtt.mobile.deevnet.net")
	t.Setenv("MQTT_PORT", "8883")
	t.Setenv("MQTT_USERNAME", "tdemo.lightd")
	t.Setenv("MQTT_PASSWORD", "secret")
	t.Setenv("MQTT_CA_FILE", "/kit/site-ca.pem")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b := cfg.Broker
	if b.URL != "tls://mqtt.mobile.deevnet.net:8883" {
		t.Errorf("url = %q", b.URL)
	}
	if b.Username != "tdemo.lightd" || b.Password != "secret" || b.CAFile != "/kit/site-ca.pem" {
		t.Errorf("credentials = %q / %q / %q", b.Username, b.Password, b.CAFile)
	}
	if b.TopicPrefix != "tdemo" {
		t.Errorf("prefix = %q", b.TopicPrefix)
	}
}

// A LIGHTD_* setting is the more specific one, so it wins over kit.env.
func TestLightdWinsOverKitEnv(t *testing.T) {
	t.Setenv("MQTT_HOST", "mqtt.mobile.deevnet.net")
	t.Setenv("MQTT_USERNAME", "from-kit")
	t.Setenv("DEEVNET_TENANT", "tdemo")
	t.Setenv("LIGHTD_MQTT_URL", "tcp://127.0.0.1:21883")
	t.Setenv("LIGHTD_MQTT_USERNAME", "from-lightd")
	t.Setenv("LIGHTD_TOPIC_PREFIX", "eds")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Broker.URL != "tcp://127.0.0.1:21883" || cfg.Broker.Username != "from-lightd" || cfg.Broker.TopicPrefix != "eds" {
		t.Errorf("kit.env won: %+v", cfg.Broker)
	}
}

func TestKitEnvDefaultPort(t *testing.T) {
	t.Setenv("MQTT_HOST", "mqtt.example")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Broker.URL != "tls://mqtt.example:8883" {
		t.Errorf("url = %q", cfg.Broker.URL)
	}
}

// A typo in a unit file should stop the daemon, not silently light the room
// wrong.
func TestRejectsInvalidValues(t *testing.T) {
	cases := map[string]map[string]string{
		"brightness out of range": {"LIGHTD_BRIGHTNESS": "4"},
		"speed out of range":      {"LIGHTD_SPEED": "-1"},
		"saturation zero":         {"LIGHTD_SATURATION": "0"},
		"max colors zero":         {"LIGHTD_MAX_COLORS": "0"},
		"swatches too many":       {"LIGHTD_SWATCHES": "50"},
		"empty stand":             {"LIGHTD_STAND_ID": ""},
		"empty effect":            {"LIGHTD_EFFECT": ""},
		"non-numeric brightness":  {"LIGHTD_BRIGHTNESS": "bright"},
	}

	for name, env := range cases {
		t.Run(name, func(t *testing.T) {
			for k, v := range env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
