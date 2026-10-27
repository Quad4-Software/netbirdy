package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistModeMergesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management.json")
	doc := map[string]any{
		"Stuns":    []any{map[string]any{"URI": "stun:stun.example:3478"}},
		"Datadir":  "/var/lib/netbirdy",
		"security": map[string]any{"learning_period": "24h"},
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := persistMode(path, "observe"); err != nil {
		t.Fatal(err)
	}

	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]any
	if err := json.Unmarshal(out, &merged); err != nil {
		t.Fatal(err)
	}
	sec, ok := merged["security"].(map[string]any)
	if !ok {
		t.Fatal("security section lost")
	}
	if sec["mode"] != "observe" {
		t.Fatalf("mode %v", sec["mode"])
	}
	if sec["learning_period"] != "24h" {
		t.Fatal("existing security keys lost")
	}
	if merged["Datadir"] != "/var/lib/netbirdy" || merged["Stuns"] == nil {
		t.Fatal("unrelated config keys lost")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode %o", info.Mode().Perm())
	}
}

func TestPersistModeCreatesSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "management.json")
	if err := os.WriteFile(path, []byte(`{"Datadir":"/var/lib/netbirdy"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := persistMode(path, "prevent"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]any
	if err := json.Unmarshal(out, &merged); err != nil {
		t.Fatal(err)
	}
	sec, ok := merged["security"].(map[string]any)
	if !ok || sec["mode"] != "prevent" {
		t.Fatalf("security section %+v", merged["security"])
	}
}

func TestPersistModeRejectsBadInput(t *testing.T) {
	if err := persistMode("", "observe"); err == nil {
		t.Fatal("empty path accepted")
	}
	path := filepath.Join(t.TempDir(), "management.json")
	if err := persistMode(path, "observe"); err == nil {
		t.Fatal("missing file accepted")
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := persistMode(path, "observe"); err == nil {
		t.Fatal("corrupt file accepted")
	}
}
