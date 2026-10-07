package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/envis/envis/internal/crypto"
	"github.com/envis/envis/internal/identity"
	"github.com/envis/envis/internal/model"
	"github.com/envis/envis/internal/secrets"
	"github.com/envis/envis/internal/vault"
)

// mask is the fixed placeholder shown for a secret value when values are
// hidden.
const mask = "*****"

// defaultID derives a sensible default recipient id from the OS username and
// hostname (user@host), representing this user/device.
func defaultID() string {
	user := os.Getenv("USER")
	if user == "" {
		user = "user"
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "device"
	}
	return user + "@" + host
}

// cmdInit implements `envis init [--id ID] [--no-hook]`.
func cmdInit(args []string) error {
	var id string
	noHook := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--id":
			if i+1 >= len(args) {
				return errorf("--id requires a value")
			}
			i++
			id = args[i]
		case "--no-hook":
			noHook = true
		default:
			return errorf("unknown argument: %s", args[i])
		}
	}

	// Refuse to overwrite an existing .envis in the current directory.
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	envisPath := filepath.Join(cwd, model.FileName)
	if _, err := os.Stat(envisPath); err == nil {
		return errorf("%s already exists in this directory", model.FileName)
	}

	// Load or create the local identity.
	ident, err := identity.Load()
	if err == identity.ErrNotFound {
		if id == "" {
			id = defaultID()
		}
		ident, err = identity.Create(id)
		if err != nil {
			return err
		}
		infof("Generated new identity %q (private key stored locally).", ident.ID)
	} else if err != nil {
		return err
	} else if id != "" && id != ident.ID {
		ident.ID = id
	}

	// Generate repo key K and encrypt it for the admin (this identity).
	k, err := crypto.GenerateK()
	if err != nil {
		return err
	}
	pub, err := crypto.DecodePublicKey(ident.PublicKey)
	if err != nil {
		return err
	}
	encK, err := crypto.EncryptKForRecipient(k, pub)
	if err != nil {
		return err
	}

	ef := model.New()
	ef.Users = []model.User{{
		ID:     ident.ID,
		Status: model.StatusActive,
		PK:     ident.PublicKey,
		DEKEnc: encK,
	}}

	// Empty initial secrets.
	set := secrets.New()
	nonce, ciphertext, err := crypto.EncryptPayload(set.Serialize(), k)
	if err != nil {
		return err
	}
	ef.Payload = model.Payload{Nonce: nonce, Ciphertext: ciphertext}

	if err := ef.Save(envisPath); err != nil {
		return err
	}

	infof("Initialized %s as %q.", model.FileName, ident.ID)
	infof("Commit %s to share encrypted secrets with your team.", model.FileName)
	infof("Import existing variables with `envis import .env` if you have a dotenv file.")

	if noHook {
		infof("Skipped shell hook install (--no-hook). Secrets won't auto-load; add `eval \"$(envis hook zsh)\"` to your shell startup file to enable it.")
	} else {
		_, msg := installHook()
		infof("%s", msg)
	}
	return nil
}

// cmdSet implements `envis set [NAME [VALUE]]`.
//
//	envis set NAME VALUE   -> set directly
//	envis set NAME         -> prompt for value (no echo)
//	envis set              -> on a TTY, show a menu of existing names plus a
//	                          "new secret" entry: picking an existing name
//	                          prompts for its new value; picking "new" prompts
//	                          for a name then a value. Non-interactively it
//	                          prompts for name then value line-by-line.
func cmdSet(args []string) error {
	if len(args) > 2 {
		return errorf("usage: envis set [NAME [VALUE]]")
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	var name, value string
	switch len(args) {
	case 2:
		name, value = args[0], args[1]
	case 1:
		name = args[0]
		value, err = promptSecret(fmt.Sprintf("Value for %s: ", name))
		if err != nil {
			return err
		}
	case 0:
		name, value, err = setInteractive(set)
		if err != nil {
			if err == errMenuCancelled {
				infof("Cancelled.")
				return nil
			}
			return err
		}
	}

	set.Set(name, value)
	if err := v.WriteSecrets(set); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	infof("Set %s.", name)
	return nil
}

// newSecretLabel is the sentinel menu entry for creating a new secret.
const newSecretLabel = "+ new secret"

// setInteractive drives the no-argument `set` flow and returns the chosen
// name and value.
func setInteractive(set *secrets.Set) (name, value string, err error) {
	keys := set.Keys()

	if isInteractive() && len(keys) > 0 {
		items := append([]string{newSecretLabel}, keys...)
		idx, serr := selectMenu("Select a secret to update, or create a new one:", items)
		if serr != nil {
			return "", "", serr
		}
		if idx == 0 {
			name, err = promptLine("New secret name: ")
			if err != nil {
				return "", "", err
			}
			if name == "" {
				return "", "", errorf("name cannot be empty")
			}
		} else {
			name = items[idx]
		}
		value, err = promptSecret(fmt.Sprintf("Value for %s: ", name))
		if err != nil {
			return "", "", err
		}
		return name, value, nil
	}

	// Non-interactive (or no existing secrets): prompt name then value.
	name, err = promptLine("Name: ")
	if err != nil {
		return "", "", err
	}
	if name == "" {
		return "", "", errorf("name cannot be empty")
	}
	value, err = promptSecret(fmt.Sprintf("Value for %s: ", name))
	if err != nil {
		return "", "", err
	}
	return name, value, nil
}

// cmdDelete implements `envis delete [NAME]`.
//
// With NAME it deletes that secret. With no NAME on an interactive terminal it
// shows an arrow-key menu of secret names to pick from. Either way it asks for
// confirmation before deleting. Non-interactively, NAME is required.
func cmdDelete(args []string) error {
	if len(args) > 1 {
		return errorf("usage: envis delete [NAME]")
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	var key string
	if len(args) == 1 {
		key = args[0]
	} else {
		if !isInteractive() {
			return errorf("usage: envis delete NAME")
		}
		keys := set.Keys()
		if len(keys) == 0 {
			return errorf("no secrets to delete")
		}
		idx, err := selectMenu("Select a secret to delete:", keys)
		if err != nil {
			if err == errMenuCancelled {
				infof("Cancelled.")
				return nil
			}
			return err
		}
		key = keys[idx]
	}

	if _, ok := set.Get(key); !ok {
		return errorf("secret %q not found", key)
	}

	ok, err := confirm(fmt.Sprintf("Delete %s?", key))
	if err != nil {
		return err
	}
	if !ok {
		infof("Cancelled.")
		return nil
	}

	set.Delete(key)
	if err := v.WriteSecrets(set); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	infof("Deleted %s.", key)
	return nil
}

// cmdGet implements `envis get [NAME] [--show]`.
//
//	envis get              -> on a TTY, show an arrow-key menu of secret names
//	                          plus an "all" entry. Selecting a name prints that
//	                          secret's value; selecting "all" prints every
//	                          value. (Drilling in always reveals the value.)
//	                          Non-interactively, prints all names masked
//	                          (NAME: *****) unless --show is given.
//	envis get NAME          -> prints that secret masked, or revealed with --show
//	--show (anywhere)       -> reveal values in the non-interactive paths
func cmdGet(args []string) error {
	show := false
	var name string
	nameSet := false
	for _, a := range args {
		switch a {
		case "--show":
			show = true
		default:
			if nameSet {
				return errorf("usage: envis get [NAME] [--show]")
			}
			name = a
			nameSet = true
		}
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	printKV := func(k string) {
		val, _ := set.Get(k)
		fmt.Printf("%s: %s\n", k, val)
	}
	printMasked := func(k string) {
		fmt.Printf("%s: %s\n", k, mask)
	}

	// Specific name requested.
	if nameSet {
		if _, ok := set.Get(name); !ok {
			return errorf("secret %q not found", name)
		}
		if show {
			printKV(name)
		} else {
			printMasked(name)
		}
		return nil
	}

	// No name: interactive menu on a TTY.
	if isInteractive() && !show {
		keys := set.Keys()
		if len(keys) == 0 {
			infof("No secrets yet.")
			return nil
		}
		const allLabel = "all"
		items := append([]string{allLabel}, keys...)
		idx, err := selectMenu("Select a secret to reveal, or 'all':", items)
		if err != nil {
			if err == errMenuCancelled {
				infof("Cancelled.")
				return nil
			}
			return err
		}
		if idx == 0 {
			for _, k := range keys {
				printKV(k)
			}
		} else {
			printKV(items[idx])
		}
		return nil
	}

	// Non-interactive (or --show): list everything, masked unless --show.
	for _, k := range set.Keys() {
		if show {
			printKV(k)
		} else {
			printMasked(k)
		}
	}
	return nil
}
