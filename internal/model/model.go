// Package model defines the on-disk schema for the committed .envis file and
// helpers to locate, read, and write it.
package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// FileName is the name of the committed Envis file.
const FileName = ".envis"

// Status describes whether a user has been granted access.
//
// For backward compatibility an empty status is treated as active: existing
// .envis files written before statuses existed have no status field and must
// continue to work.
type Status string

const (
	// StatusActive means the user holds a usable encrypted copy of K.
	StatusActive Status = "active"
	// StatusPending means the user has requested access (their public key is
	// recorded) but has not yet been granted K. A pending user has an empty
	// dek_enc and can decrypt nothing.
	StatusPending Status = "pending"
)

// User is an authorized or pending user/device: a public key plus (when
// active) the repo key K encrypted for that public key.
//
// Fields:
//   - PK:     the user's Curve25519 public key (base64).
//   - DEKEnc: the data-encryption key K, sealed for PK (base64); empty while
//     the user is pending.
//
// Note: legacy files may contain a `role` field; it is no longer used and is
// silently ignored on load (yaml ignores unknown fields).
type User struct {
	ID     string `yaml:"id"`
	Status Status `yaml:"status,omitempty"`
	PK     string `yaml:"pk"`
	DEKEnc string `yaml:"dek_enc"`
}

// IsActive reports whether the user has been granted access. A missing status
// is treated as active for backward compatibility.
func (u *User) IsActive() bool {
	return u.Status == StatusActive || u.Status == ""
}

// IsPending reports whether the user is awaiting approval.
func (u *User) IsPending() bool {
	return u.Status == StatusPending
}

// Payload is the encrypted secrets blob.
type Payload struct {
	Nonce      string `yaml:"nonce"`
	Ciphertext string `yaml:"ciphertext"`
}

// Envis is the full committed file.
//
// Rotated counts how many times the repo key K has been rotated. It starts at
// 0 for a freshly initialized file and increments on every rotation.
type Envis struct {
	Version int     `yaml:"version"`
	Rotated int     `yaml:"rotated"`
	Users   []User  `yaml:"users"`
	Payload Payload `yaml:"payload"`
}

// New returns an empty Envis file at version 1 with rotation count 0.
func New() *Envis {
	return &Envis{Version: 1, Rotated: 0}
}

// Find walks up from the current working directory looking for a .envis file.
// It returns the absolute path to the file.
func Find() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return FindFrom(dir)
}

// FindFrom walks up from start looking for a .envis file.
func FindFrom(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, FileName)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s file found in %s or any parent directory", FileName, start)
		}
		dir = parent
	}
}

// Load reads and parses an .envis file at the given path.
func Load(path string) (*Envis, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var e Envis
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &e, nil
}

// Save writes the .envis file to the given path atomically.
func (e *Envis) Save(path string) error {
	data, err := yaml.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshaling envis file: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("renaming temp file: %w", err)
	}
	return nil
}

// FindUserByPK returns the user matching the given public key, or nil if none.
func (e *Envis) FindUserByPK(pk string) *User {
	for i := range e.Users {
		if e.Users[i].PK == pk {
			return &e.Users[i]
		}
	}
	return nil
}

// FindActiveUserByPK returns the active user matching the given public key, or
// nil. Pending entries are ignored so that merely having requested access
// grants nothing.
func (e *Envis) FindActiveUserByPK(pk string) *User {
	for i := range e.Users {
		if e.Users[i].PK == pk && e.Users[i].IsActive() {
			return &e.Users[i]
		}
	}
	return nil
}

// ActiveUsers returns the users that have been granted access.
func (e *Envis) ActiveUsers() []User {
	var out []User
	for _, u := range e.Users {
		if u.IsActive() {
			out = append(out, u)
		}
	}
	return out
}

// PendingUsers returns the users awaiting approval.
func (e *Envis) PendingUsers() []User {
	var out []User
	for _, u := range e.Users {
		if u.IsPending() {
			out = append(out, u)
		}
	}
	return out
}

// FindUserByID returns the user matching the given id, or nil.
func (e *Envis) FindUserByID(id string) *User {
	for i := range e.Users {
		if e.Users[i].ID == id {
			return &e.Users[i]
		}
	}
	return nil
}

// AddUser appends a user. It errors if the id already exists.
func (e *Envis) AddUser(u User) error {
	if e.FindUserByID(u.ID) != nil {
		return fmt.Errorf("user %q already exists", u.ID)
	}
	e.Users = append(e.Users, u)
	return nil
}

// RemoveUser removes the user with the given id. It errors if not found.
func (e *Envis) RemoveUser(id string) error {
	for i := range e.Users {
		if e.Users[i].ID == id {
			e.Users = append(e.Users[:i], e.Users[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("user %q not found", id)
}

// ErrNoUser indicates the local key does not match any active user.
var ErrNoUser = errors.New("local key is not an authorized user")
