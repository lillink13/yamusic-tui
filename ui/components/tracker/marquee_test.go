package tracker

import "testing"

func TestMarquee(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		width int
		tick  uint64
		want  string
	}{
		{"fits exactly", "abc", 3, 0, "abc"},
		{"padded when shorter than width", "ab", 4, 0, "ab  "},
		{"empty text padded to width", "", 3, 0, "   "},
		{"zero width yields empty", "abc", 0, 5, ""},
		{"scroll starts at the head", "abcdef", 3, 0, "abc"},
		{"scroll advances one column per step", "abcdef", 3, _MARQUEE_FRAMES_PER_STEP, "bcd"},
		{"scroll holds within a step", "abcdef", 3, _MARQUEE_FRAMES_PER_STEP - 1, "abc"},
		{"scroll wraps after a full period", "abcdef", 3, uint64(len("abcdef")+_MARQUEE_GAP) * _MARQUEE_FRAMES_PER_STEP, "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := marquee(tt.text, tt.width, tt.tick); got != tt.want {
				t.Fatalf("marquee(%q, %d, %d) = %q, want %q", tt.text, tt.width, tt.tick, got, tt.want)
			}
		})
	}
}

// TestMarqueeWindowWidth ensures the scrolling window is always exactly `width`
// runes wide across a full period, so the now-playing layout never jitters.
func TestMarqueeWindowWidth(t *testing.T) {
	const text = "a long enough title to scroll"
	const width = 10
	period := len([]rune(text)) + _MARQUEE_GAP
	for step := 0; step < period+2; step++ {
		tick := uint64(step) * _MARQUEE_FRAMES_PER_STEP
		got := marquee(text, width, tick)
		if n := len([]rune(got)); n != width {
			t.Fatalf("tick %d: window width = %d, want %d (%q)", tick, n, width, got)
		}
	}
}
