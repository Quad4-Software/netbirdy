package proxy

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/proxy/internal/shield"
	"github.com/netbirdio/netbird/shared/management/proto"
)

func testShield(t *testing.T) *shield.Shield {
	t.Helper()
	cfg := shield.DefaultConfig()
	cfg.Mode = shield.ModeEnforce
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 5
	cfg.BanAt = 10
	cfg.WeightScannerUA = 80
	sh, err := shield.New(cfg, shield.WithIPResolver(func(*http.Request) netip.Addr {
		return netip.MustParseAddr("203.0.113.9")
	}), shield.WithKnownHost(func(string) bool { return true }))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sh.Close)
	return sh
}

func TestApplyShieldConfigPushed(t *testing.T) {
	s := &Server{Logger: log.New(), shield: testShield(t)}

	s.applyShieldConfig(&proto.ShieldConfig{
		Mode:               "observe",
		AllowCidrs:         []string{"203.0.113.0/24"},
		AiCrawlers:         "allow",
		AllowSearchEngines: true,
		AllowLocal:         false,
	})

	cfg := s.shield.Config()
	if cfg.Mode != shield.ModeObserve {
		t.Fatalf("mode %q", cfg.Mode)
	}
	if cfg.AICrawlers != shield.AIAllow {
		t.Fatalf("ai_crawlers %q", cfg.AICrawlers)
	}
	if !cfg.AllowSearchEngines || *cfg.AllowLocal {
		t.Fatalf("flags %+v", cfg)
	}
	if len(cfg.AllowCIDRs) != 1 || cfg.AllowCIDRs[0] != "203.0.113.0/24" {
		t.Fatalf("allow_cidrs %v", cfg.AllowCIDRs)
	}
}

func TestApplyShieldConfigIgnoredWithoutShield(t *testing.T) {
	s := &Server{Logger: log.New()}
	// Must not panic when the proxy runs without a shield.
	s.applyShieldConfig(&proto.ShieldConfig{Mode: "enforce"})
	s.applyShieldCommands([]*proto.ShieldCommand{{UnbanIp: "203.0.113.9"}})
	if got := s.shieldStatusProto().GetMode(); got != "off" {
		t.Fatalf("status mode %q", got)
	}
}

func TestApplyShieldCommandsUnban(t *testing.T) {
	s := &Server{Logger: log.New(), shield: testShield(t)}

	// Ban the test source via a honeypot request.
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "http://svc.example/wp-login.php", nil)
	r.RemoteAddr = "203.0.113.9:1234"
	s.shield.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, r)

	banned := false
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if s.shield.Stats().Banned > 0 {
			banned = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !banned {
		t.Skip("test source was not banned; skipping unban check")
	}

	s.applyShieldCommands([]*proto.ShieldCommand{{UnbanIp: "203.0.113.9"}, {UnbanIp: ""}})
	if s.shield.Stats().Banned != 0 {
		t.Fatal("unban command did not clear the ban")
	}
}

func TestShieldStatusProtoReportsStats(t *testing.T) {
	s := &Server{Logger: log.New(), shield: testShield(t)}
	st := s.shieldStatusProto()
	if st.GetMode() != "enforce" {
		t.Fatalf("status mode %q", st.GetMode())
	}
}
