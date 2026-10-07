// Command envis is a Git-native encrypted environment manager.
//
// One committed .envis file holds authorized recipients (public keys plus the
// repo key encrypted for each) and an encrypted secrets payload. Private keys
// never leave the local device.
package main

import (
	"os"

	"github.com/envis/envis/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
