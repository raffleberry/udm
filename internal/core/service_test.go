package core

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/raffleberry/udm/internal/dl"
)

// errCanceled stands in for what a real engine reports when a transfer is
// deliberately stopped. The service must never surface it as a failure.
var errCanceled = errors.New("context canceled")

// fake is a dl.Downloader whose transfers the test drives by hand, so the
// service can be exercised with no network and no disk writes.
type fake struct {
	mu    sync.Mutex
	txs   []*fakeTx
	fail  error
	delay time.Duration // how long Start blocks, to force queueing
}

type fakeTx struct {
	mu        sync.Mutex
	got, size int64
	err       error
	paused    bool
	cancelled bool
	closed    bool
	done      chan struct{}
}

func (f *fake) Start(dl.Spec) (dl.Transfer, error) {
	f.mu.Lock()
	delay, fail := f.delay, f.fail
	f.mu.Unlock()

	if delay > 0 {
		time.Sleep(delay)
	}
	if fail != nil {
		return nil, fail
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	t := &fakeTx{size: 1000, done: make(chan struct{})}
	f.txs = append(f.txs, t)
	return t, nil
}

func (t *fakeTx) Progress() (got, size, rate int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.got, t.size, 0
}

func (t *fakeTx) Done() <-chan struct{} { return t.done }

func (t *fakeTx) Err() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}

func (t *fakeTx) Pause() error {
	t.mu.Lock()
	t.paused = true
	t.mu.Unlock()
	return t.settle(errCanceled)
}

func (t *fakeTx) Cancel() error {
	t.mu.Lock()
	t.cancelled = true
	t.mu.Unlock()
	return t.settle(errCanceled)
}

// settle ends the transfer the way a real engine does when it is stopped.
func (t *fakeTx) settle(err error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.err = err
	if !t.closed {
		t.closed = true
		close(t.done)
	}
	return nil
}

func (t *fakeTx) finish(err error) { _ = t.settle(err) }

func (t *fakeTx) wasPaused() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.paused
}

func (f *fake) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.txs)
}

func (f *fake) at(i int) *fakeTx {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.txs[i]
}

func newSvc(t *testing.T, f *fake, conc int) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	return New(f, Options{Dir: dir, MaxConc: conc}), dir
}

// waitFor polls rather than sleeping a fixed amount, so the tests are neither
// flaky nor slow.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func stateOf(s *Service, id string) State {
	for _, j := range s.List() {
		if j.ID == id {
			return j.State
		}
	}
	return ""
}

func TestAddRejectsJunk(t *testing.T) {
	s, _ := newSvc(t, &fake{}, 1)
	defer s.Close()
	for _, bad := range []string{"", "not a url", "ftp://x/y", "file:///etc/passwd", "://"} {
		if _, err := s.Add(Req{URL: bad}); !errors.Is(err, ErrBadURL) {
			t.Errorf("Add(%q) = %v, want ErrBadURL", bad, err)
		}
	}
}

func TestAddStartsAndCompletes(t *testing.T) {
	f := &fake{}
	s, _ := newSvc(t, f, 2)
	defer s.Close()

	j, err := s.Add(Req{URL: "https://example.com/a.zip", Name: "a.zip"})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "running", func() bool { return stateOf(s, j.ID) == Running })

	f.at(0).finish(nil)
	waitFor(t, "done", func() bool { return stateOf(s, j.ID) == Done })
}

func TestAddNamesCollisionsApart(t *testing.T) {
	s, dir := newSvc(t, &fake{}, 1)
	defer s.Close()

	a, _ := s.Add(Req{URL: "https://e.com/1", Name: "same.zip"})
	// Materialise the file so the second Add has to work around it.
	if err := os.WriteFile(filepath.Join(dir, "same.zip"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Add(Req{URL: "https://e.com/2", Name: "same.zip"})

	if a.Path == b.Path {
		t.Fatalf("both jobs got %s", a.Path)
	}
	if got := filepath.Base(b.Path); got != "same (2).zip" {
		t.Errorf("second name = %q, want %q", got, "same (2).zip")
	}
}

func TestCleanStripsTraversal(t *testing.T) {
	for in, want := range map[string]string{
		"../../etc/passwd":      "passwd",
		`..\..\windows\sys.ini`: "sys.ini",
		"/absolute/name.bin":    "name.bin",
		"ok.txt":                "ok.txt",
		"":                      "",
		"/":                     "",
	} {
		if got := clean(in); got != want {
			t.Errorf("clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPauseIsNotAFailure(t *testing.T) {
	f := &fake{}
	s, _ := newSvc(t, f, 2)
	defer s.Close()

	j, _ := s.Add(Req{URL: "https://e.com/x"})
	waitFor(t, "running", func() bool { return stateOf(s, j.ID) == Running })
	tx := f.at(0)

	if err := s.Pause(j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "paused", func() bool { return stateOf(s, j.ID) == Paused })
	if !tx.wasPaused() {
		t.Error("Pause never reached the transfer")
	}

	// A real engine reports a cancel error here. The service must treat that as
	// the pause we asked for, not as a failed job.
	time.Sleep(30 * time.Millisecond)
	if got := stateOf(s, j.ID); got != Paused {
		t.Errorf("state after pause = %s, want paused", got)
	}
	if s.List()[0].Err != "" {
		t.Errorf("pause recorded an error: %q", s.List()[0].Err)
	}
}

func TestResumeStartsAFreshAttempt(t *testing.T) {
	f := &fake{}
	s, _ := newSvc(t, f, 2)
	defer s.Close()

	j, _ := s.Add(Req{URL: "https://e.com/x"})
	waitFor(t, "running", func() bool { return stateOf(s, j.ID) == Running })
	if err := s.Pause(j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "paused", func() bool { return stateOf(s, j.ID) == Paused })

	if err := s.Start(j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "a second transfer", func() bool { return f.count() == 2 })
	waitFor(t, "running again", func() bool { return stateOf(s, j.ID) == Running })

	if err := s.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stopped", func() bool { return stateOf(s, j.ID) == Stopped })
}

func TestStopBeforeRunningNeverTransfers(t *testing.T) {
	f := &fake{}
	f.mu.Lock()
	f.delay = 80 * time.Millisecond
	f.mu.Unlock()

	s, _ := newSvc(t, f, 1)
	defer s.Close()

	j, _ := s.Add(Req{URL: "https://e.com/x"})
	if err := s.Stop(j.ID); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "stopped", func() bool { return stateOf(s, j.ID) == Stopped })

	// The queued goroutine wakes, sees it was superseded, and cancels whatever
	// the engine handed back.
	time.Sleep(200 * time.Millisecond)
	if got := stateOf(s, j.ID); got != Stopped {
		t.Errorf("state = %s, want it to stay stopped", got)
	}
}

func TestFailureIsReported(t *testing.T) {
	f := &fake{fail: errors.New("no route to host")}
	s, _ := newSvc(t, f, 1)
	defer s.Close()

	j, _ := s.Add(Req{URL: "https://e.com/x"})
	waitFor(t, "failed", func() bool { return stateOf(s, j.ID) == Failed })
	if msg := s.List()[0].Err; !strings.Contains(msg, "no route") {
		t.Errorf("Err = %q, want it to mention the cause", msg)
	}
}

func TestRemoveForgets(t *testing.T) {
	s, _ := newSvc(t, &fake{}, 1)
	defer s.Close()

	j, _ := s.Add(Req{URL: "https://e.com/x"})
	if err := s.Remove(j.ID); err != nil {
		t.Fatal(err)
	}
	if n := len(s.List()); n != 0 {
		t.Errorf("List has %d jobs after Remove, want 0", n)
	}
	if err := s.Start(j.ID); !errors.Is(err, ErrNoJob) {
		t.Errorf("Start on a removed job = %v, want ErrNoJob", err)
	}
}

func TestRevFiresAndRefreshes(t *testing.T) {
	s, _ := newSvc(t, &fake{}, 1)
	defer s.Close()

	// Rev must hand out a fresh channel each call: a redraw loop that started
	// after a change would otherwise wait for the next one forever.
	first := s.Rev()
	if _, err := s.Add(Req{URL: "https://e.com/x"}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("Rev did not fire after Add")
	}
	if s.Rev() == first {
		t.Error("Rev returned the same channel twice")
	}
}

func TestConcurrencyIsCapped(t *testing.T) {
	f := &fake{}
	f.mu.Lock()
	f.delay = 60 * time.Millisecond
	f.mu.Unlock()

	s, _ := newSvc(t, f, 2)
	defer s.Close()

	for i := 0; i < 6; i++ {
		if _, err := s.Add(Req{URL: "https://e.com/" + string(rune('a'+i))}); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, "the cap to be reached", func() bool { return f.count() == 2 })
	// A leaked third slot would show up in this window.
	time.Sleep(200 * time.Millisecond)
	if got := f.count(); got > 2 {
		t.Errorf("%d transfers running, cap is 2", got)
	}
}
