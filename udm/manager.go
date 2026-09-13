package udm

import (
	"context"
	"uuid"
)

type Job struct {
	Uuid uuid.UUID

	// FileName
	Out string
	// Download Directory
	Dir string
	// Download Url
	Uri string
	// default in B, can add K or M
	MaxDownloadLimit string
}

func NewJobFromUri(uri string) Job {
	return Job{
		Uuid:             uuid.New(),
		Out:              "",
		Dir:              "",
		Uri:              uri,
		MaxDownloadLimit: "",
	}
}

// User(Start) -> Waiting[inqueue for a download] -> Active[downloading]
//
// User(Pause) -> Paused -> User(Unpause) -> Waiting -> Active[downloading]
//
// - System(Error) -> Error(End)
//
// - Complete[Download Completed]
var DStatusTyp = struct {
	Complete string
	Active   string
	Start    string
	Error    string
	Paused   string
	// overridden by Error or Complete
	Stop string
	// in A2 = waiting
	Queued string
}{
	"complete",
	"active",
	"start",
	"error",
	"paused",
	"stop",
	"queued",
}

type Dstatus struct {
	Type string
	//bytes/sec
	Rate uint
	Gid  string

	SizeTotal  uint
	SizeLoaded uint
	BitField   string

	Err  error
	Done bool
}

type Manager interface {
	Start(ctx context.Context) error
	AddDownload(j Job) (string, error)
	AddDownloadFromUri(uri string) (string, error)
	JobStart(jobId string) error
	JobPause(jobId string) error
	JobStop(jobId string) error
	Sub(gid string) <-chan Dstatus
	Shutdown(ctx context.Context) error
}
