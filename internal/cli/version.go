package cli

import (
	"fmt"
	"runtime"
)

// Version is the released version string. It is overridden at build time via
//
//	-ldflags "-X github.com/envis/envis/internal/cli.Version=v1.2.3"
//
// For local/dev builds it stays "dev".
var Version = "dev"

// cmdVersion implements `envis version`. It prints the version and build
// platform.
func cmdVersion(args []string) error {
	fmt.Printf("envis %s (%s/%s)\n", Version, runtime.GOOS, runtime.GOARCH)
	return nil
}
