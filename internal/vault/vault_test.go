package vault

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/envis/envis/internal/crypto"
	"github.com/envis/envis/internal/identity"
	"github.com/envis/envis/internal/model"
)

// makeIdentity creates an identity in an isolated ENVIS_HOME and returns it.
// It restores ENVIS_HOME afterward via t.Cleanup.
func makeIdentity(t *testing.T, id string) *identity.Identity {
	t.Helper()
	home := t.TempDir()
	old, had := os.LookupEnv("ENVIS_HOME")
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() {
		if had {
			os.Setenv("ENVIS_HOME", old)
		} else {
			os.Unsetenv("ENVIS_HOME")
		}
	})
	ident, err := identity.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	return ident
}

// initEnvis creates a .envis in dir with the given admin identity holding one
// secret, and returns the path.
func initEnvis(t *testing.T, dir string, admin *identity.Identity) string {
	t.Helper()
	k, err := crypto.GenerateK()
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := crypto.DecodePublicKey(admin.PublicKey)
	encK, err := crypto.EncryptKForRecipient(k, pub)
	if err != nil {
		t.Fatal(err)
	}
	nonce, ct, err := crypto.EncryptPayload([]byte("DATABASE_URL=postgres://x\n"), k)
	if err != nil {
		t.Fatal(err)
	}
	ef := model.New()
	ef.Users = []model.User{{
		ID: admin.ID, PK: admin.PublicKey, DEKEnc: encK,
	}}
	ef.Payload = model.Payload{Nonce: nonce, Ciphertext: ct}

	path := filepath.Join(dir, model.FileName)
	if err := ef.Save(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFullFlow(t *testing.T) {
	// Admin identity + .envis file.
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	admin, err := identity.Create("alice")
	if err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	path := initEnvis(t, repo, admin)

	// Admin opens the vault and can read the secret.
	v, err := Open(path)
	if err != nil {
		t.Fatalf("admin open: %v", err)
	}
	set, err := v.Secrets()
	if err != nil {
		t.Fatal(err)
	}
	if val, _ := set.Get("DATABASE_URL"); val != "postgres://x" {
		t.Fatalf("DATABASE_URL = %q", val)
	}

	// Admin sets a new secret and saves.
	set.Set("STRIPE_KEY", "sk_live_1")
	if err := v.WriteSecrets(set); err != nil {
		t.Fatal(err)
	}
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}

	// A second user (bob) generates a keypair out-of-band.
	bobKP, _ := crypto.GenerateKeyPair()
	bobPub := crypto.EncodePublicKey(bobKP.Public)

	// Admin adds bob: encrypt current K for bob's public key.
	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	encKForBob, err := crypto.EncryptKForRecipient(v2.K(), bobKP.Public)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.File.AddUser(model.User{
		ID: "bob", PK: bobPub, DEKEnc: encKForBob,
	}); err != nil {
		t.Fatal(err)
	}
	if err := v2.Save(); err != nil {
		t.Fatal(err)
	}

	// Bob can now recover K and decrypt the payload directly.
	ef, _ := model.Load(path)
	bobRec := ef.FindUserByPK(bobPub)
	if bobRec == nil {
		t.Fatal("bob not found as recipient")
	}
	bobK, err := crypto.DecryptKWithPrivate(bobRec.DEKEnc, bobKP)
	if err != nil {
		t.Fatalf("bob decrypt K: %v", err)
	}
	plain, err := crypto.DecryptPayload(ef.Payload.Nonce, ef.Payload.Ciphertext, bobK)
	if err != nil {
		t.Fatalf("bob decrypt payload: %v", err)
	}
	if len(plain) == 0 {
		t.Fatal("bob got empty payload")
	}

	// Admin removes bob and rotates to K2.
	v3, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	oldGen := v3.File.Rotated
	if err := v3.File.RemoveUser("bob"); err != nil {
		t.Fatal(err)
	}
	if err := v3.Rotate(); err != nil {
		t.Fatal(err)
	}
	if err := v3.Save(); err != nil {
		t.Fatal(err)
	}
	if v3.File.Rotated != oldGen+1 {
		t.Fatalf("generation = %d, want %d", v3.File.Rotated, oldGen+1)
	}

	// After rotation, bob's OLD K must not decrypt the NEW payload.
	ef2, _ := model.Load(path)
	if _, err := crypto.DecryptPayload(ef2.Payload.Nonce, ef2.Payload.Ciphertext, bobK); err == nil {
		t.Fatal("bob's old K should not decrypt rotated payload")
	}

	// Admin can still read everything after rotation.
	v4, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	set4, err := v4.Secrets()
	if err != nil {
		t.Fatal(err)
	}
	if val, _ := set4.Get("STRIPE_KEY"); val != "sk_live_1" {
		t.Fatalf("after rotation STRIPE_KEY = %q", val)
	}
	if val, _ := set4.Get("DATABASE_URL"); val != "postgres://x" {
		t.Fatalf("after rotation DATABASE_URL = %q", val)
	}
}

func TestOpenNonRecipientFails(t *testing.T) {
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	admin, _ := identity.Create("alice")
	repo := t.TempDir()
	path := initEnvis(t, repo, admin)

	// Replace local identity with a stranger not in the file.
	stranger, _ := crypto.GenerateKeyPair()
	ident := &identity.Identity{
		ID:         "mallory",
		PublicKey:  crypto.EncodePublicKey(stranger.Public),
		PrivateKey: crypto.EncodePrivateKey(stranger.Private),
	}
	if err := ident.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err != model.ErrNoUser {
		t.Fatalf("expected ErrNoRecipient, got %v", err)
	}
}

// TestPendingRequestLifecycle verifies the request/approve semantics at the
// data layer: a pending recipient (empty encrypted_k) cannot open the vault;
// after approval (K sealed, status flipped active) it can; and rotation does
// not touch pending entries.
func TestPendingRequestLifecycle(t *testing.T) {
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	admin, _ := identity.Create("alice")
	repo := t.TempDir()
	path := initEnvis(t, repo, admin)

	// Bob files a request: a pending entry with empty encrypted_k.
	bobKP, _ := crypto.GenerateKeyPair()
	bobPub := crypto.EncodePublicKey(bobKP.Public)
	ef, _ := model.Load(path)
	ef.Users = append(ef.Users, model.User{
		ID: "bob", Status: model.StatusPending,
		PK: bobPub, DEKEnc: "",
	})
	if err := ef.Save(path); err != nil {
		t.Fatal(err)
	}

	// Bob's key must NOT be able to open the vault while pending.
	bobIdent := &identity.Identity{
		ID: "bob", PublicKey: bobPub,
		PrivateKey: crypto.EncodePrivateKey(bobKP.Private),
	}
	if err := bobIdent.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err != model.ErrNoUser {
		t.Fatalf("pending recipient should not open vault; got %v", err)
	}

	// Admin approves: reload as admin, seal K for bob's recorded key, flip active.
	admIdent := &identity.Identity{
		ID: admin.ID, PublicKey: admin.PublicKey, PrivateKey: admin.PrivateKey,
	}
	if err := admIdent.Save(); err != nil {
		t.Fatal(err)
	}
	v, err := Open(path)
	if err != nil {
		t.Fatalf("admin open: %v", err)
	}
	rec := v.File.FindUserByID("bob")
	if rec == nil || !rec.IsPending() {
		t.Fatal("expected pending bob")
	}
	enc, err := crypto.EncryptKForRecipient(v.K(), bobKP.Public)
	if err != nil {
		t.Fatal(err)
	}
	rec.DEKEnc = enc
	rec.Status = model.StatusActive
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}

	// Now bob can open the vault.
	if err := bobIdent.Save(); err != nil {
		t.Fatal(err)
	}
	vb, err := Open(path)
	if err != nil {
		t.Fatalf("approved bob should open vault; got %v", err)
	}
	if _, err := vb.Secrets(); err != nil {
		t.Fatalf("bob read secrets: %v", err)
	}
}

// TestRotateSkipsPending ensures rotation does not seal K for a pending
// recipient (their encrypted_k must stay empty).
func TestRotateSkipsPending(t *testing.T) {
	home := t.TempDir()
	os.Setenv("ENVIS_HOME", home)
	t.Cleanup(func() { os.Unsetenv("ENVIS_HOME") })

	admin, _ := identity.Create("alice")
	repo := t.TempDir()
	path := initEnvis(t, repo, admin)

	// Add an active member (carol) and a pending request (bob).
	carolKP, _ := crypto.GenerateKeyPair()
	bobKP, _ := crypto.GenerateKeyPair()

	v, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	encCarol, _ := crypto.EncryptKForRecipient(v.K(), carolKP.Public)
	v.File.Users = append(v.File.Users,
		model.User{ID: "carol", Status: model.StatusActive,
			PK: crypto.EncodePublicKey(carolKP.Public), DEKEnc: encCarol},
		model.User{ID: "bob", Status: model.StatusPending,
			PK: crypto.EncodePublicKey(bobKP.Public), DEKEnc: ""},
	)
	if err := v.Save(); err != nil {
		t.Fatal(err)
	}

	// Reopen and rotate.
	v2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := v2.Rotate(); err != nil {
		t.Fatal(err)
	}
	if err := v2.Save(); err != nil {
		t.Fatal(err)
	}

	// Pending bob's encrypted_k must still be empty; carol's must be non-empty.
	ef, _ := model.Load(path)
	bob := ef.FindUserByID("bob")
	if bob.DEKEnc != "" {
		t.Fatal("rotation should not seal K for a pending recipient")
	}
	carol := ef.FindUserByID("carol")
	if carol.DEKEnc == "" {
		t.Fatal("rotation should re-seal K for active recipient carol")
	}
	// Carol can still decrypt with her key after rotation.
	ck, err := crypto.DecryptKWithPrivate(carol.DEKEnc, carolKP)
	if err != nil {
		t.Fatalf("carol decrypt K after rotation: %v", err)
	}
	if _, err := crypto.DecryptPayload(ef.Payload.Nonce, ef.Payload.Ciphertext, ck); err != nil {
		t.Fatalf("carol decrypt payload after rotation: %v", err)
	}
}
