package cli

import (
	"fmt"

	"github.com/envis/envis/internal/identity"
	"github.com/envis/envis/internal/model"
)

// cmdRequest implements `envis request`.
//
// It records this device's identity (id + public key) in the committed .envis
// file as a PENDING user with an empty dek_enc. This grants no access: a
// pending user cannot recover K and therefore cannot decrypt anything. An
// admin later resolves the request with `envis grant <id>`.
//
// Unlike most commands, request does not require the local key to already be an
// authorized user — the whole point is that the requester has no access yet.
// It therefore operates on the .envis file directly rather than opening a
// vault.
func cmdRequest(args []string) error {
	if len(args) != 0 {
		return errorf("usage: envis request")
	}

	// Ensure a local identity exists; create one if needed so the user does
	// not need any separate setup step.
	ident, err := identity.Load()
	if err == identity.ErrNotFound {
		ident, err = identity.Create(defaultID())
		if err != nil {
			return err
		}
		infof("Generated new identity %q (private key stored locally).", ident.ID)
	} else if err != nil {
		return err
	}

	path, err := model.Find()
	if err != nil {
		return err
	}
	ef, err := model.Load(path)
	if err != nil {
		return err
	}

	// If an entry already exists for this public key, handle gracefully.
	if existing := ef.FindUserByPK(ident.PublicKey); existing != nil {
		if existing.IsActive() {
			return errorf("you already have access as %q", existing.ID)
		}
		// Already pending: refresh id in case it changed, no error.
		existing.ID = ident.ID
		existing.Status = model.StatusPending
		existing.DEKEnc = ""
		if err := ef.Save(path); err != nil {
			return err
		}
		infof("Access request for %q is already pending (refreshed).", ident.ID)
		infof("Commit %s and ask an admin to run `envis grant %s`.", model.FileName, ident.ID)
		return nil
	}

	// Guard against colliding with an existing id held by a different key.
	if byID := ef.FindUserByID(ident.ID); byID != nil {
		return errorf("id %q is already in use by a different key in %s", ident.ID, model.FileName)
	}

	ef.Users = append(ef.Users, model.User{
		ID:     ident.ID,
		Status: model.StatusPending,
		PK:     ident.PublicKey,
		DEKEnc: "",
	})

	if err := ef.Save(path); err != nil {
		return err
	}

	infof("Requested access as %q (pending).", ident.ID)
	infof("Commit %s and ask an admin to run `envis grant %s`.", model.FileName, ident.ID)
	return nil
}

// cmdRequests implements `envis requests`: lists pending access requests.
func cmdRequests(args []string) error {
	path, err := model.Find()
	if err != nil {
		return err
	}
	ef, err := model.Load(path)
	if err != nil {
		return err
	}
	pending := ef.PendingUsers()
	if len(pending) == 0 {
		infof("No pending access requests.")
		return nil
	}
	for _, u := range pending {
		fmt.Printf("%-24s %s\n", u.ID, u.PK)
	}
	return nil
}
