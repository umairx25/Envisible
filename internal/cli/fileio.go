package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/envis/envis/internal/secrets"
	"github.com/envis/envis/internal/vault"
)

// cmdImport implements `envis import [FILE]`.
//
// It reads a dotenv-shaped file (KEY=VALUE lines, with optional `export `
// prefix and quoted values) and merges those secrets into the vault. Existing
// keys are overwritten by the imported values. If FILE is omitted the user is
// prompted for a path. Defaults to .env when the prompt is left blank.
func cmdImport(args []string) error {
	if len(args) > 1 {
		return errorf("usage: envis import [FILE]")
	}

	var path string
	if len(args) == 1 {
		path = args[0]
	} else {
		p, err := promptLine("File to import [.env]: ")
		if err != nil {
			return err
		}
		if p == "" {
			p = ".env"
		}
		path = p
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", path, err)
	}
	imported, err := secrets.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", path, err)
	}

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	n := 0
	for _, k := range imported.Keys() {
		val, _ := imported.Get(k)
		set.Set(k, val)
		n++
	}

	if err := v.WriteSecrets(set); err != nil {
		return err
	}
	if err := v.Save(); err != nil {
		return err
	}
	infof("Imported %d secret(s) from %s.", n, path)
	return nil
}

// cmdExportFile implements `envis export [FILE]`.
//
// It writes all secrets to a dotenv file as KEY=value lines. If FILE is
// omitted it defaults to .env. To avoid clobbering an existing file, if the
// target already exists it writes to .env1, .env2, ... (for the default) or
// FILE1, FILE2, ... (for an explicit name) using the first available name.
func cmdExportFile(args []string) error {
	if len(args) > 1 {
		return errorf("usage: envis export [FILE]")
	}

	base := ".env"
	if len(args) == 1 {
		base = args[0]
	}

	target := nextAvailableName(base)

	v, err := vault.OpenFound()
	if err != nil {
		return err
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	var b strings.Builder
	for _, k := range set.Keys() {
		val, _ := set.Get(k)
		fmt.Fprintf(&b, "%s=%s\n", k, dotenvQuote(val))
	}

	// Write with user-only permissions: this is plaintext on disk.
	if err := os.WriteFile(target, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", target, err)
	}
	infof("Wrote %d secret(s) to %s.", set.Len(), target)
	infof("This file is plaintext — do not commit it.")
	return nil
}

// nextAvailableName returns base if it does not exist, otherwise base+"1",
// base+"2", ... until an unused name is found.
func nextAvailableName(base string) string {
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return base
	}
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s%d", base, i)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// dotenvQuote returns the value quoted only when necessary. A value needs
// quoting if it is empty or contains whitespace, quotes, '#', or '=' which
// would otherwise be ambiguous in a dotenv file. Double quotes are used and
// embedded double quotes/backslashes are escaped.
func dotenvQuote(v string) string {
	if v == "" {
		return `""`
	}
	if !strings.ContainsAny(v, " \t\r\n\"'#=") {
		return v
	}
	esc := strings.ReplaceAll(v, `\`, `\\`)
	esc = strings.ReplaceAll(esc, `"`, `\"`)
	return `"` + esc + `"`
}
