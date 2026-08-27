package udm

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
	"time"

	"github.com/siku2/arigo"
)

var (
	ErrAriaNotFound     = fmt.Errorf("aria2c not found. Please Install it and try again")
	ErrUnexpected       = fmt.Errorf("Unexpected")
	ErrAriaNotStarted   = fmt.Errorf("aria2c not Started :)")
	ErrAriaNotConnected = fmt.Errorf("aria2c not Connected????? o_O")
)

type Download struct {
	// FileName
	Out string
	// Download Directory
	Dir string
	// Download Url
	Uri string
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

var DStatusMsg = struct {
	Progress int
	Complete int
	Error    int
	Stop     int
}{
	0, 1, 2, 3,
}

type Dstatus struct {
	Type int
	Rate int
	Gid  string
	Err  error
	Done bool
}

type A2 struct {
	cfg       *Config
	conn      *arigo.Client
	cmd       *exec.Cmd
	startedUs bool
	mu        sync.Mutex
	muPoll    sync.Mutex

	unSubCompl arigo.UnsubscribeFunc
	unSubStop  arigo.UnsubscribeFunc
	unSubErr   arigo.UnsubscribeFunc
	unSubPause arigo.UnsubscribeFunc
	unSubStart arigo.UnsubscribeFunc

	wantPoll   []string
	dlChMp     map[string]chan Dstatus
	stopPollCh chan struct{}
}

func NewA2(c *Config) *A2 {
	a2 := &A2{cfg: c}
	a2.dlChMp = make(map[string]chan Dstatus)
	return a2
}

func (m *A2) WSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", m.cfg.Aria2Port)
}

// starts sending `Dstatus` info for download with 'gid'
func (m *A2) AddPollFor(gid string) {
	m.muPoll.Lock()
	defer m.muPoll.Unlock()
	m.wantPoll = append(m.wantPoll, gid)
}

// stops sending `Dstatus` info for download with 'gid'
func (m *A2) RemPollFor(gid string) {
	m.muPoll.Lock()
	defer m.muPoll.Unlock()
	for i := range m.wantPoll {
		if m.wantPoll[i] == gid {
			m.wantPoll[i] = m.wantPoll[len(m.wantPoll)-1]
			m.wantPoll = m.wantPoll[:len(m.wantPoll)-1]
		}
	}
}

// TODO: Implement a fan out broadcast like the example u saw.
func (m *A2) poll() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopPollCh:
			return
		case <-ticker.C:
			m.muPoll.Lock()
			for _, gid := range m.wantPoll {
				s, err := m.conn.TellStatus(gid)
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
			m.muPoll.Unlock()
		}
	}
}

func (m *A2) AddDownload(d Download) (string, error, <-chan Dstatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return "", ErrAriaNotStarted, nil
	}

	opts := arigo.Options{
		Out:              d.Out,
		Dir:              d.Dir,
		MaxDownloadLimit: d.MaxDownloadLimit,
	}
	gid, err := m.conn.AddURI([]string{d.Uri}, &opts)

	if err != nil {
		return "", err, nil
	}

	respCh := make(chan Dstatus)

	m.dlChMp[gid.GID] = respCh

	return gid.GID, nil, respCh
}

func (m *A2) startManager() {
	go m.monitor()
	go m.poll()
}

func (m *A2) monitor() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.monitorStop()

	// Check if the m.dlChMp needs closing after one of these events.

	m.unSubStart = m.conn.Subscribe(arigo.StartEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Start", "event - ", e)
		// ch, ok := m.dlChMp[e.GID]
		// if !ok {
		// 	slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
		// 	return
		// }
		// ch <- Dstatus{
		// 	Type: DStatusMsg.Stop,
		// }
	})

	m.unSubStop = m.conn.Subscribe(arigo.StopEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Stop", "event - ", e)
		// ch, ok := m.dlChMp[e.GID]
		// if !ok {
		// 	slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
		// 	return
		// }
		// ch <- Dstatus{
		// 	Type: DStatusMsg.Stop,
		// }
	})

	m.unSubPause = m.conn.Subscribe(arigo.PauseEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Pause", "event - ", e)
		// ch, ok := m.dlChMp[e.GID]
		// if !ok {
		// 	slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
		// 	return
		// }
		// ch <- Dstatus{
		// 	Type: DStatusMsg.Stop,
		// }
	})

	m.unSubCompl = m.conn.Subscribe(arigo.CompleteEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Complete", "event - ", e)

		ch, ok := m.dlChMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		ch <- Dstatus{
			Done: true,
			Type: DStatusMsg.Complete,
		}

		delete(m.dlChMp, e.GID)
		close(ch)
	})

	m.unSubErr = m.conn.Subscribe(arigo.ErrorEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Error", "event - ", e)
		// ch, ok := m.dlChMp[e.GID]
		// if !ok {
		// 	slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
		// 	return
		// }
		// ch <- Dstatus{
		// 	Type: DStatusMsg.Error,
		// }
	})

}

func (m *A2) monitorStop() {
	if m.unSubStart != nil {
		m.unSubStop()
	}
	if m.unSubStop != nil {
		m.unSubStop()
	}
	if m.unSubPause != nil {
		m.unSubStop()
	}
	if m.unSubCompl != nil {
		m.unSubCompl()
	}
	if m.unSubErr != nil {
		m.unSubErr()
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
		// "--log-level=notice",
		"--log-level=info",

		"--console-log-level=warn",
	}
	if st, err := os.Stat(session); err == nil && st.Size() > 0 {
		args = append(args, "--input-file="+session)
	}
	if m.cfg.StopWithApp {
		args = append(args, "--stop-with-process="+strconv.Itoa(os.Getpid()))
	}
	args = append(args, m.cfg.ExtraArgs...)

	cmd := exec.Command(bin, args...)
	cmd.Dir = m.cfg.CfgDir
	hideConsole(cmd) // no-op on unix; CREATE_NO_WINDOW on windows TODO: Test if this works

	slog.Info("Starting Aria2")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start aria2c: %w", err)
	}
	m.cmd = cmd
	m.startedUs = true
	slog.Info("Started Aria2 cmd")

	go func() {
		err := cmd.Wait()
		debug.PrintStack()
		slog.Info("Stopped Aria2 cmd", "err", err)
	}()
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

	slog.Info("Stopping Poll")

	if m.conn != nil {
		err := m.conn.SaveSession()
		if err != nil {
			slog.Error("A2 Shutdown: conn.SaveSession", "err", err)
		}
		err = m.conn.Shutdown()
		if err != nil {
			slog.Error("A2 Shutdown: conn.Shutdown", "err", err)
		}
		err = m.conn.Close()
		if err != nil {
			slog.Error("A2 Shutdown: conn.Close", "err", err)
		}
	} else {
		slog.Error("A2 Shutdown: conn", "err", ErrAriaNotStarted)
	}

	if m.cmd == nil || m.cmd.Process == nil {
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- m.cmd.Wait() }()

	select {
	case <-done:
		slog.Info("A2 Shutdown: Success")
		return nil
	case <-ctx.Done():
		slog.Info("A2 Shutdown: Deadline exceed while cmd.Wait")
		_ = m.cmd.Process.Kill()
		<-done
		return ctx.Err()
	case <-time.After(m.cfg.ShutdownTimeout):
		slog.Warn("Aria2 cmd shutdown timedout.killing..")
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
