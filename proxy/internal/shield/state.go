package shield

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// sourceState is the per-source reputation record.
type sourceState struct {
	key       netip.Addr
	score     float64
	lastSeen  time.Time
	banUntil  time.Time
	banCount  int
	paths     map[string]time.Time // recent probe-ish paths
	probes    []time.Time          // 4xx/5xx hits, bounded
	limiter   *rateBucket
	enumPrune time.Time
}

// rateBucket is a tiny token bucket.
type rateBucket struct {
	tokens float64
	last   time.Time
}

const shards = 64

type shard struct {
	mu      sync.Mutex
	entries map[netip.Addr]*sourceState
}

type store struct {
	cfg           Config
	clock         func() time.Time
	shards        [shards]*shard
	activeTarpits atomic.Int32
	dirty         chan struct{}
}

func newStore(cfg Config, clock func() time.Time) *store {
	s := &store{cfg: cfg, clock: clock, dirty: make(chan struct{}, 1)}
	for i := range s.shards {
		s.shards[i] = &shard{entries: map[netip.Addr]*sourceState{}}
	}
	return s
}

func key(ip netip.Addr) netip.Addr {
	if ip.Is6() && !ip.Is4In6() {
		return netip.PrefixFrom(ip, 64).Masked().Addr()
	}
	return ip
}

func (s *store) get(ip netip.Addr) (*shard, *sourceState) {
	k := key(ip)
	sh := s.shards[k.As16()[0]&byte(shards-1)]
	sh.mu.Lock()
	defer sh.mu.Unlock()
	st := sh.entries[k]
	if st == nil {
		// evict least recently seen non-banned when at cap
		if len(sh.entries)*shards >= s.cfg.MaxEntries {
			var oldest netip.Addr
			var oldT time.Time
			now := s.clock()
			for a, e := range sh.entries {
				if e.banUntil.After(now) {
					continue
				}
				if oldT.IsZero() || e.lastSeen.Before(oldT) {
					oldT, oldest = e.lastSeen, a
				}
			}
			if !oldest.IsValid() {
				sh.mu.Unlock()
				// whole shard banned; drop oldest anyway to stay bounded
				for a := range sh.entries {
					oldest = a
					break
				}
				sh.mu.Lock()
			}
			delete(sh.entries, oldest)
		}
		st = &sourceState{key: k, paths: map[string]time.Time{}}
		sh.entries[k] = st
	}
	return sh, st
}

// decay applies exponential decay to st.score relative to now.
func (s *store) decay(st *sourceState) {
	now := s.clock()
	el := now.Sub(st.lastSeen)
	if st.score > 0 && el > 0 {
		half := float64(s.cfg.DecayHalfLife)
		st.score *= pow2(-float64(el) / half)
	}
	st.lastSeen = now
}

func pow2(x float64) float64 {
	if x <= -64 {
		return 0
	}
	// 2^x via exp2 approximation using math.Exp2 equivalent
	return fastExp2(x)
}

// isBanned reports whether st is currently banned.
func (s *store) isBanned(st *sourceState) bool {
	return st.banUntil.After(s.clock())
}

// ban applies an escalating TTL ban.
func (s *store) ban(st *sourceState) time.Duration {
	now := s.clock()
	if st.banUntil.After(now) {
		return st.banUntil.Sub(now)
	}
	// quiet period resets the escalation ladder
	if !st.banUntil.IsZero() && now.Sub(st.banUntil) > s.cfg.BanQuietReset {
		st.banCount = 0
	}
	ttl := s.cfg.BanBaseTTL
	for i := 0; i < st.banCount; i++ {
		ttl *= time.Duration(s.cfg.BanEscalation)
		if ttl >= s.cfg.BanMaxTTL {
			ttl = s.cfg.BanMaxTTL
			break
		}
	}
	st.banCount++
	st.banUntil = now.Add(ttl)
	return ttl
}

// snapshot/restore for state file
type banRecord struct {
	IP    string    `json:"ip"`
	Until time.Time `json:"until"`
	Count int       `json:"count"`
}

func (s *store) saveBans() error {
	if s.cfg.StateFile == "" {
		return nil
	}
	now := s.clock()
	recs := make([]banRecord, 0, s.cfg.StateMaxBans)
	for _, sh := range s.shards {
		sh.mu.Lock()
		for a, e := range sh.entries {
			if e.banUntil.After(now) && len(recs) < s.cfg.StateMaxBans {
				recs = append(recs, banRecord{IP: a.String(), Until: e.banUntil, Count: e.banCount})
			}
		}
		sh.mu.Unlock()
	}
	b, err := json.Marshal(recs)
	if err != nil {
		return err
	}
	tmp := s.cfg.StateFile + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.cfg.StateFile)
}

func (s *store) loadBans() {
	if s.cfg.StateFile == "" {
		return
	}
	b, err := os.ReadFile(filepath.Clean(s.cfg.StateFile))
	if err != nil {
		return
	}
	var recs []banRecord
	if err := json.Unmarshal(b, &recs); err != nil {
		return // tolerate corruption
	}
	now := s.clock()
	for _, r := range recs {
		ip, err := netip.ParseAddr(r.IP)
		if err != nil || !r.Until.After(now) {
			continue
		}
		_, st := s.get(ip)
		st.banUntil = r.Until
		st.banCount = r.Count
	}
}
