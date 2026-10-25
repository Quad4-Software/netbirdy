package updater

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"

	"github.com/netbirdio/netbird/shared/updater/reposign"
)

type recEngine struct {
	calls     []string
	failOn    map[string]error // method -> error
	healthBad bool
}

func (r *recEngine) rec(s string) { r.calls = append(r.calls, s) }
func (r *recEngine) ImageExists(ctx context.Context, ref string) (bool, error) {
	r.rec("exists")
	if e := r.failOn["exists"]; e != nil {
		return false, e
	}
	return r.failOn["exists_skip_pull"] == nil, nil // true by default: no pull
}
func (r *recEngine) Pull(ctx context.Context, ref string) error {
	r.rec("pull")
	return r.failOn["pull"]
}
func (r *recEngine) FindContainer(ctx context.Context, p, s string) (ContainerInfo, error) {
	r.rec("find:" + s)
	if e := r.failOn["find:"+s]; e != nil {
		return ContainerInfo{}, e
	}
	return ContainerInfo{ID: "id-" + s, Name: p + "-" + s, Config: map[string]any{}, Host: map[string]any{}, Net: map[string]any{}}, nil
}
func (r *recEngine) Stop(ctx context.Context, id string) error {
	r.rec("stop:" + id)
	return r.failOn["stop"]
}
func (r *recEngine) Rename(ctx context.Context, id, n string) error {
	r.rec("rename:" + n)
	return r.failOn["rename"]
}
func (r *recEngine) CreateLike(ctx context.Context, o ContainerInfo, img, n string) (string, error) {
	r.rec("create:" + n)
	if e := r.failOn["create:"+n]; e != nil {
		return "", e
	}
	return "new-" + o.ID, nil
}
func (r *recEngine) Start(ctx context.Context, id string) error {
	r.rec("start:" + id)
	return r.failOn["start:"+id]
}
func (r *recEngine) WaitHealthy(ctx context.Context, id string, d time.Duration) error {
	r.rec("healthy:" + id)
	if r.healthBad {
		return errors.New("unhealthy")
	}
	return nil
}
func (r *recEngine) Remove(ctx context.Context, id string) error { r.rec("remove:" + id); return nil }

func deps(cfg Config, eng *recEngine, imgs map[string]string) Deps {
	return Deps{
		Cfg:  cfg,
		Eng:  eng,
		Root: func() ([]reposign.PublicKey, error) { return nil, nil },
		Fetch: func(ctx context.Context, c Config, v string, r RootProvider) (map[string]string, error) {
			return imgs, nil
		},
	}
}

func setup(t *testing.T) (Config, *recEngine, *statusStore) {
	data := t.TempDir()
	bk := t.TempDir()
	os.WriteFile(filepath.Join(data, "state.db"), []byte("v1"), 0o600)
	cfg := Config{DataDir: data, BackupDir: bk, Services: []string{"server", "dashboard"}, Project: "p", HealthTimeout: time.Second, BackupsKeep: 3}
	return cfg, &recEngine{failOn: map[string]error{}}, newStatusStore(bk)
}

func TestHappyPathOrder(t *testing.T) {
	cfg, eng, st := setup(t)
	imgs := map[string]string{"server": "p-server@sha256:" + strings.Repeat("a", 64), "dashboard": "p-dash@sha256:" + strings.Repeat("b", 64)}
	RunUpdate(context.Background(), st, "v2.0.0", deps(cfg, eng, imgs))
	got := eng.calls
	// reverse stop order, forward create/start
	wantOrder := []string{"exists", "exists", "find:dashboard", "stop:id-dashboard", "rename:p-dashboard-prev",
		"find:server", "stop:id-server", "rename:p-server-prev",
		"create:p-server", "start:new-id-server", "create:p-dashboard", "start:new-id-dashboard",
		"healthy:new-id-server", "healthy:new-id-dashboard", "remove:id-server", "remove:id-dashboard"}
	if fmt.Sprint(got) != fmt.Sprint(wantOrder) {
		t.Fatalf("order:\n got %v\nwant %v", got, wantOrder)
	}
	if st.get().State != "succeeded" {
		t.Fatalf("state %s", st.get().State)
	}
	// backup manifest exists
	ents, _ := os.ReadDir(cfg.BackupDir)
	var found bool
	for _, e := range ents {
		if e.IsDir() && e.Name() != "." {
			found = true
			if err := verifyManifest(filepath.Join(cfg.BackupDir, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("no backup created")
	}
}

func TestPullFailureAbortsBeforeStop(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.failOn["pull"] = errors.New("pull failed")
	eng.failOn["exists_skip_pull"] = errors.New("force pull")
	imgs := map[string]string{"server": "s@sha256:" + strings.Repeat("a", 64), "dashboard": "d@sha256:" + strings.Repeat("b", 64)}
	RunUpdate(context.Background(), st, "v2", deps(cfg, eng, imgs))
	for _, c := range eng.calls {
		if strings.HasPrefix(c, "stop:") {
			t.Fatalf("stop called after pull fail: %v", eng.calls)
		}
	}
	if st.get().State != "failed" || st.get().Step != "pull" {
		t.Fatalf("%+v", st.get())
	}
}

func TestUnhealthyTriggersRollback(t *testing.T) {
	cfg, eng, st := setup(t)
	eng.healthBad = true
	// simulate the new version corrupting data: restore must bring it back
	imgs := map[string]string{"server": "s@sha256:" + strings.Repeat("a", 64), "dashboard": "d@sha256:" + strings.Repeat("b", 64)}
	RunUpdate(context.Background(), st, "v2", deps(cfg, eng, imgs))
	if st.get().State != "rolled_back" {
		t.Fatalf("state %s calls %v", st.get().State, eng.calls)
	}
	// old containers started again, new removed
	var sawRemoveNew, sawStartOld bool
	for _, c := range eng.calls {
		if c == "remove:new-id-server" {
			sawRemoveNew = true
		}
		if c == "start:id-server" {
			sawStartOld = true
		}
	}
	if !sawRemoveNew || !sawStartOld {
		t.Fatalf("rollback incomplete: %v", eng.calls)
	}
	b, _ := os.ReadFile(filepath.Join(cfg.DataDir, "state.db"))
	if string(b) != "v1" {
		t.Fatalf("data not restored: %q", b)
	}
}

func TestVerifyFailures(t *testing.T) {
	for _, name := range []string{"bad-sig", "unchained", "tampered"} {
		cfg, eng, st := setup(t)
		RunUpdate(context.Background(), st, "v2", Deps{Cfg: cfg, Eng: eng,
			Root: func() ([]reposign.PublicKey, error) { return nil, nil },
			Fetch: func(ctx context.Context, c Config, v string, r RootProvider) (map[string]string, error) {
				return nil, fmt.Errorf("verify: %s", name)
			}})
		if len(eng.calls) != 0 || st.get().State != "failed" {
			t.Fatalf("%s: calls %v state %s", name, eng.calls, st.get().State)
		}
	}
}

func TestSecondUpdateWhileRunning(t *testing.T) {
	cfg := Config{Token: "x123456789012345678901234", BackupDir: t.TempDir()}
	block := make(chan struct{})
	s := NewServer(cfg, func(ctx context.Context, st *statusStore, v string) {
		st.set(Status{State: "running", Version: v})
		<-block
	})
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	defer close(block)
	post := func() int {
		req, _ := http.NewRequest("POST", srv.URL+"/v1/update", strings.NewReader(`{"version":"v1.0.0"}`))
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
		r, _ := http.DefaultClient.Do(req)
		r.Body.Close()
		return r.StatusCode
	}
	if c := post(); c != 202 {
		t.Fatalf("first: %d", c)
	}
	time.Sleep(50 * time.Millisecond)
	if c := post(); c != 409 {
		t.Fatalf("second while running: %d", c)
	}
}

func TestSemverAndParseImages(t *testing.T) {
	for _, ok := range []struct {
		s    string
		want bool
	}{
		{"v1.2.3", true}, {"1.2.3", true}, {"v1.2.3-rc.1", true},
		{"v1.2", false}, {"", false}, {"v1.2.3.4", false},
	} {
		if got := semverRe.MatchString(ok.s); got != ok.want {
			t.Errorf("semver %q: %v", ok.s, got)
		}
	}
	for _, in := range []string{"\n\n", "x\r\n", "pre\x00fix", "@sha256:" + strings.Repeat("a", 64)} {
		if _, err := ParseImages([]byte(in), "p-", []string{"s"}); err == nil {
			t.Errorf("ParseImages accepted %q", in)
		}
	}
}
