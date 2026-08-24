package udm

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/siku2/arigo"
)

var (
	ErrAriaNotFound = fmt.Errorf("aria2c not found. Please Install it and try again")
	ErrUnexpected   = fmt.Errorf("Unexpected")
)

type Download struct {
	FileName string
	Dir      string
	Uri      string
	// default in B, can add K or M
	MaxDownloadLimit string
}

func getAriaBin() (string, error) {

	exeName := "aria2c"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}

	path, err := exec.LookPath(exeName)
	if err != nil {
		slog.Warn("Aria2", "Msg", "Aria2c not in path")
	} else {
		return path, nil
	}
	path, err = os.Executable()
	if err != nil {
		return "", err
	}
	path, err = filepath.EvalSymlinks(path)
	if err == nil {
		path = filepath.Join(filepath.Dir(path), exeName)
		if _, err := os.Stat(path); err != nil {
			slog.Warn("Aria2 not found in exec dir", "path", path)
		}
	} else {
		slog.Warn("Aria2 failed to find exec path", "path", path, "err", err)
	}

	if IsGoRun() {
		filepath.Join("third_party", runtime.GOOS, exeName)
		if _, err := os.Stat(path); err != nil {
			slog.Warn("Aria2 not found in third_party dir", "path", path)
		}
	}

	return "", ErrAriaNotFound

}

type A2 struct {
	cfg       *Config
	conn      *arigo.Client
	cmd       *exec.Cmd
	startedUs bool
	gids      []arigo.GID
	mu        sync.Mutex
}

func NewA2(c *Config) *A2 {
	return &A2{cfg: c}
}

func (m *A2) WSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", m.cfg.Aria2Port)
}

func (m *A2) AddDownload(d Download) (error, <-chan struct{}) {
	done := make(chan struct{}, 1)
	opts := arigo.Options{
		Out:              d.FileName,
		MaxDownloadLimit: d.MaxDownloadLimit,
		Dir:              d.Dir,
	}
	gid, err := m.conn.AddURI([]string{d.Uri}, &opts)
	if err != nil {
		slog.Error("A2: Error while adding uri", "err", err)
		return err, nil
	}
	m.gids = append(m.gids, gid)

	gid.Subscribe(arigo.CompleteEvent, func(e *arigo.DownloadEvent) {
		log.Println("Download Complete", e.GID)
		m.gids = slices.DeleteFunc(m.gids, func(g arigo.GID) bool {
			return g.GID == e.GID
		})
	})

	gid.Subscribe(arigo.ErrorEvent, func(e *arigo.DownloadEvent) {
		log.Println("something went wrong while downloading", e.GID)
	})

	gid.Subscribe(arigo.StopEvent, func(e *arigo.DownloadEvent) {
		log.Println("download stopped", e.GID)
	})

	go func(d arigo.GID) {
		s, err := d.TellStatus()
		if err != nil {
			log.Println("Failed to get status")
		}
		log.Println(s.Status, s.DownloadSpeed, s.CompletedLength, s.TotalLength)
		if !slices.Contains(m.gids, d) {
			return
		}
		time.Sleep(time.Second * 1)
	}(gid)

	return nil, done
}

func (m *A2) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tryConnect(ctx) {
		m.startedUs = false
		return nil
	}

	bin, err := getAriaBin()
	if err != nil {
		return err
	}

	session := filepath.Join(m.cfg.CfgDir, "aria2.session")
	args := []string{
		"--enable-rpc=true",
		"--rpc-listen-all=false",
		"--rpc-listen-port=" + strconv.Itoa(m.cfg.Aria2Port),
		"--rpc-secret=" + m.cfg.Secret,
		"--dir=" + m.cfg.DownloadDir,
		"--continue=true",
		"--save-session=" + session,
		"--save-session-interval=30",
		"--log=" + filepath.Join(m.cfg.CfgDir, "aria2.log"),
		"--log-level=notice",
		"--console-log-level=warn",
	}
	if st, err := os.Stat(session); err == nil && st.Size() > 0 {
		args = append(args, "--input-file="+session)
	}
	if m.cfg.StopWithApp {
		args = append(args, "--stop-with-process="+strconv.Itoa(os.Getpid()))
	}
	args = append(args, m.cfg.ExtraArgs...)

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = m.cfg.CfgDir
	hideConsole(cmd) // no-op on unix; CREATE_NO_WINDOW on windows TODO: Test if this works

	slog.Info("Starting Aria2")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start aria2c: %w", err)
	}
	m.cmd = cmd
	m.startedUs = true
	slog.Info("Started Aria2")

	go func() { _ = cmd.Wait() }()
	deadline := time.Now().Add(m.cfg.ReadyTimeout)
	for time.Now().Before(deadline) {
		if m.tryConnect(ctx) {
			return nil
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("aria2c exited during startup")
		}
		select {
		case <-ctx.Done():
			slog.Error("Error connecting to aria2c", "Err", ctx.Err())
			return ctx.Err()
		case <-time.After(3000 * time.Millisecond):
		}
	}
	_ = cmd.Process.Kill()
	return fmt.Errorf("aria2c RPC not ready within %s", m.cfg.ReadyTimeout)
}

func (m *A2) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	err := m.conn.SaveSession()
	if err != nil {
		slog.Error("Failed to save session", "Err", err)
	}

	err = m.conn.Close()
	if err != nil {
		slog.Error("Failed to close aria2 rpc connection", "Err", err)
	}

	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		_ = m.cmd.Process.Kill()
		<-done
		return ctx.Err()
	case <-time.After(m.cfg.ShutdownTimeout):
		_ = m.cmd.Process.Kill()
		<-done
		return nil
	}
}

func (m *A2) tryConnect(ctx context.Context) bool {
	var err error
	m.conn, err = arigo.DialContext(ctx, m.WSURL(), m.cfg.Secret)
	if err != nil {
		return false
	}
	return true
}
