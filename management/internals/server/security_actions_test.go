package server

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netbirdio/netbird/management/server/activity"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"github.com/netbirdio/netbird/management/server/security"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/management/server/types"
)

type fakeStore struct {
	store.Store
	mu     sync.Mutex
	status map[string]*nbpeer.PeerStatus
	saved  int
}

func (f *fakeStore) ExecuteInTransaction(ctx context.Context, op func(s store.Store) error) error {
	return op(f)
}
func (f *fakeStore) SavePeerStatus(ctx context.Context, accountID, peerID string, status nbpeer.PeerStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status == nil {
		f.status = make(map[string]*nbpeer.PeerStatus)
	}
	cp := status
	f.status[accountID+"/"+peerID] = &cp
	f.saved++
	return nil
}

type fakeBackend struct {
	mu        sync.Mutex
	peer      *nbpeer.Peer
	settings  *types.Settings
	user      *types.User
	store     *fakeStore
	events    []map[string]any
	broadcast int
}

func (b *fakeBackend) GetPeer(ctx context.Context, accountID, peerID, userID string) (*nbpeer.Peer, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// reflect status saved in the fake store
	if st := b.store.status[accountID+"/"+peerID]; st != nil {
		p := *b.peer
		p.Status = st
		return &p, nil
	}
	return b.peer, nil
}
func (b *fakeBackend) GetAccountSettings(ctx context.Context, accountID, userID string) (*types.Settings, error) {
	return b.settings, nil
}
func (b *fakeBackend) GetUserByID(ctx context.Context, id string) (*types.User, error) {
	return b.user, nil
}
func (b *fakeBackend) GetStore() store.Store { return b.store }
func (b *fakeBackend) StoreEvent(ctx context.Context, initiatorID, targetID, accountID string, activityID activity.ActivityDescriber, meta map[string]any) {
	b.mu.Lock()
	defer b.mu.Unlock()
	meta["initiator"] = initiatorID
	b.events = append(b.events, meta)
}
func (b *fakeBackend) UpdateAccountPeers(ctx context.Context, accountID string, reason types.UpdateReason) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.broadcast++
}
func (b *fakeBackend) broadcastCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.broadcast
}
func (b *fakeBackend) eventCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.events)
}
func (b *fakeBackend) statusFor(accountID, peerID string) *nbpeer.PeerStatus {
	b.store.mu.Lock()
	defer b.store.mu.Unlock()
	return b.store.status[accountID+"/"+peerID]
}

func newFakeBackend(t *testing.T, approvalEnabled, ownerUser bool) *fakeBackend {
	t.Helper()
	st := &nbpeer.PeerStatus{}
	b := &fakeBackend{
		peer:     &nbpeer.Peer{ID: "p1", AccountID: "a1", UserID: "u1", Status: &nbpeer.PeerStatus{}},
		settings: &types.Settings{Extra: &types.ExtraSettings{PeerApprovalEnabled: approvalEnabled}},
		user:     &types.User{Id: "u1", Role: types.UserRoleUser},
		store:    &fakeStore{status: map[string]*nbpeer.PeerStatus{"a1/p1": st}},
	}
	if ownerUser {
		b.user = &types.User{Id: "u1", Role: types.UserRoleOwner}
	}
	return b
}

func TestRequireApprovalSetsStatus(t *testing.T) {
	b := newFakeBackend(t, true, false)
	a := managementActions{am: b}
	if err := a.RequireApproval(context.Background(), "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	st := b.store.status["a1/p1"]
	if st == nil || !st.RequiresApproval {
		t.Fatalf("RequiresApproval not set: %+v", st)
	}
	if len(b.events) != 1 || b.events[0]["security_action"] != "require_approval" {
		t.Fatalf("events: %v", b.events)
	}
	time.Sleep(50 * time.Millisecond) // broadcast is async
	if b.broadcastCount() == 0 {
		t.Fatal("no peer map broadcast")
	}
}

func TestRequireApprovalFallbackWhenDisabled(t *testing.T) {
	b := newFakeBackend(t, false, false)
	a := managementActions{am: b}
	if err := a.RequireApproval(context.Background(), "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	st := b.store.status["a1/p1"]
	if st == nil || !st.LoginExpired || st.RequiresApproval {
		t.Fatalf("expected login-expiry fallback: %+v", st)
	}
	if b.events[0]["security_action"] != "expire_login" {
		t.Fatalf("fallback not recorded: %v", b.events)
	}
}

func TestExpireLoginAndBlockAlias(t *testing.T) {
	b := newFakeBackend(t, true, false)
	a := managementActions{am: b}
	if err := a.ExpireLogin(context.Background(), "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	if !b.store.status["a1/p1"].LoginExpired {
		t.Fatal("LoginExpired not set")
	}
	// idempotent: second call no extra event
	n := len(b.events)
	if err := a.ExpireLogin(context.Background(), "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	if len(b.events) != n {
		t.Fatal("second expire emitted an event")
	}
	// BlockPeer is the documented alias for expire login.
	b2 := newFakeBackend(t, true, false)
	a2 := managementActions{am: b2}
	if err := a2.BlockPeer(context.Background(), "a1", "p1"); err != nil {
		t.Fatal(err)
	}
	if !b2.store.status["a1/p1"].LoginExpired {
		t.Fatal("BlockPeer did not expire login")
	}
}

func TestOwnerGuard(t *testing.T) {
	b := newFakeBackend(t, true, true)
	a := managementActions{am: b}
	if err := a.ExpireLogin(context.Background(), "a1", "p1"); err == nil {
		t.Fatal("owner peer expired login should be refused")
	}
	if err := a.RequireApproval(context.Background(), "a1", "p1"); err != nil {
		t.Fatalf("require_approval on owner peer should be allowed: %v", err)
	}
}

func TestEngineAdapterConcurrentPrevent(t *testing.T) {
	// after learning, concurrent_connections triggers require_approval once
	// per cooldown; during learning and in observe mode nothing acts
	for _, mode := range []security.Mode{security.ModePrevent, security.ModeObserve} {
		b := newFakeBackend(t, true, false)
		clk := &testClock{t: time.Now()}
		cfg := security.DefaultConfig()
		cfg.Mode = mode
		cfg.LearningPeriod = time.Hour
		e := security.NewEngine(cfg, clk, nil, security.MultiSink{}, managementActions{am: b})
		defer e.Close()
		ctx := context.Background()
		// still learning: no action even in prevent mode
		e.ObserveStreamOpen(ctx, "k1", "1.1.1.1", "a1", "p1", "n")
		clk.t = clk.t.Add(90 * time.Second)
		e.ObserveStreamOpen(ctx, "k1", "2.2.2.2", "a1", "p1", "n")
		time.Sleep(50 * time.Millisecond)
		if b.broadcastCount() != 0 {
			t.Fatalf("mode %s: action during learning", mode)
		}
		clk.t = clk.t.Add(cfg.LearningPeriod)
		e.ObserveStreamOpen(ctx, "k2", "3.3.3.3", "a1", "p1", "n")
		clk.t = clk.t.Add(90 * time.Second)
		e.ObserveStreamOpen(ctx, "k2", "4.4.4.4", "a1", "p1", "n")
		time.Sleep(80 * time.Millisecond)
		// within cooldown a second stream pair should not fire again
		e.ObserveStreamOpen(ctx, "k3", "5.5.5.5", "a1", "p1", "n")
		clk.t = clk.t.Add(90 * time.Second)
		e.ObserveStreamOpen(ctx, "k3", "6.6.6.6", "a1", "p1", "n")
		time.Sleep(80 * time.Millisecond)
		if mode == security.ModePrevent {
			if st := b.statusFor("a1", "p1"); st == nil || !st.RequiresApproval {
				t.Fatal("prevent mode: no require_approval set")
			}
			if b.eventCount() != 1 {
				t.Fatalf("expected exactly one action event, got %d", b.eventCount())
			}
		} else {
			st := b.statusFor("a1", "p1")
			if b.eventCount() != 0 || (st != nil && st.RequiresApproval) {
				t.Fatal("observe mode acted")
			}
		}
	}
}

type testClock struct{ t time.Time }

func (c *testClock) Now() time.Time { return c.t }
