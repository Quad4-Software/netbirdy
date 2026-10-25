package internal

import (
	"context"
	"net/url"
	"time"
	"testing"

	nbdns "github.com/netbirdio/netbird/dns"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
	)

func u(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestProtectedOfflineHosts(t *testing.T) {
	cfg := &mgmProto.NetbirdConfig{
		Signal: &mgmProto.HostConfig{Uri: "grpcs://signal.example.com:10000"},
		Relay: &mgmProto.RelayConfig{
			Urls: []string{"rels://relay.example.com:80", "https://10.0.0.9:443"},
		},
	}
	got := protectedOfflineHosts(cfg, u(t, "http://mgmt.example.com:18081"))
	for _, want := range []string{"mgmt.example.com", "signal.example.com", "relay.example.com"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("missing protected host %s", want)
		}
	}
	if len(got) != 3 {
		t.Fatalf("unexpected extra hosts: %v", got)
	}
}

func TestFilterOfflineNSGroups(t *testing.T) {
	protected := map[string]struct{}{"mgmt.example.com": {}}
	groups := []*nbdns.NameServerGroup{
		{Domains: []string{"example.com"}},              // covers mgmt.example.com -> dropped
		{Domains: []string{"corp.example.com"}},          // unrelated -> kept
		{Domains: []string{"."}},                          // match-all -> dropped
		{Domains: []string{"mgmt.example.com"}},           // exact -> dropped
	}
	out := filterOfflineNSGroups(groups, protected)
	if len(out) != 1 || len(out[0].Domains) == 0 || out[0].Domains[0] != "corp.example.com" {
		t.Fatalf("expected only the unrelated group, got %v", out)
	}
}

func TestResolveMgmtDialTarget(t *testing.T) {
	cached := []string{"10.0.0.1", "bad-addr", "192.168.0.2"}

	// lookup works -> hostname kept
	lookupMgmt = func(ctx context.Context, host string) ([]string, error) { return []string{"1.2.3.4"}, nil }
	got := resolveMgmtDialTarget(context.Background(), "mgmt.example.com:443", cached)
	if got != "mgmt.example.com:443" {
		t.Fatalf("expected hostname passthrough, got %s", got)
	}

	// lookup fails -> first valid cached addr + port preserved
	lookupMgmt = func(ctx context.Context, host string) ([]string, error) { return nil, context.DeadlineExceeded }
	got = resolveMgmtDialTarget(context.Background(), "mgmt.example.com:443", cached)
	if got != "10.0.0.1:443" {
		t.Fatalf("expected cached addr 10.0.0.1:443, got %s", got)
	}

	// literal IP -> passthrough without touching lookup
	got = resolveMgmtDialTarget(context.Background(), "10.0.0.5:443", cached)
	if got != "10.0.0.5:443" {
		t.Fatalf("expected IP passthrough, got %s", got)
	}
}

func TestCaptureMgmtAddrs(t *testing.T) {
	// lookup success: addrs land in memory and in the envelope
	lookupMgmt = func(ctx context.Context, host string) ([]string, error) { return []string{"10.1.2.3"}, nil }
	dir := t.TempDir()
	snap := dir + "/networkmap.bin"
	captureMgmtAddrs(context.Background(), "mgmt.example.com", snap)
	got := LastMgmtAddrs()
	if len(got) != 1 || got[0] != "10.1.2.3" {
		t.Fatalf("expected [10.1.2.3], got %v", got)
	}
	// a later save must carry the captured addrs (merge-forward)
	resp := &mgmProto.SyncResponse{}
	if err := saveOfflineSnapshot(snap, resp, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadOfflineSnapshot(snap, time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := LastMgmtAddrs(); len(got) != 1 || got[0] != "10.1.2.3" {
		t.Fatalf("expected addrs after load, got %v", got)
	}
	// a save without capture keeps them
	lastMgmtAddrs = nil
	if err := saveOfflineSnapshot(snap, resp, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadOfflineSnapshot(snap, time.Hour, time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := LastMgmtAddrs(); len(got) != 1 || got[0] != "10.1.2.3" {
		t.Fatalf("expected addrs preserved, got %v", got)
	}
}
