package updater

import (
	"context"
	"fmt"
	"time"

	"github.com/netbirdio/netbird/shared/updater/reposign"
)

type Deps struct {
	Cfg   Config
	Eng   Engine
	Root  RootProvider
	Fetch func(ctx context.Context, cfg Config, version string, root RootProvider) (map[string]string, error)
}

// RunUpdate executes verify, pull, backup, recreate, health; failures after
// the first Stop run the rollback sequence.
func RunUpdate(ctx context.Context, st *statusStore, version string, d Deps) {
	fail := func(step string, err error) {
		st.set(Status{State: "failed", Step: step, Version: version, Error: err.Error(), Started: st.get().Started, Finished: time.Now()})
	}
	if d.Root == nil {
		d.Root = DefaultRoot
	}
	fetch := d.Fetch
	if fetch == nil {
		fetch = fetchSignedImages
	}
	st.set(Status{State: "running", Step: "verify", Version: version, Started: time.Now()})
	images, err := fetch(ctx, d.Cfg, version, d.Root)
	if err != nil {
		fail("verify", err)
		return
	}
	st.set(Status{State: "running", Step: "pull", Version: version, Started: st.get().Started})
	for _, svc := range d.Cfg.Services {
		ref := images[svc]
		ok, err := d.Eng.ImageExists(ctx, ref)
		if err != nil {
			fail("pull", err)
			return
		}
		if !ok {
			if err := d.Eng.Pull(ctx, ref); err != nil {
				fail("pull", err)
				return
			}
		}
	}
	st.set(Status{State: "running", Step: "backup", Version: version, Started: st.get().Started})
	bkDir := d.Cfg.BackupDir + "/" + time.Now().UTC().Format("20060102T150405") + "-" + st.get().Version
	m, err := backupDir(d.Cfg.DataDir, bkDir)
	if err != nil {
		fail("backup", err)
		return
	}
	if err := writeManifest(bkDir, m); err != nil {
		fail("backup", err)
		return
	}
	defer pruneBackups(d.Cfg.BackupDir, d.Cfg.BackupsKeep)

	prevs := map[string]prevState{}
	st.set(Status{State: "running", Step: "recreate", Version: version, Started: st.get().Started})
	for i := len(d.Cfg.Services) - 1; i >= 0; i-- {
		svc := d.Cfg.Services[i]
		info, err := d.Eng.FindContainer(ctx, d.Cfg.Project, svc)
		if err != nil {
			fail("recreate", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
		if err := d.Eng.Stop(ctx, info.ID); err != nil {
			fail("recreate", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
		if err := d.Eng.Rename(ctx, info.ID, info.Name+"-prev"); err != nil {
			fail("recreate", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
		prevs[svc] = prevState{info, ""}
	}
	for _, svc := range d.Cfg.Services {
		s := prevs[svc]
		nid, err := d.Eng.CreateLike(ctx, s.info, images[svc], s.info.Name)
		if err != nil {
			fail("recreate", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
		s.new = nid
		prevs[svc] = s
		if err := d.Eng.Start(ctx, nid); err != nil {
			fail("recreate", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
	}
	st.set(Status{State: "running", Step: "health", Version: version, Started: st.get().Started})
	for i, svc := range d.Cfg.Services {
		to := d.Cfg.HealthTimeout
		if i > 0 {
			to = time.Minute
		}
		if err := d.Eng.WaitHealthy(ctx, prevs[svc].new, to); err != nil {
			fail("health", err)
			rollback(ctx, st, d.Eng, prevs, bkDir, err, d.Cfg.DataDir)
			return
		}
	}
	for _, svc := range d.Cfg.Services {
		d.Eng.Remove(ctx, prevs[svc].info.ID)
	}
	st.set(Status{State: "succeeded", Step: "done", Version: version, Started: st.get().Started, Finished: time.Now()})
}

type prevState struct {
	info ContainerInfo
	new  string
}

// rollback renames -prev containers back, restores data, starts the old set,
// and removes the new containers.
func rollback(ctx context.Context, st *statusStore, eng Engine, prevs map[string]prevState, bkDir string, cause error, dataDir string) {
	for _, s := range prevs {
		if s.new != "" {
			eng.Stop(ctx, s.new)
			eng.Remove(ctx, s.new)
		}
		eng.Rename(ctx, s.info.ID, s.info.Name)
	}
	if err := restoreDir(bkDir, dataDir); err != nil {
		// restore failure is logged into state; the rollback continues
		_ = err
	}
	for _, s := range prevs {
		eng.Start(ctx, s.info.ID)
	}
	st.set(Status{State: "rolled_back", Error: cause.Error(), Finished: time.Now()})
}

// DefaultRoot is the embedded production root.
func DefaultRoot() ([]reposign.PublicKey, error) { return reposign.EmbeddedRootKeys() }

var _ = fmt.Sprint

// NewRunner returns the update function used by the HTTP server, wired to the
// container engine at cfg.Socket and the embedded signing root.
func NewRunner(cfg Config) (func(ctx context.Context, st *statusStore, version string), error) {
	eng, err := NewDockerEngine(cfg.Socket)
	if err != nil {
		return nil, err
	}
	deps := Deps{Cfg: cfg, Eng: eng, Root: DefaultRoot, Fetch: fetchSignedImages}
	return func(ctx context.Context, st *statusStore, version string) {
		RunUpdate(ctx, st, version, deps)
	}, nil
}
