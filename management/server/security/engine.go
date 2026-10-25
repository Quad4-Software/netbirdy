package security

import (
	"context"
	"net/netip"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Engine evaluates observations against the learned baseline and emits
// findings. All Observe* methods are non-blocking and panic-safe; actions
// run through a bounded worker queue so the gRPC path never waits.
type Engine struct {
	cfg   *Config
	clock Clock
	geo   GeoResolver
	sink  Sink
	acts  Actions

	store        *baselineStore
	ipFlapCounts *distinctCounter
	failCounts   *countWindow
	setupKeyIPs  *distinctCounter
	setupKeyRegs *countWindow
	patIPs       *distinctCounter

	mu            sync.Mutex
	liveStreams   map[string]map[string]time.Time // peerKey -> srcIP -> opened
	userCountries map[string]map[string]struct{}
	cooldownUntil map[string]time.Time   // rule+subject -> suppress until
	preventCount  map[string][]time.Time // accountID -> prevention times
	actionQueue   chan actionTask
	wg            sync.WaitGroup
	stopQ         chan struct{}
}

type actionTask struct {
	fn              func(ctx context.Context) error
	ctx             context.Context
	rule, accountID string
}

// NewEngine builds the engine. geo may be nil. sink receives all emitted
// events; acts receives prevention calls.
func NewEngine(cfg *Config, clock Clock, geo GeoResolver, sink Sink, acts Actions) *Engine {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if clock == nil {
		clock = SystemClock{}
	}
	e := &Engine{
		cfg:           cfg,
		clock:         clock,
		geo:           geo,
		sink:          sink,
		acts:          acts,
		store:         newBaselineStore(clock.Now()),
		ipFlapCounts:  newDistinctCounter(),
		failCounts:    newCountWindow(),
		setupKeyIPs:   newDistinctCounter(),
		setupKeyRegs:  newCountWindow(),
		patIPs:        newDistinctCounter(),
		liveStreams:   make(map[string]map[string]time.Time),
		userCountries: make(map[string]map[string]struct{}),
		cooldownUntil: make(map[string]time.Time),
		preventCount:  make(map[string][]time.Time),
		actionQueue:   make(chan actionTask, cfg.QueueSize),
		stopQ:         make(chan struct{}),
	}
	e.wg.Add(1)
	go e.actionWorker()
	return e
}

// Close drains the action queue, persists a final baseline and stops the
// worker.
func (e *Engine) Close() {
	close(e.stopQ)
	e.wg.Wait()
}

// runSnapshotLoop persists the baseline on an interval until Close.
func (e *Engine) RunSnapshotLoop(path string, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-e.stopQ:
			if err := e.SaveSnapshot(path); err != nil {
				log.Warnf("security baseline save: %v", err)
			}
			return
		case <-t.C:
			if err := e.SaveSnapshot(path); err != nil {
				log.Warnf("security baseline save: %v", err)
			}
		}
	}
}

func (e *Engine) actionWorker() {
	defer e.wg.Done()
	for {
		select {
		case <-e.stopQ:
			return
		case t := <-e.actionQueue:
			if err := t.fn(t.ctx); err != nil {
				log.WithContext(t.ctx).Warnf("security action %s failed for %s: %v", t.rule, t.accountID, err)
			}
		}
	}
}

func (e *Engine) learning() bool {
	return e.clock.Now().Sub(e.store.startedAt) < e.cfg.LearningPeriod
}

func severityFor(score int, cfg *Config) Severity {
	switch {
	case score >= cfg.SeverityHigh:
		return SeverityHigh
	case score >= cfg.SeverityMedium:
		return SeverityMedium
	case score > 0:
		return SeverityLow
	}
	return SeverityInfo
}

func (e *Engine) onCooldown(rule, subject string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if until, ok := e.cooldownUntil[rule+"/"+subject]; ok && now.Before(until) {
		return true
	}
	e.cooldownUntil[rule+"/"+subject] = now.Add(e.cfg.Cooldown)
	return false
}

// emit applies cooldown, severity gating, prevention decisions and sinks.
func (e *Engine) emit(rule string, score int, ev Event, subject string, prevention func(ctx context.Context) error) {
	if score <= 0 {
		return
	}
	now := e.clock.Now()
	if e.onCooldown(rule, subject, now) {
		return
	}
	learning := e.learning()
	ev.Timestamp = now
	ev.Score = score
	ev.Rule = rule
	ev.Mode = e.cfg.Mode
	ev.Severity = severityFor(score, e.cfg)
	if learning && ev.Severity != SeverityInfo {
		ev.Severity = SeverityInfo // learning: record, never escalate
	}
	// decide whether the rule would prevent
	action := e.cfg.ruleAction(rule)
	prevent := e.cfg.Mode == ModePrevent && !learning && action != ActionAlert && e.allowedPrevention(ev.AccountID, now)
	if prevent && prevention != nil {
		select {
		case e.actionQueue <- actionTask{fn: prevention, ctx: context.Background(), rule: rule, accountID: ev.AccountID}:
			ev.ActionTaken = action
			e.recordPrevention(ev.AccountID, now)
		default:
			log.Warnf("security action queue full, dropping %s for %s", rule, ev.AccountID)
		}
	} else if !learning && e.cfg.Mode == ModePrevent && action != ActionAlert {
		ev.WouldPrevent = false // in prevent mode but capped/learning
	}
	if !prevent && e.cfg.Mode != ModeOff && action != ActionAlert && !learning {
		ev.WouldPrevent = true
	}
	ev.ActionTaken = ActionAlert
	if prevent {
		ev.ActionTaken = action
	}
	if e.sink != nil {
		e.sink.Emit(ev)
	}
}

func (e *Engine) allowedPrevention(accountID string, now time.Time) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	cutoff := now.Add(-time.Hour)
	kept := e.preventCount[accountID][:0]
	for _, ts := range e.preventCount[accountID] {
		if ts.After(cutoff) {
			kept = append(kept, ts)
		}
	}
	e.preventCount[accountID] = kept
	return len(kept) < e.cfg.MaxPreventionsPerAccountPerHour
}

func (e *Engine) recordPrevention(accountID string, now time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.preventCount[accountID] = append(e.preventCount[accountID], now)
}

func observe(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("security engine panic in %s: %v", name, r)
		}
	}()
	fn()
}

// ObservePeerLogin records a peer login observation.
func (e *Engine) ObservePeerLogin(ctx context.Context, obs PeerObservation) {
	if e.cfg.Mode == ModeOff {
		return
	}
	observe("peer_login", func() {
		now := e.clock.Now()
		e.observePeer(obs, now)
	})
}

// ObservePeerSync records a sync/stream observation.
func (e *Engine) ObservePeerSync(ctx context.Context, obs PeerObservation) {
	if e.cfg.Mode == ModeOff {
		return
	}
	observe("peer_sync", func() {
		e.observePeer(obs, e.clock.Now())
	})
}

func (e *Engine) observePeer(obs PeerObservation, now time.Time) {
	if e.cfg.trusted(obs.SrcIP) {
		return
	}
	b := e.store.peer(obs, now, e.cfg.MaxPeers)
	var country string
	var asn uint32
	if e.geo != nil {
		country, asn = e.geo.Lookup(obs.SrcIP)
	}
	base := Event{
		Type: "peer", AccountID: obs.AccountID, PeerID: obs.PeerID,
		PeerName: obs.PeerName, SrcIP: obs.SrcIP.String(), Country: country, ASN: asn,
	}
	for ip := range b.ips {
		base.PreviousIPs = append(base.PreviousIPs, ip)
	}
	if e.cfg.ruleEnabled("peer_new_ip") {
		if score, reason := e.peerNewIP(obs, b, now); score > 0 {
			ev := base
			ev.Reason = reason
			e.emit("peer_new_ip", score, ev, obs.PeerID, nil)
		}
	}
	if e.cfg.ruleEnabled("peer_ip_flap") {
		if score, reason := e.peerIPFlap(obs, now); score > 0 {
			ev := base
			ev.Reason = reason
			e.emit("peer_ip_flap", score, ev, obs.PeerID, nil)
		}
	}
	if e.cfg.ruleEnabled("peer_meta_mismatch") {
		if score, reason := e.peerMetaMismatch(obs, b); score > 0 {
			ev := base
			ev.Reason = reason
			e.emit("peer_meta_mismatch", score, ev, obs.PeerID, nil)
		}
	}
	// update baseline after scoring
	if country != "" {
		b.countries[country] = struct{}{}
	}
	if asn != 0 {
		b.asns[asn] = struct{}{}
	}
	if obs.Meta.Hostname != "" || obs.Meta.Serial != "" {
		b.meta = obs.Meta
	}
	b.remember(obs, now, e.cfg.MaxIPsPerPeer)
}

// ObserveStreamOpen registers a live sync stream for concurrent detection.
func (e *Engine) ObserveStreamOpen(ctx context.Context, peerKey, srcIP, accountID, peerID, peerName string) {
	if e.cfg.Mode == ModeOff {
		return
	}
	observe("stream_open", func() {
		now := e.clock.Now()
		e.mu.Lock()
		if e.liveStreams[peerKey] == nil {
			e.liveStreams[peerKey] = make(map[string]time.Time)
		}
		score, reason, other := e.concurrentConnection(peerKey, srcIP, now)
		e.liveStreams[peerKey][srcIP] = now
		e.mu.Unlock()
		if score > 0 && e.cfg.ruleEnabled("concurrent_connections") && !e.cfg.trusted(mustAddr(srcIP)) {
			e.emit("concurrent_connections", score, Event{
				Type: "peer", AccountID: accountID, PeerID: peerID, PeerName: peerName,
				SrcIP: srcIP, PreviousIPs: []string{other}, Reason: reason,
			}, peerKey, func(ctx context.Context) error {
				return e.acts.RequireApproval(ctx, accountID, peerID)
			})
		}
	})
}

// ObserveStreamClose removes a live stream.
func (e *Engine) ObserveStreamClose(peerKey, srcIP string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if m := e.liveStreams[peerKey]; m != nil {
		delete(m, srcIP)
		if len(m) == 0 {
			delete(e.liveStreams, peerKey)
		}
	}
}

// ObserveAuthFailure records an auth rejection.
func (e *Engine) ObserveAuthFailure(ctx context.Context, f AuthFailure) {
	if e.cfg.Mode == ModeOff || e.cfg.trusted(f.SourceIP) {
		return
	}
	observe("auth_failure", func() {
		if score, reason := e.authFailureBurst(f, e.clock.Now()); score > 0 && e.cfg.ruleEnabled("auth_failure_burst") {
			e.emit("auth_failure_burst", score, Event{
				Type: "auth", AccountID: f.AccountID, UserID: f.Subject,
				SrcIP: f.SourceIP.String(), Reason: reason,
			}, f.Subject+f.SourceIP.String(), nil)
		}
	})
}

// ObserveAuthSuccess records an auth success.
func (e *Engine) ObserveAuthSuccess(ctx context.Context, s AuthSuccess) {
	if e.cfg.Mode == ModeOff || e.cfg.trusted(s.SourceIP) {
		return
	}
	observe("auth_success", func() {
		if score, reason := e.authSuccessAfterFailures(s, e.clock.Now()); score > 0 && e.cfg.ruleEnabled("auth_success_after_failures") {
			e.emit("auth_success_after_failures", score, Event{
				Type: "auth", AccountID: s.AccountID, UserID: s.Subject,
				SrcIP: s.SourceIP.String(), Reason: reason,
			}, s.Subject, nil)
		}
	})
}

// ObserveSetupKeyUse records a setup-key registration.
func (e *Engine) ObserveSetupKeyUse(ctx context.Context, o SetupKeyObservation) {
	if e.cfg.Mode == ModeOff || e.cfg.trusted(o.SourceIP) {
		return
	}
	observe("setup_key", func() {
		if score, reason := e.setupKeyMisuse(o, e.clock.Now()); score > 0 && e.cfg.ruleEnabled("setup_key_misuse") {
			e.emit("setup_key_misuse", score, Event{
				Type: "setup_key", AccountID: o.AccountID, PeerID: o.PeerID,
				SrcIP: o.SourceIP.String(), Reason: reason,
			}, o.KeyID, nil)
		}
	})
}

// ObserveUserLogin records a dashboard user login.
func (e *Engine) ObserveUserLogin(ctx context.Context, accountID, userID string, srcIP netip.Addr) {
	if e.cfg.Mode == ModeOff || e.cfg.trusted(srcIP) {
		return
	}
	observe("user_login", func() {
		var country string
		if e.geo != nil {
			country, _ = e.geo.Lookup(srcIP)
		}
		if score, reason := e.userNewCountry(accountID, userID, country, e.clock.Now()); score > 0 && e.cfg.ruleEnabled("user_new_country") {
			e.emit("user_new_country", score, Event{
				Type: "user", AccountID: accountID, UserID: userID,
				SrcIP: srcIP.String(), Country: country, Reason: reason,
			}, userID, nil)
		}
	})
}

// ObservePATUse records a PAT-authenticated API call.
func (e *Engine) ObservePATUse(ctx context.Context, accountID, tokenID string, srcIP netip.Addr) {
	if e.cfg.Mode == ModeOff || e.cfg.trusted(srcIP) {
		return
	}
	observe("pat_use", func() {
		if score, reason := e.patNewIP(accountID, tokenID, srcIP, e.clock.Now()); score > 0 && e.cfg.ruleEnabled("pat_new_ip") {
			e.emit("pat_new_ip", score, Event{
				Type: "pat", AccountID: accountID, UserID: tokenID,
				SrcIP: srcIP.String(), Reason: reason,
			}, tokenID, nil)
		}
	})
}

// Snapshot returns the learned baseline for persistence or tests.
func (e *Engine) Snapshot() ([]byte, error) { return e.store.Snapshot() }

// Restore loads a baseline snapshot.
func (e *Engine) Restore(data []byte) error {
	return e.store.Restore(data, e.clock.Now(), e.cfg.LearningPeriod*4)
}

// SaveSnapshot persists the baseline to path.
func (e *Engine) SaveSnapshot(path string) error { return e.store.SaveSnapshot(path) }

// LoadSnapshot restores the baseline from path.
func (e *Engine) LoadSnapshot(path string) error {
	return e.store.LoadSnapshot(path, e.clock.Now(), e.cfg.LearningPeriod*4)
}

func mustAddr(s string) netip.Addr {
	a, _ := netip.ParseAddr(s)
	return a
}
