// Package core holds the domain: what a job is, what states it can be in, and
// the service that drives transfers on behalf of a downloader.
//
// Nothing here imports grab, aria2, net/http or Tk. The GUI and the HTTP API
// are adapters hanging off the narrow interfaces in ports.go.
package core

import (
	"time"

	"github.com/raffleberry/udm/internal/dl"
)

// State is where a job sits in its lifecycle.
type State string

const (
	// Queued means the job wants to run but is waiting for a free slot.
	Queued State = "queued"
	// Running means bytes are moving.
	Running State = "running"
	// Paused means stopped on purpose with its partial file kept.
	Paused State = "paused"
	// Done means the file landed intact.
	Done State = "done"
	// Failed means the engine gave up. Err says why.
	Failed State = "failed"
	// Stopped means cancelled on purpose; any partial file is gone.
	Stopped State = "stopped"
)

// Job is a snapshot of one download. Everything the UI and the API show lives
// here, and nothing here is a live handle — see job for that.
type Job struct {
	ID       string    `json:"id"`
	URL      string    `json:"url"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Referrer string    `json:"referrer,omitempty"`
	Added    time.Time `json:"added"`
	Size     int64     `json:"size"`
	Got      int64     `json:"got"`
	Rate     int64     `json:"rate"`
	State    State     `json:"state"`
	Err      string    `json:"err,omitempty"`
}

// Pct is completion as a whole percent, or 0 when the total size is unknown.
func (j Job) Pct() int {
	if j.Size <= 0 {
		return 0
	}
	return int(float64(j.Got) / float64(j.Size) * 100)
}

// Live reports whether the job is doing anything right now.
func (j Job) Live() bool { return j.State == Running || j.State == Queued }

// job is a Job plus the machinery that only the service is allowed to touch.
type job struct {
	Job
	t    dl.Transfer // nil unless State == Running
	kill chan struct{}
}

// snapshot copies the job for a caller outside the service lock. It must not
// touch the rest of job: Rate is sampled in place and zeroing it here would
// both lose data and race with the sampler.
func (j *job) snapshot() Job { return j.Job }
