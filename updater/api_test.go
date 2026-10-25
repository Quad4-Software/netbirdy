package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func postUpdate(url, token, body string) int {
	req, _ := http.NewRequest("POST", url+"/v1/update", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	_ = r.Body.Close()
	return r.StatusCode
}

func TestUpdateRunsWithContextThatOutlivesTheRequest(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	got := make(chan error, 1)
	s := NewServer(cfg, func(ctx context.Context, st *statusStore, v string) {
		time.Sleep(100 * time.Millisecond)
		got <- ctx.Err()
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	if c := postUpdate(srv.URL, cfg.Token, `{"version":"v1.0.0"}`); c != 202 {
		t.Fatalf("status %d", c)
	}
	select {
	case err := <-got:
		if err != nil {
			t.Fatalf("update context was cancelled after the response: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("update did not run")
	}
}

func TestConcurrentUpdatesStartExactlyOne(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	var started atomic.Int32
	block := make(chan struct{})
	s := NewServer(cfg, func(ctx context.Context, st *statusStore, v string) {
		started.Add(1)
		<-block
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	defer close(block)

	var wg sync.WaitGroup
	codes := make(chan int, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- postUpdate(srv.URL, cfg.Token, `{"version":"v1.0.0"}`)
		}()
	}
	wg.Wait()
	close(codes)
	accepted := 0
	for c := range codes {
		switch c {
		case 202:
			accepted++
		case 409:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d updates, want 1", accepted)
	}
	time.Sleep(50 * time.Millisecond)
	if n := started.Load(); n != 1 {
		t.Fatalf("started %d runs, want 1", n)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	var started atomic.Int32
	s := NewServer(cfg, func(context.Context, *statusStore, string) { started.Add(1) })
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	big := `{"version":"v1.0.0","pad":"` + strings.Repeat("a", 8192) + `"}`
	if c := postUpdate(srv.URL, cfg.Token, big); c != 400 {
		t.Fatalf("oversized body status %d", c)
	}
	if started.Load() != 0 {
		t.Fatal("run started for an invalid request")
	}
}
