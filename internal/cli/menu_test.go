package cli

import "testing"

func TestDecodeKey(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want menuAction
	}{
		{"enter-cr", []byte{'\r'}, menuSelect},
		{"enter-lf", []byte{'\n'}, menuSelect},
		{"ctrl-c", []byte{3}, menuCancel},
		{"q", []byte{'q'}, menuCancel},
		{"esc", []byte{27}, menuCancel},
		{"k", []byte{'k'}, menuUp},
		{"j", []byte{'j'}, menuDown},
		{"arrow-up", []byte{27, '[', 'A'}, menuUp},
		{"arrow-down", []byte{27, '[', 'B'}, menuDown},
		{"arrow-right-ignored", []byte{27, '[', 'C'}, menuNone},
		{"random", []byte{'x'}, menuNone},
	}
	for _, c := range cases {
		if got := decodeKey(c.in); got != c.want {
			t.Errorf("%s: decodeKey(%v) = %d, want %d", c.name, c.in, got, c.want)
		}
	}
}

func TestClampIndex(t *testing.T) {
	// length 3, valid indices 0..2
	if got := clampIndex(0, 3, menuUp); got != 0 {
		t.Errorf("up at top: got %d, want 0", got)
	}
	if got := clampIndex(2, 3, menuDown); got != 2 {
		t.Errorf("down at bottom: got %d, want 2", got)
	}
	if got := clampIndex(1, 3, menuUp); got != 0 {
		t.Errorf("up from 1: got %d, want 0", got)
	}
	if got := clampIndex(1, 3, menuDown); got != 2 {
		t.Errorf("down from 1: got %d, want 2", got)
	}
	if got := clampIndex(1, 3, menuSelect); got != 1 {
		t.Errorf("select keeps index: got %d, want 1", got)
	}
}
