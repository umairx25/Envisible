// Package crypto implements the cryptographic primitives for Envis.
//
// Design:
//   - Each user/device has a NaCl box (Curve25519) keypair.
//   - A random 32-byte repo key K is generated. All secrets are encrypted with
//     K using NaCl secretbox (XSalsa20-Poly1305).
//   - K is encrypted per-recipient using NaCl box sealed boxes
//     (anonymous public-key encryption), so any holder of a recipient's
//     private key can recover K. Encrypting K for a new recipient requires
//     only their public key.
package crypto

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/nacl/box"
	"golang.org/x/crypto/nacl/secretbox"
)

const (
	// KeySize is the size of the repo key K in bytes.
	KeySize = 32
	// nonceSize is the size of a secretbox nonce.
	nonceSize = 24
)

// KeyPair holds a Curve25519 public/private keypair.
type KeyPair struct {
	Public  [32]byte
	Private [32]byte
}

// GenerateKeyPair creates a new random Curve25519 keypair.
func GenerateKeyPair() (*KeyPair, error) {
	pub, priv, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generating keypair: %w", err)
	}
	kp := &KeyPair{Public: *pub, Private: *priv}
	return kp, nil
}

// GenerateK creates a new random 32-byte repo key K.
func GenerateK() ([]byte, error) {
	k := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, k); err != nil {
		return nil, fmt.Errorf("generating repo key: %w", err)
	}
	return k, nil
}

// EncryptKForRecipient encrypts the repo key K for a recipient's public key
// using an anonymous sealed box. Returns a base64-encoded ciphertext.
func EncryptKForRecipient(k []byte, recipientPub [32]byte) (string, error) {
	sealed, err := box.SealAnonymous(nil, k, &recipientPub, rand.Reader)
	if err != nil {
		return "", fmt.Errorf("sealing K for recipient: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptKWithPrivate decrypts a recipient's encrypted copy of K using their
// keypair.
func DecryptKWithPrivate(encryptedK string, kp *KeyPair) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(encryptedK)
	if err != nil {
		return nil, fmt.Errorf("decoding encrypted K: %w", err)
	}
	k, ok := box.OpenAnonymous(nil, sealed, &kp.Public, &kp.Private)
	if !ok {
		return nil, errors.New("failed to decrypt K: wrong key or corrupted data")
	}
	return k, nil
}

// EncryptPayload encrypts plaintext with K using secretbox. Returns a
// base64-encoded nonce and base64-encoded ciphertext.
func EncryptPayload(plaintext, k []byte) (nonceB64, ciphertextB64 string, err error) {
	if len(k) != KeySize {
		return "", "", fmt.Errorf("invalid key size: got %d, want %d", len(k), KeySize)
	}
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", "", fmt.Errorf("generating nonce: %w", err)
	}
	var key [KeySize]byte
	copy(key[:], k)

	sealed := secretbox.Seal(nil, plaintext, &nonce, &key)
	return base64.StdEncoding.EncodeToString(nonce[:]),
		base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptPayload decrypts a base64-encoded ciphertext with K using the given
// base64-encoded nonce.
func DecryptPayload(nonceB64, ciphertextB64 string, k []byte) ([]byte, error) {
	if len(k) != KeySize {
		return nil, fmt.Errorf("invalid key size: got %d, want %d", len(k), KeySize)
	}
	nonceBytes, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return nil, fmt.Errorf("decoding nonce: %w", err)
	}
	if len(nonceBytes) != nonceSize {
		return nil, fmt.Errorf("invalid nonce size: got %d, want %d", len(nonceBytes), nonceSize)
	}
	ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("decoding ciphertext: %w", err)
	}
	var nonce [nonceSize]byte
	copy(nonce[:], nonceBytes)
	var key [KeySize]byte
	copy(key[:], k)

	plaintext, ok := secretbox.Open(nil, ciphertext, &nonce, &key)
	if !ok {
		return nil, errors.New("failed to decrypt payload: wrong key or corrupted data")
	}
	return plaintext, nil
}

// EncodePublicKey encodes a public key as base64.
func EncodePublicKey(pub [32]byte) string {
	return base64.StdEncoding.EncodeToString(pub[:])
}

// DecodePublicKey decodes a base64-encoded public key.
func DecodePublicKey(s string) ([32]byte, error) {
	var pub [32]byte
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return pub, fmt.Errorf("decoding public key: %w", err)
	}
	if len(b) != 32 {
		return pub, fmt.Errorf("invalid public key size: got %d, want 32", len(b))
	}
	copy(pub[:], b)
	return pub, nil
}

// EncodePrivateKey encodes a private key as base64.
func EncodePrivateKey(priv [32]byte) string {
	return base64.StdEncoding.EncodeToString(priv[:])
}

// DecodePrivateKey decodes a base64-encoded private key.
func DecodePrivateKey(s string) ([32]byte, error) {
	var priv [32]byte
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return priv, fmt.Errorf("decoding private key: %w", err)
	}
	if len(b) != 32 {
		return priv, fmt.Errorf("invalid private key size: got %d, want 32", len(b))
	}
	copy(priv[:], b)
	return priv, nil
}
