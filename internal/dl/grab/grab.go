// Package grab adapts github.com/cavaliergopher/grab to the dl.Downloader port.
//
// grab has no pause of its own. Pausing is therefore "cancel, keep the bytes":
// grab resumes by re-issuing a Range request against the partial file it left
// behind, which is exactly what dl.Transfer.Pause promises.
package grab

import (
	"path/filepath"

	g "github.com/cavaliergopher/grab/v3"

	"github.com/raffleberry/udm/internal/dl"
)

// Downloader fetches with grab. The zero value is not usable; call New.
type Downloader struct {
	c *g.Client
}

// New returns a Downloader backed by one shared grab client, so grabs share a
// connection pool and the process's file-descriptor budget.
func New() *Downloader { return &Downloader{c: g.NewClient()} }

// Start begins fetching s.URL into s.Path. It returns as soon as the request is
// under way; the transfer is watched through Transfer.Done.
func (d *Downloader) Start(s dl.Spec) (dl.Transfer, error) {
	req, err := g.NewRequest(filepath.Dir(s.Path), s.URL)
	if err != nil {
		return nil, err
	}
	// grab would otherwise pick the name from Content-Disposition or the URL.
	// The core decided the name, so pin it.
	req.Filename = s.Path
	return &transfer{resp: d.c.Do(req)}, nil
}

type transfer struct{ resp *g.Response }

func (t *transfer) Progress() (got, size, rate int64) {
	return t.resp.BytesComplete(), t.resp.Size(), int64(t.resp.BytesPerSecond())
}

func (t *transfer) Done() <-chan struct{} { return t.resp.Done }

func (t *transfer) Err() error { return t.resp.Err() }

// Pause cancels the request but leaves the partial file for grab to resume.
func (t *transfer) Pause() error { return t.resp.Cancel() }

// Cancel stops the transfer. grab renames or removes its own partial file on
// error, so there is nothing to clean up here.
func (t *transfer) Cancel() error { return t.resp.Cancel() }
