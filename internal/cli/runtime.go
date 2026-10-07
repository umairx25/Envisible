package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/envis/envis/internal/model"
	"github.com/envis/envis/internal/vault"
)

// parseShellArg returns the shell named by the first argument, defaulting to
// "bash". Accepts bash, zsh, powershell (aliases: pwsh, ps). This lets the
// generated hook tell inject/uninject which syntax to emit.
func parseShellArg(args []string) (string, error) {
	if len(args) == 0 {
		return "bash", nil
	}
	switch args[0] {
	case "bash", "zsh":
		return "bash", nil // POSIX shells share export/unset syntax
	case "powershell", "pwsh", "ps":
		return "powershell", nil
	default:
		return "", errorf("unsupported shell %q (want bash, zsh, or powershell)", args[0])
	}
}

// cmdInject implements `envis inject [bash|zsh|powershell]`. It prints
// statements that set each secret as an environment variable, plus tracking
// variables listing the injected keys so the hook can later unset them.
//
// POSIX shells get `export NAME='value'`; PowerShell gets `$env:NAME='value'`.
// Intended for use by the shell hook (e.g. `eval "$(envis inject)"` or, in
// PowerShell, `envis inject powershell | Invoke-Expression`).
//
// For writing secrets to a dotenv file on disk, see `envis export`.
func cmdInject(args []string) error {
	shell, err := parseShellArg(args)
	if err != nil {
		return err
	}

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
	joined := strings.Join(keys, " ")
	rotated := strconv.Itoa(v.File.Rotated)

	if shell == "powershell" {
		for _, k := range keys {
			val, _ := set.Get(k)
			fmt.Printf("$env:%s = %s\n", k, psQuote(val))
		}
		fmt.Printf("$env:__ENVIS_INJECTED = %s\n", psQuote(joined))
		fmt.Printf("$env:__ENVIS_ROTATED = %s\n", psQuote(rotated))
		return nil
	}

	for _, k := range keys {
		val, _ := set.Get(k)
		fmt.Printf("export %s=%s\n", k, shellQuote(val))
	}
	fmt.Printf("export __ENVIS_INJECTED=%s\n", shellQuote(joined))
	fmt.Printf("export __ENVIS_ROTATED=%s\n", shellQuote(rotated))
	return nil
}

// cmdUninject implements `envis uninject [bash|zsh|powershell]`. It prints
// statements that unset the keys recorded in __ENVIS_INJECTED (passed via the
// environment by the hook).
func cmdUninject(args []string) error {
	shell, err := parseShellArg(args)
	if err != nil {
		return err
	}

	injected := os.Getenv("__ENVIS_INJECTED")
	if injected == "" {
		return nil
	}
	keys := strings.Fields(injected)

	if shell == "powershell" {
		for _, k := range keys {
			fmt.Printf("Remove-Item Env:%s -ErrorAction SilentlyContinue\n", k)
		}
		fmt.Println("Remove-Item Env:__ENVIS_INJECTED -ErrorAction SilentlyContinue")
		fmt.Println("Remove-Item Env:__ENVIS_ROTATED -ErrorAction SilentlyContinue")
		return nil
	}

	for _, k := range keys {
		fmt.Printf("unset %s\n", k)
	}
	fmt.Println("unset __ENVIS_INJECTED")
	fmt.Println("unset __ENVIS_ROTATED")
	return nil
}

// shellQuote single-quotes a value for safe inclusion in a POSIX shell command,
// escaping embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// psQuote single-quotes a value for PowerShell. In single-quoted PowerShell
// strings the only character needing escaping is the single quote itself,
// which is doubled.
func psQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// cmdHook implements `envis hook [bash|zsh|powershell]`. It prints a snippet
// the user adds to their shell startup file. The snippet installs a hook that
// detects whether the current directory is inside an Envis project and injects
// or restores environment variables accordingly.
func cmdHook(args []string) error {
	shell := "bash"
	if len(args) >= 1 {
		shell = args[0]
	}
	switch shell {
	case "bash", "zsh":
		fmt.Print(hookScriptPOSIX(shell))
		return nil
	case "powershell", "pwsh", "ps":
		fmt.Print(hookScriptPowerShell())
		return nil
	default:
		return errorf("unsupported shell %q (want bash, zsh, or powershell)", shell)
	}
}

// hookScriptPOSIX returns the bash/zsh integration snippet. Both use a
// precmd-style hook that runs before each prompt.
func hookScriptPOSIX(shell string) string {
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

// hookScriptPowerShell returns the PowerShell integration snippet. PowerShell
// has no precmd hook, so it wraps the `prompt` function: our logic runs each
// time the prompt is drawn (i.e. after every command), mirroring the POSIX
// behavior. Secrets are injected on entering an Envis project directory and
// removed on leaving it.
func hookScriptPowerShell() string {
	return `function global:__Envis-FindRoot {
  $dir = (Get-Location).Path
  while ($dir) {
    if (Test-Path (Join-Path $dir '` + model.FileName + `')) { return $dir }
    $parent = Split-Path $dir -Parent
    if ($parent -eq $dir) { break }
    $dir = $parent
  }
  return $null
}

function global:__Envis-Apply {
  $root = __Envis-FindRoot
  if ($root) {
    if ($env:__ENVIS_ROOT -ne $root) {
      if ($env:__ENVIS_INJECTED) { envis uninject powershell | Invoke-Expression }
      Push-Location $root
      try { envis inject powershell | Invoke-Expression } finally { Pop-Location }
      $env:__ENVIS_ROOT = $root
    }
  } else {
    if ($env:__ENVIS_INJECTED) {
      envis uninject powershell | Invoke-Expression
      Remove-Item Env:__ENVIS_ROOT -ErrorAction SilentlyContinue
    }
  }
}

# Wrap the existing prompt so our hook runs before each prompt is drawn.
if (-not (Test-Path function:global:__Envis-OrigPrompt)) {
  Copy-Item function:prompt function:global:__Envis-OrigPrompt
}
function global:prompt {
  __Envis-Apply
  __Envis-OrigPrompt
}
`
}
