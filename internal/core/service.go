package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/raffleberry/udm/internal/dl"
)

var (
	// ErrBadURL is returned by Add for anything that is not a fetchable http(s) URL.
	ErrBadURL = errors.New("not a downloadable url")
	// ErrNoJob is returned when an id does not match a known job.
	ErrNoJob = errors.New("no such job")
)

// Req is a request to download something.
type Req struct {
	URL      string
	Name     string // optional; guessed from the URL when empty
	Referrer string
}

// Options configures a Service.
type Options struct {
	// Dir is the default download destination.
	Dir string
	// MaxConc caps simultaneous transfers. Must be >= 1.
	MaxConc int
}

// Service is the application core and the single owner of job state.
//
// Locking: s.mu guards the job slice, the byID index and rev. The unexported
// start/pause/stop/remove helpers are always called with s.mu already held.
// s.mu is never held across a downloader call or a channel send, so neither
// the GUI nor an HTTP handler can be blocked by a slow disk.
type Service struct {
	dl   dl.Downloader
	opts Options

	mu   sync.Mutex
	jobs []*job
	byID map[string]*job
	rev  chan struct{}

	slot  chan struct{} // capacity MaxConc: the concurrency limiter
	quit  chan struct{}
	close sync.Once
}

// New builds a Service and starts its progress sampler. Call Close when done.
func New(d dl.Downloader, o Options) *Service {
	if o.MaxConc < 1 {
		o.MaxConc = 1
	}
	s := &Service{
		dl:   d,
		opts: o,
		byID: map[string]*job{},
		rev:  make(chan struct{}),
		slot: make(chan struct{}, o.MaxConc),
		quit: make(chan struct{}),
	}
	go s.sample()
	return s
}

// Close stops the sampler. In-flight transfers are left alone; Stop them first
// if that matters.
func (s *Service) Close() { s.close.Do(func() { close(s.quit) }) }

// Add registers a download and starts it. Adding the same URL twice is allowed:
// each call gets its own id and its own destination file.
func (s *Service) Add(r Req) (Job, error) {
	u, err := url.Parse(strings.TrimSpace(r.URL))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return Job{}, fmt.Errorf("%w: %q", ErrBadURL, r.URL)
	}

	name := clean(r.Name)
	if name == "" {
		name = clean(path.Base(u.Path))
	}
	if name == "" {
		name = "download"
	}

	s.mu.Lock()
	j := &job{Job: Job{
		ID:       newID(),
		URL:      u.String(),
		Name:     name,
		Referrer: r.Referrer,
		Added:    time.Now(),
		State:    Queued,
	}}
	j.Path = s.unique(s.opts.Dir, j.Name)
	j.kill = make(chan struct{})
	s.jobs = append(s.jobs, j)
	s.byID[j.ID] = j
	k := j.kill
	snap := j.snapshot()
	s.mu.Unlock()

	s.bump()
	go s.run(j, k)
	return snap, nil
}

// List returns every job, newest last.
func (s *Service) List() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Job, len(s.jobs))
	for i, j := range s.jobs {
		out[i] = j.snapshot()
	}
	return out
}

// Rev returns the current change channel; see Rev in ports.go.
func (s *Service) Rev() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rev
}

// Start runs or resumes the given jobs. Paused and Stopped jobs pick up from
// whatever bytes are still on disk.
func (s *Service) Start(ids ...string) error { return s.each(ids, s.start) }

// Pause stops the given jobs but keeps their partial files.
func (s *Service) Pause(ids ...string) error { return s.each(ids, s.pause) }

// Stop cancels the given jobs and discards their partial files.
func (s *Service) Stop(ids ...string) error { return s.each(ids, s.stop) }

// Remove stops the given jobs and forgets them.
func (s *Service) Remove(ids ...string) error { return s.each(ids, s.remove) }

// each applies fn to every id. It reports the first failure but never stops
// half way: a toolbar click over five rows should act on all five.
func (s *Service) each(ids []string, fn func(*job) error) error {
	var first error
	s.mu.Lock()
	for _, id := range ids {
		j, ok := s.byID[id]
		if !ok {
			if first == nil {
				first = fmt.Errorf("%w: %s", ErrNoJob, id)
			}
			continue
		}
		if err := fn(j); err != nil && first == nil {
			first = err
		}
	}
	s.mu.Unlock()
	s.bump()
	return first
}

// The four helpers below run with s.mu held.

// start is also how a resume works: a fresh kill channel marks the attempt, so
// a stale goroutine from a previous attempt can tell it has been superseded.
func (s *Service) start(j *job) error {
	if j.State == Running || j.State == Queued {
		return nil
	}
	j.Err, j.Rate = "", 0
	j.State = Queued
	j.kill = make(chan struct{})
	go s.run(j, j.kill)
	return nil
}

func (s *Service) pause(j *job) error {
	if j.kill != nil { // queued, not running yet
		close(j.kill)
		j.kill = nil
		j.State = Paused
		return nil
	}
	if j.State == Running {
		j.State = Paused // recorded first; the settler reads it and stays quiet
		return j.t.Pause()
	}
	return nil
}

func (s *Service) stop(j *job) error {
	j.State = Stopped
	if j.kill != nil { // still queued
		close(j.kill)
		j.kill = nil
		return nil
	}
	if j.t != nil {
		return j.t.Cancel()
	}
	return nil
}

func (s *Service) remove(j *job) error {
	if err := s.stop(j); err != nil {
		return err
	}
	for i, cur := range s.jobs {
		if cur == j {
			s.jobs = append(s.jobs[:i], s.jobs[i+1:]...)
			break
		}
	}
	delete(s.byID, j.ID)
	return nil
}

// run waits for a free slot, starts the transfer, then blocks until it settles.
func (s *Service) run(j *job, k chan struct{}) {
	select {
	case s.slot <- struct{}{}:
	case <-k: // paused or stopped while queued
		return
	case <-s.quit:
		return
	}
	defer func() { <-s.slot }()

	t, err := s.dl.Start(dl.Spec{URL: j.URL, Path: j.Path})
	if err != nil {
		s.mu.Lock()
		if j.kill == k {
			j.State, j.Err = Failed, err.Error()
		}
		s.mu.Unlock()
		s.bump()
		return
	}

	s.mu.Lock()
	if j.kill != k { // superseded, or paused/stopped during startup
		s.mu.Unlock()
		_ = t.Cancel()
		return
	}
	j.t, j.State, j.kill = t, Running, nil
	s.mu.Unlock()
	s.bump()

	<-t.Done()

	got, size, _ := t.Progress()

	s.mu.Lock()
	if j.t != t {
		// A newer attempt owns this job now — a resume overtook this one. Its
		// own goroutine settles the state, so this result is only history.
		s.mu.Unlock()
		return
	}
	j.t = nil
	j.Got, j.Size, j.Rate = got, size, 0
	// State is the operator's intent. If they asked for pause or stop, that is
	// already the answer, and whatever grab reports as an error is expected.
	if j.State != Paused && j.State != Stopped {
		if err := t.Err(); err != nil {
			j.State, j.Err = Failed, err.Error()
		} else {
			j.State = Done
		}
	}
	s.mu.Unlock()
	s.bump()
}

// sample refreshes progress for running jobs once a second, keeping the
// download goroutines' hot path free of bookkeeping.
func (s *Service) sample() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-tick.C:
			s.mu.Lock()
			moving := false
			for _, j := range s.jobs {
				if j.t == nil {
					continue
				}
				got, size, rate := j.t.Progress()
				moving = moving || got != j.Got || size != j.Size
				j.Got, j.Size, j.Rate = got, size, rate
			}
			s.mu.Unlock()
			if moving {
				s.bump()
			}
		}
	}
}

// bump closes the current change channel and installs a new one.
func (s *Service) bump() {
	s.mu.Lock()
	close(s.rev)
	s.rev = make(chan struct{})
	s.mu.Unlock()
}

// unique returns a free path for name under dir, appending " (2)", " (3)"...
// on collision, so two identical downloads never fight over one file.
func (s *Service) unique(dir, name string) string {
	p := filepath.Join(dir, filepath.FromSlash(name))
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return p
	}
	ext, stem := filepath.Ext(p), strings.TrimSuffix(p, filepath.Ext(p))
	for n := 2; n < 1000; n++ {
		if q := fmt.Sprintf("%s (%d)%s", stem, n, ext); !exists(q) {
			return q
		}
	}
	return p
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// clean makes a browser-supplied name safe to join onto a directory: no
// separators, no traversal, no control characters.
func clean(s string) string {
	s = strings.ReplaceAll(s, "\\", "/")
	if s = path.Base(strings.TrimSpace(s)); s == "/" || s == "." {
		return ""
	}
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

func newID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}
