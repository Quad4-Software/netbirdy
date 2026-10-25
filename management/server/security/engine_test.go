package security

import (
	"context"
	"encoding/json"
	"net/netip"
	"os"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

type fakeGeo struct {
	m map[string]struct {
		country string
		asn     uint32
	}
}

func (g *fakeGeo) Lookup(ip netip.Addr) (string, uint32) {
	if g.m == nil {
		return "", 0
	}
	v, ok := g.m[ip.String()]
	if !ok {
		return "", 0
	}
	return v.country, v.asn
}

type recSink struct {
	mu  sync.Mutex
	evt []Event
}

func (s *recSink) Emit(e Event) { s.mu.Lock(); s.evt = append(s.evt, e); s.mu.Unlock() }
func (s *recSink) all() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Event(nil), s.evt...)
}
func (s *recSink) countRule(r string) int {
	n := 0
	for _, e := range s.all() {
		if e.Rule == r {
			n++
		}
	}
	return n
}

type fakeActions struct {
	mu        sync.Mutex
	approvals []string
	expires   []string
	blocks    []string
}

func (a *fakeActions) RequireApproval(ctx context.Context, acct, peer string) error {
	a.mu.Lock()
	a.approvals = append(a.approvals, acct+"/"+peer)
	a.mu.Unlock()
	return nil
}
func (a *fakeActions) ExpireLogin(ctx context.Context, acct, peer string) error {
	a.mu.Lock()
	a.expires = append(a.expires, acct+"/"+peer)
	a.mu.Unlock()
	return nil
}
func (a *fakeActions) BlockPeer(ctx context.Context, acct, peer string) error {
	a.mu.Lock()
	a.blocks = append(a.blocks, acct+"/"+peer)
	a.mu.Unlock()
	return nil
}
func (a *fakeActions) approvalCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.approvals)
}

func ip(s string) netip.Addr { return netip.MustParseAddr(s) }

func newEngine(t *testing.T, cfg *Config, geo GeoResolver) (*Engine, *recSink, *fakeActions) {
	t.Helper()
	if cfg == nil {
		cfg = DefaultConfig()
	}
	sink := &recSink{}
	acts := &fakeActions{}
	e := NewEngine(cfg, &fakeClock{t: time.Now()}, geo, sink, acts)
	t.Cleanup(e.Close)
	return e, sink, acts
}

func obs(account, peer, srcIP string) PeerObservation {
	return PeerObservation{AccountID: account, PeerID: peer, PeerKey: "k" + peer,
		PeerName: peer, SrcIP: ip(srcIP), Meta: PeerMeta{Hostname: "h1", OS: "linux", Kernel: "6.x", Serial: "s1"}}
}

func TestPeerNewIP(t *testing.T) {
	e, sink, _ := newEngine(t, nil, &fakeGeo{m: map[string]struct {
		country string
		asn     uint32
	}{"1.2.3.4": {"DE", 100}, "9.9.9.9": {"US", 200}}})

	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "1.2.3.4"))
	// same IP again: no event
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "1.2.3.4"))
	if sink.countRule("peer_new_ip") != 0 {
		t.Fatal("baseline IP triggered")
	}
	// new country+ASN IP
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "9.9.9.9"))
	evs := sink.all()
	if len(evs) == 0 || evs[len(evs)-1].Rule != "peer_new_ip" {
		t.Fatalf("expected peer_new_ip event, got %v", evs)
	}
	last := evs[len(evs)-1]
	if last.Score < 60 {
		t.Fatalf("expected medium+ score for country+asn change, got %d", last.Score)
	}
}

func TestPeerIPFlap(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	o := obs("a1", "p1", "1.1.1.1")
	for i := 0; i < 6; i++ {
		o.SrcIP = ip("1.1.1." + string(rune('0'+i)))
		e.ObservePeerSync(context.Background(), o)
	}
	if sink.countRule("peer_ip_flap") == 0 {
		t.Fatal("flap not detected")
	}
}

func TestMetaMismatch(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "1.1.1.1"))
	o := obs("a1", "p1", "1.1.1.1")
	o.Meta.Serial = "different"
	e.ObservePeerSync(context.Background(), o)
	if sink.countRule("peer_meta_mismatch") == 0 {
		t.Fatal("serial mismatch missed")
	}
}

func TestConcurrentConnections(t *testing.T) {
	e, sink, acts := newEngine(t, nil, nil)
	clock := e.clock.(*fakeClock)
	ctx := context.Background()
	e.ObserveStreamOpen(ctx, "key1", "1.1.1.1", "a1", "p1", "n1")
	clock.advance(90 * time.Second) // past grace period
	e.ObserveStreamOpen(ctx, "key1", "9.9.9.9", "a1", "p1", "n1")
	if sink.countRule("concurrent_connections") == 0 {
		t.Fatal("concurrent streams missed")
	}
	// actions called only in prevent mode; observe: none
	if acts.approvalCount() != 0 {
		t.Fatal("observe mode must not act")
	}
	// close the older stream; the newer alone is fine
	e.ObserveStreamClose("key1", "1.1.1.1")
	clock.advance(90 * time.Second)
	e.ObserveStreamOpen(ctx, "key1", "5.5.5.5", "a1", "p1", "n1")
	// 9.9.9.9 is still live: the new stream triggers again, cooldown may
	// suppress a duplicate; just ensure no crash
	e.ObserveStreamClose("key1", "9.9.9.9")
	e.ObserveStreamClose("key1", "5.5.5.5")
}

func TestConcurrentPrevention(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModePrevent
	cfg.LearningPeriod = time.Hour
	e, sink, acts := newEngine(t, cfg, nil)
	clock := e.clock.(*fakeClock)
	ctx := context.Background()
	e.ObserveStreamOpen(ctx, "key1", "1.1.1.1", "a1", "p1", "n1")
	clock.advance(90 * time.Second)
	e.ObserveStreamOpen(ctx, "key1", "9.9.9.9", "a1", "p1", "n1")
	// still inside learning period: no prevention, event emitted
	if acts.approvalCount() != 0 {
		t.Fatal("prevention during learning")
	}
	// leave learning period
	clock.advance(cfg.LearningPeriod)
	e.ObserveStreamOpen(ctx, "key2", "2.2.2.2", "a1", "p2", "n2")
	clock.advance(90 * time.Second)
	e.ObserveStreamOpen(ctx, "key2", "8.8.8.8", "a1", "p2", "n2")
	time.Sleep(50 * time.Millisecond) // let the action worker run
	if acts.approvalCount() == 0 {
		t.Fatal("prevent mode did not call RequireApproval")
	}
	evs := sink.all()
	var prevented *Event
	for i := range evs {
		if evs[i].Rule == "concurrent_connections" && evs[i].ActionTaken == ActionRequireApproval {
			prevented = &evs[i]
		}
	}
	if prevented == nil {
		t.Fatal("no event marked with the prevention action")
	}
}

func TestAuthFailureBurst(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	for i := 0; i < 25; i++ {
		e.ObserveAuthFailure(context.Background(), AuthFailure{Kind: AuthKindAPI, SourceIP: ip("6.6.6.6"), AccountID: "a1"})
	}
	if sink.countRule("auth_failure_burst") == 0 {
		t.Fatal("burst missed")
	}
}

func TestAuthSuccessAfterFailures(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	for i := 0; i < 6; i++ {
		e.ObserveAuthFailure(context.Background(), AuthFailure{Kind: AuthKindDashboard, SourceIP: ip("7.7.7.7"), Subject: "u1", AccountID: "a1"})
	}
	e.ObserveAuthSuccess(context.Background(), AuthSuccess{Kind: AuthKindDashboard, SourceIP: ip("7.7.7.7"), Subject: "u1", AccountID: "a1"})
	if sink.countRule("auth_success_after_failures") == 0 {
		t.Fatal("success-after-failures missed")
	}
}

func TestSetupKeyMisuse(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	for i := 0; i < 6; i++ {
		e.ObserveSetupKeyUse(context.Background(), SetupKeyObservation{KeyID: "k1", AccountID: "a1",
			SourceIP: ip("10.0.0." + string(rune('0'+i)))})
	}
	if sink.countRule("setup_key_misuse") == 0 {
		t.Fatal("setup key multi-IP misuse missed")
	}
}

func TestUserNewCountry(t *testing.T) {
	e, sink, _ := newEngine(t, nil, &fakeGeo{m: map[string]struct {
		country string
		asn     uint32
	}{"1.1.1.1": {"DE", 1}, "2.2.2.2": {"US", 2}}})
	e.ObserveUserLogin(context.Background(), "a1", "u1", ip("1.1.1.1"))
	if sink.countRule("user_new_country") != 0 {
		t.Fatal("first country should seed silently")
	}
	e.ObserveUserLogin(context.Background(), "a1", "u1", ip("2.2.2.2"))
	if sink.countRule("user_new_country") == 0 {
		t.Fatal("new country missed")
	}
}

func TestPATNewIP(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	e.ObservePATUse(context.Background(), "a1", "tok1", ip("3.3.3.3"))
	e.ObservePATUse(context.Background(), "a1", "tok1", ip("4.4.4.4"))
	if sink.countRule("pat_new_ip") == 0 {
		t.Fatal("PAT new IP missed")
	}
}

func TestLearningPeriodGating(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LearningPeriod = time.Hour
	e, sink, _ := newEngine(t, cfg, nil)
	// learning: events emitted but capped at info
	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "1.1.1.1"))
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "9.9.9.9"))
	for _, ev := range sink.all() {
		if ev.Severity != SeverityInfo {
			t.Fatalf("learning emitted non-info severity %s on %s", ev.Severity, ev.Rule)
		}
		if ev.ActionTaken != ActionAlert {
			t.Fatalf("action during learning: %s", ev.ActionTaken)
		}
	}
}

func TestCooldown(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "1.1.1.1"))
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "9.9.9.9"))
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "8.8.8.8"))
	if n := sink.countRule("peer_new_ip"); n > 1 {
		t.Fatalf("cooldown failed, %d events", n)
	}
}

func TestTrustedCIDRs(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TrustedCIDRs = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	e, sink, _ := newEngine(t, cfg, nil)
	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "10.1.2.3"))
	e.ObservePeerSync(context.Background(), obs("a1", "p1", "10.9.9.9"))
	if sink.countRule("peer_new_ip") != 0 {
		t.Fatal("trusted CIDR triggered")
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	e, _, _ := newEngine(t, nil, &fakeGeo{m: map[string]struct {
		country string
		asn     uint32
	}{"1.2.3.4": {"DE", 100}}})
	e.ObservePeerLogin(context.Background(), obs("a1", "p1", "1.2.3.4"))
	bs, err := e.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	e2, _, _ := newEngine(t, nil, &fakeGeo{m: map[string]struct {
		country string
		asn     uint32
	}{"1.2.3.4": {"DE", 100}, "5.5.5.5": {"US", 5}}})
	if err := e2.Restore(bs); err != nil {
		t.Fatal(err)
	}
	// restored baseline knows the IP -> no peer_new_ip event
	s2 := e2.store.get("a1", "p1")
	if s2 == nil || !s2.knownIP(ip("1.2.3.4")) {
		t.Fatal("baseline not restored")
	}
}

func TestSnapshotCorruption(t *testing.T) {
	e, _, _ := newEngine(t, nil, nil)
	if err := e.Restore([]byte("{bad json")); err == nil {
		t.Fatal("corrupt snapshot accepted")
	}
	if err := e.Restore([]byte(`{"version":99,"peers":{}}`)); err == nil {
		t.Fatal("bad version accepted")
	}
}

func TestFileSink(t *testing.T) {
	dir := t.TempDir()
	fs, err := NewFileSink(dir+"/sec.jsonl", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	ev := Event{Timestamp: time.Now(), Type: "peer", Severity: SeverityHigh,
		AccountID: "a1", PeerID: "p1", SrcIP: "1.2.3.4", Rule: "peer_new_ip",
		Score: 75, ActionTaken: ActionAlert, Mode: ModeObserve, Country: "DE", ASN: 100}
	fs.Emit(ev)
	var got Event
	bs, _ := os.ReadFile(dir + "/sec.jsonl")
	if err := json.Unmarshal(bs[:len(bs)-1], &got); err != nil {
		t.Fatalf("jsonl line: %v", err)
	}
	if got.Rule != "peer_new_ip" || got.Score != 75 || got.Mode != ModeObserve {
		t.Fatalf("schema mismatch: %+v", got)
	}
}

func TestMultiSinkPanicSafe(t *testing.T) {
	fs := MultiSink{&panicSink{}, &recSink{}}
	fs.Emit(Event{Rule: "x"})
}

type panicSink struct{}

func (panicSink) Emit(Event) { panic("boom") }

func TestPanicSafety(t *testing.T) {
	e, _, _ := newEngine(t, nil, nil)
	// poison: nil geo still ok; force panic path via nil baseline map? Instead
	// exercise observe wrapper with a nil-ish observation.
	o := obs("a1", "p1", "1.1.1.1")
	o.Meta = PeerMeta{}
	e.ObservePeerSync(context.Background(), o) // must not crash
}

func TestConcurrencyHammer(t *testing.T) {
	e, sink, _ := newEngine(t, nil, nil)
	ctx := context.Background()
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				e.ObservePeerSync(ctx, obs("a1", "p"+string(rune('0'+w)), "1.2.3.4"))
				e.ObserveAuthFailure(ctx, AuthFailure{Kind: AuthKindAPI, SourceIP: ip("6.6.6.6")})
				e.ObserveStreamOpen(ctx, "k", "1.1.1.1", "a1", "p", "n")
				e.ObserveStreamClose("k", "1.1.1.1")
			}
		}(w)
	}
	wg.Wait()
	_ = sink.all()
}

func BenchmarkObservePeerSync(b *testing.B) {
	sink := &recSink{}
	e := NewEngine(DefaultConfig(), SystemClock{}, nil, sink, nil)
	defer e.Close()
	o := obs("a1", "p1", "1.2.3.4")
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.ObservePeerSync(ctx, o)
	}
}
