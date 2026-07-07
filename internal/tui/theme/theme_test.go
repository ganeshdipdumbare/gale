package theme

import (
	"strings"
	"testing"
	"time"

	"github.com/mattn/go-runewidth"
)

func TestTruncate(t *testing.T) {
	cases := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{"fits exactly", "hello", 5, "hello"},
		{"shorter than max", "hi", 10, "hi"},
		{"needs ellipsis", "hello world", 7, "hello …"},
		{"max zero", "hello", 0, ""},
		{"max negative", "hello", -1, ""},
		{"max one", "hello", 1, "h"},
		{"empty string", "", 5, ""},
		{"wide runes", "こんにちは", 4, "こ…"}, // each kana is width 2
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Truncate(c.s, c.max)
			if got != c.want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", c.s, c.max, got, c.want)
			}
			if c.max > 0 && runewidth.StringWidth(got) > c.max {
				t.Errorf("Truncate(%q, %d) = %q has display width %d > max", c.s, c.max, got, runewidth.StringWidth(got))
			}
		})
	}
}

func TestPad(t *testing.T) {
	got := Pad("hi", 5)
	if got != "hi   " {
		t.Errorf("Pad(hi, 5) = %q, want %q", got, "hi   ")
	}
	if w := runewidth.StringWidth(Pad("this is way too long", 6)); w != 6 {
		t.Errorf("Pad truncated result has width %d, want 6", w)
	}
	if got := Pad("x", 0); got != "" {
		t.Errorf("Pad with width 0 should be empty, got %q", got)
	}
}

func TestPadLeft(t *testing.T) {
	got := PadLeft("hi", 5)
	if got != "   hi" {
		t.Errorf("PadLeft(hi, 5) = %q, want %q", got, "   hi")
	}
}

func TestDotBarBounds(t *testing.T) {
	if b := DotBar(-1, 10); strings.Count(b, "█") != 0 {
		t.Errorf("negative ratio should render zero filled cells, got %q", b)
	}
	if b := DotBar(2, 10); strings.Count(stripANSI(b), "█") > 10 {
		t.Errorf("ratio > 1 should clamp to full bar")
	}
	// NaN guard
	nan := 0.0
	nan = nan / nan
	if b := DotBar(nan, 10); b == "" {
		t.Errorf("NaN ratio should not panic or produce empty output")
	}
	// Width should never go below the visual floor even for tiny requests.
	if b := DotBar(0.5, -5); len(stripANSI(b)) == 0 {
		t.Errorf("DotBar should render something even for non-positive width")
	}
}

func TestETA(t *testing.T) {
	if got := ETA(-5 * time.Second); got != "00:00:00" {
		t.Errorf("negative duration should clamp to zero, got %q", got)
	}
	if got := ETA(90 * time.Second); got != "00:01:30" {
		t.Errorf("ETA(90s) = %q, want 00:01:30", got)
	}
	if got := ETA(3661 * time.Second); got != "01:01:01" {
		t.Errorf("ETA(3661s) = %q, want 01:01:01", got)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{
		0:           "0 B",
		512:         "512 B",
		1024:        "1.0 KB",
		1536:        "1.5 KB",
		1024 * 1024: "1.0 MB",
		1 << 30:     "1.0 GB",
	}
	for n, want := range cases {
		if got := HumanBytes(n); got != want {
			t.Errorf("HumanBytes(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestHumanSpeedGuardsNaN(t *testing.T) {
	nan := 0.0
	nan = nan / nan
	if got := HumanSpeed(nan); got == "" {
		t.Errorf("HumanSpeed(NaN) should not panic or be empty, got %q", got)
	}
	if got := HumanSpeed(-1); got != "0 B/s" {
		t.Errorf("HumanSpeed(negative) = %q, want 0 B/s", got)
	}
}

func TestScrollHint(t *testing.T) {
	if got := ScrollHint(false, false, 20); got != "" {
		t.Errorf("no scroll hint expected when nothing hidden, got %q", got)
	}
	if got := ScrollHint(true, false, 20); got == "" {
		t.Errorf("expected scroll hint when content above is hidden")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
