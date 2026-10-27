// Package shield persists the management-controlled proxy shield
// configuration. The config lives in shield-config.json inside the datadir
// and is pushed to every connected proxy over the sync stream; proxies that
// have no managed config keep their local shield settings.
package shield

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sync"

	"github.com/netbirdio/netbird/shared/management/proto"
)

// ConfigFileName is the persisted shield config file inside the datadir.
const ConfigFileName = "shield-config.json"

// Shield modes pushed to proxies. They mirror the proxy-side shield modes.
const (
	ModeOff     = "off"
	ModeObserve = "observe"
	ModeEnforce = "enforce"
)

// AI crawler policies pushed to proxies.
const (
	AIDeny      = "deny"
	AIAllow     = "allow"
	AIChallenge = "challenge"
)

// Config is the management-controlled subset of the proxy shield
// configuration. All fields are pushed together; the proxy merges them on
// top of its running config.
type Config struct {
	// Mode is off, observe or enforce.
	Mode string `json:"mode"`
	// AllowCIDRs lists source ranges that always bypass the shield.
	AllowCIDRs []string `json:"allow_cidrs"`
	// AICrawlers is deny, allow or challenge.
	AICrawlers string `json:"ai_crawlers"`
	// AllowSearchEngines lets verified search-engine crawlers pass.
	AllowSearchEngines bool `json:"allow_search_engines"`
	// AllowLocal lets loopback, RFC1918, ULA and link-local sources bypass.
	AllowLocal bool `json:"allow_local"`
}

// Validate checks the config the same way the proxy does for the pushed
// subset, so a bad value is rejected here instead of surfacing on every
// proxy at apply time.
func (c Config) Validate() error {
	switch c.Mode {
	case ModeOff, ModeObserve, ModeEnforce:
	default:
		return fmt.Errorf("unknown mode %q: must be off, observe or enforce", c.Mode)
	}
	switch c.AICrawlers {
	case AIDeny, AIAllow, AIChallenge:
	default:
		return fmt.Errorf("unknown ai_crawlers policy %q: must be deny, allow or challenge", c.AICrawlers)
	}
	for _, s := range c.AllowCIDRs {
		if _, err := netip.ParsePrefix(s); err != nil {
			return fmt.Errorf("allow_cidrs %q: %w", s, err)
		}
	}
	return nil
}

// Manager holds the shield configuration, backed by shield-config.json in
// the datadir. When the file is absent the config is unmanaged and nothing
// is pushed to proxies.
type Manager struct {
	mu   sync.RWMutex
	path string
	cfg  *Config
}

// NewManager loads the persisted config from path. A missing file leaves
// the manager empty; a corrupt file is reported but not fatal.
func NewManager(path string) (*Manager, error) {
	m := &Manager{path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return m, nil
		}
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	m.cfg = &cfg
	return m, nil
}

// NewEmptyManager returns a manager with no managed config bound to path,
// for callers that keep going after a persisted config fails to load.
func NewEmptyManager(path string) *Manager {
	return &Manager{path: path}
}

// Configured reports whether a managed config exists.
func (m *Manager) Configured() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg != nil
}

// Get returns a copy of the managed config, or nil when unmanaged.
func (m *Manager) Get() *Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.cfg == nil {
		return nil
	}
	cfg := *m.cfg
	cfg.AllowCIDRs = append([]string(nil), m.cfg.AllowCIDRs...)
	return &cfg
}

// Set validates, persists and installs cfg as the managed config.
func (m *Manager) Set(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("shield config is required")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o700); err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, m.path); err != nil {
		os.Remove(tmp)
		return err
	}
	m.mu.Lock()
	stored := *cfg
	stored.AllowCIDRs = append([]string(nil), cfg.AllowCIDRs...)
	m.cfg = &stored
	m.mu.Unlock()
	return nil
}

// ShieldProto returns the managed config as a proto message for the sync
// stream, or nil when no config is managed.
func (m *Manager) ShieldProto() *proto.ShieldConfig {
	cfg := m.Get()
	if cfg == nil {
		return nil
	}
	return &proto.ShieldConfig{
		Mode:               cfg.Mode,
		AllowCidrs:         cfg.AllowCIDRs,
		AiCrawlers:         cfg.AICrawlers,
		AllowSearchEngines: cfg.AllowSearchEngines,
		AllowLocal:         cfg.AllowLocal,
	}
}
