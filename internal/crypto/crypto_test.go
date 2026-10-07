package crypto

import (
	"bytes"
	"testing"
)

func TestKEncryptDecryptRoundTrip(t *testing.T) {
	kp, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	k, err := GenerateK()
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != KeySize {
		t.Fatalf("K size = %d, want %d", len(k), KeySize)
	}

	enc, err := EncryptKForRecipient(k, kp.Public)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptKWithPrivate(enc, kp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, k) {
		t.Fatalf("recovered K mismatch")
	}
}

func TestKDecryptWrongKeyFails(t *testing.T) {
	kp1, _ := GenerateKeyPair()
	kp2, _ := GenerateKeyPair()
	k, _ := GenerateK()

	enc, err := EncryptKForRecipient(k, kp1.Public)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptKWithPrivate(enc, kp2); err == nil {
		t.Fatal("expected decryption with wrong key to fail")
	}
}

func TestPayloadRoundTrip(t *testing.T) {
	k, _ := GenerateK()
	plaintext := []byte("DATABASE_URL=postgres://localhost\nSTRIPE_KEY=sk_test_abc\n")

	nonce, ct, err := EncryptPayload(plaintext, k)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecryptPayload(nonce, ct, k)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("payload mismatch: got %q", got)
	}
}

func TestPayloadWrongKeyFails(t *testing.T) {
	k1, _ := GenerateK()
	k2, _ := GenerateK()
	nonce, ct, err := EncryptPayload([]byte("SECRET=1"), k1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptPayload(nonce, ct, k2); err == nil {
		t.Fatal("expected decrypt with wrong key to fail")
	}
}

func TestPublicKeyEncodeDecode(t *testing.T) {
	kp, _ := GenerateKeyPair()
	enc := EncodePublicKey(kp.Public)
	dec, err := DecodePublicKey(enc)
	if err != nil {
		t.Fatal(err)
	}
	if dec != kp.Public {
		t.Fatal("public key round-trip mismatch")
	}
}
