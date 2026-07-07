package detail

import (
	"strings"
	"testing"

	"github.com/ganeshdipdumbare/gale/internal/index"
	"github.com/mattn/go-runewidth"
)

func TestWrapRespectsDisplayWidth(t *testing.T) {
	s := "the quick brown fox jumps over the lazy dog"
	wrapped := wrap(s, 10)
	for _, line := range strings.Split(wrapped, "\n") {
		if w := runewidth.StringWidth(line); w > 10 {
			t.Errorf("line %q has display width %d > 10", line, w)
		}
	}
	if strings.Join(strings.Fields(wrapped), " ") != s {
		t.Errorf("wrapping should not drop or reorder words: got %q", wrapped)
	}
}

func TestWrapDoesNotSplitWideRunesMidCharacter(t *testing.T) {
	// A naive byte-slice wrap would have corrupted multi-byte UTF-8 runes;
	// this must stay valid UTF-8 and preserve every rune.
	s := "パッケージ マネージャー です"
	wrapped := wrap(s, 6)
	if !strings.Contains(wrapped, "パッケージ") || !strings.Contains(wrapped, "マネージャー") {
		t.Errorf("expected wide-rune words to survive wrapping intact, got %q", wrapped)
	}
}

func TestWrapZeroWidthReturnsInput(t *testing.T) {
	s := "hello world"
	if got := wrap(s, 0); got != s {
		t.Errorf("wrap with width<=0 should return input unchanged, got %q", got)
	}
}

func TestRenderBodyHandlesEmptyPackage(t *testing.T) {
	m := NewModel(index.Package{Name: "empty", Version: "1.0"})
	body := m.renderBody()
	if !strings.Contains(body, "no additional metadata") {
		t.Errorf("expected fallback message for a package with no extra metadata, got %q", body)
	}
}

func TestPlainIncludesHomepage(t *testing.T) {
	p := index.Package{Name: "foo", Version: "1.0", Homepage: "https://example.com"}
	out := Plain(p)
	if !strings.Contains(out, "https://example.com") {
		t.Errorf("expected homepage in plain output, got %q", out)
	}
}
