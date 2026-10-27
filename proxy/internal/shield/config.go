// Package shield blocks scanners, crawlers and abusive clients in front of
// proxied services using a decaying per-source score. See docs/shield.md.
package shield

import (
	"fmt"
	"net/netip"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Mode controls enforcement: "off" disables, "observe" scores and logs but
// never alters responses, "enforce" applies actions.
type Mode string

const (
	ModeOff     Mode = "off"
	ModeObserve Mode = "observe"
	ModeEnforce Mode = "enforce"
)

// AICrawlerPolicy decides what happens to known AI training crawlers.
type AICrawlerPolicy string

const (
	AIDeny      AICrawlerPolicy = "deny"
	AIAllow     AICrawlerPolicy = "allow"
	AIChallenge AICrawlerPolicy = "challenge"
)

// Config drives the shield. All fields are optional; defaults are sane.
type Config struct {
	Mode Mode `yaml:"mode"`

	// Score thresholds: below ChallengeAt the request passes untouched.
	ChallengeAt float64 `yaml:"challenge_at"`
	TarpitAt    float64 `yaml:"tarpit_at"`
	BanAt       float64 `yaml:"ban_at"`

	// Weights per signal class.
	WeightHoneypot      float64 `yaml:"weight_honeypot"`
	WeightScannerUA     float64 `yaml:"weight_scanner_ua"`
	WeightResearchUA    float64 `yaml:"weight_research_ua"`
	WeightEmptyUA       float64 `yaml:"weight_empty_ua"`
	WeightFakeBrowserUA float64 `yaml:"weight_fake_browser_ua"`
	WeightProbeRate     float64 `yaml:"weight_probe_rate"`
	WeightEnumeration   float64 `yaml:"weight_enumeration"`
	WeightUnknownHost   float64 `yaml:"weight_unknown_host"`
	WeightOddRequest    float64 `yaml:"weight_odd_request"`
	WeightRate          float64 `yaml:"weight_rate"`
	WeightFakeSearchBot float64 `yaml:"weight_fake_search_bot"`
	WeightTLS           float64 `yaml:"weight_tls"`
	WeightAuthFailure   float64 `yaml:"weight_auth_failure"`

	// Decay: score halves every DecayHalfLife.
	DecayHalfLife time.Duration `yaml:"decay_half_life"`

	// Ban lifecycle.
	BanBaseTTL    time.Duration `yaml:"ban_base_ttl"`
	BanMaxTTL     time.Duration `yaml:"ban_max_ttl"`
	BanQuietReset time.Duration `yaml:"ban_quiet_reset"`
	BanEscalation int           `yaml:"ban_escalation"`

	// Limits.
	MaxEntries  int           `yaml:"max_entries"`
	MaxTarpits  int           `yaml:"max_tarpits"`
	TarpitDelay time.Duration `yaml:"tarpit_delay"`

	// Rate limit per source: sustained RPS and burst.
	RatePerSecond float64 `yaml:"rate_per_second"`
	RateBurst     int     `yaml:"rate_burst"`

	// Behavioral windows.
	ProbeWindow    time.Duration `yaml:"probe_window"`
	ProbeThreshold int           `yaml:"probe_threshold"`
	EnumWindow     time.Duration `yaml:"enum_window"`
	EnumThreshold  int           `yaml:"enum_threshold"`

	// Challenge.
	ChallengeDifficulty int           `yaml:"challenge_difficulty"`
	ChallengeTTL        time.Duration `yaml:"challenge_ttl"`
	PassCookieTTL       time.Duration `yaml:"pass_cookie_ttl"`
	HMACKey             string        `yaml:"hmac_key"` // hex or raw; generated if empty

	// Lists.
	AllowHosts      []string `yaml:"allow_hosts"`
	AllowCIDRs      []string `yaml:"allow_cidrs"`
	ExtraHoneypots  []string `yaml:"extra_honeypots"`
	RemoveHoneypots []string `yaml:"remove_honeypots"`
	ExtraScannerUA  []string `yaml:"extra_scanner_ua"`
	RemoveScannerUA []string `yaml:"remove_scanner_ua"`

	AllowSearchEngines bool            `yaml:"allow_search_engines"`
	AICrawlers         AICrawlerPolicy `yaml:"ai_crawlers"`

	// AllowLocal lets loopback, RFC1918, ULA and link-local sources bypass
	// the shield entirely. Defaults true: clients on the same LAN or a
	// private range cannot be internet scanners. The overlay CGNAT range
	// (100.64.0.0/10) is not included; add it to allow_cidrs if needed.
	AllowLocal *bool `yaml:"allow_local"`

	// Persistence of bans across restarts. Empty disables.
	StateFile    string `yaml:"state_file"`
	StateMaxBans int    `yaml:"state_max_bans"`

	// Close silently instead of answering 403 on hard honeypot hits.
	DropOnHoneypot bool `yaml:"drop_on_honeypot"`

	cidrs []netip.Prefix
	hosts map[string]bool
}

// DefaultConfig returns the production defaults.
func DefaultConfig() Config {
	return Config{
		Mode:                ModeEnforce,
		ChallengeAt:         40,
		TarpitAt:            70,
		BanAt:               100,
		WeightHoneypot:      100,
		WeightScannerUA:     80,
		WeightResearchUA:    20,
		WeightEmptyUA:       10,
		WeightFakeBrowserUA: 25,
		WeightProbeRate:     40,
		WeightEnumeration:   40,
		WeightUnknownHost:   15,
		WeightOddRequest:    15,
		WeightRate:          50,
		WeightFakeSearchBot: 90,
		WeightTLS:           30,
		WeightAuthFailure:   30,
		DecayHalfLife:       10 * time.Minute,
		BanBaseTTL:          15 * time.Minute,
		BanMaxTTL:           24 * time.Hour,
		BanQuietReset:       time.Hour,
		BanEscalation:       4,
		MaxEntries:          100000,
		MaxTarpits:          256,
		TarpitDelay:         5 * time.Second,
		RatePerSecond:       100,
		RateBurst:           300,
		ProbeWindow:         2 * time.Minute,
		ProbeThreshold:      12,
		EnumWindow:          time.Minute,
		EnumThreshold:       60,
		ChallengeDifficulty: 16,
		ChallengeTTL:        10 * time.Minute,
		PassCookieTTL:       time.Hour,
		AICrawlers:          AIDeny,
		StateMaxBans:        10000,
	}
}

// LoadConfig merges a YAML file onto the defaults and validates.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("parse %s: %w", path, err)
		}
	}
	return cfg, cfg.validate()
}

func (c *Config) validate() error {
	switch c.Mode {
	case ModeOff, ModeObserve, ModeEnforce:
	default:
		return fmt.Errorf("unknown mode %q", c.Mode)
	}
	switch c.AICrawlers {
	case AIDeny, AIAllow, AIChallenge:
	default:
		return fmt.Errorf("unknown ai_crawlers policy %q", c.AICrawlers)
	}
	if !(0 < c.ChallengeAt && c.ChallengeAt <= c.TarpitAt && c.TarpitAt <= c.BanAt) {
		return fmt.Errorf("thresholds must satisfy 0 < challenge_at <= tarpit_at <= ban_at")
	}
	if c.MaxEntries < 100 || c.MaxTarpits < 1 || c.BanEscalation < 1 {
		return fmt.Errorf("invalid limits")
	}
	var prefixes []netip.Prefix
	for _, s := range c.AllowCIDRs {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return fmt.Errorf("allow_cidrs %q: %w", s, err)
		}
		prefixes = append(prefixes, p.Masked())
	}
	if c.allowLocal() {
		prefixes = append(localPrefixes, prefixes...)
	}
	c.cidrs = prefixes
	hosts := map[string]bool{}
	for _, h := range c.AllowHosts {
		hosts[h] = true
	}
	c.hosts = hosts
	return nil
}

var localPrefixes = []netip.Prefix{
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("::1/128"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
}

func (c *Config) allowLocal() bool {
	return c.AllowLocal == nil || *c.AllowLocal
}

func (c *Config) allowIP(ip netip.Addr) bool {
	for _, p := range c.cidrs {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}
