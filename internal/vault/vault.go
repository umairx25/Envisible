// Package vault ties together the committed .envis file, the local identity,
// and the crypto layer to provide high-level operations: recover K, decrypt
// secrets, mutate them, and re-encrypt.
package vault

import (
	"fmt"

	"github.com/envis/envis/internal/crypto"
	"github.com/envis/envis/internal/identity"
	"github.com/envis/envis/internal/model"
	"github.com/envis/envis/internal/secrets"
)

// Vault is an opened .envis file bound to a local identity, with K recovered.
type Vault struct {
	Path     string
	File     *model.Envis
	Identity *identity.Identity
	k        []byte
}

// Open loads the .envis file at path, loads the local identity, and recovers K
// by matching the identity's public key against a recipient.
func Open(path string) (*Vault, error) {
	ef, err := model.Load(path)
	if err != nil {
		return nil, err
	}
	id, err := identity.Load()
	if err != nil {
		return nil, err
	}
	return bind(path, ef, id)
}

// OpenFound locates the nearest .envis file by walking up the directory tree,
// then opens it.
func OpenFound() (*Vault, error) {
	path, err := model.Find()
	if err != nil {
		return nil, err
	}
	return Open(path)
}

func bind(path string, ef *model.Envis, id *identity.Identity) (*Vault, error) {
	kp, err := id.KeyPair()
	if err != nil {
		return nil, err
	}
	rec := ef.FindActiveUserByPK(id.PublicKey)
	if rec == nil {
		return nil, model.ErrNoUser
	}
	k, err := crypto.DecryptKWithPrivate(rec.DEKEnc, kp)
	if err != nil {
		return nil, fmt.Errorf("recovering repo key: %w", err)
	}
	return &Vault{Path: path, File: ef, Identity: id, k: k}, nil
}

// K returns the recovered repo key.
func (v *Vault) K() []byte { return v.k }

// Secrets decrypts and parses the payload into a mutable Set.
func (v *Vault) Secrets() (*secrets.Set, error) {
	if v.File.Payload.Ciphertext == "" {
		return secrets.New(), nil
	}
	plaintext, err := crypto.DecryptPayload(v.File.Payload.Nonce, v.File.Payload.Ciphertext, v.k)
	if err != nil {
		return nil, err
	}
	return secrets.Parse(plaintext)
}

// WriteSecrets encrypts the given Set with the current K and updates the file's
// payload in memory. Call Save to persist.
func (v *Vault) WriteSecrets(s *secrets.Set) error {
	nonce, ciphertext, err := crypto.EncryptPayload(s.Serialize(), v.k)
	if err != nil {
		return err
	}
	v.File.Payload.Nonce = nonce
	v.File.Payload.Ciphertext = ciphertext
	return nil
}

// Save persists the in-memory .envis file to disk.
func (v *Vault) Save() error {
	return v.File.Save(v.Path)
}

// Rotate generates a new repo key K2, re-encrypts the current secrets under
// K2, and re-encrypts K2 for every active user. Used when revoking a user or
// on an explicit rotate. The current secrets are read with the old K before
// rotation.
func (v *Vault) Rotate() error {
	// Decrypt current secrets with old K.
	s, err := v.Secrets()
	if err != nil {
		return fmt.Errorf("decrypting secrets before rotation: %w", err)
	}

	k2, err := crypto.GenerateK()
	if err != nil {
		return err
	}

	// Re-encrypt K2 for all remaining active users. Pending users have not
	// been granted access, so their empty dek_enc is left intact.
	for i := range v.File.Users {
		if !v.File.Users[i].IsActive() {
			continue
		}
		pub, err := crypto.DecodePublicKey(v.File.Users[i].PK)
		if err != nil {
			return fmt.Errorf("user %q: %w", v.File.Users[i].ID, err)
		}
		enc, err := crypto.EncryptKForRecipient(k2, pub)
		if err != nil {
			return fmt.Errorf("user %q: %w", v.File.Users[i].ID, err)
		}
		v.File.Users[i].DEKEnc = enc
	}

	// Swap in K2 and re-encrypt payload.
	v.k = k2
	if err := v.WriteSecrets(s); err != nil {
		return err
	}
	v.File.Rotated++
	return nil
}
