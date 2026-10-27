package shield

import (
	"os"
	"path/filepath"
	"testing"
)

func validConfig() *Config {
	return &Config{
		Mode:               ModeEnforce,
		AllowCIDRs:         []string{"10.0.0.0/8"},
		AICrawlers:         AIDeny,
		AllowSearchEngines: false,
		AllowLocal:         true,
	}
}

func TestConfigValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	c := validConfig()
	c.Mode = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("invalid mode accepted")
	}
	c = validConfig()
	c.AICrawlers = "bogus"
	if err := c.Validate(); err == nil {
		t.Fatal("invalid ai_crawlers accepted")
	}
	c = validConfig()
	c.AllowCIDRs = []string{"not-a-cidr"}
	if err := c.Validate(); err == nil {
		t.Fatal("invalid cidr accepted")
	}
}

func TestManagerMissingFileIsUnmanaged(t *testing.T) {
	m, err := NewManager(filepath.Join(t.TempDir(), ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if m.Configured() {
		t.Fatal("expected unmanaged config")
	}
	if m.Get() != nil || m.ShieldProto() != nil {
		t.Fatal("unmanaged manager returned a config")
	}
}

func TestManagerSetPersistsAndReloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	m, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Set(validConfig()); err != nil {
		t.Fatal(err)
	}
	if !m.Configured() {
		t.Fatal("expected managed config")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file mode %o", info.Mode().Perm())
	}

	m2, err := NewManager(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := m2.Get()
	if cfg == nil || cfg.Mode != ModeEnforce || cfg.AllowCIDRs[0] != "10.0.0.0/8" {
		t.Fatalf("reloaded config %+v", cfg)
	}

	p := m2.ShieldProto()
	if p == nil || p.GetMode() != ModeEnforce || p.GetAllowCidrs()[0] != "10.0.0.0/8" || !p.GetAllowLocal() {
		t.Fatalf("proto %+v", p)
	}
}

func TestManagerSetRejectsInvalid(t *testing.T) {
	m := NewEmptyManager(filepath.Join(t.TempDir(), ConfigFileName))
	c := validConfig()
	c.Mode = "bogus"
	if err := m.Set(c); err == nil {
		t.Fatal("invalid config persisted")
	}
	if m.Configured() {
		t.Fatal("invalid config installed")
	}
}

func TestManagerCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ConfigFileName)
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewManager(path); err == nil {
		t.Fatal("corrupt file loaded")
	}
}
