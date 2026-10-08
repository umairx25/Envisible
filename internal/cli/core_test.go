package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInitExistingAuthorizedProjectIsIdempotent(t *testing.T) {
	project := t.TempDir()
	identityHome := filepath.Join(t.TempDir(), "identity")
	withEnv(t, "ENVIS_HOME", identityHome)

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	if err := cmdInit([]string{"--id", "test-device", "--no-hook"}); err != nil {
		t.Fatalf("first init: %v", err)
	}
	before, err := os.ReadFile(filepath.Join(project, ".envis"))
	if err != nil {
		t.Fatal(err)
	}

	if err := cmdInit([]string{"--no-hook"}); err != nil {
		t.Fatalf("second init: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(project, ".envis"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("second init modified the existing .envis file")
	}
}
