// Package host is the Firefox native messaging host. Its only job is to start
// the download manager when the extension finds nobody listening, and to hand
// the download over once it is up.
//
// It deliberately does not own the app's protocol: the extension talks HTTP to
// the app, and the host exists only because a browser cannot start a process by
// itself. That is the whole reason this package is small.
package host

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/raffleberry/udm/internal/cfg"
)

// Msg is one frame of the native messaging protocol: 4-byte little-endian
// length followed by JSON, on the host's stdin and stdout.
type Msg struct {
	Action string `json:"action"`
	Data   string `json:"data,omitempty"` // the url, for actDownload
	Name   string `json:"name,omitempty"`
	Ref    string `json:"ref,omitempty"` // the page the click came from
}

// Actions the extension sends.
const (
	actPing     = "ping"
	actDownload = "download"
)

// boot is how long a cold start may take. The browser blocks on this message,
// so it cannot be generous, but starting a window is not instant either.
const boot = 8 * time.Second

// Run serves one native messaging session, reading until the browser hangs up.
func Run() error {
	c, err := cfg.Load()
	if err != nil {
		return err
	}
	base := fmt.Sprintf("http://127.0.0.1:%d", c.Port)

	for {
		var m Msg
		if err := read(&m); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var res any
		switch m.Action {
		case actPing:
			res = map[string]any{"ok": true, "action": "pong", "ts": time.Now().UnixMilli()}

		case actDownload:
			started, herr := hand(base, m)
			if herr != nil {
				// Report the reason so the popup can say something useful.
				res = map[string]any{"ok": false, "error": herr.Error()}
				break
			}
			res = map[string]any{"ok": true, "launched": started}

		default:
			res = map[string]any{"ok": false, "error": "unknown action: " + m.Action}
		}

		if err := write(res); err != nil {
			return err
		}
	}
}

// hand forwards the URL to the app, starting the app first if nobody answers.
// It reports whether it had to start the app.
func hand(base string, m Msg) (bool, error) {
	body := map[string]string{"url": m.Data, "name": m.Name, "referrer": m.Ref}

	if _, err := post(base+"/api/add", body); err == nil {
		return false, nil
	} else if !nobody(err) {
		return false, err // the app is up and refused; do not restart it
	}

	if err := start(); err != nil {
		return false, err
	}
	if err := wait(base, boot); err != nil {
		return true, err
	}
	_, err := post(base+"/api/add", body)
	return true, err
}

// start launches udm. The child's stdio is deliberately left nil: the browser
// is still holding this process's stdout, and a child that inherited it would
// keep the browser waiting for EOF long after udm had done its job.
func start() error {
	bin, err := exe()
	if err != nil {
		return err
	}
	slog.Info("host: starting udm", "bin", bin)
	return exec.Command(bin).Start()
}

// exe finds the udm binary: beside the host, which is where the installer puts
// it, then on PATH, which is what a `go run` or `go install` build uses.
func exe() (string, error) {
	if self, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(self), binName()); isFile(p) {
			return p, nil
		}
	}
	if p, err := exec.LookPath(binName()); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("cannot find %s: not beside the host, not on PATH", binName())
}

func binName() string {
	if runtime.GOOS == "windows" {
		return "udm.exe"
	}
	return "udm"
}

// wait polls /api/ping until the app answers or the deadline passes.
func wait(base string, d time.Duration) error {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if ping(base) == nil {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}
	return fmt.Errorf("nothing answered %s within %s", base, d)
}

func ping(base string) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/ping", nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("ping: %s", res.Status)
	}
	return nil
}

func post(url string, body any) (string, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(res.Body, 1<<16))
	if res.StatusCode >= 400 {
		return "", fmt.Errorf("%s: %s", res.Status, bytes.TrimSpace(raw))
	}
	var out struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &out)
	return out.ID, nil
}

// nobody reports whether an error means "no process is listening on that port",
// which is the one failure worth starting the app for. A timeout must not
// trigger a second copy, so only a plain refused dial counts.
func nobody(err error) bool {
	var op *net.OpError
	return errors.As(err, &op) && op.Op == "dial" && !op.Timeout()
}

func read(v any) error {
	var n uint32
	if err := binary.Read(os.Stdin, binary.LittleEndian, &n); err != nil {
		return err
	}
	if n == 0 || n > 1<<20 {
		return fmt.Errorf("implausible frame length: %d", n)
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(os.Stdin, buf); err != nil {
		return err
	}
	return json.Unmarshal(buf, v)
}

func write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := binary.Write(os.Stdout, binary.LittleEndian, uint32(len(b))); err != nil {
		return err
	}
	_, err = os.Stdout.Write(b)
	return err
}
