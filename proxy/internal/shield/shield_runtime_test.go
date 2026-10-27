package shield

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
)

func scanReq(path string) *http.Request {
	r := httptest.NewRequest("GET", "http://svc.example"+path, nil)
	r.Header.Set("User-Agent", "sqlmap/1.7")
	r.RemoteAddr = "203.0.113.9:1234"
	return r
}

func runtimeShield(t *testing.T, cfg Config, opts ...Option) *Shield {
	t.Helper()
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	base := []Option{WithIPResolver(func(*http.Request) netip.Addr { return testIP })}
	sh, err := New(cfg, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sh.Close)
	return sh
}

func serve(sh *Shield) (*httptest.ResponseRecorder, bool) {
	rec := httptest.NewRecorder()
	called := false
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })).ServeHTTP(rec, scanReq("/"))
	return rec, called
}

func TestSetModeTogglesShield(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 5
	cfg.BanAt = 10
	cfg.WeightScannerUA = 80
	sh := runtimeShield(t, cfg, WithKnownHost(func(string) bool { return true }))

	rec, _ := serve(sh)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected enforce block, got %d", rec.Code)
	}
	if err := sh.SetMode("off"); err != nil {
		t.Fatal(err)
	}
	if sh.Mode() != string(ModeOff) {
		t.Fatalf("mode %q", sh.Mode())
	}
	rec, called := serve(sh)
	if !called {
		t.Fatal("off mode still blocks")
	}
	if err := sh.SetMode("enforce"); err != nil {
		t.Fatal(err)
	}
	if err := sh.SetMode("bogus"); err == nil {
		t.Fatal("bogus mode accepted")
	}
}

func TestReloadAppliesConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shield.yaml")
	if err := os.WriteFile(path, []byte("mode: observe\n"), 0600); err != nil {
		t.Fatal(err)
	}
	sh := runtimeShield(t, DefaultConfig(), WithConfigPath(path))
	if err := sh.Reload(); err != nil {
		t.Fatal(err)
	}
	if sh.Mode() != string(ModeObserve) {
		t.Fatalf("mode after reload %q", sh.Mode())
	}
	if err := os.WriteFile(path, []byte("mode: bogus\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := sh.Reload(); err == nil {
		t.Fatal("invalid file reloaded")
	}
	if sh.Mode() != string(ModeObserve) {
		t.Fatal("failed reload clobbered running config")
	}
}

func TestReloadWithoutConfigPath(t *testing.T) {
	sh := runtimeShield(t, DefaultConfig())
	if err := sh.Reload(); err == nil {
		t.Fatal("reload without a config path should fail")
	}
}

func TestAllowLocalDefault(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 1
	cfg.BanAt = 1
	cfg.WeightScannerUA = 80
	sh := runtimeShield(t, cfg,
		WithIPResolver(func(*http.Request) netip.Addr { return netip.MustParseAddr("10.1.2.3") }),
		WithKnownHost(func(string) bool { return true }))
	rec, called := serve(sh)
	if !called || rec.Code != 200 {
		t.Fatalf("private source blocked despite allow_local default: %d", rec.Code)
	}
}

func TestAllowLocalDisabled(t *testing.T) {
	off := false
	cfg := DefaultConfig()
	cfg.AllowLocal = &off
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 1
	cfg.BanAt = 1
	cfg.WeightScannerUA = 80
	sh := runtimeShield(t, cfg,
		WithIPResolver(func(*http.Request) netip.Addr { return netip.MustParseAddr("10.1.2.3") }),
		WithKnownHost(func(string) bool { return true }))
	rec, _ := serve(sh)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected block with allow_local=false, got %d", rec.Code)
	}
}

func TestApplyConfigSwapsRunningConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 5
	cfg.BanAt = 10
	cfg.WeightScannerUA = 80
	sh := runtimeShield(t, cfg, WithKnownHost(func(string) bool { return true }))

	next := cfg
	next.Mode = ModeObserve
	next.AllowCIDRs = []string{"203.0.113.0/24"}
	if err := sh.ApplyConfig(next); err != nil {
		t.Fatal(err)
	}
	if sh.Mode() != string(ModeObserve) {
		t.Fatalf("mode %q", sh.Mode())
	}
	// observe mode scores but never alters responses, and the pushed cidr
	// range bypasses the shield entirely.
	rec, called := serve(sh)
	if !called {
		t.Fatal("allow_cidrs source still inspected")
	}
	if rec.Code != 200 {
		t.Fatalf("observe mode blocked: %d", rec.Code)
	}
	active := sh.Config()
	if !active.allowIP(netip.MustParseAddr("203.0.113.9")) {
		t.Fatal("pushed cidr not installed")
	}

	bad := cfg
	bad.Mode = "bogus"
	if err := sh.ApplyConfig(bad); err == nil {
		t.Fatal("invalid config applied")
	}
	if sh.Mode() != string(ModeObserve) {
		t.Fatal("rejected config clobbered running config")
	}
}
