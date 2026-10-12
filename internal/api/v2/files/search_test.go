package files

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEscapeLike(t *testing.T) {
	cases := []struct{ in, want string }{
		{"50%", `50\%`},               // otherwise it matches everything
		{"a_b", `a\_b`},               // _ is a single-char wildcard
		{`back\slash`, `back\\slash`}, // the escape character itself
		{"合同模板", "合同模板"},              // multi-byte text passes through
		{"plain", "plain"},
	}
	for _, c := range cases {
		if got := escapeLike(c.in); got != c.want {
			t.Errorf("escapeLike(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNonEmptyParts(t *testing.T) {
	got := nonEmptyParts([]string{"a", "", "   ", "b"})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("nonEmptyParts = %v, want [a b]", got)
	}
	if len(nonEmptyParts(nil)) != 0 {
		t.Error("expected an empty result for nil input")
	}
}

func TestCapIndexedContent(t *testing.T) {
	short := "合同模板"
	if got := capIndexedContent(short); got != short {
		t.Errorf("short content must pass through unchanged, got %q", got)
	}

	// Multi-byte characters make the byte-wise cut land mid-rune.
	long := strings.Repeat("合同", maxIndexedContentBytes)
	capped := capIndexedContent(long)
	if len(capped) > maxIndexedContentBytes {
		t.Errorf("capped length = %d, want <= %d", len(capped), maxIndexedContentBytes)
	}
	if !utf8.ValidString(capped) {
		t.Error("truncation must not leave invalid UTF-8 behind")
	}
}
