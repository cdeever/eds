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
