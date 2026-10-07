package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// supportedShell maps a detected shell name to the rc file (relative to $HOME)
// and the `envis hook` argument.
type shellInfo struct {
	name   string // bash | zsh
	rcFile string // e.g. .zshrc
}

// detectShell inspects $SHELL to determine the user's interactive shell.
// Returns ok=false if the shell is unknown/unsupported.
func detectShell() (shellInfo, bool) {
	sh := os.Getenv("SHELL")
	base := filepath.Base(sh)
	switch base {
	case "zsh":
		return shellInfo{name: "zsh", rcFile: ".zshrc"}, true
	case "bash":
		return shellInfo{name: "bash", rcFile: ".bashrc"}, true
	default:
		return shellInfo{}, false
	}
}

// hookEvalLine is the single line appended to the user's rc file. It is
// detected verbatim to keep installation idempotent.
func hookEvalLine(shell string) string {
	return fmt.Sprintf(`eval "$(envis hook %s)"`, shell)
}

// hookInstalled reports whether the rc file already contains the envis hook
// eval line (any shell variant), making installation idempotent.
func hookInstalled(rcPath string) (bool, error) {
	f, err := os.Open(rcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, "envis hook") {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// installHook appends the envis hook eval line to the user's shell rc file if
// it is not already present. It is best-effort: it reports what it did (or why
// it could not) but never returns a hard failure that should abort init.
//
// Returns (installed, message). installed is true only when a new line was
// written this call.
func installHook() (bool, string) {
	info, ok := detectShell()
	if !ok {
		sh := os.Getenv("SHELL")
		return false, fmt.Sprintf(
			"Could not auto-detect your shell (SHELL=%q). To enable automatic secret loading, add this to your shell startup file:\n    eval \"$(envis hook bash)\"   # or: zsh",
			sh)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return false, fmt.Sprintf("Could not locate home directory to install the shell hook: %v", err)
	}
	rcPath := filepath.Join(home, info.rcFile)

	already, err := hookInstalled(rcPath)
	if err != nil {
		return false, fmt.Sprintf("Could not read %s to install the shell hook: %v", rcPath, err)
	}
	if already {
		return false, fmt.Sprintf("Shell hook already present in %s.", rcPath)
	}

	line := hookEvalLine(info.name)
	block := fmt.Sprintf("\n# envis: load encrypted environment on directory entry\n%s\n", line)

	f, err := os.OpenFile(rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Sprintf("Could not write the shell hook to %s: %v\nAdd this line manually:\n    %s", rcPath, err, line)
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return false, fmt.Sprintf("Could not write the shell hook to %s: %v\nAdd this line manually:\n    %s", rcPath, err, line)
	}

	return true, fmt.Sprintf(
		"Installed shell hook in %s. Run `source %s` (or open a new terminal) to activate automatic secret loading.",
		rcPath, rcPath)
}
