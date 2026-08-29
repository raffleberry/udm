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
	Start    int
	Stop     int
	Pause    int
}{
	0, 1, 2, 3, 4, 5,
}

type Dstatus struct {
	Type int
	//bytes/sec
	Rate uint
	Gid  string

	SizeTotal  uint
	SizeLoaded uint
	BitField   string

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

	unSubStart arigo.UnsubscribeFunc
	unSubStop  arigo.UnsubscribeFunc
	unSubPause arigo.UnsubscribeFunc
	unSubCompl arigo.UnsubscribeFunc
	unSubErr   arigo.UnsubscribeFunc

	subMp      map[string][]chan Dstatus
	stopPollCh chan struct{}
}

func NewA2(c *Config) *A2 {
	a2 := &A2{cfg: c}
	a2.subMp = make(map[string][]chan Dstatus)
	a2.stopPollCh = make(chan struct{})
	return a2
}

func (m *A2) WSURL() string {
	return fmt.Sprintf("ws://127.0.0.1:%d/jsonrpc", m.cfg.Aria2Port)
}

func (m *A2) pub(gid string, ch chan Dstatus, s Dstatus) {
	select {
	case ch <- s:
		slog.Debug("A2 pub: Success", "gid", gid, "type", s.Type)
		return
	case lost := <-ch:
		slog.Warn("A2 pub: Lost", "gid", gid, "lost", lost)
	default:
		slog.Error("A2 pub: CATASTROPHIC", "gid", gid, "s", s, "len(ch)(possibly stale)", len(ch))
	}
}

// starts sending `Dstatus` info for download with 'gid'
func (m *A2) Sub(gid string) <-chan Dstatus {
	m.muPoll.Lock()
	defer m.muPoll.Unlock()
	_, ok := m.subMp[gid]
	if !ok {
		m.subMp[gid] = []chan Dstatus{}
	}
	ch := make(chan Dstatus, 8)
	m.subMp[gid] = append(m.subMp[gid], ch)
	return ch
}

// stops sending `Dstatus` info for download with 'gid'
func (m *A2) UnSub(gid string, ch chan Dstatus) {
	m.muPoll.Lock()
	defer m.muPoll.Unlock()
	for i, chMp := range m.subMp[gid] {
		if chMp == ch {
			close(m.subMp[gid][i])
			m.subMp[gid][i] = nil
			m.subMp[gid] = append(m.subMp[gid][:i], m.subMp[gid][i+1:]...)
			return
		}
	}
}

func (m *A2) poll() {
	slog.Info("Started Polling")
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stopPollCh:
			slog.Debug("poll: Stop received")
			return
		case <-ticker.C:
			m.muPoll.Lock()
			for k, v := range m.subMp {
				for _, ch := range v {
					s, err := m.conn.TellStatus(k)
					if err != nil {
						return
					}

					d := Dstatus{
						Type:       DStatusMsg.Progress,
						Gid:        k,
						Rate:       s.DownloadSpeed,
						SizeTotal:  s.TotalLength,
						SizeLoaded: s.CompletedLength,
						BitField:   s.BitField,
					}
					if s.ErrorMessage != "" {
						d.Err = fmt.Errorf("%s", s.ErrorMessage)
					}
					m.pub(k, ch, d)
				}
			}
			m.muPoll.Unlock()
		}
	}
}

func (m *A2) AddDownload(d Download) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.conn == nil {
		return "", ErrAriaNotStarted
	}

	opts := arigo.Options{
		Out:              d.Out,
		Dir:              d.Dir,
		MaxDownloadLimit: d.MaxDownloadLimit,
	}
	gid, err := m.conn.AddURI([]string{d.Uri}, &opts)

	if err != nil {
		return "", err
	}

	return gid.GID, nil
}

func (m *A2) applyGlobalSettings() {
	if m.conn == nil {
		slog.Error("Failed to set global A2 settings, no connection. [conn == nil]")
		return
	}
	// m.conn.ChangeGlobalOptions() TODO:
}

func (m *A2) startManager() {
	go m.monitor()
	go m.poll()
	m.applyGlobalSettings()
}

func (m *A2) monitor() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.monitorStop()

	m.unSubStart = m.conn.Subscribe(arigo.StartEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Start", "event - ", e)
		chList, ok := m.subMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		for _, ch := range chList {
			ch <- Dstatus{
				Type: DStatusMsg.Start,
			}
		}
	})

	m.unSubStop = m.conn.Subscribe(arigo.StopEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Stop", "event - ", e)
		chList, ok := m.subMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		for _, ch := range chList {
			ch <- Dstatus{
				Type: DStatusMsg.Stop,
			}
		}
	})

	m.unSubPause = m.conn.Subscribe(arigo.PauseEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Pause", "event - ", e)
		chList, ok := m.subMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		for _, ch := range chList {
			ch <- Dstatus{
				Type: DStatusMsg.Pause,
			}
		}

	})

	m.unSubCompl = m.conn.Subscribe(arigo.CompleteEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Complete", "event - ", e)

		chList, ok := m.subMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		for i, ch := range chList {
			ch <- Dstatus{
				Done: true,
				Type: DStatusMsg.Complete,
			}
			close(ch)
			chList[i] = nil
		}
		delete(m.subMp, e.GID)
	})

	m.unSubErr = m.conn.Subscribe(arigo.ErrorEvent, func(e *arigo.DownloadEvent) {
		slog.Info("Events:Error", "event - ", e)
		chList, ok := m.subMp[e.GID]
		if !ok {
			slog.Error("UNKNOWN Download, NOT TRACKING!", "gid", e.GID)
			return
		}
		for _, ch := range chList {
			ch <- Dstatus{
				Type: DStatusMsg.Error,
			}
		}
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
		// "--continue=true",
		"--save-session=" + session,
		"--save-session-interval=30",
		"--log=" + filepath.Join(m.cfg.CfgDir, "aria2.log"),
		"--console-log-level=warn",
	}

	if IsGoRun() {
		args = append(args, "--log-level=info")
	} else {
		args = append(args, "--log-level=notice")
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
	m.stopPollCh <- struct{}{}

	for k, v := range m.subMp {
		for i, ch := range v {
			close(ch)
			v[i] = nil
		}
		m.subMp[k] = nil
	}

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

var A2Error = []struct {
	ErrCode int
	ErrMsg  string
}{{0, "all downloads were successful."},
	{1, "an unknown error occurred."},
	{2, "time out occurred."},
	{3, "a resource was not found."},
	{4,
		"aria2 saw the specified number of resource not found error. See --max-file-not-found option."},
	{5, "a download aborted because download speed was too slow. See --lowest-speed-limit option."},
	{6, "network problem occurred."},
	{7,
		"there were unfinished downloads. This error is only reported if all finished downloads were successful and there were ,unfinished downloads in a queue when aria2 exited by pressing Ctrl-C by an user or sending TERM or INT signal."},
	{8, "remote server did not support resume when resume was required to complete download."},
	{9, "there was not enough disk space available."},
	{10,
		"piece length was different from one in .aria2 control file. See --allow-piece-length-change option."},
	{11, "aria2 was downloading same file at that moment."},
	{12, "aria2 was downloading same info hash torrent at that moment."},
	{13, "file already existed. See --allow-overwrite option."},
	{14, "renaming file failed. See --auto-file-renaming option."},
	{15, "aria2 could not open existing file."},
	{16, "aria2 could not create new file or truncate existing file."},
	{17, "file I/O error occurred."},
	{18, "aria2 could not create directory."},
	{19, "name resolution failed."},
	{20, "aria2 could not parse Metalink document."},
	{21, "FTP command failed."},
	{22, "HTTP response header was bad or unexpected."},
	{23, "too many redirects occurred."},
	{24, "HTTP authorization failed."},
	{25, "aria2 could not parse bencoded file (usually '.torrent' file)."},
	{26, ".torrent file was corrupted or missing information that aria2 needed."},
	{27, "Magnet URI was bad."},
	{28, "bad/unrecognized option was given or unexpected option argument was given."},
	{29,
		"the server was unable to handle the request due to a temporary overloading or maintenance."},
	{30, "aria2 could not parse JSON-RPC request."},
	{31, "Reserved. Not used."},
	{32, "checksum validation failed. "},
}
