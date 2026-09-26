// Package dl is the port that the rest of udm talks to. It knows nothing about
// grab, aria2, or any other engine — swapping the transfer engine means adding
// one package that satisfies Downloader and changing one line in cmd/udm.
package dl

// Spec describes one transfer. Implementations must accept concurrent Starts.
type Spec struct {
	// URL is the thing to fetch. http and https only.
	URL string
	// Path is the full destination file path, extension included. The engine
	// must write to exactly this path so that Dir+Name stay authoritative.
	Path string
}

// Transfer is a download already in flight.
type Transfer interface {
	// Progress reports bytes so far, total bytes (0 if unknown) and the
	// current rate in bytes/sec. It is safe to call at any time, including
	// after Done has fired.
	Progress() (got, size, rate int64)
	// Done fires exactly once, when the transfer has stopped for any reason.
	Done() <-chan struct{}
	// Err is meaningful only after Done. A paused or cancelled transfer reports
	// a context error here; the caller decides whether that is a failure.
	Err() error
	// Pause stops the transfer but keeps the bytes already on disk, so a later
	// Start can resume from them.
	Pause() error
	// Cancel stops the transfer and discards the partial file.
	Cancel() error
}

// Downloader creates transfers. Implementations must be safe for concurrent use.
type Downloader interface {
	Start(Spec) (Transfer, error)
}
