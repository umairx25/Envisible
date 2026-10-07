package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/envis/envis/internal/model"
	"github.com/envis/envis/internal/vault"
)

// cmdInject implements `envis inject`. It prints shell `export` statements for
// each secret, plus a tracking variable listing injected keys so the shell
// hook can later unset them. Intended for use by the shell hook via
// `eval "$(envis inject)"`.
//
// (This is the shell-integration path. For writing secrets to a dotenv file on
// disk, see `envis export`.)
func cmdInject(args []string) error {
	v, err := vault.OpenFound()
	if err != nil {
		// Not an error in hook context: emit nothing so eval is a no-op.
		return nil
	}
	set, err := v.Secrets()
	if err != nil {
		return err
	}

	keys := set.Keys()
	for _, k := range keys {
		val, _ := set.Get(k)
		fmt.Printf("export %s=%s\n", k, shellQuote(val))
	}
	// Record which keys Envis injected and the rotation count, so the hook
	// can unset them on leaving the directory.
	fmt.Printf("export __ENVIS_INJECTED=%s\n", shellQuote(strings.Join(keys, " ")))
	fmt.Printf("export __ENVIS_ROTATED=%s\n", strconv.Itoa(v.File.Rotated))
	return nil
}

// cmdUninject implements `envis uninject`. It prints `unset` statements for the
// keys recorded in __ENVIS_INJECTED (passed via the environment by the hook).
func cmdUninject(args []string) error {
	injected := os.Getenv("__ENVIS_INJECTED")
	if injected == "" {
		return nil
	}
	for _, k := range strings.Fields(injected) {
		fmt.Printf("unset %s\n", k)
	}
	fmt.Println("unset __ENVIS_INJECTED")
	fmt.Println("unset __ENVIS_ROTATED")
	return nil
}

// shellQuote single-quotes a value for safe inclusion in a shell command,
// escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// cmdHook implements `envis hook [bash|zsh]`. It prints a shell snippet the
// user adds to their rc file. The snippet installs a prompt hook that detects
// whether the current directory is inside an Envis project and injects or
// restores environment variables accordingly.
func cmdHook(args []string) error {
	shell := "bash"
	if len(args) >= 1 {
		shell = args[0]
	}
	switch shell {
	case "bash", "zsh":
		fmt.Print(hookScript(shell))
		return nil
	default:
		return errorf("unsupported shell %q (want bash or zsh)", shell)
	}
}

// hookScript returns the shell integration snippet. Both bash and zsh use a
// precmd-style hook that runs before each prompt.
func hookScript(shell string) string {
	// The core logic is shell-agnostic; only the hook registration differs.
	common := `__envis_find_root() {
  local dir="$PWD"
  while [ "$dir" != "/" ]; do
    if [ -f "$dir/` + model.FileName + `" ]; then
      printf '%s\n' "$dir"
      return 0
    fi
    dir="$(dirname "$dir")"
  done
  return 1
}

__envis_apply() {
  local root
  if root="$(__envis_find_root)"; then
    # Only re-inject if the project root changed since last time.
    if [ "$__ENVIS_ROOT" != "$root" ]; then
      # Leaving a previous project: unset its vars first.
      if [ -n "$__ENVIS_INJECTED" ]; then
        eval "$(envis uninject 2>/dev/null)"
      fi
      eval "$(cd "$root" && envis inject 2>/dev/null)"
      export __ENVIS_ROOT="$root"
    fi
  else
    # Not in a project: clear any previously injected vars.
    if [ -n "$__ENVIS_INJECTED" ]; then
      eval "$(envis uninject 2>/dev/null)"
      unset __ENVIS_ROOT
    fi
  fi
}
`
	switch shell {
	case "zsh":
		return common + `
autoload -Uz add-zsh-hook 2>/dev/null
if typeset -f add-zsh-hook >/dev/null; then
  add-zsh-hook precmd __envis_apply
else
  precmd_functions+=(__envis_apply)
fi
`
	default: // bash
		return common + `
case "$PROMPT_COMMAND" in
  *__envis_apply*) ;;
  *) PROMPT_COMMAND="__envis_apply${PROMPT_COMMAND:+;$PROMPT_COMMAND}" ;;
esac
`
	}
}
