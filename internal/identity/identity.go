// Package identity manages the local, per-device Envis identity: the private
// key and associated metadata, stored in a protected file under the user's
// config directory (not in Git).
package identity

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/yaml.v3"

	"github.com/envis/envis/internal/crypto"
)

// ProtectionNote returns a human-readable caveat about how well the private
// key file is protected on the current OS, or an empty string when the normal
// 0600 Unix permissions apply. On Windows, Go's file mode maps only to the
// read-only bit and does not create an ACL restricting other accounts, so the
// key is not protected the way it is on Unix.
func ProtectionNote() string {
	if runtime.GOOS == "windows" {
		path, err := Path()
		if err != nil {
			path = "the identity file"
		}
		return fmt.Sprintf(
			"Note: on Windows the private key at %s is NOT restricted by an OS ACL "+
				"(Unix 0600 permissions don't translate). Protect this file and your "+
				"user account accordingly.", path)
	}
	return ""
}

// Identity is the locally stored device/user identity.
type Identity struct {
	// ID is the recipient id this identity registers as in .envis files.
	ID string `yaml:"id"`
	// PublicKey is the base64-encoded Curve25519 public key.
	PublicKey string `yaml:"public_key"`
	// PrivateKey is the base64-encoded Curve25519 private key. Secret.
	PrivateKey string `yaml:"private_key"`
}

// KeyPair returns the crypto.KeyPair for this identity.
func (id *Identity) KeyPair() (*crypto.KeyPair, error) {
	pub, err := crypto.DecodePublicKey(id.PublicKey)
	if err != nil {
		return nil, err
	}
	priv, err := crypto.DecodePrivateKey(id.PrivateKey)
	if err != nil {
		return nil, err
	}
	return &crypto.KeyPair{Public: pub, Private: priv}, nil
}

// ErrNotFound indicates no local identity exists yet.
var ErrNotFound = errors.New("no local Envis identity found; run `envis keygen` or `envis init`")

// Dir returns the directory where the identity file is stored. It honors
// ENVIS_HOME, then XDG_CONFIG_HOME, then $HOME/.config.
func Dir() (string, error) {
	if h := os.Getenv("ENVIS_HOME"); h != "" {
		return h, nil
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "envis"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "envis"), nil
}

// Path returns the full path to the identity file.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "identity.yaml"), nil
}

// Load reads the local identity. Returns ErrNotFound if it does not exist.
func Load() (*Identity, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("reading identity file: %w", err)
	}
	var id Identity
	if err := yaml.Unmarshal(data, &id); err != nil {
		return nil, fmt.Errorf("parsing identity file: %w", err)
	}
	return &id, nil
}

// Exists reports whether a local identity file is present.
func Exists() (bool, error) {
	path, err := Path()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// Save writes the identity to a protected file (0600) with a 0700 directory.
func (id *Identity) Save() error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating identity dir: %w", err)
	}
	path := filepath.Join(dir, "identity.yaml")

	data, err := yaml.Marshal(id)
	if err != nil {
		return fmt.Errorf("marshaling identity: %w", err)
	}

	// Write with restrictive permissions from the start.
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("creating identity file: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("writing identity file: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("saving identity file: %w", err)
	}
	return nil
}

// Create generates a new keypair and persists it as the local identity with
// the given id.
func Create(id string) (*Identity, error) {
	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		return nil, err
	}
	ident := &Identity{
		ID:         id,
		PublicKey:  crypto.EncodePublicKey(kp.Public),
		PrivateKey: crypto.EncodePrivateKey(kp.Private),
	}
	if err := ident.Save(); err != nil {
		return nil, err
	}
	return ident, nil
}
