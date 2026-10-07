package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDotenvQuote(t *testing.T) {
	cases := map[string]string{
		"simple":          "simple",
		"with space":      `"with space"`,
		"":                `""`,
		"a=b":             `"a=b"`,
		"has#hash":        `"has#hash"`,
		`has"quote`:       `"has\"quote"`,
		`back\slash yes`:  `"back\\slash yes"`,
		"no_special-123.": "no_special-123.",
	}
	for in, want := range cases {
		if got := dotenvQuote(in); got != want {
			t.Errorf("dotenvQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextAvailableName(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, ".env")

	// Nothing exists yet -> base.
	if got := nextAvailableName(base); got != base {
		t.Fatalf("got %q, want %q", got, base)
	}

	// Create base -> expect base1.
	if err := os.WriteFile(base, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	want1 := base + "1"
	if got := nextAvailableName(base); got != want1 {
		t.Fatalf("got %q, want %q", got, want1)
	}

	// Create base1 too -> expect base2.
	if err := os.WriteFile(want1, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	want2 := base + "2"
	if got := nextAvailableName(base); got != want2 {
		t.Fatalf("got %q, want %q", got, want2)
	}
}
