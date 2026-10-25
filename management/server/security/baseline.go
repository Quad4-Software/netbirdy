package security

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const baselineVersion = 1

type ipEntry struct {
	ip       netip.Addr
	lastSeen time.Time
	country  string
	asn      uint32
}

// peerBaseline is the learned state for one peer.
type peerBaseline struct {
	ips       map[string]*ipEntry // addr string -> entry
	countries map[string]struct{}
	asns      map[uint32]struct{}
	meta      PeerMeta
	firstSeen time.Time
	lastSeen  time.Time
}

func newPeerBaseline(now time.Time) *peerBaseline {
	return &peerBaseline{
		ips:       make(map[string]*ipEntry),
		countries: make(map[string]struct{}),
		asns:      make(map[uint32]struct{}),
		firstSeen: now,
		lastSeen:  now,
	}
}

func (b *peerBaseline) remember(obs PeerObservation, now time.Time, maxIPs int) {
	key := obs.SrcIP.String()
	b.ips[key] = &ipEntry{ip: obs.SrcIP, lastSeen: now, country: obs.Meta.OS, asn: 0}
	if len(b.ips) > maxIPs {
		// evict the oldest-seen entry
		var oldest string
		var ts time.Time
		for k, e := range b.ips {
			if oldest == "" || e.lastSeen.Before(ts) {
				oldest, ts = k, e.lastSeen
			}
		}
		delete(b.ips, oldest)
	}
	b.lastSeen = now
}

func (b *peerBaseline) knownIP(ip netip.Addr) bool {
	_, ok := b.ips[ip.String()]
	return ok
}

// countWindow is a sliding-window counter for auth failures, setup key use
// and flaps.
type countWindow struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func newCountWindow() *countWindow {
	return &countWindow{hits: make(map[string][]time.Time)}
}

// hit records one occurrence and returns the count within window.
func (w *countWindow) hit(key string, now time.Time, window time.Duration, maxKeys int) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := now.Add(-window)
	kept := w.hits[key][:0]
	for _, ts := range w.hits[key] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	kept = append(kept, now)
	w.hits[key] = kept
	// crude bound: evict an arbitrary stale key when over cap
	if len(w.hits) > maxKeys {
		for k, ts := range w.hits {
			old := true
			for _, t := range ts {
				if t.After(cutoff) {
					old = false
					break
				}
			}
			if old {
				delete(w.hits, k)
			}
		}
		if len(w.hits) > maxKeys {
			for k := range w.hits {
				delete(w.hits, k)
				break
			}
		}
	}
	return len(w.hits[key])
}

func (w *countWindow) count(key string, now time.Time, window time.Duration) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	cutoff := now.Add(-window)
	n := 0
	for _, ts := range w.hits[key] {
		if ts.After(cutoff) {
			n++
		}
	}
	return n
}

// distinctCounter tracks distinct values in a window (e.g. IPs per setup key).
type distinctCounter struct {
	mu   sync.Mutex
	vals map[string]map[string]time.Time
}

func newDistinctCounter() *distinctCounter {
	return &distinctCounter{vals: make(map[string]map[string]time.Time)}
}

func (d *distinctCounter) add(key, val string, now time.Time, window time.Duration, maxKeys int) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.vals[key] == nil {
		d.vals[key] = make(map[string]time.Time)
	}
	cutoff := now.Add(-window)
	for v, ts := range d.vals[key] {
		if ts.Before(cutoff) {
			delete(d.vals[key], v)
		}
	}
	d.vals[key][val] = now
	if len(d.vals) > maxKeys {
		for k := range d.vals {
			delete(d.vals, k)
			break
		}
	}
	return len(d.vals[key])
}

// baselineStore holds all learned state; snapshot/restore for restart.
type baselineStore struct {
	mu        sync.RWMutex
	peers     map[string]*peerBaseline // accountID/peerID -> baseline
	startedAt time.Time
}

func newBaselineStore(now time.Time) *baselineStore {
	return &baselineStore{peers: make(map[string]*peerBaseline), startedAt: now}
}

func (s *baselineStore) key(obs PeerObservation) string {
	return obs.AccountID + "/" + obs.PeerID
}

func (s *baselineStore) peer(obs PeerObservation, now time.Time, max int) *peerBaseline {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.key(obs)
	b, ok := s.peers[k]
	if !ok {
		if len(s.peers) >= max {
			// evict the coldest baseline
			var oldest string
			var ts time.Time
			for pk, pb := range s.peers {
				if oldest == "" || pb.lastSeen.Before(ts) {
					oldest, ts = pk, pb.lastSeen
				}
			}
			delete(s.peers, oldest)
		}
		b = newPeerBaseline(now)
		s.peers[k] = b
	}
	return b
}

func (s *baselineStore) get(accountID, peerID string) *peerBaseline {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.peers[accountID+"/"+peerID]
}

type baselineSnapshot struct {
	Version   int                              `json:"version"`
	SavedAt   time.Time                        `json:"saved_at"`
	StartedAt time.Time                        `json:"started_at"`
	Peers     map[string]*peerBaselineSnapshot `json:"peers"`
}

type peerBaselineSnapshot struct {
	IPs       []string  `json:"ips"`
	Countries []string  `json:"countries,omitempty"`
	ASNs      []uint32  `json:"asns,omitempty"`
	Meta      PeerMeta  `json:"meta"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// Snapshot serializes the learned baseline for save or test inspection.
func (s *baselineStore) Snapshot() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	snap := baselineSnapshot{
		Version: baselineVersion, SavedAt: time.Now(), StartedAt: s.startedAt,
		Peers: make(map[string]*peerBaselineSnapshot, len(s.peers)),
	}
	for k, b := range s.peers {
		ps := &peerBaselineSnapshot{Meta: b.meta, FirstSeen: b.firstSeen, LastSeen: b.lastSeen}
		for ip := range b.ips {
			ps.IPs = append(ps.IPs, ip)
		}
		for c := range b.countries {
			ps.Countries = append(ps.Countries, c)
		}
		for a := range b.asns {
			ps.ASNs = append(ps.ASNs, a)
		}
		snap.Peers[k] = ps
	}
	return json.Marshal(snap)
}

// Restore loads a snapshot; corrupt entries are skipped, expired ones pruned.
func (s *baselineStore) Restore(data []byte, now time.Time, maxAge time.Duration) error {
	var snap baselineSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("baseline snapshot: %w", err)
	}
	if snap.Version != baselineVersion {
		return fmt.Errorf("baseline snapshot version %d", snap.Version)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startedAt = snap.StartedAt
	for k, ps := range snap.Peers {
		if now.Sub(ps.LastSeen) > maxAge {
			continue // stale baseline for a peer that has not shown up
		}
		b := newPeerBaseline(ps.FirstSeen)
		b.lastSeen = ps.LastSeen
		b.meta = ps.Meta
		for _, raw := range ps.IPs {
			ip, err := netip.ParseAddr(raw)
			if err != nil {
				continue
			}
			b.ips[ip.String()] = &ipEntry{ip: ip, lastSeen: ps.LastSeen}
		}
		for _, c := range ps.Countries {
			b.countries[c] = struct{}{}
		}
		for _, a := range ps.ASNs {
			b.asns[a] = struct{}{}
		}
		s.peers[k] = b
	}
	return nil
}

// SaveSnapshot writes the baseline atomically to path with 0600.
func (s *baselineStore) SaveSnapshot(path string) error {
	data, err := s.Snapshot()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadSnapshot restores the baseline from path if present.
func (s *baselineStore) LoadSnapshot(path string, now time.Time, maxAge time.Duration) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return s.Restore(data, now, maxAge)
}
