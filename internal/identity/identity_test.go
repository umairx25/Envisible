package identity

import (
	"os"
	"testing"
)

func TestCreateAndLoadWithProtectedPerms(t *testing.T) {
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	ident, err := Create("alice@laptop")
	if err != nil {
		t.Fatal(err)
	}
	if ident.PublicKey == "" || ident.PrivateKey == "" {
		t.Fatal("expected keys to be populated")
	}

	path, _ := Path()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("identity file perm = %o, want 600", perm)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != ident.ID || loaded.PrivateKey != ident.PrivateKey {
		t.Fatal("loaded identity mismatch")
	}

	// KeyPair decodes correctly.
	if _, err := loaded.KeyPair(); err != nil {
		t.Fatal(err)
	}
}

func TestLoadNotFound(t *testing.T) {
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	if _, err := Load(); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
