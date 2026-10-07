package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withEnv(t *testing.T, key, val string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	os.Setenv(key, val)
	t.Cleanup(func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

func TestInstallHookAppendsThenIdempotent(t *testing.T) {
	home := t.TempDir()
	withEnv(t, "HOME", home)
	withEnv(t, "SHELL", "/bin/zsh")

	rc := filepath.Join(home, ".zshrc")

	// First install: should write the hook.
	installed, msg := installHook()
	if !installed {
		t.Fatalf("expected hook to be installed, got false; msg=%q", msg)
	}
	data, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `eval "$(envis hook zsh)"`) {
		t.Fatalf("rc file missing hook line:\n%s", data)
	}

	// Second install: should be a no-op (idempotent).
	installed2, _ := installHook()
	if installed2 {
		t.Fatal("expected second install to be a no-op")
	}
	data2, _ := os.ReadFile(rc)
	if n := strings.Count(string(data2), "envis hook"); n != 1 {
		t.Fatalf("expected exactly 1 hook line, found %d", n)
	}
}

func TestInstallHookBash(t *testing.T) {
	home := t.TempDir()
	withEnv(t, "HOME", home)
	withEnv(t, "SHELL", "/usr/bin/bash")

	installed, _ := installHook()
	if !installed {
		t.Fatal("expected bash hook to install")
	}
	data, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `eval "$(envis hook bash)"`) {
		t.Fatalf("rc missing bash hook:\n%s", data)
	}
}

func TestInstallHookUnknownShell(t *testing.T) {
	home := t.TempDir()
	withEnv(t, "HOME", home)
	withEnv(t, "SHELL", "/usr/bin/fish")

	installed, msg := installHook()
	if installed {
		t.Fatal("expected no install for unsupported shell")
	}
	if !strings.Contains(msg, "auto-detect") {
		t.Fatalf("expected guidance message, got %q", msg)
	}
	// No rc files should have been created.
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal("unexpected .zshrc created for fish shell")
	}
}

func TestPowerShellProfilePathUsesOverride(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, "OneDrive", "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	withEnv(t, "ENVIS_POWERSHELL_PROFILE", want)

	if got := powerShellProfilePath(home); got != want {
		t.Fatalf("powerShellProfilePath() = %q, want %q", got, want)
	}
}

func TestPowerShellProfilePathUsesOneDrive(t *testing.T) {
	home := t.TempDir()
	oneDrive := filepath.Join(home, "OneDrive")
	withEnv(t, "ENVIS_POWERSHELL_PROFILE", "")
	withEnv(t, "OneDriveConsumer", "")
	withEnv(t, "OneDriveCommercial", "")
	withEnv(t, "OneDrive", oneDrive)

	originalQuery := queryPowerShellProfile
	queryPowerShellProfile = func() string { return "" }
	t.Cleanup(func() { queryPowerShellProfile = originalQuery })

	want := filepath.Join(oneDrive, "Documents", "WindowsPowerShell", "Microsoft.PowerShell_profile.ps1")
	if got := powerShellProfilePath(home); got != want {
		t.Fatalf("powerShellProfilePath() = %q, want %q", got, want)
	}
}

func TestUpdateHookLine(t *testing.T) {
	profile := filepath.Join(t.TempDir(), "profile.ps1")
	oldLine := "envis hook powershell | Out-String | Invoke-Expression"
	newLine := "Invoke-Expression ((& 'C:\\Tools\\envis.exe' hook powershell) -join [Environment]::NewLine)"
	if err := os.WriteFile(profile, []byte("# custom\r\n"+oldLine+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	updated, err := updateHookLine(profile, "hook powershell", newLine)
	if err != nil {
		t.Fatal(err)
	}
	if !updated {
		t.Fatal("expected the old hook line to be updated")
	}
	data, err := os.ReadFile(profile)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); !strings.Contains(got, "# custom\r\n"+newLine+"\r\n") {
		t.Fatalf("updated profile did not preserve content or CRLF newlines: %q", got)
	}

	updated, err = updateHookLine(profile, "hook powershell", newLine)
	if err != nil {
		t.Fatal(err)
	}
	if updated {
		t.Fatal("expected an up-to-date hook line to be unchanged")
	}
}
