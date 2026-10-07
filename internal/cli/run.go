package cli

import (
	"fmt"
	"os"
)

const usage = `envis — Git-native encrypted environment manager

Usage:
  envis <command> [arguments]

Secrets:
  init [--id ID] [--no-hook]     Initialize .envis in this directory
  set [NAME [VALUE]]             Create or update a secret. Use without args for interactive mode.
  delete NAME                    Delete a secret. Use without args for interactive mode.
  get [NAME] [--show]            Show secret names (masked); --show reveals values. Use without args for interactive mode.
  import [FILE]                  Import secrets from a dotenv file. Defaults to .env
  export [FILE]                  Write secrets to a dotenv file. Defaults to .env or if .env is present, .env[number].

Access:
  request                        Request access: record this device as a pending user
  requests                       List pending access requests
  grant [USERNAME [PUBLIC_KEY]]  Grant access to a pending request. Use without args for interactive mode.
  revoke [USERNAME]              Revoke a user (rotates the key). Use without args for interactive mode.
  rotate                         Generate a new repo key and re-seal it for active users
  users                          List users and their status
  status                         Show this device's identity and access state

Runtime:
  inject                         Print shell 'export' statements (used by the shell hook)
  uninject                       Print 'unset' statements for previously injected keys
  hook [bash|zsh]                Print shell integration for automatic cd-based injection

  help                           Show this help

Private keys never leave this device. Only .envis is committed to Git.
`

// Run dispatches a command. It returns an exit code.
func Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	cmd, rest := args[0], args[1:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(rest)
	case "set":
		err = cmdSet(rest)
	case "delete", "rm", "unset":
		err = cmdDelete(rest)
	case "get":
		err = cmdGet(rest)
	case "import":
		err = cmdImport(rest)
	case "export":
		err = cmdExportFile(rest)
	case "request":
		err = cmdRequest(rest)
	case "requests":
		err = cmdRequests(rest)
	case "grant":
		err = cmdGrant(rest)
	case "revoke":
		err = cmdRevoke(rest)
	case "rotate":
		err = cmdRotate(rest)
	case "users", "who":
		err = cmdUsers(rest)
	case "status":
		err = cmdStatus(rest)
	case "inject":
		err = cmdInject(rest)
	case "uninject":
		err = cmdUninject(rest)
	case "hook":
		err = cmdHook(rest)
	case "help", "-h", "--help":
		fmt.Fprint(os.Stderr, usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "envis: %v\n", err)
		return 1
	}
	return 0
}
