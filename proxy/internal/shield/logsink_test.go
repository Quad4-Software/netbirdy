package shield

import (
	"encoding/json"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
)

func TestDecisionLogWritesNonPass(t *testing.T) {
	dir := t.TempDir()
	l, err := NewDecisionLog(dir+"/shield.jsonl", 0, 0, "enforce")
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "http://h.example/scanner?secret=1", nil)
	r.Header.Set("User-Agent", "badbot/1.0")
	ip := netip.MustParseAddr("6.6.6.6")
	l.write(r, Result{Action: ActionBlock, Score: 42, Reasons: []string{"honeypot"}}, ip)
	l.write(r, Result{Action: ActionPass}, ip) // must not be written
	data, err := os.ReadFile(dir + "/shield.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected only non-pass lines, got %d", len(lines))
	}
	var ev DecisionEvent
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatal(err)
	}
	if ev.Action != "block" || ev.SrcIP != "6.6.6.6" || ev.Host != "h.example" ||
		ev.Path != "/scanner" || ev.Severity != "high" || ev.Type != "shield" ||
		ev.Score != 42 || len(ev.Reasons) != 1 {
		t.Fatalf("bad event: %+v", ev)
	}
	if strings.Contains(string(data), "secret=1") {
		t.Fatal("query leaked into path")
	}
}

func TestDecisionLogTruncation(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewDecisionLog(dir+"/s.jsonl", 0, 0, "observe")
	long := strings.Repeat("a", 400)
	r := httptest.NewRequest("GET", "http://h/"+long, nil)
	r.Header.Set("User-Agent", strings.Repeat("u", 400))
	l.write(r, Result{Action: ActionDrop}, netip.MustParseAddr("1.1.1.1"))
	data, _ := os.ReadFile(dir + "/s.jsonl")
	var ev DecisionEvent
	json.Unmarshal(data[:len(data)-1], &ev)
	if len(ev.Path) != 256 || len(ev.UserAgent) != 200 {
		t.Fatalf("truncation: path=%d ua=%d", len(ev.Path), len(ev.UserAgent))
	}
}

func TestDecisionLogRotation(t *testing.T) {
	dir := t.TempDir()
	l, _ := NewDecisionLog(dir+"/s.jsonl", 300, 2, "observe")
	r := httptest.NewRequest("GET", "http://h/x", nil)
	for i := 0; i < 20; i++ {
		l.write(r, Result{Action: ActionBlock}, netip.MustParseAddr("1.1.1.1"))
	}
	if _, err := os.Stat(dir + "/s.jsonl.1"); err != nil {
		t.Fatal("no rotation backup")
	}
}
