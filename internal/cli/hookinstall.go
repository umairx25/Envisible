package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// shellTarget describes where and how to install the hook for a given shell.
type shellTarget struct {
	name     string // bash | zsh | powershell
	rcPath   string // absolute path to the startup file
	line     string // the exact line to append (also used for idempotency)
	marker   string // substring that identifies our line in the file
	activate string // user-facing hint to activate it now
}

// detectShellTarget determines the user's interactive shell and the startup
// file to modify. On Windows it targets PowerShell; otherwise it inspects
// $SHELL for bash/zsh. Returns ok=false if the shell is unknown/unsupported.
func detectShellTarget() (shellTarget, bool) {
	if runtime.GOOS == "windows" {
		return powerShellTarget()
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return shellTarget{}, false
	}
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		return posixTarget("zsh", filepath.Join(home, ".zshrc")), true
	case "bash":
		return posixTarget("bash", filepath.Join(home, ".bashrc")), true
	default:
		return shellTarget{}, false
	}
}

// posixTarget builds a bash/zsh target that evals the hook on startup.
func posixTarget(name, rcPath string) shellTarget {
	return shellTarget{
		name:     name,
		rcPath:   rcPath,
		line:     fmt.Sprintf(`eval "$(envis hook %s)"`, name),
		marker:   "envis hook",
		activate: fmt.Sprintf("source %s", rcPath),
	}
}

// powerShellTarget builds a PowerShell target that pipes the hook into
// Invoke-Expression from the user's $PROFILE. It prefers the PowerShell 7+
// profile location, falling back to Windows PowerShell 5.1.
func powerShellTarget() (shellTarget, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return shellTarget{}, false
	}
	docs := filepath.Join(home, "Documents")
	// PowerShell 7+ profile; fall back to Windows PowerShell 5.1 if that dir
	// already exists and the pwsh one does not.
	ps7 := filepath.Join(docs, "PowerShell", "Microsoft.PowerShell_profile.ps1")
	ps5 := filepath.Join(docs, "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	rcPath := ps7
	if _, err := os.Stat(filepath.Dir(ps7)); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Dir(ps5)); err == nil {
			rcPath = ps5
		}
	}
	return shellTarget{
		name:     "powershell",
		rcPath:   rcPath,
		line:     "envis hook powershell | Out-String | Invoke-Expression",
		marker:   "envis hook powershell",
		activate: ". $PROFILE",
	}, true
}

// hookInstalled reports whether the startup file already contains our hook
// line, making installation idempotent.
func hookInstalled(rcPath, marker string) (bool, error) {
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
		if strings.Contains(strings.TrimSpace(scanner.Text()), marker) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// installHook appends the hook line to the user's shell startup file if it is
// not already present. Best-effort: it reports what it did (or why it could
// not) but never returns a hard failure that should abort init.
//
// Returns (installed, message). installed is true only when a new line was
// written this call.
func installHook() (bool, string) {
	t, ok := detectShellTarget()
	if !ok {
		if runtime.GOOS == "windows" {
			return false, "Could not locate your PowerShell profile. To enable automatic secret loading, add this line to your $PROFILE:\n    envis hook powershell | Out-String | Invoke-Expression"
		}
		return false, fmt.Sprintf(
			"Could not auto-detect your shell (SHELL=%q). To enable automatic secret loading, add this to your shell startup file:\n    eval \"$(envis hook bash)\"   # or: zsh",
			os.Getenv("SHELL"))
	}

	already, err := hookInstalled(t.rcPath, t.marker)
	if err != nil {
		return false, fmt.Sprintf("Could not read %s to install the shell hook: %v", t.rcPath, err)
	}
	if already {
		return false, fmt.Sprintf("Shell hook already present in %s.", t.rcPath)
	}

	// Ensure the parent directory exists (notably the PowerShell profile dir).
	if err := os.MkdirAll(filepath.Dir(t.rcPath), 0o755); err != nil {
		return false, fmt.Sprintf("Could not create %s to install the shell hook: %v\nAdd this line manually:\n    %s", filepath.Dir(t.rcPath), err, t.line)
	}

	block := fmt.Sprintf("\n# envis: load encrypted environment on directory entry\n%s\n", t.line)
	f, err := os.OpenFile(t.rcPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, fmt.Sprintf("Could not write the shell hook to %s: %v\nAdd this line manually:\n    %s", t.rcPath, err, t.line)
	}
	defer f.Close()
	if _, err := f.WriteString(block); err != nil {
		return false, fmt.Sprintf("Could not write the shell hook to %s: %v\nAdd this line manually:\n    %s", t.rcPath, err, t.line)
	}

	return true, fmt.Sprintf(
		"Installed shell hook in %s. Run `%s` (or open a new terminal) to activate automatic secret loading.",
		t.rcPath, t.activate)
}
