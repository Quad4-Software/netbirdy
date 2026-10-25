package updater

import (
	"context"
	"errors"
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

// RunUpdate executes verify, pull, stop, backup, recreate and health. A
// failure after the first Stop runs the rollback sequence.
func RunUpdate(ctx context.Context, st *statusStore, version string, d Deps) {
	started := time.Now()
	set := func(step string) {
		st.set(Status{State: "running", Step: step, Version: version, Started: started})
	}
	fail := func(step string, err error) {
		st.set(Status{State: "failed", Step: step, Version: version, Error: err.Error(), Started: started, Finished: time.Now()})
	}
	if d.Root == nil {
		d.Root = DefaultRoot
	}
	fetch := d.Fetch
	if fetch == nil {
		fetch = fetchSignedImages
	}

	set("verify")
	images, err := fetch(ctx, d.Cfg, version, d.Root)
	if err != nil {
		fail("verify", err)
		return
	}

	set("pull")
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

	rb := &rollbackState{st: st, eng: d.Eng, dataDir: d.Cfg.DataDir, version: version, started: started}

	set("stop")
	for i := len(d.Cfg.Services) - 1; i >= 0; i-- {
		info, err := d.Eng.FindContainer(ctx, d.Cfg.Project, d.Cfg.Services[i])
		if err != nil {
			rb.run(ctx, "stop", err)
			return
		}
		p := &prevState{info: info}
		rb.prevs = append([]*prevState{p}, rb.prevs...)
		if err := d.Eng.Stop(ctx, info.ID); err != nil {
			rb.run(ctx, "stop", err)
			return
		}
	}

	set("backup")
	bkDir := d.Cfg.BackupDir + "/" + time.Now().UTC().Format("20060102T150405") + "-to-" + version
	m, err := backupDir(d.Cfg.DataDir, bkDir)
	if err == nil {
		err = writeManifest(bkDir, m)
	}
	if err != nil {
		rb.run(ctx, "backup", err)
		return
	}
	rb.bkDir = bkDir
	defer pruneBackups(d.Cfg.BackupDir, d.Cfg.BackupsKeep)

	set("recreate")
	for i := len(rb.prevs) - 1; i >= 0; i-- {
		p := rb.prevs[i]
		if err := d.Eng.Rename(ctx, p.info.ID, p.info.Name+"-prev"); err != nil {
			rb.run(ctx, "recreate", err)
			return
		}
		p.renamed = true
	}
	for i, svc := range d.Cfg.Services {
		p := rb.prevs[i]
		nid, err := d.Eng.CreateLike(ctx, p.info, images[svc], p.info.Name)
		if err != nil {
			rb.run(ctx, "recreate", err)
			return
		}
		p.new = nid
		if err := d.Eng.Start(ctx, nid); err != nil {
			rb.run(ctx, "recreate", err)
			return
		}
	}

	set("health")
	for i, p := range rb.prevs {
		to := d.Cfg.HealthTimeout
		if i > 0 {
			to = time.Minute
		}
		if err := d.Eng.WaitHealthy(ctx, p.new, to); err != nil {
			rb.run(ctx, "health", err)
			return
		}
	}
	for _, p := range rb.prevs {
		_ = d.Eng.Remove(ctx, p.info.ID)
	}
	st.set(Status{State: "succeeded", Step: "done", Version: version, Started: started, Finished: time.Now()})
}

type prevState struct {
	info    ContainerInfo
	new     string
	renamed bool
}

type rollbackState struct {
	st      *statusStore
	eng     Engine
	prevs   []*prevState
	bkDir   string
	dataDir string
	version string
	started time.Time
}

// run removes the new containers, renames the old ones back, restores the data
// directory from the backup when one exists and starts the old containers in
// service order. It uses a context that outlives cancellation so an aborted
// update still finishes its rollback.
func (r *rollbackState) run(ctx context.Context, step string, cause error) {
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Minute)
	defer cancel()
	errs := []error{}
	for _, p := range r.prevs {
		if p.new != "" {
			if err := r.eng.Stop(rctx, p.new); err != nil {
				errs = append(errs, fmt.Errorf("stop new container: %w", err))
			}
			if err := r.eng.Remove(rctx, p.new); err != nil {
				errs = append(errs, fmt.Errorf("remove new container: %w", err))
			}
		}
		if p.renamed {
			if err := r.eng.Rename(rctx, p.info.ID, p.info.Name); err != nil {
				errs = append(errs, fmt.Errorf("rename %s back: %w", p.info.Name, err))
			}
		}
	}
	if r.bkDir != "" {
		if err := restoreDir(r.bkDir, r.dataDir); err != nil {
			errs = append(errs, fmt.Errorf("restore data: %w", err))
		}
	}
	for _, p := range r.prevs {
		if err := r.eng.Start(rctx, p.info.ID); err != nil {
			errs = append(errs, fmt.Errorf("start %s: %w", p.info.Name, err))
		}
	}
	state := "rolled_back"
	msg := cause.Error()
	if len(errs) > 0 {
		state = "failed"
		msg = errors.Join(append([]error{cause}, errs...)...).Error()
	}
	r.st.set(Status{State: state, Step: step, Version: r.version, Error: msg, Started: r.started, Finished: time.Now()})
}

// DefaultRoot is the embedded production root.
func DefaultRoot() ([]reposign.PublicKey, error) { return reposign.EmbeddedRootKeys() }

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
