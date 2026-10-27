package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/util"
)

const (
	// runTimeout bounds one backup run.
	runTimeout = 30 * time.Minute
)

var (
	// ErrNotConfigured is returned when a run is requested but the feature
	// has no bucket configured.
	ErrNotConfigured = errors.New("backups are not configured: set NB_BACKUP_S3_BUCKET or update the backup config")
	// ErrNoEncryptionKey is returned when a run is requested without an
	// encryption passphrase. Plaintext snapshots are never uploaded.
	ErrNoEncryptionKey = errors.New("backup encryption key is not set: set NB_BACKUP_ENCRYPTION_KEY to enable encrypted backups")
	// ErrSnapshotUnsupported is returned when the store engine cannot be
	// snapshotted.
	ErrSnapshotUnsupported = errors.New("store engine does not support snapshots")
	// ErrRunInProgress is returned when a run is requested while another is
	// still executing.
	ErrRunInProgress = errors.New("a backup run is already in progress")
)

// RunResult describes one completed backup run.
type RunResult struct {
	// Key is the object key the snapshot was stored under.
	Key string
	// Size is the stored object size in bytes.
	Size int64
	// Duration is how long the run took.
	Duration time.Duration
}

// Status is a point-in-time snapshot of the manager for the API.
type Status struct {
	// Enabled reports whether a bucket is configured.
	Enabled bool
	// Running reports whether a backup run is in progress.
	Running bool
	// LastRun is when the last run finished, zero when never.
	LastRun time.Time
	// LastError is the last run's error, empty when it succeeded or never ran.
	LastError string
	// LastSnapshot is the object key of the last successful upload.
	LastSnapshot string
	// NextRun is the next scheduled run, zero when scheduling is off.
	NextRun time.Time
	// Config is the effective config with secrets masked.
	Config Config
}

// Manager schedules and runs encrypted backups of the management store to an
// S3-compatible bucket.
type Manager struct {
	dataDir     string
	snapshotter Snapshotter
	newStore    func(Config) (ObjectStore, error)
	now         func() time.Time

	mu      sync.Mutex
	cfg     Config
	started bool
	running bool
	cancel  context.CancelFunc
	wake    chan struct{}
	lastRun time.Time
	lastErr string
	lastKey string
	nextRun time.Time
}

// NewManager creates a Manager for the store reachable through snapshotter.
// The effective config is the persisted backup.json in dataDir when present,
// otherwise the NB_BACKUP_* environment. A corrupt persisted config is logged
// and ignored rather than fatal.
func NewManager(dataDir string, snapshotter Snapshotter) *Manager {
	m := &Manager{
		dataDir:     dataDir,
		snapshotter: snapshotter,
		newStore:    NewObjectStore,
		now:         time.Now,
		cfg:         ConfigFromEnv(),
		wake:        make(chan struct{}, 1),
	}
	if persisted, found, err := m.loadPersistedConfig(); err != nil {
		log.Warnf("ignoring persisted backup config %s: %v", m.configPath(), err)
	} else if found {
		m.cfg = persisted
	}
	return m
}

// configPath is where runtime config updates are persisted, next to the
// sqlite store file in the datadir.
func (m *Manager) configPath() string {
	return filepath.Join(m.dataDir, configFileName)
}

// loadPersistedConfig reads backup.json. A missing file reports found=false
// and no error.
func (m *Manager) loadPersistedConfig() (Config, bool, error) {
	data, err := os.ReadFile(m.configPath())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Config{}, false, nil
		}
		return Config{}, false, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, false, err
	}
	return cfg, true, nil
}

// persistConfigLocked writes the current config to backup.json with
// owner-only permissions because it contains the S3 secret key and the
// encryption passphrase. The caller must hold m.mu.
func (m *Manager) persistConfigLocked(ctx context.Context) error {
	if err := util.WriteJsonWithRestrictedPermission(ctx, m.configPath(), m.cfg); err != nil {
		return fmt.Errorf("failed to persist backup config: %w", err)
	}
	return nil
}

// config returns the live config under lock.
func (m *Manager) config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Start launches the scheduler. With no interval configured it starts in
// manual-only mode: no timer is armed, only RunNow does work.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	m.mu.Lock()
	m.rescheduleLocked()
	m.mu.Unlock()
	go m.loop(ctx)

	cfg := m.config()
	if !cfg.Enabled() {
		log.Info("backups are disabled: no S3 bucket configured")
	}
}

// Stop halts the scheduler. In-flight runs are cancelled through the run
// context.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.started = false
}

// RunNow executes one backup run synchronously: snapshot the store, encrypt
// the archive with the age passphrase, upload it to the bucket.
func (m *Manager) RunNow(ctx context.Context) (*RunResult, error) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, ErrRunInProgress
	}
	m.running = true
	m.mu.Unlock()

	result, err := m.run(ctx)

	m.mu.Lock()
	m.running = false
	m.lastRun = m.now()
	if err != nil {
		m.lastErr = err.Error()
	} else {
		m.lastErr = ""
		m.lastKey = result.Key
	}
	m.mu.Unlock()
	return result, err
}

// run is the body of one backup execution.
func (m *Manager) run(ctx context.Context) (*RunResult, error) {
	cfg := m.config()
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	if cfg.EncryptionKey == "" {
		return nil, ErrNoEncryptionKey
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	objectStore, err := m.newStore(cfg)
	if err != nil {
		return nil, err
	}

	key := objectName(cfg.Prefix(), m.now())

	// The archive streams through a pipe into the age encryptor and out to
	// S3, so the plaintext snapshot never exists outside this process.
	pr, pw := io.Pipe()
	snapErr := make(chan error, 1)
	go func() {
		_, err := m.snapshotter.Snapshot(ctx, pw)
		if err != nil {
			_ = pw.CloseWithError(err)
		} else {
			_ = pw.Close()
		}
		snapErr <- err
	}()

	encR, encW := io.Pipe()
	encErr := make(chan error, 1)
	go func() {
		_, err := encrypt(encW, pr, cfg.EncryptionKey)
		if err != nil {
			_ = encW.CloseWithError(err)
		} else {
			_ = encW.Close()
		}
		encErr <- err
	}()

	start := m.now()
	size, putErr := objectStore.Put(ctx, key, encR, -1)
	if putErr != nil {
		// Unblock the snapshot and encrypt goroutines, which may still be
		// writing into the pipes nobody reads any more.
		_ = encR.CloseWithError(putErr)
		_ = pr.CloseWithError(putErr)
		// Drain both goroutines so nothing leaks.
		for i := 0; i < 2; i++ {
			var drainErr error
			select {
			case drainErr = <-snapErr:
			case drainErr = <-encErr:
			}
			_ = drainErr
		}
		return nil, fmt.Errorf("failed to upload backup %s: %w", key, putErr)
	}
	if err := <-snapErr; err != nil {
		return nil, fmt.Errorf("failed to snapshot store: %w", err)
	}
	if err := <-encErr; err != nil {
		return nil, err
	}

	return &RunResult{Key: key, Size: size, Duration: m.now().Sub(start)}, nil
}

// ListSnapshots returns every snapshot object under the configured prefix.
func (m *Manager) ListSnapshots(ctx context.Context) ([]ObjectInfo, error) {
	cfg := m.config()
	if !cfg.Enabled() {
		return nil, ErrNotConfigured
	}
	objectStore, err := m.newStore(cfg)
	if err != nil {
		return nil, err
	}
	return objectStore.List(ctx)
}

// UpdateConfig validates, merges and persists a new config, then reschedules
// the timer. Empty or masked secret fields keep their existing values.
func (m *Manager) UpdateConfig(ctx context.Context, in Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	merged := m.cfg.MergeSecrets(in)
	if err := merged.Validate(); err != nil {
		return err
	}
	m.cfg = merged
	if err := m.persistConfigLocked(ctx); err != nil {
		return err
	}
	m.rescheduleLocked()
	return nil
}

// Status returns a snapshot of the manager state with secrets masked.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Status{
		Enabled:      m.cfg.Enabled(),
		Running:      m.running,
		LastRun:      m.lastRun,
		LastError:    m.lastErr,
		LastSnapshot: m.lastKey,
		NextRun:      m.nextRun,
		Config:       m.cfg.Masked(),
	}
}

// rescheduleLocked re-arms the run timer for the current interval. Interval
// zero clears the next run, putting the scheduler in manual-only mode. The
// caller must hold m.mu.
func (m *Manager) rescheduleLocked() {
	d, err := m.cfg.IntervalDuration()
	if err != nil || d <= 0 || !m.cfg.Enabled() {
		m.nextRun = time.Time{}
	} else {
		m.nextRun = m.now().Add(d)
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// loop is the scheduler goroutine. It sleeps until nextRun, runs the backup
// inline and reschedules. wake forces a reschedule after a config update.
func (m *Manager) loop(ctx context.Context) {
	for {
		m.mu.Lock()
		next := m.nextRun
		m.mu.Unlock()

		var timer *time.Timer
		var fire <-chan time.Time
		if !next.IsZero() {
			delay := time.Until(next)
			if delay < 0 {
				delay = 0
			}
			timer = time.NewTimer(delay)
			fire = timer.C
		}

		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case <-m.wake:
			if timer != nil {
				timer.Stop()
			}
		case <-fire:
			// Runs execute in the loop so scheduled runs can never overlap:
			// a run longer than the interval pushes the next one back
			// instead of failing on the run guard.
			runCtx, cancel := context.WithTimeout(ctx, runTimeout)
			if _, err := m.RunNow(runCtx); err != nil {
				log.WithContext(runCtx).Errorf("scheduled backup failed: %v", err)
			}
			cancel()
			m.mu.Lock()
			m.rescheduleLocked()
			m.mu.Unlock()
		}
	}
}
