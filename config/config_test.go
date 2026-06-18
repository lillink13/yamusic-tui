package config

import "testing"

func TestParseConfigFillsScalarDefaults(t *testing.T) {
	// A config that omits the audio scalars must not end up muted, with a zero
	// playback buffer, or with no rewind step.
	cfg, err := parseConfig([]byte("token: abc\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}

	if cfg.Token != "abc" {
		t.Errorf("Token = %q, want %q", cfg.Token, "abc")
	}
	if cfg.Volume != defaultConfig.Volume {
		t.Errorf("Volume = %v, want default %v", cfg.Volume, defaultConfig.Volume)
	}
	if cfg.VolumeStep != defaultConfig.VolumeStep {
		t.Errorf("VolumeStep = %v, want default %v", cfg.VolumeStep, defaultConfig.VolumeStep)
	}
	if cfg.BufferSize != defaultConfig.BufferSize {
		t.Errorf("BufferSize = %v, want default %v", cfg.BufferSize, defaultConfig.BufferSize)
	}
	if cfg.RewindDuration != defaultConfig.RewindDuration {
		t.Errorf("RewindDuration = %v, want default %v", cfg.RewindDuration, defaultConfig.RewindDuration)
	}
}

func TestParseConfigKeepsExplicitValues(t *testing.T) {
	cfg, err := parseConfig([]byte("volume: 0.8\nbuffer-size-ms: 120\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Volume != 0.8 {
		t.Errorf("Volume = %v, want 0.8", cfg.Volume)
	}
	if cfg.BufferSize != 120 {
		t.Errorf("BufferSize = %v, want 120", cfg.BufferSize)
	}
}

func TestParseConfigKeepsExplicitZeroValues(t *testing.T) {
	// An explicit 0 must be honored (start muted / disable rewind), not treated
	// as "field omitted" and clobbered back to the default.
	cfg, err := parseConfig([]byte("volume: 0\nrewind-duration-s: 0\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Volume != 0 {
		t.Errorf("Volume = %v, want explicit 0", cfg.Volume)
	}
	if cfg.RewindDuration != 0 {
		t.Errorf("RewindDuration = %v, want explicit 0", cfg.RewindDuration)
	}
	// An omitted scalar should still fall back to the default.
	if cfg.BufferSize != defaultConfig.BufferSize {
		t.Errorf("BufferSize = %v, want default %v", cfg.BufferSize, defaultConfig.BufferSize)
	}
}

func TestParseConfigPopulatesNestedDefaults(t *testing.T) {
	cfg, err := parseConfig([]byte("token: abc\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Controls == nil || cfg.Style == nil || cfg.Style.Colors == nil || cfg.Style.Icons == nil || cfg.Search == nil {
		t.Fatalf("nested config sections must be non-nil after merge: %+v", cfg)
	}
	if cfg.Controls.ShowAllKeys == nil || !cfg.Controls.ShowAllKeys.Contains("?") {
		t.Errorf("ShowAllKeys default should contain '?', got %+v", cfg.Controls.ShowAllKeys)
	}
}

func TestParseConfigShowAllKeysCorrectKey(t *testing.T) {
	cfg, err := parseConfig([]byte("controls:\n    show-all-keys: x\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Controls.ShowAllKeys == nil || !cfg.Controls.ShowAllKeys.Contains("x") {
		t.Errorf("ShowAllKeys should contain 'x', got %+v", cfg.Controls.ShowAllKeys)
	}
}

func TestParseConfigShowAllKeysLegacyMisspelledKey(t *testing.T) {
	// Older versions saved this control under the misspelled "show-all-kyes".
	cfg, err := parseConfig([]byte("controls:\n    show-all-kyes: x\n"))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	if cfg.Controls.ShowAllKeys == nil || !cfg.Controls.ShowAllKeys.Contains("x") {
		t.Errorf("legacy show-all-kyes should be honored, got %+v", cfg.Controls.ShowAllKeys)
	}
}

func TestParseConfigInvalidYAML(t *testing.T) {
	_, err := parseConfig([]byte("token: [unterminated\n"))
	if err == nil {
		t.Fatal("expected error for malformed YAML, got nil")
	}
}
