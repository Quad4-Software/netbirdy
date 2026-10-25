package internal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
	"google.golang.org/protobuf/proto"
)

const offlineSnapshotVersion = 2

var (
	errOfflineNoSnapshot = errors.New("offline snapshot not found")
	errOfflineExpired    = errors.New("offline snapshot expired")
	errOfflineCorrupt    = errors.New("offline snapshot corrupt")
	errOfflineVersion    = errors.New("offline snapshot version mismatch")
	errOfflineDisabled   = errors.New("offline snapshot disabled")
)

type offlineSnapshot struct {
	Version int       `json:"version"`
	SavedAt time.Time `json:"saved_at"`
	Sync    []byte    `json:"sync"`
	// Endpoints maps remote peer WireGuard public keys to their last known
	// direct endpoint (ip:port). Only recorded for direct P2P connections.
	Endpoints map[string]string `json:"endpoints,omitempty"`

	// MgmtAddrs caches the last successfully resolved management IPs so the
	// reconnect loop can dial them directly when DNS no longer resolves the
	// hostname (offline DNS interception or a dead upstream).
	MgmtAddrs []string `json:"mgmt_addrs,omitempty"`
}

// offlineSnapshotPath returns the snapshot path for a given config path.
func offlineSnapshotPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "networkmap.bin")
}

func saveOfflineSnapshot(path string, resp *mgmProto.SyncResponse, endpoints map[string]string, now time.Time) error {
	if resp == nil {
		return fmt.Errorf("nil sync response")
	}
	raw, err := proto.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal sync response: %w", err)
	}
	mgmtAddrs := LastMgmtAddrs()
	if len(mgmtAddrs) == 0 {
		if bs, rerr := os.ReadFile(path); rerr == nil {
			var prev offlineSnapshot
			if json.Unmarshal(bs, &prev) == nil {
				mgmtAddrs = prev.MgmtAddrs
			}
		}
	}
	env, err := json.Marshal(offlineSnapshot{Version: offlineSnapshotVersion, SavedAt: now, Sync: raw, Endpoints: endpoints, MgmtAddrs: mgmtAddrs})
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open snapshot: %w", err)
	}
	if _, err = f.Write(env); err == nil {
		err = f.Sync()
	}
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename snapshot: %w", err)
	}
	return nil
}

func loadOfflineSnapshot(path string, maxAge time.Duration, now time.Time) (*mgmProto.SyncResponse, time.Time, error) {
	loaded = nil
	resp2, t, err := loadSnapshotInner(path, maxAge, now)
	lastEndpoints = loaded
	return resp2, t, err
}

var (
	loaded          map[string]string
	lastEndpoints   map[string]string
	lastMgmtAddrs   []string
	lastMgmtAddrsMu sync.Mutex
)

func loadSnapshotInner(path string, maxAge time.Duration, now time.Time) (*mgmProto.SyncResponse, time.Time, error) {
	if maxAge <= 0 {
		return nil, time.Time{}, errOfflineDisabled
	}
	bs, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, time.Time{}, errOfflineNoSnapshot
		}
		return nil, time.Time{}, err
	}
	var env offlineSnapshot
	if err := json.Unmarshal(bs, &env); err != nil {
		return nil, time.Time{}, fmt.Errorf("%w: %v", errOfflineCorrupt, err)
	}
	if env.Version < 1 || env.Version > offlineSnapshotVersion {
		return nil, time.Time{}, errOfflineVersion
	}
	if now.Sub(env.SavedAt) > maxAge {
		return nil, time.Time{}, errOfflineExpired
	}
	resp := &mgmProto.SyncResponse{}
	if err := proto.Unmarshal(env.Sync, resp); err != nil {
		return nil, time.Time{}, fmt.Errorf("%w: %v", errOfflineCorrupt, err)
	}
	for i, a := range env.MgmtAddrs {
		if _, err := netip.ParseAddr(a); err != nil {
			env.MgmtAddrs[i] = ""
		}
	}
	cleaned := env.MgmtAddrs[:0]
	for _, a := range env.MgmtAddrs {
		if a != "" {
			cleaned = append(cleaned, a)
		}
	}
	env.MgmtAddrs = cleaned
	for k, v := range env.Endpoints {
		ap, err := netip.ParseAddrPort(v)
		if err != nil || ap.Addr().IsUnspecified() || ap.Addr().IsMulticast() {
			delete(env.Endpoints, k)
		}
	}
	loaded = env.Endpoints
	lastMgmtAddrsMu.Lock()
	lastMgmtAddrs = env.MgmtAddrs
	lastMgmtAddrsMu.Unlock()
	return resp, env.SavedAt, nil
}

func deleteOfflineSnapshot(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// offlineDebouncer gates snapshot writes to at most one per interval; the
// write itself runs in a goroutine so the sync path never blocks.
type offlineDebouncer struct {
	interval time.Duration
	lastSave atomic.Int64 // unixnano
	inFlight atomic.Bool
	pending  atomic.Bool
	saver    func() error
	clockNow func() time.Time
	logErr   func(error)
}

func newOfflineDebouncer(interval time.Duration, saver func() error, logErr func(error)) *offlineDebouncer {
	return &offlineDebouncer{interval: interval, saver: saver, logErr: logErr, clockNow: time.Now}
}

// Save schedules a snapshot write if the interval has elapsed; returns true if
// a write was scheduled.
func (d *offlineDebouncer) Save() bool {
	now := d.clockNow().UnixNano()
	if since := now - d.lastSave.Load(); since < int64(d.interval) {
		// Dropped writes are retried once when the interval elapses so a
		// late event (e.g. a peer endpoint learned right after a map
		// update) still lands in the snapshot.
		if d.pending.CompareAndSwap(false, true) {
			go func() {
				select {
				case <-time.After(d.interval - time.Duration(since)):
				}
				d.pending.Store(false)
				d.Save()
			}()
		}
		return false
	}
	if !d.inFlight.CompareAndSwap(false, true) {
		return false
	}
	d.lastSave.Store(now)
	go func() {
		defer d.inFlight.Store(false)
		if err := d.saver(); err != nil && d.logErr != nil {
			d.logErr(err)
		}
	}()
	return true
}

// OfflineMaxAge reads NB_OFFLINE_MAX_AGE (Go duration). Default 168h; "0"
// disables both writing and using the snapshot.
func OfflineMaxAge() time.Duration {
	v := os.Getenv("NB_OFFLINE_MAX_AGE")
	if v == "" {
		return 168 * time.Hour
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 168 * time.Hour
	}
	return d
}

// DeleteOfflineSnapshotFor removes the snapshot stored next to configPath.
func DeleteOfflineSnapshotFor(configPath string) error {
	return deleteOfflineSnapshot(offlineSnapshotPath(configPath))
}

// LastEndpoints returns the validated peer endpoints from the most recent
// successful loadOfflineSnapshot call.
func LastEndpoints() map[string]string { return lastEndpoints }

// LastMgmtAddrs returns the cached management IPs from the snapshot.
func LastMgmtAddrs() []string {
	lastMgmtAddrsMu.Lock()
	defer lastMgmtAddrsMu.Unlock()
	return append([]string(nil), lastMgmtAddrs...)
}

// updateMgmtAddrs persists freshly resolved management IPs into an existing
// snapshot envelope immediately, so a later debounced write cannot race the
// capture. No-op when the snapshot does not exist yet.
func updateMgmtAddrs(path string, addrs []string) {
	if len(addrs) == 0 {
		return
	}
	bs, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var env offlineSnapshot
	if err := json.Unmarshal(bs, &env); err != nil {
		return
	}
	env.MgmtAddrs = addrs
	out, err := json.Marshal(env)
	if err != nil {
		return
	}
	if err := os.WriteFile(path, out, 0o600); err == nil {
		lastMgmtAddrsMu.Lock()
		lastMgmtAddrs = addrs
		lastMgmtAddrsMu.Unlock()
	}
}

// SetMgmtAddrs records freshly resolved management IPs for the snapshot.
func SetMgmtAddrs(addrs []string) {
	lastMgmtAddrsMu.Lock()
	lastMgmtAddrs = addrs
	lastMgmtAddrsMu.Unlock()
}

// mergeEndpoints keeps endpoints for peers that exist in the current map:
// freshly captured values win, previous values fill gaps for peers that are
// currently disconnected.
func mergeEndpoints(prev, current map[string]string, peerKeys map[string]struct{}) map[string]string {
	out := make(map[string]string, len(peerKeys))
	for k := range peerKeys {
		if v, ok := current[k]; ok {
			out[k] = v
		} else if v, ok := prev[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
