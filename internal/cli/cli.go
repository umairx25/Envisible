// Package cli implements the Envis command-line interface.
package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// stdinReader is a single buffered reader over stdin, shared across all
// prompts in a command invocation. Using one reader avoids losing input that a
// previous bufio.Reader had already buffered (which breaks piped multi-prompt
// flows such as `printf 'NAME\nVALUE\n' | envis set`).
var stdinReader = bufio.NewReader(os.Stdin)

// promptSecret reads a secret value from the terminal without echoing. If stdin
// is not a terminal, it reads a single line (useful for piping).
func promptSecret(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	// Non-interactive: read a line from the shared reader.
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptLine reads a plain line of input from the terminal.
func promptLine(label string) (string, error) {
	fmt.Fprint(os.Stderr, label)
	line, err := stdinReader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// confirm prompts the user with a yes/no question and returns true only if the
// answer begins with 'y' (case-insensitive). An empty answer is treated as no.
func confirm(question string) (bool, error) {
	ans, err := promptLine(question + " [y/N]: ")
	if err != nil {
		return false, err
	}
	ans = strings.ToLower(strings.TrimSpace(ans))
	return ans == "y" || ans == "yes", nil
}

// errorf prints an error to stderr and returns a non-zero exit code.
func errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// infof prints an informational message to stderr.
func infof(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}
