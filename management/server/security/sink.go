package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// MultiSink fans events out to several sinks.
type MultiSink []Sink

// Emit forwards to every sink; a panicking sink cannot break the chain.
func (m MultiSink) Emit(ev Event) {
	for _, s := range m {
		func(s Sink) {
			defer func() { _ = recover() }()
			s.Emit(ev)
		}(s)
	}
}

// FileSink appends events as JSON lines to a file with size-based rotation
// (3 backups) and 0600 permissions.
type FileSink struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
}

// NewFileSink creates a sink writing one JSON object per line. maxSize is
// the rotation threshold in bytes (0 = 10 MiB default), backups keeps N
// rotated files (0 = 3 default).
func NewFileSink(path string, maxSize int64, backups int) (*FileSink, error) {
	if path == "" {
		return nil, fmt.Errorf("security log path empty")
	}
	if maxSize <= 0 {
		maxSize = 10 << 20
	}
	if backups <= 0 {
		backups = 3
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return &FileSink{path: path, maxSize: maxSize, backups: backups}, nil
}

// Emit appends one JSON line.
func (f *FileSink) Emit(ev Event) { f.EmitJSON(ev) }

// EmitJSON appends one JSON line for an arbitrary payload.
func (f *FileSink) EmitJSON(v any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	bs, err := json.Marshal(v)
	if err != nil {
		return
	}
	f.rotateIfNeeded(int64(len(bs)))
	fp, err := os.OpenFile(f.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer fp.Close()
	bs = append(bs, '\n')
	_, _ = fp.Write(bs)
}

func (f *FileSink) rotateIfNeeded(next int64) {
	st, err := os.Stat(f.path)
	if err != nil || st.Size()+next <= f.maxSize {
		return
	}
	// shift backups: .3 <- .2 <- .1 <- current
	oldest := fmt.Sprintf("%s.%d", f.path, f.backups)
	os.Remove(oldest)
	for i := f.backups - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", f.path, i), fmt.Sprintf("%s.%d", f.path, i+1))
	}
	os.Rename(f.path, f.path+".1")
}
