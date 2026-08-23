package udm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/siku2/arigo"
)

var (
	ErrAriaNotFound = fmt.Errorf("aria2c not found. Please Install it and try again")
)

func getAriaBin() (string, error) {

	path, err := exec.LookPath("aria2c")
	if err != nil {
		slog.Warn("Aria2", "Msg", "Aria2c not in path")
	} else {
		return path, nil
	}
	path, err = os.Executable()
	// TODO: Get aria2 from pkg or somewhere.

	return path, nil

}

type Manager struct {
	cfg       *Config
	conn      *arigo.Client
	cmd       *exec.Cmd
	startedUs bool
	mu        sync.Mutex
}

func NewManager(c *Config) *Manager {
	return &Manager{cfg: c}
}

func (m *Manager) WSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", m.cfg.Aria2Port)
}

func (m *Manager) Start(ctx context.Context) error {
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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start aria2c: %w", err)
	}
	m.cmd = cmd
	m.startedUs = true

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
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	_ = cmd.Process.Kill()
	return fmt.Errorf("aria2c RPC not ready within %s", m.cfg.ReadyTimeout)
}

func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Always try graceful RPC first (even if we attached).
	err := m.conn.SaveSession() // saveSession is automatic on aria2 exit
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

func (m *Manager) tryConnect(ctx context.Context) bool {
	var err error
	m.conn, err = arigo.DialContext(ctx, m.WSURL(), m.cfg.Secret)
	if err != nil {
		return false
	}
	return true
}
