package udm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

type addRequest struct {
	download Download
	respCh   chan<- addResponse
}

type addResponse struct {
	err  error
	done <-chan struct{}
}

func getAriaBin() (string, error) {
	exeName := "aria2c"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}

	// 1. Look in PATH
	if path, err := exec.LookPath(exeName); err == nil {
		return path, nil
	}
	slog.Warn("Aria2c not in path")

	// 2. Look in same directory as executable
	if path, err := os.Executable(); err == nil {
		path = filepath.Join(filepath.Dir(path), exeName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		slog.Warn("Aria2 not found in exec dir", "path", path)
	}

	// 3. Look in third_party directory (for development)
	if IsGoRun() {
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			panic("No caller information available")
		}

		currentDir := filepath.Dir(filename)

		path := filepath.Join(currentDir, "..", "third_party", runtime.GOOS, exeName)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		slog.Warn("Aria2 not found in third_party dir", "path", path)
	}

	return "", ErrAriaNotFound
}

type A2 struct {
	cfg       *Config
	conn      *arigo.Client
	cmd       *exec.Cmd
	startedUs bool
	mu        sync.Mutex

	addCh  chan addRequest
	stopCh chan struct{}
	wg     sync.WaitGroup
}

func NewA2(c *Config) *A2 {
	return &A2{cfg: c}
}

func (m *A2) WSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", m.cfg.Aria2Port)
}

func (m *A2) AddDownload(d Download) (error, <-chan struct{}) {
	m.mu.Lock()
	addCh := m.addCh
	m.mu.Unlock()

	if addCh == nil {
		return fmt.Errorf("aria2 manager not started"), nil
	}

	respCh := make(chan addResponse, 1)
	select {
	case addCh <- addRequest{download: d, respCh: respCh}:
		res := <-respCh
		return res.err, res.done
	case <-time.After(5 * time.Second):
		return fmt.Errorf("aria2 manager not responding"), nil
	}
}

func (m *A2) startManager() {
	if m.addCh != nil {
		return
	}
	m.addCh = make(chan addRequest)
	m.stopCh = make(chan struct{})
	m.wg.Add(1)
	go m.runLoop()
}

func (m *A2) runLoop() {
	defer m.wg.Done()
	active := make(map[string]chan struct{})
	events := make(chan *arigo.DownloadEvent, 64)

	// Subscribe to global events
	m.conn.Subscribe(arigo.CompleteEvent, func(e *arigo.DownloadEvent) {
		select {
		case events <- e:
		default:
		}
	})
	m.conn.Subscribe(arigo.ErrorEvent, func(e *arigo.DownloadEvent) {
		select {
		case events <- e:
		default:
		}
	})
	m.conn.Subscribe(arigo.StopEvent, func(e *arigo.DownloadEvent) {
		select {
		case events <- e:
		default:
		}
	})

	for {
		select {
		case req := <-m.addCh:
			opts := arigo.Options{
				Out:              req.download.FileName,
				MaxDownloadLimit: req.download.MaxDownloadLimit,
				Dir:              req.download.Dir,
			}
			gid, err := m.conn.AddURI([]string{req.download.Uri}, &opts)
			if err != nil {
				req.respCh <- addResponse{err: err}
				continue
			}

			done := make(chan struct{})
			active[gid.GID] = done
			req.respCh <- addResponse{done: done}
			go m.monitor(gid, done)

		case e := <-events:
			if done, ok := active[e.GID]; ok {
				close(done)
				delete(active, e.GID)
			}

		case <-m.stopCh:
			for gid, done := range active {
				close(done)
				delete(active, gid)
			}
			return
		}
	}
}

func (m *A2) monitor(gid arigo.GID, done <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			s, err := gid.TellStatus()
			if err != nil {
				return
			}
			var progress float64
			if s.TotalLength > 0 {
				progress = float64(s.CompletedLength) / float64(s.TotalLength) * 100
			}
			slog.Info("Download progress",
				"gid", s.GID,
				"status", s.Status,
				"speed", s.DownloadSpeed,
				"progress", fmt.Sprintf("%.2f%%", progress))
		}
	}
}

func (m *A2) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.tryConnect(ctx) {
		m.startedUs = false
		m.startManager()
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
			m.startManager()
			return nil
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("aria2c exited during startup")
		}
		select {
		case <-ctx.Done():
			slog.Error("Error connecting to aria2c", "Err", ctx.Err())
			return ctx.Err()
		case <-time.After(1000 * time.Millisecond):
		}
	}
	_ = cmd.Process.Kill()
	return fmt.Errorf("aria2c RPC not ready within %s", m.cfg.ReadyTimeout)
}

func (m *A2) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopCh != nil {
		close(m.stopCh)
		m.wg.Wait()
		m.stopCh = nil
		m.addCh = nil
	}

	if m.conn != nil {
		_ = m.conn.SaveSession()
		_ = m.conn.Close()
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
