package shield

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/netbirdio/netbird/proxy/internal/proxy"
	"time"

	log "github.com/sirupsen/logrus"
)

// Action is what Wrap decided for a request.
type Action string

const (
	ActionPass      Action = "pass"
	ActionChallenge Action = "challenge"
	ActionTarpit    Action = "tarpit"
	ActionBlock     Action = "block"
	ActionDrop      Action = "drop"
)

// Event is an out-of-band signal delivered via Observe.
type Event struct {
	Kind   EventKind
	Detail string
}

type EventKind int

const (
	// TLS-level
	EventSNIMissing EventKind = iota
	EventSNIIsIP
	EventSNIUnknown
	// from auth wiring
	EventAuthFailure
)

// BanInfo describes an active ban.
type BanInfo struct {
	IP    string
	Until time.Time
	Count int
}

// Stats is a point-in-time snapshot.
type Stats struct {
	Tracked       int
	Banned        int
	ActiveTarpits int
}

// MetricsSink is an optional observability hook.
type MetricsSink interface {
	Decision(action, reason string)
	BannedGauge(n int)
	TarpitActive(n int)
}

// Option configures a Shield.
type Option func(*Shield)

func WithIPResolver(fn func(*http.Request) netip.Addr) Option {
	return func(s *Shield) { s.ipResolver = fn }
}
func WithKnownHost(fn func(string) bool) Option {
	return func(s *Shield) { s.knownHost = fn }
}
func WithMetrics(m MetricsSink) Option {
	return func(s *Shield) { s.metrics = m }
}
func WithLogger(l *log.Entry) Option {
	return func(s *Shield) { s.log = l }
}
func WithClock(c func() time.Time) Option {
	return func(s *Shield) { s.store = newStore(s.cfg, c) }
}

// VerifyPeer is an optional DNS resolver for claimed search engines.
type SearchVerifier interface {
	// Verify returns true when ip is confirmed to belong to the named bot.
	Verify(ctx context.Context, ip netip.Addr, ua string) bool
}

func WithSearchVerifier(v SearchVerifier) Option {
	return func(s *Shield) { s.verifier = v }
}

type ctxKey struct{}

type resultHolder struct{ r Result }

// Result is what Wrap recorded for a request.
type Result struct {
	Score   float64
	Action  Action
	Reasons []string
}

// ResultFromContext returns the decision Wrap stored for this request.
func ResultFromContext(ctx context.Context) Result {
	if h, ok := ctx.Value(ctxKey{}).(*resultHolder); ok {
		return h.r
	}
	return Result{Action: ActionPass}
}

// Shield is the defense engine.
type Shield struct {
	cfg         Config
	store       *store
	ipResolver  func(*http.Request) netip.Addr
	knownHost   func(string) bool
	metrics     MetricsSink
	decisionLog *DecisionLog
	log         *log.Entry
	verifier    SearchVerifier
	hmacKey     []byte
	uaCache     sync.Map
	uaCacheLen  atomic.Int64
	stopCh      chan struct{}
	closed      chan struct{}
	lastPanic   atomic.Int64
}

// New builds a Shield; clock defaults to real time unless WithClock given.
func New(cfg Config, opts ...Option) (*Shield, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	if cfg.HMACKey != "" {
		// hex or raw
	}
	s := &Shield{
		cfg: cfg, log: log.NewEntry(log.StandardLogger()),
		stopCh: make(chan struct{}), closed: make(chan struct{}),
	}
	s.store = newStore(cfg, time.Now)
	if cfg.HMACKey != "" {
		if b, err := hex.DecodeString(cfg.HMACKey); err == nil && len(b) >= 16 {
			s.hmacKey = b
		} else {
			k := sha256.Sum256([]byte(cfg.HMACKey))
			s.hmacKey = k[:]
		}
	} else {
		s.hmacKey = make([]byte, 32)
		if _, err := rand.Read(s.hmacKey); err != nil {
			return nil, fmt.Errorf("shield: hmac key: %w", err)
		}
	}
	for _, o := range opts {
		o(s)
	}
	s.store.loadBans()
	go s.saver()
	return s, nil
}

func (s *Shield) saver() {
	defer close(s.closed)
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-s.stopCh:
			_ = s.store.saveBans()
			return
		case <-t.C:
			_ = s.store.saveBans()
		}
	}
}

// Close stops background work and persists bans.
func (s *Shield) Close() {
	close(s.stopCh)
	<-s.closed
	_ = s.store.saveBans()
}

// Observe injects an out-of-band signal for a source.
func (s *Shield) Observe(ip netip.Addr, ev Event) {
	if !ip.IsValid() || s.cfg.Mode == ModeOff || s.cfg.allowIP(ip) {
		return
	}
	_, st := s.store.get(ip)
	s.store.decay(st)
	var w float64
	var reason string
	switch ev.Kind {
	case EventSNIMissing, EventSNIIsIP, EventSNIUnknown:
		w, reason = s.cfg.WeightTLS, ev.Detail
	case EventAuthFailure:
		w, reason = s.cfg.WeightAuthFailure, "auth-failure"
	}
	st.score += w
	s.maybeBan(st, reason)
}

// Stats returns counters.
func (s *Shield) Stats() Stats {
	var tracked, banned int
	now := s.store.clock()
	for _, sh := range s.store.shards {
		sh.mu.Lock()
		tracked += len(sh.entries)
		for _, e := range sh.entries {
			if e.banUntil.After(now) {
				banned++
			}
		}
		sh.mu.Unlock()
	}
	return Stats{Tracked: tracked, Banned: banned, ActiveTarpits: int(s.store.activeTarpits.Load())}
}

// Bans lists active bans.
func (s *Shield) Bans() []BanInfo {
	var out []BanInfo
	now := s.store.clock()
	for _, sh := range s.store.shards {
		sh.mu.Lock()
		for a, e := range sh.entries {
			if e.banUntil.After(now) {
				out = append(out, BanInfo{IP: a.String(), Until: e.banUntil, Count: e.banCount})
			}
		}
		sh.mu.Unlock()
	}
	return out
}

// Unban clears a ban.
func (s *Shield) Unban(ip netip.Addr) bool {
	sh, st := s.store.get(ip)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if st == nil || !st.banUntil.After(s.store.clock()) {
		return false
	}
	st.banUntil = time.Time{}
	st.banCount = 0
	return true
}

func (s *Shield) maybeBan(st *sourceState, reason string) {
	if st.score >= s.cfg.BanAt {
		sh := s.store.shards[st.key.As16()[0]&byte(shards-1)]
		sh.mu.Lock()
		ttl := s.store.ban(st)
		sh.mu.Unlock()
		s.log.Infof("shield: banned for %s (%s)", ttl, reason)
	}
}

// Wrap wraps the request path. Fail-open on panic.
func (s *Shield) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Mode == ModeOff {
			next.ServeHTTP(w, r)
			return
		}
		sw := newStatusWriter(w)
		defer func() {
			if rec := recover(); rec != nil {
				if n := s.lastPanic.Load(); time.Now().Unix()-n > 60 {
					s.lastPanic.Store(time.Now().Unix())
					s.log.Warnf("shield: panic %v (failing open)", rec)
				}
				if sw.status == 0 {
					next.ServeHTTP(w, r) // retry; if it panics again it propagates to server recover
				}
			}
		}()
		s.serve(sw, r, next)
	})
}

func (s *Shield) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	res := Result{Action: ActionPass}
	var h *resultHolder
	install := func() {
		if h == nil {
			h = &resultHolder{}
			*r = *r.WithContext(context.WithValue(r.Context(), ctxKey{}, h))
		}
	}
	defer func() {
		if h != nil {
			h.r = res
		}
		if res.Action != ActionPass {
			if cd := proxy.CapturedDataFromContext(r.Context()); cd != nil {
				cd.SetMetadata("shield.action", string(res.Action))
				cd.SetMetadata("shield.reasons", strings.Join(res.Reasons, ","))
			}
		}
	}()

	ip := s.clientIP(r)
	if !ip.IsValid() {
		next.ServeHTTP(w, r)
		return
	}
	host := hostOnly(r.Host)
	if s.cfg.allowIP(ip) || s.cfg.hosts[host] {
		next.ServeHTTP(w, r)
		return
	}
	sh, st := s.store.get(ip)
	now := s.store.clock()

	// single lock: banned check, rate bucket, window tracking, decay, score
	sh.mu.Lock()
	st.lastSeen = now
	if s.store.isBanned(st) {
		sh.mu.Unlock()
		res.Action = ActionBlock
		res.Reasons = append(res.Reasons, "banned")
		install()
		s.emit(r, res)
		s.block(w, r)
		return
	}
	sh.mu.Unlock()
	if s.hasValidPass(r, ip) {
		next.ServeHTTP(w, r)
		return
	}
	sh.mu.Lock()
	if st.limiter == nil {
		st.limiter = &rateBucket{tokens: float64(s.cfg.RateBurst), last: now}
	}
	overRate := false
	el := now.Sub(st.limiter.last).Seconds()
	st.limiter.tokens += el * s.cfg.RatePerSecond
	if st.limiter.tokens > float64(s.cfg.RateBurst) {
		st.limiter.tokens = float64(s.cfg.RateBurst)
	}
	st.limiter.last = now
	if st.limiter.tokens < 1 {
		overRate = true
	} else {
		st.limiter.tokens--
	}
	p := r.URL.Path
	st.paths[p] = now
	enum := false
	if len(st.paths) > s.cfg.EnumThreshold {
		enum = true
		st.paths = map[string]time.Time{p: now}
	} else if st.enumPrune.Before(now.Add(-time.Minute)) {
		for k, t := range st.paths {
			if t.Before(now.Add(-s.cfg.EnumWindow * 2)) {
				delete(st.paths, k)
			}
		}
		st.enumPrune = now
	}
	known := s.isKnownHost(host)
	ua := r.UserAgent()
	class, browser, _, ai := s.classifyUA(ua)

	sigN := 0
	var sigBuf [8]signal
	addSig := func(wt float64, reason string) {
		if sigN < len(sigBuf) {
			sigBuf[sigN] = signal{wt, reason}
			sigN++
		}
	}
	if !known || isIPLiteralHost(r.Host) {
		addSig(s.cfg.WeightUnknownHost, "unknown-host")
	}
	switch class {
	case "scanner":
		addSig(s.cfg.WeightScannerUA, "scanner-ua")
	case "research-scanner":
		addSig(s.cfg.WeightResearchUA, "research-ua")
	case "empty":
		addSig(s.cfg.WeightEmptyUA, "empty-ua")
	case "fake-ua":
		addSig(s.cfg.WeightFakeBrowserUA, "fake-ua")
	case "search":
		if !s.cfg.AllowSearchEngines {
			addSig(0, "search-blocked")
		} else if s.verifier != nil && !s.verifier.Verify(r.Context(), ip, ua) {
			addSig(s.cfg.WeightFakeSearchBot, "fake-search-bot")
		}
	}
	if ai && s.cfg.AICrawlers == AIChallenge {
		addSig(s.cfg.ChallengeAt, "ai-crawler")
	}
	if browser && browserLikeMissingHeaders(r) {
		addSig(s.cfg.WeightFakeBrowserUA/4, "browser-inconsistent")
	}
	if r.Method == http.MethodTrace || r.Method == http.MethodConnect ||
		r.ProtoMajor < 1 || len(r.Header) > 64 ||
		reJNDI.MatchString(r.URL.RawPath+r.URL.RawQuery+ua) ||
		reShellMeta.MatchString(r.URL.RawQuery) {
		addSig(s.cfg.WeightOddRequest, "odd-request")
	}
	if overRate {
		addSig(s.cfg.WeightRate, "rate-limit")
	}
	if enum {
		addSig(s.cfg.WeightEnumeration, "enumeration")
	}
	s.store.decay(st)
	for k := 0; k < sigN; k++ {
		if sg := sigBuf[k]; sg.weight > 0 {
			st.score += sg.weight
			res.Reasons = append(res.Reasons, sg.reason)
		}
	}
	score := st.score
	banNow := score >= s.cfg.BanAt
	if banNow && s.cfg.Mode == ModeEnforce {
		s.store.ban(st)
	}
	sh.mu.Unlock()
	res.Score = score

	// hard AI deny does not depend on score
	if ai && s.cfg.AICrawlers == AIDeny {
		res.Action = ActionBlock
		res.Reasons = append(res.Reasons, "ai-crawler")
		install()
		s.emit(r, res)
		if s.cfg.Mode == ModeEnforce {
			s.block(w, r)
			return
		}
	}
	if overRate {
		res.Action = ActionBlock
		res.Reasons = append(res.Reasons, "rate-limit")
		install()
		s.emit(r, res)
		if s.cfg.Mode == ModeEnforce {
			w.Header().Set("Retry-After", "5")
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
	}
	if banNow {
		res.Action = ActionBlock
		install()
		s.emit(r, res)
		if s.cfg.Mode == ModeEnforce {
			s.block(w, r)
			return
		}
	}
	if score >= s.cfg.TarpitAt {
		res.Action = ActionTarpit
		install()
		s.emit(r, res)
		if s.cfg.Mode == ModeEnforce {
			if !s.tarpit(w, r) {
				s.block(w, r)
			}
			return
		}
	}
	if score >= s.cfg.ChallengeAt && s.canChallenge(r, browser) {
		res.Action = ActionChallenge
		install()
		s.emit(r, res)
		if s.cfg.Mode == ModeEnforce {
			if s.serveChallenge(w, r, ip) {
				return
			}
		}
	}
	if len(res.Reasons) > 0 || res.Action != ActionPass {
		install()
	}

	// pass through, observe status for post-response signals
	sw, _ := w.(*statusWriter)
	if sw == nil {
		sw = newStatusWriter(w)
	}
	next.ServeHTTP(sw, r)
	code := sw.status
	if code == 0 {
		code = 200
	}

	// post-response signals
	var post [2]signal
	postN := 0
	sh.mu.Lock()
	if code == 404 || code == 403 || code == 400 || code == 405 {
		st.probes = append(st.probes, now)
		pcut := now.Add(-s.cfg.ProbeWindow)
		pk := st.probes[:0]
		for _, t := range st.probes {
			if t.After(pcut) {
				pk = append(pk, t)
			}
		}
		st.probes = pk
		if len(st.probes) > s.cfg.ProbeThreshold*4 {
			st.probes = st.probes[len(st.probes)-s.cfg.ProbeThreshold*2:]
		}
		if len(st.probes) >= s.cfg.ProbeThreshold && postN < len(post) {
			post[postN] = signal{s.cfg.WeightProbeRate, "probe-rate"}
			postN++
		}
	}
	if isHoneypotPath(r.URL.Path, s.cfg.ExtraHoneypots, s.cfg.RemoveHoneypots) &&
		(!known || code == 404 || code == 403 || code == 400 || code == 405) &&
		postN < len(post) {
		post[postN] = signal{s.cfg.WeightHoneypot, "honeypot"}
		postN++
	}
	if postN > 0 {
		s.store.decay(st)
		for k := 0; k < postN; k++ {
			st.score += post[k].weight
			res.Reasons = append(res.Reasons, post[k].reason)
		}
		res.Score = st.score
		banReason := post[postN-1].reason
		var ttl time.Duration
		if st.score >= s.cfg.BanAt && s.cfg.Mode == ModeEnforce {
			ttl = s.store.ban(st)
		}
		sh.mu.Unlock()
		install()
		if ttl > 0 {
			s.log.Infof("shield: banned for %s (%s)", ttl, banReason)
		}
	} else {
		sh.mu.Unlock()
	}
	s.emit(r, res)
}

// hostOnly strips the port without allocations.
func hostOnly(host string) string {
	if i := strings.LastIndexByte(host, ':'); i > 0 {
		return host[:i]
	}
	if host != "" && host[0] == '[' {
		if i := strings.IndexByte(host, ']'); i > 0 {
			return host[:i+1]
		}
	}
	return host
}

type signal struct {
	weight float64
	reason string
}

// classifyUA caches classification per UA string; the map resets past 4096
// distinct UAs to stay bounded.
func (s *Shield) classifyUA(ua string) (string, bool, bool, bool) {
	type cls struct {
		class   string
		browser bool
		search  bool
		ai      bool
	}
	if v, ok := s.uaCache.Load(ua); ok {
		c := v.(cls)
		return c.class, c.browser, c.search, c.ai
	}
	class, browser, search, ai := classifyUA(ua)
	if s.uaCacheLen.Add(1) > 4096 {
		s.uaCache.Range(func(k, _ any) bool { s.uaCache.Delete(k); return true })
		s.uaCacheLen.Store(0)
	}
	s.uaCache.Store(ua, cls{class, browser, search, ai})
	return class, browser, search, ai
}

func (s *Shield) clientIP(r *http.Request) netip.Addr {
	if s.ipResolver != nil {
		return s.ipResolver(r)
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		h = r.RemoteAddr
	}
	ip, _ := netip.ParseAddr(h)
	return ip
}

func (s *Shield) isKnownHost(host string) bool {
	h := strings.SplitN(host, ":", 2)[0]
	if s.knownHost != nil {
		return s.knownHost(h)
	}
	return true // default: treat as known (no host map wired yet)
}

func (s *Shield) canChallenge(r *http.Request, browserUA bool) bool {
	if r.Method != http.MethodGet {
		return false
	}
	if r.Header.Get("Authorization") != "" {
		return false
	}
	acc := r.Header.Get("Accept")
	if !strings.Contains(acc, "text/html") {
		return false
	}
	if m := r.Header.Get("Sec-Fetch-Mode"); m != "" && m != "navigate" {
		return false
	}
	return browserUA
}

func (s *Shield) tarpit(w http.ResponseWriter, r *http.Request) bool {
	n := s.store.activeTarpits.Add(1)
	defer s.store.activeTarpits.Add(-1)
	if n > int32(s.cfg.MaxTarpits) {
		return false
	}
	t := time.NewTimer(s.cfg.TarpitDelay)
	defer t.Stop()
	select {
	case <-r.Context().Done():
		return true
	case <-t.C:
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("forbidden\n"))
	return true
}

func (s *Shield) block(w http.ResponseWriter, r *http.Request) {
	if s.cfg.DropOnHoneypot && r.ProtoMajor == 1 {
		if hj, ok := w.(http.Hijacker); ok {
			c, _, err := hj.Hijack()
			if err == nil {
				_ = c.Close()
				return
			}
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte("forbidden\n"))
}

func (s *Shield) emit(req *http.Request, res Result) {
	if s.metrics != nil {
		for _, reason := range res.Reasons {
			s.metrics.Decision(string(res.Action), reason)
		}
		if len(res.Reasons) == 0 {
			s.metrics.Decision(string(res.Action), "")
		}
	}
	if s.decisionLog != nil && res.Action != ActionPass {
		s.decisionLog.write(req, res, s.clientIP(req))
	}
}

// DebugReporter adapts Shield for the debug endpoint's JSON surface.
func (s *Shield) StatsJSON() ([]byte, error) {
	st := s.Stats()
	return json.Marshal(map[string]any{
		"tracked": st.Tracked, "banned": st.Banned, "tarpits": st.ActiveTarpits,
	})
}

func (s *Shield) BansJSON() ([]byte, error) {
	return json.Marshal(s.Bans())
}

// UnbanIP unwraps a string IP for the debug endpoint.
func (s *Shield) UnbanIP(ip string) bool {
	a, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	return s.Unban(a)
}
