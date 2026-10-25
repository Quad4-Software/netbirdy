package server

import (
	"context"
	"sync"
	"testing"

	"github.com/netbirdio/netbird/management/server/activity"
	"github.com/netbirdio/netbird/management/server/security"
)

func TestActivitySinkAdapter(t *testing.T) {
	var mu sync.Mutex
	var got []string
	ad := activitySinkAdapter{storeEvent: func(ctx context.Context, initiator, target, accountID string, act activity.Activity, meta map[string]any) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, act.StringCode()+":"+initiator+":"+target)
	}}
	ad.Emit(security.Event{Rule: "peer_new_ip", AccountID: "a1", PeerID: "p1", SrcIP: "1.2.3.4"})
	if len(got) != 1 || got[0] != "peer.ip.new:system/security:p1" {
		t.Fatalf("unexpected: %v", got)
	}
	// unknown rule dropped silently
	ad.Emit(security.Event{Rule: "bogus", AccountID: "a1"})
	if len(got) != 1 {
		t.Fatal("bogus rule stored")
	}
}
