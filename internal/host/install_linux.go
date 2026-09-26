//go:build linux

package host

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/raffleberry/udm/internal/cfg"
)

// Install wires the host into Firefox and makes sure a udm binary sits next to
// it, so the lazy launch works from the browser with no PATH involved.
//
// It must be run from internal/host, which is where the justfile points.
func Install() error {
	dir, err := cfg.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	hostBin, err := buildHost(dir)
	if err != nil {
		return err
	}
	if err := os.Chmod(hostBin, 0o755); err != nil {
		return err
	}

	extfiles := filepath.Join(dir, "extfiles")
	if err := os.MkdirAll(extfiles, 0o755); err != nil {
		return err
	}
	staged := filepath.Join(extfiles, manifestName)
	if err := writeManifest(staged, hostBin); err != nil {
		return err
	}

	ff, err := firefoxDir()
	if err != nil {
		return err
	}
	live := filepath.Join(ff, manifestName)
	if err := copyFile(staged, live); err != nil {
		return err
	}
	slog.Info("host: manifest installed", "path", live)

	if err := installIcon(); err != nil {
		slog.Warn("host: icon not installed, the tray will use a fallback", "err", err)
	}

	// Copy a udm binary in beside the host if there is one. exe() checks this
	// location first, which is what makes a double-clicked launch work.
	if p, err := exec.LookPath(binName()); err == nil {
		dst := filepath.Join(dir, binName())
		if err := copyFile(p, dst); err != nil {
			slog.Warn("host: could not copy the udm binary", "err", err)
		} else {
			slog.Info("host: udm binary installed", "path", dst)
		}
	}

	fmt.Printf("udm host installed\n  host:     %s\n  manifest: %s\n\nReload the udm extension in Firefox.\n",
		hostBin, live)
	return nil
}

func firefoxDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(home, ".mozilla", "native-messaging-hosts")
	return d, os.MkdirAll(d, 0o755)
}
