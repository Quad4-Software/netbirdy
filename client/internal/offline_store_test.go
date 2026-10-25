package internal

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
)

func sampleSyncResponse() *mgmProto.SyncResponse {
	return &mgmProto.SyncResponse{
		NetworkMap: &mgmProto.NetworkMap{
			Serial: 42,
			RemotePeers: []*mgmProto.RemotePeerConfig{
				{WgPubKey: "A1+/AAAA", AllowedIps: []string{"10.0.0.0/24"}},
			},
			PeerConfig: &mgmProto.PeerConfig{Address: "100.64.0.2"},
		},
		NetbirdConfig: &mgmProto.NetbirdConfig{
			Stuns: []*mgmProto.HostConfig{{Uri: "stun:stun.netbirdy.local:3478"}},
			Relay: &mgmProto.RelayConfig{
				Urls: []string{"rels://relay.netbirdy.local:80"},
			},
		},
	}
}

func TestOfflineSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	age := 24 * time.Hour

	t.Run("round trip", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		resp := sampleSyncResponse()
		if err := saveOfflineSnapshot(p, resp, nil, now); err != nil {
			t.Fatalf("save: %v", err)
		}
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if runtime.GOOS != "windows" { // unix perm bits do not exist on Windows
			if got := fi.Mode().Perm(); got != 0o600 {
				t.Fatalf("mode = %o, want 0600", got)
			}
		}
		got, savedAt, err := loadOfflineSnapshot(p, age, now.Add(time.Hour))
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if !savedAt.Equal(now) {
			t.Fatalf("savedAt = %v, want %v", savedAt, now)
		}
		if got.GetNetworkMap().GetSerial() != 42 {
			t.Fatalf("serial = %d", got.GetNetworkMap().GetSerial())
		}
		if got.GetNetbirdConfig().GetRelay().GetUrls()[0] != "rels://relay.netbirdy.local:80" {
			t.Fatalf("relay url mismatch")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, _, err := loadOfflineSnapshot(filepath.Join(t.TempDir(), "nope.bin"), age, now)
		if !errors.Is(err, errOfflineNoSnapshot) {
			t.Fatalf("err = %v, want errOfflineNoSnapshot", err)
		}
	})

	t.Run("expired", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := saveOfflineSnapshot(p, sampleSyncResponse(), nil, now); err != nil {
			t.Fatalf("save: %v", err)
		}
		_, _, err := loadOfflineSnapshot(p, age, now.Add(48*time.Hour))
		if !errors.Is(err, errOfflineExpired) {
			t.Fatalf("err = %v, want errOfflineExpired", err)
		}
	})

	t.Run("disabled", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := saveOfflineSnapshot(p, sampleSyncResponse(), nil, now); err != nil {
			t.Fatalf("save: %v", err)
		}
		_, _, err := loadOfflineSnapshot(p, 0, now)
		if !errors.Is(err, errOfflineDisabled) {
			t.Fatalf("err = %v, want errOfflineDisabled", err)
		}
	})

	t.Run("corrupt", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := os.WriteFile(p, []byte("not-json"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadOfflineSnapshot(p, age, now)
		if !errors.Is(err, errOfflineCorrupt) {
			t.Fatalf("err = %v, want errOfflineCorrupt", err)
		}
	})

	t.Run("version mismatch", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := saveOfflineSnapshot(p, sampleSyncResponse(), nil, now); err != nil {
			t.Fatalf("save: %v", err)
		}
		bs, _ := os.ReadFile(p)
		var env map[string]json.RawMessage
		if err := json.Unmarshal(bs, &env); err != nil {
			t.Fatal(err)
		}
		env["version"] = json.RawMessage("99")
		bs2, err2 := json.Marshal(env)
		if err2 != nil {
			t.Fatal(err2)
		}
		if err := os.WriteFile(p, bs2, 0o600); err != nil {
			t.Fatal(err)
		}
		_, _, err := loadOfflineSnapshot(p, age, now)
		if !errors.Is(err, errOfflineVersion) {
			t.Fatalf("err = %v, want errOfflineVersion", err)
		}
	})

	t.Run("nil response refused", func(t *testing.T) {
		dir := t.TempDir()
		if err := saveOfflineSnapshot(filepath.Join(dir, "s.bin"), nil, nil, now); err == nil {
			t.Fatal("expected error for nil response")
		}
	})

	t.Run("delete", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		if err := saveOfflineSnapshot(p, sampleSyncResponse(), nil, now); err != nil {
			t.Fatalf("save: %v", err)
		}
		if err := deleteOfflineSnapshot(p); err != nil {
			t.Fatalf("delete: %v", err)
		}
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("snapshot still exists")
		}
		if err := deleteOfflineSnapshot(p); err != nil {
			t.Fatalf("delete missing: %v", err)
		}
	})

	t.Run("atomic under race", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "networkmap.bin")
		resp := sampleSyncResponse()
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func(off int) {
				defer wg.Done()
				for j := 0; j < 50; j++ {
					_ = saveOfflineSnapshot(p, resp, nil, now.Add(time.Duration(off+j)*time.Second))
				}
			}(i)
		}
		for j := 0; j < 200; j++ {
			r, _, err := loadOfflineSnapshot(p, 0 /*unused*/, time.Time{})
			if err == errOfflineDisabled {
				continue
			}
			if err != nil && !errors.Is(err, errOfflineNoSnapshot) {
				t.Fatalf("concurrent load: %v", err)
			}
			if r != nil && r.GetNetworkMap().GetSerial() != 42 {
				t.Fatalf("corrupt read")
			}
		}
		wg.Wait()
	})

	t.Run("debounce", func(t *testing.T) {
		var saves atomic.Int32
		var clock atomic.Int64
		clock.Store(int64(10 * time.Second))
		d := newOfflineDebouncer(time.Second, func() error {
			saves.Add(1)
			return nil
		}, nil)
		d.clockNow = func() time.Time { return time.Unix(0, clock.Load()).UTC() }
		if !d.Save() {
			t.Fatal("first save should schedule")
		}
		if d.Save() {
			t.Fatal("second save within interval must not schedule")
		}
		clock.Store(int64(12 * time.Second))
		time.Sleep(60 * time.Millisecond) // let the in-flight write finish
		if !d.Save() {
			t.Fatal("save after interval should schedule")
		}
	})
}

func TestMergeEndpoints(t *testing.T) {
	peers := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	for name, tc := range map[string]struct {
		prev, cur map[string]string
		want      map[string]string
	}{
		"empty":        {nil, nil, nil},
		"new only":     {nil, map[string]string{"a": "1.1.1.1:51820"}, map[string]string{"a": "1.1.1.1:51820"}},
		"prev kept":    {map[string]string{"b": "2.2.2.2:51820"}, nil, map[string]string{"b": "2.2.2.2:51820"}},
		"new wins":     {map[string]string{"a": "1.1.1.1:1"}, map[string]string{"a": "1.1.1.1:51820"}, map[string]string{"a": "1.1.1.1:51820"}},
		"dropped peer": {map[string]string{"x": "3.3.3.3:51820"}, nil, nil},
		"merge":        {map[string]string{"a": "1.1.1.1:51820", "b": "2.2.2.2:51820"}, map[string]string{"a": "9.9.9.9:51820"}, map[string]string{"a": "9.9.9.9:51820", "b": "2.2.2.2:51820"}},
	} {
		t.Run(name, func(t *testing.T) {
			got := mergeEndpoints(tc.prev, tc.cur, peers)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Fatalf("key %s: got %q want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestOfflineSnapshotV2Endpoints(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "networkmap.bin")
	now := time.Now()
	eps := map[string]string{"peerA": "203.0.113.5:51820", "peerB": "bad-value", "peerC": "0.0.0.0:1", "peerD": "224.0.0.1:51820"}
	if err := saveOfflineSnapshot(p, sampleSyncResponse(), eps, now); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, _, err := loadOfflineSnapshot(p, time.Hour, now); err != nil {
		t.Fatalf("load: %v", err)
	}
	got := LastEndpoints()
	if got["peerA"] != "203.0.113.5:51820" {
		t.Fatalf("valid endpoint missing: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("invalid endpoints not dropped: %v", got)
	}
}
