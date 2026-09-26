// Command host is the Firefox native messaging host. Firefox starts it and talks
// to it over stdio; everything else it does is HTTP to the udm app.
package main

import (
	"log/slog"
	"os"

	"github.com/raffleberry/udm/internal/host"
)

func main() {
	// The browser is reading our stdout, so logs go to stderr to keep the
	// protocol stream clean.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	if err := host.Run(); err != nil {
		slog.Error("host: exiting", "err", err)
		os.Exit(1)
	}
}
