// Command install registers the native messaging host with Firefox, builds the
// host binary if needed, and drops the app icon into the hicolor theme.
//
// Run it from internal/host:
//
//	go run ./internal/host/cmd/install     (or: just install-host)
package main

import (
	"fmt"
	"os"

	"github.com/raffleberry/udm/internal/host"
)

func main() {
	if err := host.Install(); err != nil {
		fmt.Fprintln(os.Stderr, "install:", err)
		os.Exit(1)
	}
}
