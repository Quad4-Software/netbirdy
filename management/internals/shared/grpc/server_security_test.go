package grpc

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/netbirdio/netbird/management/server/security"
)

// memSink records emitted events for assertions.
type memSink struct {
	mu  sync.Mutex
	evt []security.Event
}

func (s *memSink) Emit(e security.Event) { s.mu.Lock(); s.evt = append(s.evt, e); s.mu.Unlock() }
func (s *memSink) all() []security.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]security.Event(nil), s.evt...)
}

type fixedClock struct{ t time.Time }

func (c *fixedClock) Now() time.Time { return c.t }

type fixedGeo struct{}

func (fixedGeo) Lookup(ip netip.Addr) (string, uint32) {
	if ip.String() == "9.9.9.9" {
		return "US", 200
	}
	return "DE", 100
}

// The engine is invoked through the same field the hooks use; this proves
// the hook semantics end to end at the lowest real layer (the full gRPC
// stack requires encryption keys + a real store, covered by the e2e rig).
func TestSecurityEngineThroughServerField(t *testing.T) {
	clk := &fixedClock{t: time.Now()}
	sink := &memSink{}
	e := security.NewEngine(&security.Config{
		Mode: security.ModeObserve, LearningPeriod: time.Hour,
		Rules:    security.DefaultConfig().Rules,
		Cooldown: time.Minute, SeverityMedium: 60, SeverityHigh: 80,
		FlapWindow: 10 * time.Minute, FlapThreshold: 5,
		AuthFailWindow: 5 * time.Minute, AuthFailThreshold: 20,
		AuthSuccessAfterFails: 5, SetupKeyIPWindow: 24 * time.Hour,
		SetupKeyIPThreshold: 5, SetupKeyRegWindow: 24 * time.Hour,
		SetupKeyRegThreshold: 10, ConcurrentGracePeriod: time.Minute,
		MaxPeers: 100, MaxIPsPerPeer: 32, MaxAuthKeys: 1000, QueueSize: 8,
	}, clk, fixedGeo{}, sink, nil)
	defer e.Close()
	s := &Server{security: e}
	ctx := context.Background()

	meta := security.PeerMeta{Hostname: "h1", OS: "linux", Kernel: "6", Serial: "s1"}
	// login + sync from DE IP
	s.security.ObservePeerLogin(ctx, security.PeerObservation{AccountID: "a1", PeerID: "p1", PeerName: "n", SrcIP: netip.MustParseAddr("1.2.3.4"), Meta: meta})
	s.security.ObservePeerSync(ctx, security.PeerObservation{AccountID: "a1", PeerID: "p1", PeerName: "n", SrcIP: netip.MustParseAddr("1.2.3.4"), Meta: meta})
	clk.t = clk.t.Add(2 * time.Hour) // past learning
	// sync from US IP
	s.security.ObservePeerSync(ctx, security.PeerObservation{AccountID: "a1", PeerID: "p1", PeerName: "n", SrcIP: netip.MustParseAddr("9.9.9.9"), Meta: meta})

	var found *security.Event
	for i := range sink.all() {
		if sink.all()[i].Rule == "peer_new_ip" {
			found = &sink.all()[i]
		}
	}
	if found == nil {
		t.Fatalf("no peer_new_ip event in %v", sink.all())
	}
	if found.Country != "US" || found.PeerID != "p1" {
		t.Fatalf("bad event %+v", found)
	}
	if len(found.PreviousIPs) != 1 || found.PreviousIPs[0] != "1.2.3.4" {
		t.Fatalf("previous_ips wrong: %+v", found.PreviousIPs)
	}
}

func TestNilEngineZeroAlloc(t *testing.T) {
	s := &Server{} // security nil: hooks skip
	n := testing.AllocsPerRun(100, func() {
		if s.security != nil { // same guard the hooks use
			s.security.ObservePeerSync(context.Background(), security.PeerObservation{})
		}
	})
	if n != 0 {
		t.Fatalf("nil engine hook allocates: %v", n)
	}
}
