package config

import "testing"

func TestKeyShiftLetterMatchesUppercase(t *testing.T) {
	// A "shift+<letter>" binding must match the uppercase rune the terminal
	// actually delivers (Shift+F arrives as "F", not "shift+f").
	k := NewKey("shift+f")
	if !k.Contains("F") {
		t.Fatalf(`NewKey("shift+f") should match "F", got keyNames=%v`, k.keyNames)
	}
	if k.Contains("shift+f") {
		t.Fatalf(`NewKey("shift+f") should not still carry the literal "shift+f"`)
	}
}

func TestNormalizeShiftLetter(t *testing.T) {
	tests := []struct{ in, want string }{
		{"shift+f", "F"},
		{"shift+F", "F"},
		{"shift+up", "shift+up"}, // special key, modifier is delivered
		{"shift+left", "shift+left"},
		{"ctrl+f", "ctrl+f"},
		{"f", "f"},
		{"shift++", "shift++"}, // not a letter
		{"shift+", "shift+"},   // empty rest
	}
	for _, tt := range tests {
		if got := normalizeShiftLetter(tt.in); got != tt.want {
			t.Fatalf("normalizeShiftLetter(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestKeyMultiBindingNormalizes(t *testing.T) {
	// Comma-separated lists normalize each token; non-shift tokens are untouched.
	k := NewKey("shift+x,ctrl+x")
	if !k.Contains("X") || !k.Contains("ctrl+x") {
		t.Fatalf(`NewKey("shift+x,ctrl+x") keyNames=%v, want both "X" and "ctrl+x"`, k.keyNames)
	}
}

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

func TestParseConfigRadioSettings(t *testing.T) {
	yaml := "radio:\n" +
		"    'genre:popular':\n" +
		"        language: russian\n" +
		"        diversity: high\n" +
		"        mood: 0.5\n" +
		"        energy: 0.25\n"
	cfg, err := parseConfig([]byte(yaml))
	if err != nil {
		t.Fatalf("parseConfig returned error: %v", err)
	}
	rs, ok := cfg.RadioSettings["genre:popular"]
	if !ok {
		t.Fatalf("expected radio settings for genre:popular, got %+v", cfg.RadioSettings)
	}
	if rs.Language != "russian" || rs.Diversity != "high" || rs.Mood != 0.5 || rs.Energy != 0.25 {
		t.Errorf("radio settings = %+v, want russian/high/0.5/0.25", rs)
	}
}

func TestRadioSettingsIsZero(t *testing.T) {
	if !(RadioSettings{}).IsZero() {
		t.Errorf("empty RadioSettings should be zero")
	}
	if (RadioSettings{Language: "any"}).IsZero() {
		t.Errorf("non-empty RadioSettings should not be zero")
	}
}

func TestRadioSettingsConcurrentAccess(t *testing.T) {
	Current.RadioSettings = map[string]RadioSettings{"default": {Language: "any"}}
	t.Cleanup(func() { Current.RadioSettings = nil })

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2000; i++ {
			_, _ = Current.GetRadioSettings("genre:x")
			_ = Current.RadioSettingsSnapshot()
		}
	}()

	for i := 0; i < 2000; i++ {
		Current.SetRadioSettings("genre:x", RadioSettings{Energy: float32(i)})
		Current.SetRadioSettings("genre:y", RadioSettings{Energy: float32(i + 1)})
	}
	<-done
}

func TestKeyContainsLayoutIndependent(t *testing.T) {
	k := NewKey("c")
	if !k.Contains("c") {
		t.Errorf("Latin 'c' should match")
	}
	if !k.Contains("с") { // Cyrillic es on the physical C key
		t.Errorf("Cyrillic 'с' (RU layout, physical C) should match binding 'c'")
	}
	if !k.Contains("С") { // uppercase Cyrillic
		t.Errorf("Cyrillic 'С' should match binding 'c'")
	}
	if k.Contains("s") || k.Contains("ы") {
		t.Errorf("unrelated keys should not match")
	}
}

func TestContainsCyrillicLayout(t *testing.T) {
	// A binding on a letter that lives on a different physical key must not fire
	// when a different Cyrillic letter is pressed.
	k := NewKey("a") // physical A -> Cyrillic Ф
	if !k.Contains("ф") {
		t.Errorf("Cyrillic 'ф' (physical A) should match binding 'a'")
	}
	if k.Contains("с") { // physical C -> Latin c
		t.Errorf("Cyrillic 'с' should not match binding 'a'")
	}
}
