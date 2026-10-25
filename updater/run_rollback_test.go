package updater

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func testImages() map[string]string {
	return map[string]string{
		"server":    "p-server@sha256:" + strings.Repeat("a", 64),
		"dashboard": "p-dash@sha256:" + strings.Repeat("b", 64),
	}
}

func mustHave(t *testing.T, calls []string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(calls, w) {
			t.Fatalf("missing call %q in %v", w, calls)
		}
	}
}

func mustNotHave(t *testing.T, calls []string, prefixes ...string) {
	t.Helper()
	for _, c := range calls {
		for _, p := range prefixes {
			if strings.HasPrefix(c, p) {
				t.Fatalf("unexpected call %q in %v", c, calls)
			}
		}
	}
}

func TestStartFailureRollsBack(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.failOn["start:new-id-server"] = errors.New("boom")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	got := st.get()
	if got.State != "rolled_back" || !strings.Contains(got.Error, "boom") {
		t.Fatalf("status %+v", got)
	}
	mustHave(t, eng.calls, "stop:new-id-server", "remove:new-id-server", "rename:p-server", "rename:p-dashboard", "start:id-server", "start:id-dashboard")
	mustNotHave(t, eng.calls, "create:p-dashboard", "healthy:")
}

func TestUnhealthyRestoresDataAndOrder(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.healthBad = true
	eng.onHealthy = func() { _ = os.WriteFile(filepath.Join(cfg.DataDir, "state.db"), []byte("migrated"), 0o600) }
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	if got := st.get(); got.State != "rolled_back" {
		t.Fatalf("status %+v", got)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "state.db"))
	if string(b) != "v1" {
		t.Fatalf("data not restored: %q", b)
	}
	si := slices.Index(eng.calls, "start:id-server")
	di := slices.Index(eng.calls, "start:id-dashboard")
	if si < 0 || di < 0 || si > di {
		t.Fatalf("old containers must restart in service order: %v", eng.calls)
	}
}

func TestRollbackFailureReportsBothErrors(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.failOn["start:new-id-server"] = errors.New("new start failed")
	eng.failOn["start:id-server"] = errors.New("old start failed")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	got := st.get()
	if got.State != "failed" || !strings.Contains(got.Error, "new start failed") || !strings.Contains(got.Error, "old start failed") {
		t.Fatalf("status %+v", got)
	}
	mustHave(t, eng.calls, "start:id-dashboard")
}

func TestRenameBackFailureReported(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.failOn["start:new-id-server"] = errors.New("cause")
	eng.failOn["rename:p-server"] = errors.New("rename back failed")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	got := st.get()
	if got.State != "failed" || !strings.Contains(got.Error, "rename back failed") {
		t.Fatalf("status %+v", got)
	}
}

func TestContextCancelStillRollsBack(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.strictCtx = true
	ctx, cancel := context.WithCancel(context.Background())
	eng.healthBad = true
	eng.onHealthy = cancel
	RunUpdate(ctx, st, "v2.0.0", deps(cfg, eng, testImages()))
	if got := st.get(); got.State != "rolled_back" {
		t.Fatalf("status %+v calls %v", got, eng.calls)
	}
	mustHave(t, eng.calls, "rename:p-server", "start:id-server", "start:id-dashboard")
}

func TestStopFailureRestartsOldContainers(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.failOn["stop"] = errors.New("cannot stop")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	if got := st.get(); got.State != "rolled_back" {
		t.Fatalf("status %+v", got)
	}
	mustHave(t, eng.calls, "start:id-dashboard")
	mustNotHave(t, eng.calls, "create:", "rename:p-dashboard-prev")
	b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "state.db"))
	if string(b) != "v1" {
		t.Fatalf("data changed: %q", b)
	}
}

func TestBackupFailureRestartsOldAndCreatesNothing(t *testing.T) {
	cfg, eng, st := setup(t)
	cfg.DataDir = filepath.Join(t.TempDir(), "missing")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	got := st.get()
	if got.State != "rolled_back" || got.Step != "backup" {
		t.Fatalf("status %+v", got)
	}
	mustHave(t, eng.calls, "start:id-server", "start:id-dashboard")
	mustNotHave(t, eng.calls, "create:", "rename:")
}

func TestUnconfiguredServicesUntouched(t *testing.T) {
	cfg, eng, st := setup(t)
	cfg.Services = []string{"server"}
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	if got := st.get(); got.State != "succeeded" {
		t.Fatalf("status %+v", got)
	}
	mustNotHave(t, eng.calls, "find:dashboard", "stop:id-dashboard", "create:p-dashboard")
}

func TestPullSkippedWhenImagePresentAndPulledWhenMissing(t *testing.T) {
	cfg, eng, st := setup(t)
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	mustNotHave(t, eng.calls, "pull")

	cfg, eng, st = setup(t)
	eng.failOn["exists_skip_pull"] = errors.New("absent")
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	if n := strings.Count(strings.Join(eng.calls, ","), "pull"); n != 2 {
		t.Fatalf("pulls %d in %v", n, eng.calls)
	}
}

func TestPruneKeepsNewestAndStatusFileMode(t *testing.T) {
	dir := t.TempDir()
	names := []string{"20260101T000000-to-v1.0.0", "20260102T000000-to-v1.0.1", "20260103T000000-to-v1.0.2", "20260104T000000-to-v1.0.3", "20260105T000000-to-v1.0.4"}
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	pruneBackups(dir, 3)
	ents, _ := os.ReadDir(dir)
	var left []string
	for _, e := range ents {
		left = append(left, e.Name())
	}
	if !slices.Equal(left, names[2:]) {
		t.Fatalf("left %v", left)
	}

	cfg, eng, st := setup(t)
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, testImages()))
	fi, err := os.Stat(filepath.Join(cfg.BackupDir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("status.json mode %v", fi.Mode().Perm())
	}
}
