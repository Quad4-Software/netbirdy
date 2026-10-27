package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeEvents(t *testing.T, path string, events []Event) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatal(err)
		}
	}
}

func TestReadEventLogMissingFile(t *testing.T) {
	events, err := ReadEventLog(filepath.Join(t.TempDir(), "absent.jsonl"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}
}

func TestReadEventLogNewestFirst(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	evs := []Event{
		{Timestamp: time.Unix(1, 0).UTC(), Rule: "peer_new_ip", SrcIP: "1.1.1.1"},
		{Timestamp: time.Unix(2, 0).UTC(), Rule: "auth_failure_burst", SrcIP: "2.2.2.2"},
		{Timestamp: time.Unix(3, 0).UTC(), Rule: "peer_ip_flap", SrcIP: "3.3.3.3"},
	}
	writeEvents(t, path, evs)

	events, err := ReadEventLog(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}
	if events[0].Rule != "peer_ip_flap" || events[2].Rule != "peer_new_ip" {
		t.Fatalf("events not newest-first: %+v", events)
	}
}

func TestReadEventLogLimitAndSkipsBadLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	evs := []Event{
		{Timestamp: time.Unix(1, 0).UTC(), Rule: "a"},
		{Timestamp: time.Unix(2, 0).UTC(), Rule: "b"},
		{Timestamp: time.Unix(3, 0).UTC(), Rule: "c"},
	}
	writeEvents(t, path, evs)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("not json\n{\"broken\":\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	events, err := ReadEventLog(path, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Rule != "c" || events[1].Rule != "b" {
		t.Fatalf("unexpected order: %+v", events)
	}
}

func TestEngineSetModeAndStatus(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeObserve
	e := NewEngine(cfg, SystemClock{}, nil, nil, nil)
	defer e.Close()

	if err := e.SetMode(ModePrevent); err != nil {
		t.Fatal(err)
	}
	st := e.Status()
	if st.Mode != ModePrevent {
		t.Fatalf("mode %q", st.Mode)
	}
	if len(st.Rules) != len(cfg.Rules) {
		t.Fatalf("rules %d != %d", len(st.Rules), len(cfg.Rules))
	}
	if err := e.SetMode(Mode("bogus")); err == nil {
		t.Fatal("bogus mode accepted")
	}
	if e.Status().Mode != ModePrevent {
		t.Fatal("failed SetMode clobbered mode")
	}
}

func TestEngineFindingsCounted(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeObserve
	cfg.LearningPeriod = 0
	e := NewEngine(cfg, SystemClock{}, nil, nil, nil)
	defer e.Close()

	// A trusted-IP observation produces no finding; an untrusted first-ever
	// IP also stays quiet. Emit directly through the internal path instead.
	e.emit("peer_new_ip", 50, Event{AccountID: "a1"}, "peer1", nil)
	e.emit("peer_new_ip", 50, Event{AccountID: "a1"}, "peer1", nil) // cooldown suppresses
	e.emit("auth_failure_burst", 70, Event{AccountID: "a1"}, "ip1", nil)

	st := e.Status()
	if st.FindingsTotal != 2 {
		t.Fatalf("findings_total %d", st.FindingsTotal)
	}
	if st.FindingsByRule["peer_new_ip"] != 1 || st.FindingsByRule["auth_failure_burst"] != 1 {
		t.Fatalf("findings_by_rule %v", st.FindingsByRule)
	}
}
