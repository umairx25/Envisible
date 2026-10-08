package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
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
	// Use the absolute path to this binary so the installed hook works even
	// when envis is not on PATH (e.g. a repo-local ./envis). Fall back to the
	// bare command if resolution fails.
	exe := "envis"
	if p, err := os.Executable(); err == nil {
		if abs, err := filepath.Abs(p); err == nil {
			exe = abs
		}
	}
	return shellTarget{
		name:     name,
		rcPath:   rcPath,
		line:     fmt.Sprintf(`eval "$(%s hook %s)"`, shellQuote(exe), name),
		marker:   "hook " + name,
		activate: fmt.Sprintf("source %s", rcPath),
	}
}

// queryPowerShellProfile asks Windows PowerShell for its actual $PROFILE path.
// This respects redirected Documents folders such as OneDrive, which cannot
// be derived reliably from os.UserHomeDir.
var queryPowerShellProfile = func() string {
	if runtime.GOOS != "windows" {
		return ""
	}
	out, err := exec.Command("powershell.exe", "-NoProfile", "-Command", "$PROFILE").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func powerShellProfilePath(home string) string {
	if p := strings.TrimSpace(os.Getenv("ENVIS_POWERSHELL_PROFILE")); p != "" {
		return p
	}
	if p := queryPowerShellProfile(); p != "" {
		return p
	}
	for _, key := range []string{"OneDriveConsumer", "OneDriveCommercial", "OneDrive"} {
		if root := strings.TrimSpace(os.Getenv(key)); root != "" {
			return filepath.Join(root, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
		}
	}
	return filepath.Join(home, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
}

// powerShellTarget builds a PowerShell target that pipes the hook into
// Invoke-Expression from the user's real $PROFILE. The absolute executable
// path makes a repo-local binary work without a separate PATH installation.
func powerShellTarget() (shellTarget, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return shellTarget{}, false
	}
	executable, err := os.Executable()
	if err != nil {
		return shellTarget{}, false
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return shellTarget{}, false
	}
	invocation := "& " + psQuote(executable)
	return shellTarget{
		name:     "powershell",
		rcPath:   powerShellProfilePath(home),
		line:     "Invoke-Expression ((" + invocation + " hook powershell) -join [Environment]::NewLine)",
		marker:   "hook powershell",
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

// updateHookLine replaces an installed hook command when its generated form
// changes between Envis versions. It preserves the profile's newline style.
func updateHookLine(rcPath, marker, line string) (bool, error) {
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return false, err
	}
	newline := "\n"
	if strings.Contains(string(data), "\r\n") {
		newline = "\r\n"
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	for i, existing := range lines {
		if !strings.Contains(strings.TrimSpace(existing), marker) {
			continue
		}
		if strings.TrimSpace(existing) == line {
			return false, nil
		}
		lines[i] = line
		info, err := os.Stat(rcPath)
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(rcPath, []byte(strings.Join(lines, newline)), info.Mode())
	}
	return false, nil
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
		updated, err := updateHookLine(t.rcPath, t.marker, t.line)
		if err != nil {
			return false, fmt.Sprintf("Could not update the shell hook in %s: %v", t.rcPath, err)
		}
		if updated {
			return true, fmt.Sprintf(
				"Updated shell hook in %s. Run `%s` (or open a new terminal) to activate automatic secret loading.",
				t.rcPath, t.activate)
		}
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
