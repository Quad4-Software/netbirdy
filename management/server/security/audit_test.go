package security

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/netbirdio/netbird/management/server/activity"
)

func TestAuditStoreMirror(t *testing.T) {
	dir := t.TempDir()
	inner := &activity.InMemoryEventStore{}
	as, err := NewAuditStore(inner, dir+"/audit.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	ev := &activity.Event{Timestamp: time.Now(), Activity: activity.PeerNewIPDetected,
		InitiatorID: "u1", InitiatorEmail: "a@b.c", TargetID: "p1", AccountID: "a1",
		Meta: map[string]any{"src_ip": "1.2.3.4", "token": "x", "long": strings.Repeat("z", 300)}}
	if _, err := as.Save(context.Background(), ev); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dir + "/audit.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var line map[string]any
	if err := json.Unmarshal(data[:len(data)-1], &line); err != nil {
		t.Fatal(err)
	}
	if line["type"] != "peer.ip.new" || line["severity"] != "high" || line["source"] != "audit" {
		t.Fatalf("line: %v", line)
	}
	meta := line["meta"].(map[string]any)
	if _, ok := meta["token"]; ok {
		t.Fatal("sensitive meta leaked")
	}
	if len(meta["long"].(string)) != 256 {
		t.Fatal("long meta not truncated")
	}
}

func TestAuditStoreInnerError(t *testing.T) {
	dir := t.TempDir()
	as, _ := NewAuditStore(&errStore{}, dir+"/a.jsonl")
	_, err := as.Save(context.Background(), &activity.Event{})
	if err == nil {
		t.Fatal("inner error not propagated")
	}
}

type errStore struct{ activity.Store }

func (e *errStore) Save(context.Context, *activity.Event) (*activity.Event, error) {
	return nil, errors.New("store down")
}
