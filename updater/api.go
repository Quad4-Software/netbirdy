package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// Server is the sidecar HTTP API.
type Server struct {
	cfg    Config
	store  *statusStore
	run    func(ctx context.Context, st *statusStore, version string) // engine entry, injectable
	server *http.Server
	mu     sync.Mutex
}

func NewServer(cfg Config, run func(ctx context.Context, st *statusStore, version string)) *Server {
	return &Server{cfg: cfg, store: newStatusStore(cfg.BackupDir), run: run}
}

func (s *Server) auth(r *http.Request) bool {
	h := r.Header.Get("Authorization")
	if len(h) < 8 || h[:7] != "Bearer " {
		return false
	}
	return authOK(h[7:], s.cfg.Token)
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(r) {
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(s.store.get())
	})
	mux.HandleFunc("POST /v1/update", func(w http.ResponseWriter, r *http.Request) {
		if !s.auth(r) {
			http.Error(w, "unauthorized", 401)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		var req struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal(body, &req); err != nil || !semverRe.MatchString(req.Version) {
			http.Error(w, "bad version", 400)
			return
		}
		s.mu.Lock()
		if s.store.get().State == "running" {
			s.mu.Unlock()
			http.Error(w, "update in progress", 409)
			return
		}
		s.store.set(Status{State: "running", Step: "queued", Version: req.Version, Started: time.Now()})
		s.mu.Unlock()
		go s.run(context.Background(), s.store, req.Version)
		w.WriteHeader(202)
	})
	return mux
}

func (s *Server) Listen() *http.Server {
	s.server = &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	return s.server
}
