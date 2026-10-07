package cli

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// errMenuCancelled is returned by selectMenu when the user aborts (q, Esc, or
// Ctrl-C) without choosing an item.
var errMenuCancelled = errors.New("selection cancelled")

// isInteractive reports whether both stdin and stderr are connected to a
// terminal, which is required to drive an arrow-key menu. Menus render to
// stderr so normal command output on stdout stays clean/pipeable.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// menuAction describes what a decoded keypress should do.
type menuAction int

const (
	menuNone menuAction = iota
	menuUp
	menuDown
	menuSelect
	menuCancel
)

// decodeKey interprets a raw key chunk (1–3 bytes) into a menu action. It is
// pure so it can be unit-tested without a terminal.
func decodeKey(key []byte) menuAction {
	n := len(key)
	switch {
	case n == 1 && (key[0] == '\r' || key[0] == '\n'):
		return menuSelect
	case n == 1 && (key[0] == 3 || key[0] == 'q' || key[0] == 27):
		return menuCancel
	case n == 1 && key[0] == 'k':
		return menuUp
	case n == 1 && key[0] == 'j':
		return menuDown
	case n == 3 && key[0] == 27 && key[1] == '[' && key[2] == 'A':
		return menuUp
	case n == 3 && key[0] == 27 && key[1] == '[' && key[2] == 'B':
		return menuDown
	default:
		return menuNone
	}
}

// clampIndex applies a menu action to the current index within [0,len) and
// returns the new index. Pure, for unit testing navigation.
func clampIndex(cur, length int, act menuAction) int {
	switch act {
	case menuUp:
		if cur > 0 {
			return cur - 1
		}
	case menuDown:
		if cur < length-1 {
			return cur + 1
		}
	}
	return cur
}

// selectMenu renders an interactive single-select list and returns the index
// of the chosen item. It requires a TTY (callers must check isInteractive
// first). Navigation: Up/Down arrows or k/j; Enter selects; q/Esc/Ctrl-C
// cancels (returns errMenuCancelled).
//
// The menu draws to stderr and restores the terminal and cursor on return.
func selectMenu(title string, items []string) (int, error) {
	if len(items) == 0 {
		return 0, errors.New("no items to choose from")
	}
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return 0, fmt.Errorf("entering raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	// Hide cursor during navigation; restore on exit.
	fmt.Fprint(os.Stderr, "\x1b[?25l")
	defer fmt.Fprint(os.Stderr, "\x1b[?25h")

	selected := 0
	draw := func(first bool) {
		if !first {
			// Move cursor up to the first item line to redraw in place.
			fmt.Fprintf(os.Stderr, "\x1b[%dA", len(items))
		}
		for i, item := range items {
			// Clear the line, then render with a pointer + highlight for the
			// selected row.
			fmt.Fprint(os.Stderr, "\r\x1b[K")
			if i == selected {
				fmt.Fprintf(os.Stderr, "\x1b[7m> %s\x1b[0m\r\n", item)
			} else {
				fmt.Fprintf(os.Stderr, "  %s\r\n", item)
			}
		}
	}

	if title != "" {
		fmt.Fprint(os.Stderr, "\r\x1b[K"+title+"\r\n")
	}
	draw(true)

	buf := make([]byte, 3)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			return 0, err
		}
		switch decodeKey(buf[:n]) {
		case menuSelect:
			return selected, nil
		case menuCancel:
			return 0, errMenuCancelled
		case menuUp:
			selected = clampIndex(selected, len(items), menuUp)
			draw(false)
		case menuDown:
			selected = clampIndex(selected, len(items), menuDown)
			draw(false)
		}
	}
}
