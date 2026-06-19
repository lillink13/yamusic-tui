package api

import (
	"reflect"
	"testing"
)

func TestParseLRCText(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []LyricPair
	}{
		{
			name: "hundredths of a second are tens of milliseconds",
			in:   "[00:12.34]Hello",
			want: []LyricPair{{12340, "Hello"}},
		},
		{
			name: "minutes and seconds",
			in:   "[01:05.00]World",
			want: []LyricPair{{65000, "World"}},
		},
		{
			name: "thousandths are kept as milliseconds",
			in:   "[00:01.500]X",
			want: []LyricPair{{1500, "X"}},
		},
		{
			name: "single fractional digit is tenths of a second",
			in:   "[00:01.5]Y",
			want: []LyricPair{{1500, "Y"}},
		},
		{
			name: "no fractional part",
			in:   "[00:30]Z",
			want: []LyricPair{{30000, "Z"}},
		},
		{
			name: "surrounding whitespace is trimmed",
			in:   "[00:10.00]  spaced  ",
			want: []LyricPair{{10000, "spaced"}},
		},
		{
			name: "multiple lines preserve order",
			in:   "[00:01.00]a\n[00:02.50]b\n[00:03.00]c",
			want: []LyricPair{{1000, "a"}, {2500, "b"}, {3000, "c"}},
		},
		{
			name: "title metadata tag is skipped",
			in:   "[ti:Song Title]",
			want: nil,
		},
		{
			name: "artist and album metadata tags are skipped",
			in:   "[ar:Some Artist]\n[al:Some Album]",
			want: nil,
		},
		{
			name: "plain text without a tag is skipped",
			in:   "just some text",
			want: nil,
		},
		{
			name: "empty input yields no lyrics",
			in:   "",
			want: nil,
		},
		{
			name: "metadata is skipped but timestamped lines are kept",
			in:   "[ti:Title]\n[ar:Artist]\n[00:00.00]first line\n[00:04.20]second line",
			want: []LyricPair{{0, "first line"}, {4200, "second line"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLRCText(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseLRCText(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseTextLyrics(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{
			name: "simple lines",
			in:   "first\nsecond\nthird",
			want: []string{"first", "second", "third"},
		},
		{
			name: "windows line endings are normalized",
			in:   "first\r\nsecond\r\n",
			want: []string{"first", "second"},
		},
		{
			name: "leading and trailing blank lines are trimmed",
			in:   "\n\nverse\n\nchorus\n\n",
			want: []string{"verse", "", "chorus"},
		},
		{
			name: "trailing whitespace is stripped per line",
			in:   "padded   \nclean",
			want: []string{"padded", "clean"},
		},
		{
			name: "empty input yields no lines",
			in:   "",
			want: []string{},
		},
		{
			name: "whitespace-only input yields no lines",
			in:   "  \n\t\n",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseTextLyrics(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseTextLyrics(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestLRCFractionToMillis(t *testing.T) {
	tests := []struct {
		frac string
		want int
	}{
		{"", 0},
		{"5", 500},
		{"34", 340},
		{"500", 500},
		{"345", 345},
		{"3456", 345}, // over-long fractions are truncated to milliseconds
		{"xx", 0},     // non-numeric fractions are ignored
	}

	for _, tt := range tests {
		t.Run(tt.frac, func(t *testing.T) {
			if got := lrcFractionToMillis(tt.frac); got != tt.want {
				t.Fatalf("lrcFractionToMillis(%q) = %d, want %d", tt.frac, got, tt.want)
			}
		})
	}
}
