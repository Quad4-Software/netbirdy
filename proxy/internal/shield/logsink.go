package shield

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DecisionEvent is the stable jsonl schema for shield decisions.
type DecisionEvent struct {
	Timestamp time.Time `json:"ts"`
	Type      string    `json:"type"` // always "shield"
	Severity  string    `json:"severity"`
	SrcIP     string    `json:"src_ip"`
	Host      string    `json:"host"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	UserAgent string    `json:"user_agent,omitempty"`
	Action    string    `json:"action"`
	Score     float64   `json:"score"`
	Reasons   []string  `json:"reasons,omitempty"`
	BanUntil  string    `json:"ban_until,omitempty"`
	Mode      string    `json:"mode"`
	RequestID string    `json:"request_id,omitempty"`
	ServiceID string    `json:"service_id,omitempty"`
}

// DecisionLog writes non-pass decisions as jsonl with size rotation.
type DecisionLog struct {
	mu      sync.Mutex
	path    string
	maxSize int64
	backups int
	mode    string
}

// NewDecisionLog opens the log at path (10 MiB x 3 backups default).
func NewDecisionLog(path string, maxSize int64, backups int, mode string) (*DecisionLog, error) {
	if path == "" {
		return nil, fmt.Errorf("shield log path empty")
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
	return &DecisionLog{path: path, maxSize: maxSize, backups: backups, mode: mode}, nil
}

// WithDecisionLog wires a decision jsonl sink into the shield.
func WithDecisionLog(l *DecisionLog) Option {
	return func(s *Shield) { s.decisionLog = l }
}

func trunc(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func (l *DecisionLog) write(r *http.Request, res Result, ip netip.Addr) {
	if l == nil || res.Action == ActionPass {
		return
	}
	path := r.URL.EscapedPath()
	ev := DecisionEvent{
		Timestamp: time.Now().UTC(),
		Type:      "shield",
		Severity:  severityFor(res.Action),
		SrcIP:     ip.String(),
		Host:      r.Host,
		Method:    r.Method,
		Path:      trunc(path, 256),
		Action:    string(res.Action),
		Score:     res.Score,
		Reasons:   res.Reasons,
		Mode:      l.mode,
		RequestID: r.Header.Get("X-Request-Id"),
	}
	if ua := r.UserAgent(); ua != "" {
		ev.UserAgent = trunc(ua, 200)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bs, err := json.Marshal(ev)
	if err != nil {
		return
	}
	l.rotateIfNeeded(int64(len(bs)))
	fp, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer fp.Close()
	bs = append(bs, '\n')
	_, _ = fp.Write(bs)
}

func severityFor(a Action) string {
	switch a {
	case ActionDrop, ActionBlock:
		return "high"
	case ActionTarpit, ActionChallenge:
		return "medium"
	}
	return "low"
}

func (l *DecisionLog) rotateIfNeeded(next int64) {
	st, err := os.Stat(l.path)
	if err != nil || st.Size()+next <= l.maxSize {
		return
	}
	oldest := fmt.Sprintf("%s.%d", l.path, l.backups)
	os.Remove(oldest)
	for i := l.backups - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
	}
	os.Rename(l.path, l.path+".1")
}
