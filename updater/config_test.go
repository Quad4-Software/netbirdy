package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestConfigShortToken(t *testing.T) {
	os.Setenv("UPDATER_TOKEN", "short")
	defer os.Unsetenv("UPDATER_TOKEN")
	if _, err := Load(); err == nil {
		t.Fatal("short token accepted")
	}
}

func TestAuth(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	s := NewServer(cfg, func(ctx context.Context, st *statusStore, v string) {})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	r, _ := http.Get(srv.URL + "/v1/status")
	if r.StatusCode != 401 {
		t.Fatalf("no token: %d", r.StatusCode)
	}
	r.Body.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/v1/status", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	r2, _ := http.DefaultClient.Do(req)
	if r2.StatusCode != 401 {
		t.Fatalf("wrong token: %d", r2.StatusCode)
	}
	r2.Body.Close()
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	r3, _ := http.DefaultClient.Do(req)
	if r3.StatusCode != 200 {
		t.Fatalf("right token: %d", r3.StatusCode)
	}
	r3.Body.Close()
}

func TestHealthzNoAuth(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	s := NewServer(cfg, nil)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	r, _ := http.Get(srv.URL + "/healthz")
	if r.StatusCode != 200 {
		t.Fatalf("healthz: %d", r.StatusCode)
	}
	r.Body.Close()
}

func TestStatusInterrupted(t *testing.T) {
	dir := t.TempDir()
	s := newStatusStore(dir)
	s.set(Status{State: "running", Started: time.Now()})
	s2 := newStatusStore(dir)
	if s2.get().State != "failed" || s2.get().Message != "interrupted" {
		t.Fatalf("interrupted: %+v", s2.get())
	}
}
