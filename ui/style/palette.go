package style

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lillink13/yamusic-tui/config"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/termenv"
)

// terminalColors holds the colors queried from the terminal via OSC 10/11.
// hasFG/hasBG are false when the terminal did not answer the query, in which
// case the caller should fall back to the configured hex values.
type terminalColors struct {
	fg    colorful.Color
	bg    colorful.Color
	dark  bool
	hasFG bool
	hasBG bool
}

// queryTerminalColors sends OSC 10 (foreground) and OSC 11 (background)
// queries and reports background lightness so the palette can be built for
// either a dark or a light terminal theme.
func queryTerminalColors() terminalColors {
	tc := terminalColors{}

	if c := termenv.ForegroundColor(); c != nil {
		if _, ok := c.(termenv.RGBColor); ok {
			rgb := termenv.ConvertToRGB(c)
			if rgb.IsValid() {
				tc.fg, tc.hasFG = rgb, true
			}
		}
	}
	if c := termenv.BackgroundColor(); c != nil {
		if _, ok := c.(termenv.RGBColor); ok {
			rgb := termenv.ConvertToRGB(c)
			if rgb.IsValid() {
				tc.bg, tc.hasBG = rgb, true
			}
		}
	}

	if tc.hasBG {
		_, _, l := tc.bg.Hsl()
		tc.dark = l < 0.5
	} else {
		tc.dark = termenv.HasDarkBackground()
	}

	return tc
}

// palette is the set of colors resolved from the terminal. Every field maps to
// one config color key; unused fields stay nil.
type palette struct {
	accent            *colorful.Color
	error             *colorful.Color
	border            *colorful.Color
	background        *colorful.Color
	playlistSelection *colorful.Color
	activeText        *colorful.Color
	normalText        *colorful.Color
	inactiveText      *colorful.Color
	trackTitleText    *colorful.Color
	trackVersionText  *colorful.Color
	trackArtistText   *colorful.Color
	lyricsPrevious    *colorful.Color
	lyricsCurrent     *colorful.Color
	lyricsNext        *colorful.Color
}

// buildPalette derives a full readable color scheme from the terminal colors.
// Text colors are based on the terminal foreground, border/selection are blends
// of the foreground and background, and the accent keeps a recognizable warm
// hue while staying readable against both dark and light backgrounds.
func buildPalette(tc terminalColors) *palette {
	pal := &palette{}

	if tc.hasFG {
		fg := tc.fg
		accent := warmAccent(fg)
		pal.accent = &accent
		pal.normalText = &fg

		active := shiftLightness(fg, tc.dark, +0.25)
		pal.activeText = &active

		inactive := blendWith(fg, tc.bg, tc.hasBG, tc.dark, 0.55)
		pal.inactiveText = inactive
		pal.trackVersionText = inactive
		pal.trackArtistText = inactive
		pal.lyricsNext = inactive

		dim := blendWith(fg, tc.bg, tc.hasBG, tc.dark, 0.75)
		pal.lyricsPrevious = dim

		bright := shiftLightness(fg, tc.dark, +0.12)
		pal.trackTitleText = &bright

		current := shiftLightness(fg, tc.dark, +0.15)
		pal.lyricsCurrent = &current

		err := errorRed(tc.dark)
		pal.error = &err
	}

	if tc.hasBG {
		bg := tc.bg
		pal.background = &bg

		border := blendWith(bg, tc.fg, tc.hasFG, tc.dark, 0.5)
		pal.border = border

		if pal.accent != nil {
			sel := blend(*pal.accent, bg, 0.85)
			pal.playlistSelection = &sel
		}
	}

	return pal
}

func (p *palette) resolve(key, raw string) lipgloss.Color {
	// An explicit hex escapes the terminal palette entirely: the user wants this
	// exact color no matter what the terminal reports.
	if isHexColor(raw) {
		return lipgloss.Color(raw)
	}
	if c := p.colorFor(key); c != nil {
		return lipgloss.Color(c.Hex())
	}
	return lipgloss.Color(raw)
}

func (p *palette) colorFor(key string) *colorful.Color {
	switch key {
	case "accent":
		return p.accent
	case "error":
		return p.error
	case "border":
		return p.border
	case "background":
		return p.background
	case "playlist-selection":
		return p.playlistSelection
	case "active-text":
		return p.activeText
	case "normal-text":
		return p.normalText
	case "inactive-text":
		return p.inactiveText
	case "track-title-text":
		return p.trackTitleText
	case "track-version-text":
		return p.trackVersionText
	case "track-artist-text":
		return p.trackArtistText
	case "lyrics-previous":
		return p.lyricsPrevious
	case "lyrics-current":
		return p.lyricsCurrent
	case "lyrics-next":
		return p.lyricsNext
	}
	return nil
}

// warmAccent picks a recognizable warm accent hue. When the terminal foreground
// already carries a hue, it is reused with full saturation; otherwise it falls
// back to a golden hue that reads well on light and dark themes alike.
func warmAccent(fg colorful.Color) colorful.Color {
	h, s, _ := fg.Hsl()
	if s < 0.15 {
		h = 0.13 // ~45°, golden
	}
	return colorful.Hsl(h, 0.85, 0.55)
}

// shiftLightness moves the color towards a darker or lighter variant depending
// on the terminal being light or dark, clamped to valid HSV range.
func shiftLightness(c colorful.Color, dark bool, delta float64) colorful.Color {
	if dark {
		h, s, v := c.Hsv()
		return colorful.Hsv(h, s, clamp(v+delta, 0, 1))
	}
	h, s, v := c.Hsv()
	return colorful.Hsv(h, s, clamp(v-delta, 0, 1))
}

// blendWith mixes a color with the terminal background so it keeps enough
// contrast while looking muted. When the background is unknown it dims the
// color in the direction dictated by the terminal being light or dark, so the
// result stays readable on a partially-reporting terminal.
func blendWith(c, bg colorful.Color, hasBG, dark bool, t float64) *colorful.Color {
	if hasBG {
		res := blend(c, bg, t)
		return &res
	}
	res := shiftLightness(c, dark, t)
	return &res
}

func blend(a, b colorful.Color, t float64) colorful.Color {
	return a.BlendRgb(b, t).Clamped()
}

// errorRed picks a red that stays visible on the current terminal theme.
func errorRed(dark bool) colorful.Color {
	if dark {
		return colorful.Hsl(0.0, 0.7, 0.55)
	}
	return colorful.Hsl(0.0, 0.75, 0.4)
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// resolvePalette builds a palette from the terminal when the user opted in via
// `style.use-terminal-colors` and the terminal answered the OSC query. Without
// the flag, or when the query fails, the configured hex values are used
// verbatim, so existing installs are unchanged.
//
// Note: parseConfig guarantees Style.Colors is non-nil on load, so styles.go can
// read it directly; the nil guard here only protects hand-built Style values
// (tests, decoupled callers) from a panic.
func resolvePalette(style *config.Style) *palette {
	if style == nil || style.Colors == nil || !style.UseTerminalColors {
		return &palette{}
	}

	tc := queryTerminalColors()
	if !tc.hasFG && !tc.hasBG {
		return &palette{}
	}
	return buildPalette(tc)
}

// isHexColor reports whether a config value is a literal hex color, in which
// case it must be used as-is instead of being resolved from the terminal.
func isHexColor(value string) bool {
	v := strings.TrimSpace(value)
	if len(v) == 0 {
		return false
	}
	if v[0] != '#' {
		return false
	}
	_, err := colorful.Hex(v)
	return err == nil
}
