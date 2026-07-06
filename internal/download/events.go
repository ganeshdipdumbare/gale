package download

import "time"

// State represents download lifecycle status.
type State string

const (
	StateQueued      State = "queued"
	StateDownloading State = "downloading"
	StateVerifying   State = "verifying"
	StateInstalling  State = "installing"
	StateDone        State = "done"
	StateFailed      State = "failed"
	StatePaused      State = "paused"
)

// Event is emitted on the progress channel during downloads.
type Event struct {
	ID            string
	State         State
	BytesDone     int64
	TotalBytes    int64
	Speed         float64 // bytes/sec
	ETA           time.Duration
	ActiveWorkers int
	ChunkIndex    int
	Error         error
}

// Progress holds aggregate download progress for a single file.
type Progress struct {
	ID            string
	BytesDone     int64
	TotalBytes    int64
	Speed         float64
	ETA           time.Duration
	ActiveWorkers int
	State         State
}
