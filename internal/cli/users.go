package cli

import (
	"fmt"
	"strings"

	"github.com/envis/envis/internal/crypto"
	"github.com/envis/envis/internal/identity"
	"github.com/envis/envis/internal/model"
	"github.com/envis/envis/internal/vault"
)

// cmdGrant implements `envis grant [USERNAME [PUBLIC_KEY]]`.
//
//	envis grant                   -> on a TTY, show a menu of pending requests
//	                                 to grant; otherwise prompt for a username
//	envis grant USERNAME          -> resolve a pending request by id
//	envis grant USERNAME PUBKEY   -> add directly (no request filed)
//
// Granting seals the current repo key K for the user's public key and marks
// them active. Secrets are NOT re-encrypted.
func cmdGrant(args []string) error {
	var positional []string
	positional = append(positional, args...)

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}

	var id, pubKeyStr string
	switch len(positional) {
	case 0:
		id, err = pickOrPromptUsername(v, "grant")
		if err != nil {
			if err == errMenuCancelled {
				infof("Cancelled.")
				return nil
			}
			return err
		}
	case 1:
		if fields := strings.Fields(positional[0]); len(fields) == 2 {
			id, pubKeyStr = fields[0], fields[1]
		} else {
			id = positional[0]
		}
	case 2:
		id, pubKeyStr = positional[0], positional[1]
	default:
		return errorf("usage: envis grant [USERNAME [PUBLIC_KEY]]")
	}

	pending := v.File.FindUserByID(id)
	if pending != nil && !pending.IsPending() {
		return errorf("%q is already an active user", id)
	}

	// Confirm before granting.
	ok, err := confirm(fmt.Sprintf("Grant %q access?", id))
	if err != nil {
		return err
	}
	if !ok {
		infof("Cancelled.")
		return nil
	}

	switch {
	case pending != nil:
		// Resolve a pending request using its recorded public key.
		if pubKeyStr != "" && pubKeyStr != pending.PK {
			return errorf("provided public key does not match the pending request for %q", id)
		}
		pub, err := crypto.DecodePublicKey(pending.PK)
		if err != nil {
			return err
		}
		encK, err := crypto.EncryptKForRecipient(v.K(), pub)
		if err != nil {
			return err
		}
		pending.DEKEnc = encK
		pending.Status = model.StatusActive
		if err := v.Save(); err != nil {
			return err
		}
		infof("Granted %q access. Commit %s so they can pull it.", id, model.FileName)
		return nil

	default:
		// No pending request: a direct grant requires a public key.
		if pubKeyStr == "" {
			return errorf("no pending request for %q.\nHave them run `envis request`, or provide a public key:\n    envis grant %s <public-key>", id, id)
		}
		pub, err := crypto.DecodePublicKey(pubKeyStr)
		if err != nil {
			return err
		}
		if existing := v.File.FindUserByPK(pubKeyStr); existing != nil {
			return errorf("public key already recorded as %q", existing.ID)
		}
		encK, err := crypto.EncryptKForRecipient(v.K(), pub)
		if err != nil {
			return err
		}
		if err := v.File.AddUser(model.User{
			ID:     id,
			Status: model.StatusActive,
			PK:     pubKeyStr,
			DEKEnc: encK,
		}); err != nil {
			return err
		}
		if err := v.Save(); err != nil {
			return err
		}
		infof("Granted %q access. Commit %s so they can pull it.", id, model.FileName)
		return nil
	}
}

// pickOrPromptUsername returns a username for grant: on a TTY it shows a menu
// of pending requests; otherwise (or if there are none to list) it prompts for
// a username on a line.
func pickOrPromptUsername(v *vault.Vault, action string) (string, error) {
	pending := v.File.PendingUsers()
	if isInteractive() && len(pending) > 0 {
		ids := make([]string, len(pending))
		for i, u := range pending {
			ids[i] = u.ID
		}
		idx, err := selectMenu("Select a pending request to grant:", ids)
		if err != nil {
			return "", err
		}
		return ids[idx], nil
	}
	id, err := promptLine("Username to " + action + ": ")
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", errorf("username cannot be empty")
	}
	return id, nil
}

// cmdRevoke implements `envis revoke [USERNAME]`.
//
// With no USERNAME on a TTY it shows a menu of users (excluding yourself) to
// pick from; otherwise it prompts for a username. Revoking an active user
// removes them and rotates the repo key so they cannot decrypt future state.
// Revoking a pending request simply drops it. Either way it asks for
// confirmation first.
func cmdRevoke(args []string) error {
	if len(args) > 1 {
		return errorf("usage: envis revoke [USERNAME]")
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}

	var id string
	if len(args) == 1 {
		id = args[0]
	} else {
		// Build the list of revokable users (everyone except me).
		me := v.Identity.PublicKey
		var others []string
		for _, u := range v.File.Users {
			if u.PK != me {
				others = append(others, u.ID)
			}
		}
		if isInteractive() && len(others) > 0 {
			idx, serr := selectMenu("Select a user to revoke:", others)
			if serr != nil {
				if serr == errMenuCancelled {
					infof("Cancelled.")
					return nil
				}
				return serr
			}
			id = others[idx]
		} else {
			id, err = promptLine("Username to revoke: ")
			if err != nil {
				return err
			}
			if id == "" {
				return errorf("username cannot be empty")
			}
		}
	}

	target := v.File.FindUserByID(id)
	if target == nil {
		return errorf("user %q not found", id)
	}
	if target.PK == v.Identity.PublicKey {
		return errorf("refusing to revoke yourself (%q)", id)
	}

	ok, err := confirm(fmt.Sprintf("Revoke %q?", id))
	if err != nil {
		return err
	}
	if !ok {
		infof("Cancelled.")
		return nil
	}

	// Dropping a pending request grants/revokes no access, so no rotation.
	if target.IsPending() {
		if err := v.File.RemoveUser(id); err != nil {
			return err
		}
		if err := v.Save(); err != nil {
			return err
		}
		infof("Dropped pending request %q.", id)
		return nil
	}

	if len(v.File.ActiveUsers()) == 1 {
		return errorf("cannot revoke the only active user %q", id)
	}

	if err := v.File.RemoveUser(id); err != nil {
		return err
	}

	// Rotate K so the revoked user cannot decrypt future state.
	if err := v.Rotate(); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	infof("Revoked %q and rotated the repo key (rotated %d).", id, v.File.Rotated)
	infof("Note: any provider credentials they already saw should be rotated upstream.")
	return nil
}

// cmdRotate implements `envis rotate`.
//
// It generates a fresh repo key, re-encrypts the secrets payload under it, and
// re-seals the new key for every active user. Pending users are left untouched
// (they hold no key). This does not change who has access; it only replaces
// the key material. Requires the caller to currently hold K.
func cmdRotate(args []string) error {
	if len(args) != 0 {
		return errorf("usage: envis rotate")
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}

	active := len(v.File.ActiveUsers())
	infof("This will generate a new repo key, re-encrypt all secrets, and re-seal")
	infof("the key for %d active user(s). Access is unchanged.", active)
	ok, err := confirm("Proceed with rotation?")
	if err != nil {
		return err
	}
	if !ok {
		infof("Rotation cancelled.")
		return nil
	}

	if err := v.Rotate(); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	infof("Rotated the repo key (rotated %d).", v.File.Rotated)
	return nil
}

// cmdStatus implements `envis status`. It reports this device's identity and
// its relationship to the current project's .envis file.
func cmdStatus(args []string) error {
	ident, err := identity.Load()
	haveIdentity := err == nil
	if err != nil && err != identity.ErrNotFound {
		return err
	}

	if haveIdentity {
		fmt.Printf("Device:     %s\n", ident.ID)
		fmt.Printf("Public key: %s\n", ident.PublicKey)
	} else {
		fmt.Println("Device:     (no local identity yet)")
	}

	// Locate the project .envis, if any.
	path, ferr := model.Find()
	if ferr != nil {
		infof("")
		infof("Envis project not initialized here. Run `envis init`, or pull a")
		infof("committed %s file if one exists in your repository.", model.FileName)
		return nil
	}

	ef, err := model.Load(path)
	if err != nil {
		return err
	}
	fmt.Printf("Project:    %s (rotated %d)\n", path, ef.Rotated)

	if !haveIdentity {
		infof("")
		infof("You don't have access yet. Run `envis request` to request it.")
		return nil
	}

	u := ef.FindUserByID(ident.ID)
	if u == nil {
		if byKey := ef.FindUserByPK(ident.PublicKey); byKey != nil {
			u = byKey
		}
	}

	switch {
	case u == nil:
		fmt.Printf("Access:     none\n")
		infof("")
		infof("You don't have access yet. Run `envis request` to request it,")
		infof("then commit %s and notify an admin.", model.FileName)
	case u.IsPending():
		fmt.Printf("Access:     pending\n")
		infof("")
		infof("Your access request is recorded and waiting for an admin to run")
		infof("`envis grant %s`. Make sure you committed %s and notified them.", u.ID, model.FileName)
	default:
		fmt.Printf("Access:     active\n")
	}
	return nil
}

// cmdUsers implements `envis users`: lists users with status.
func cmdUsers(args []string) error {
	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	me := v.Identity.PublicKey
	for i := range v.File.Users {
		u := &v.File.Users[i]
		marker := " "
		if u.PK == me {
			marker = "*"
		}
		status := model.StatusActive
		if u.IsPending() {
			status = model.StatusPending
		}
		fmt.Printf("%s %-24s %-7s %s\n", marker, u.ID, status, u.PK)
	}
	return nil
}
