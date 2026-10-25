package updater

import (
	"crypto/subtle"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Status is the JSON object returned by GET /v1/status.
type Status struct {
	State    string    `json:"state"`
	Step     string    `json:"step,omitempty"`
	Message  string    `json:"message,omitempty"`
	Version  string    `json:"version,omitempty"`
	Error    string    `json:"error,omitempty"`
	Started  time.Time `json:"started,omitempty"`
	Finished time.Time `json:"finished,omitempty"`
}

type statusStore struct {
	mu   sync.Mutex
	cur  Status
	path string
}

func newStatusStore(dir string) *statusStore {
	s := &statusStore{path: filepath.Join(dir, "status.json")}
	if b, err := os.ReadFile(s.path); err == nil {
		var st Status
		if json.Unmarshal(b, &st) == nil {
			if st.State == "running" {
				st.State = "failed"
				st.Message = "interrupted"
				st.Finished = time.Now()
			}
			s.cur = st
		}
	}
	return s
}

func (s *statusStore) set(st Status) {
	s.mu.Lock()
	s.cur = st
	b, _ := json.Marshal(st)
	os.MkdirAll(filepath.Dir(s.path), 0o750)
	tmp := s.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, s.path)
	}
	s.mu.Unlock()
}

func (s *statusStore) get() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func authOK(token, want string) bool {
	return subtle.ConstantTimeCompare([]byte(token), []byte(want)) == 1
}
