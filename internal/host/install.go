package host

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// HostName is the native messaging host name. The extension's
// browser.runtime.connectNative call uses the same string; the two must stay
// identical or Firefox will not start the host.
const HostName = "raffleberry.udm"

// manifestName is the file Firefox looks for.
const manifestName = HostName + ".json"

// manifestTmpl is the manifest written at install time. The binary path is
// absolute because Firefox will not resolve a relative one; allowed_extensions
// pins the Firefox add-on id, which is what stops another extension from
// driving your download manager.
const manifestTmpl = `{
  "name": %q,
  "description": "udm — hands downloads to the udm download manager",
  "path": %q,
  "type": "stdio",
  "allowed_extensions": ["udm@raffleberry"]
}
`

// buildHost returns the compiled host binary inside dir, building it if it is
// not there yet. Building on demand is what lets the installer be run straight
// out of a checkout with `go run`.
func buildHost(dir string) (string, error) {
	out := filepath.Join(dir, "udm-browser-integration-host")
	if isFile(out) {
		slog.Info("host: reusing binary", "path", out)
		return out, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// The installer is run from internal/host, so the entrypoint is under cmd/.
	src := filepath.Join(wd, "cmd", "host", "main.go")
	if !isFile(src) {
		return "", fmt.Errorf("cannot find %s: run the installer from internal/host", src)
	}
	slog.Info("host: compiling", "path", out)

	cmd := exec.Command("go", "build", "-o", out, src)
	if b, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %v: %s", err, b)
	}
	return out, nil
}

// writeManifest renders the manifest with the real binary path filled in.
func writeManifest(path, bin string) error {
	return os.WriteFile(path, []byte(fmt.Sprintf(manifestTmpl, HostName, bin)), 0o644)
}

// installIcon drops udm.png into the hicolor theme so the tray and any desktop
// entry can refer to it by name rather than by path.
//
// It also writes index.theme and refreshes the icon cache. Without both, a
// machine that has no local hicolor override will not see the icon at all: the
// tray registers, IconName is "udm", and the panel shows nothing.
func installIcon() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	src := ""
	for _, c := range iconCandidates() {
		if isFile(c) {
			src = c
			break
		}
	}
	if src == "" {
		return fmt.Errorf("udm.png not found in %s", strings.Join(iconCandidates(), " or "))
	}

	hicolor := filepath.Join(home, ".local", "share", "icons", "hicolor")
	dst := filepath.Join(hicolor, "256x256", "apps", "udm.png")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}

	// Only create index.theme; never clobber one that already lists more themes.
	idx := filepath.Join(hicolor, "index.theme")
	if !isFile(idx) {
		body := "[Icon Theme]\nName=Hicolor\nComment=Fallback icon theme\n" +
			"Directories=256x256/apps\n\n[256x256/apps]\nSize=256\nContext=Applications\nType=Fixed\n"
		if err := os.WriteFile(idx, []byte(body), 0o644); err != nil {
			return err
		}
	}

	// Best effort: the tool is not present everywhere, and a stale cache is
	// fixed by logging out, so a failure here is not worth reporting.
	exec.Command("gtk-update-icon-cache", "-f", "-t", hicolor).Run()
	return nil
}

func iconCandidates() []string {
	return []string{
		filepath.Join("..", "..", "..", "resources", "udm.png"),
		filepath.Join("..", "..", "resources", "udm.png"),
		filepath.Join("/usr", "share", "udm", "icons", "udm.png"),
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
