package proxy

import (
	"context"
	"testing"
)

// TestNewCopiesShieldConfig guards against Config fields being declared but
// silently dropped when the Server struct is built, which is what made the
// shield env settings no-ops.
func TestNewCopiesShieldConfig(t *testing.T) {
	cfg := Config{
		ShieldMode:               "observe",
		ShieldConfigFile:         "/etc/shield.yaml",
		ShieldStateFile:          "/var/lib/shield-bans.json",
		ShieldAllowCIDRs:         "10.0.0.0/8,192.168.0.0/16",
		ShieldLogFile:            "/var/log/shield.jsonl",
		ShieldAllowSearchEngines: true,
		ShieldAICrawlers:         "challenge",
	}
	s := New(context.Background(), cfg)
	if s.ShieldMode != cfg.ShieldMode ||
		s.ShieldConfigFile != cfg.ShieldConfigFile ||
		s.ShieldStateFile != cfg.ShieldStateFile ||
		s.ShieldAllowCIDRs != cfg.ShieldAllowCIDRs ||
		s.ShieldLogFile != cfg.ShieldLogFile ||
		s.ShieldAllowSearchEngines != cfg.ShieldAllowSearchEngines ||
		s.ShieldAICrawlers != cfg.ShieldAICrawlers {
		t.Fatal("shield config fields dropped between Config and Server")
	}
}
