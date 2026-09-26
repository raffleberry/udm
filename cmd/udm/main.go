// Command udm is the download manager: one process holding the download core,
// the localhost API the browser extension talks to, the system tray, and the
// window. It doubles as the launcher the native messaging host starts when the
// extension finds nobody home.
//
// All the dependency wiring happens here, in one readable block, because that is
// the entire point of the interfaces in internal/core/ports.go.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	tk "modernc.org/tk9.0"

	"github.com/raffleberry/udm/internal/api"
	"github.com/raffleberry/udm/internal/cfg"
	"github.com/raffleberry/udm/internal/core"
	dlgrab "github.com/raffleberry/udm/internal/dl/grab"
	"github.com/raffleberry/udm/internal/tray"
	"github.com/raffleberry/udm/internal/ui"
)

func main() {
	printPort := flag.Bool("print-port", false, "print the configured API port and exit")
	flag.Parse()

	c, err := cfg.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "udm:", err)
		os.Exit(1)
	}
	if *printPort {
		fmt.Println(c.Port)
		return
	}

	f, err := os.OpenFile(logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "udm: cannot open log:", err)
		os.Exit(1)
	}
	defer f.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
	slog.Info("udm: start", "dir", c.Dir, "port", c.Port, "conc", c.MaxConc)

	// The one place a transfer engine is chosen. Swapping grab for aria2 or
	// anything else is a one-line change here, plus a package satisfying
	// dl.Downloader.
	svc := core.New(dlgrab.New(), core.Options{Dir: c.Dir, MaxConc: c.MaxConc})
	defer svc.Close()

	srv, err := api.New(c.Port, svc, svc, svc, &c)
	if err != nil {
		slog.Error("udm: api", "err", err)
		os.Exit(1)
	}
	srv.Start()
	defer srv.Stop()

	// before runs inside the window's quit path, so the servers are still
	// healthy while they are being asked to stop.
	ui.Run(ui.App{
		Svc:  svc,
		Cfg:  &c,
		Tray: tray.New(),
		Save: func(c *cfg.Config) error { return c.Save() },
	}, func() { slog.Info("udm: exiting") })

	tk.Finalize()
}

func logPath() string {
	d, err := cfg.Dir()
	if err != nil {
		return "udm.log"
	}
	return filepath.Join(d, "udm.log")
}
