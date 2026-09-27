package style

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lucasb-eyer/go-colorful"
)

func TestResolvePaletteDisabledNoTerminal(t *testing.T) {
	// Flag off: palette must stay empty, raw values are used verbatim.
	style := &config.Style{
		UseTerminalColors: false,
		Colors: &config.Colors{
			Accent: "#ABC",
		},
	}
	pal := resolvePalette(style)
	if pal.resolve("accent", "#ABC") != lipgloss.Color("#ABC") {
		t.Errorf("flag off should keep raw #ABC")
	}
}

func darkTerminal() terminalColors {
	return terminalColors{
		fg:    colorful.Color{R: 0.9, G: 0.9, B: 0.9},
		bg:    colorful.Color{R: 0.05, G: 0.05, B: 0.05},
		dark:  true,
		hasFG: true,
		hasBG: true,
	}
}

// lightness returns the L component (index 2) of a color's HSL triple.
func lightness(c *colorful.Color) float64 {
	_, _, l := c.Hsl()
	return l
}

func TestBuildPaletteDarkBackground(t *testing.T) {
	pal := buildPalette(darkTerminal())

	if pal.normalText == nil || pal.background == nil {
		t.Fatalf("palette should fill fg/bg colors")
	}
	// Normal text on dark bg should be light.
	if l := lightness(pal.normalText); l < 0.5 {
		t.Errorf("normal text lightness = %v, want light on dark background", l)
	}
	if l := lightness(pal.background); l > 0.5 {
		t.Errorf("background lightness = %v, want dark", l)
	}
	// Accent should be present and derived from the terminal foreground (so a
	// neutral theme does NOT force a hard-coded brand colour).
	if pal.accent == nil {
		t.Fatal("accent should be set")
	}
	if l := lightness(pal.accent); l <= lightness(pal.normalText) {
		t.Errorf("accent lightness = %v, want brighter than the plain text on dark bg", l)
	}
	// Inactive text should be dimmer than active text.
	if al := lightness(pal.activeText); al < 0.5 {
		t.Errorf("active lightness = %v, want bright", al)
	}
	if il := lightness(pal.inactiveText); il >= lightness(pal.activeText) {
		t.Errorf("inactive lightness = %v, want dimmer than active", il)
	}
	// Lyrics current should be brighter than the (muted) previous line.
	if lightness(pal.lyricsCurrent) <= lightness(pal.lyricsPrevious) {
		t.Errorf("lyrics current should be brighter than previous")
	}
}

func TestAccentUsesColoredForegroundHue(t *testing.T) {
	// A terminal with a coloured foreground keeps that hue for the accent.
	tc := terminalColors{
		fg:    colorful.Hsl(0.6, 0.8, 0.5), // saturated blue
		bg:    colorful.Color{R: 0.05, G: 0.05, B: 0.05},
		dark:  true,
		hasFG: true,
		hasBG: true,
	}
	pal := buildPalette(tc)
	if pal.accent == nil {
		t.Fatal("accent should be set")
	}
	h, s, _ := pal.accent.Hsl()
	if h < 0.55 || h > 0.65 {
		t.Errorf("accent hue = %v, want ~0.6 (from foreground)", h)
	}
	if s < 0.5 {
		t.Errorf("accent saturation = %v, want colourful", s)
	}
}

func TestBuildPaletteLightBackground(t *testing.T) {
	// Simulate a light terminal: white-ish background, dark text.
	tc := terminalColors{
		fg:    colorful.Color{R: 0.1, G: 0.1, B: 0.1},
		bg:    colorful.Color{R: 0.95, G: 0.95, B: 0.95},
		dark:  false,
		hasFG: true,
		hasBG: true,
	}
	pal := buildPalette(tc)

	if pal.normalText == nil || pal.background == nil {
		t.Fatalf("palette should fill fg/bg colors")
	}
	if l := lightness(pal.normalText); l > 0.5 {
		t.Errorf("normal text lightness = %v, want dark on light background", l)
	}
	if l := lightness(pal.background); l < 0.5 {
		t.Errorf("background lightness = %v, want light", l)
	}
}

func TestResolveWithTerminalPalette(t *testing.T) {
	pal := buildPalette(darkTerminal())

	// With terminal colors enabled the palette wins over the config hex, so even
	// an explicitly configured accent follows the terminal theme.
	if got := pal.resolve("accent", "#FC0"); got == lipgloss.Color("#FC0") {
		t.Errorf("accent should be resolved from terminal palette, got %v", got)
	}
	if got := pal.resolve("accent", ""); got == lipgloss.Color("") {
		t.Errorf("empty accent should be resolved from terminal palette, got %v", got)
	}
	// Keys the palette does not fill (e.g. an unknown one) fall back to raw.
	if got := pal.resolve("unknown", "#ABC"); got != lipgloss.Color("#ABC") {
		t.Errorf("unknown key should fall back to raw #ABC, got %v", got)
	}
}
