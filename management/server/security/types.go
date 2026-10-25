// Package security implements anomaly detection for management events:
// suspicious IP changes, concurrent connections on one peer key, auth
// failure bursts and credential-anomaly patterns. Prevention is optional,
// always safe, and observe is the default mode.
package security

import (
	"context"
	"net/netip"
	"time"
)

// Mode decides whether findings only log (observe) or also trigger
// prevention actions.
type Mode string

const (
	ModeOff     Mode = "off"
	ModeObserve Mode = "observe"
	ModePrevent Mode = "prevent"
)

// Severity buckets for findings.
type Severity string

const (
	SeverityInfo   Severity = "info"
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

// Action is a prevention step taken when a rule fires in prevent mode.
type Action string

const (
	ActionAlert           Action = "alert"
	ActionRequireApproval Action = "require_approval"
	ActionExpireLogin     Action = "expire_login"
	ActionBlockPeer       Action = "block_peer"
)

// Event is the stable structured schema written to the security log and
// emitted to sinks.
type Event struct {
	Timestamp    time.Time `json:"ts"`
	Type         string    `json:"type"`
	Severity     Severity  `json:"severity"`
	AccountID    string    `json:"account_id"`
	PeerID       string    `json:"peer_id,omitempty"`
	PeerName     string    `json:"peer_name,omitempty"`
	UserID       string    `json:"user_id,omitempty"`
	SrcIP        string    `json:"src_ip"`
	PreviousIPs  []string  `json:"previous_ips,omitempty"`
	Country      string    `json:"country,omitempty"`
	ASN          uint32    `json:"asn,omitempty"`
	Rule         string    `json:"rule"`
	Score        int       `json:"score"`
	ActionTaken  Action    `json:"action_taken"`
	Mode         Mode      `json:"mode"`
	WouldPrevent bool      `json:"would_prevent,omitempty"`
	Reason       string    `json:"reason,omitempty"`
}

// GeoResolver resolves an IP to a country code and ASN. Implementations
// must be nil-safe at call sites; a nil resolver yields empty values.
type GeoResolver interface {
	Lookup(ip netip.Addr) (country string, asn uint32)
}

// Clock supplies the current time; tests inject a fake.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production clock.
type SystemClock struct{}

// Now returns time.Now.
func (SystemClock) Now() time.Time { return time.Now() }

// Sink receives emitted events.
type Sink interface {
	Emit(Event)
}

// Actions are the prevention levers. Engine calls them only in prevent
// mode, after the learning period, within caps, asynchronously.
type Actions interface {
	RequireApproval(ctx context.Context, accountID, peerID string) error
	ExpireLogin(ctx context.Context, accountID, peerID string) error
	BlockPeer(ctx context.Context, accountID, peerID string) error
}

// RuleConfig holds per-rule enable, prevention action and weight tuning.
type RuleConfig struct {
	Enabled bool   `json:"enabled"`
	Action  Action `json:"action"` // what prevent mode does when the rule fires
}

// Config tunes the engine.
type Config struct {
	Mode                            Mode                   `json:"mode"`
	LearningPeriod                  time.Duration          `json:"learning_period"`
	TrustedCIDRs                    []netip.Prefix         `json:"trusted_cidrs"`
	Rules                           map[string]*RuleConfig `json:"rules"`
	MaxPreventionsPerAccountPerHour int                    `json:"max_preventions_per_account_hour"`
	LogFile                         string                 `json:"log_file"`
	Cooldown                        time.Duration          `json:"cooldown"`
	SeverityMedium                  int                    `json:"severity_medium"`
	SeverityHigh                    int                    `json:"severity_high"`
	FlapWindow                      time.Duration          `json:"flap_window"`
	FlapThreshold                   int                    `json:"flap_threshold"`
	AuthFailWindow                  time.Duration          `json:"auth_fail_window"`
	AuthFailThreshold               int                    `json:"auth_fail_threshold"`
	AuthSuccessAfterFails           int                    `json:"auth_success_after_fails"`
	SetupKeyIPWindow                time.Duration          `json:"setup_key_ip_window"`
	SetupKeyIPThreshold             int                    `json:"setup_key_ip_threshold"`
	SetupKeyRegWindow               time.Duration          `json:"setup_key_reg_window"`
	SetupKeyRegThreshold            int                    `json:"setup_key_reg_threshold"`
	ConcurrentGracePeriod           time.Duration          `json:"concurrent_grace_period"`
	MaxPeers                        int                    `json:"max_peers"`
	MaxIPsPerPeer                   int                    `json:"max_ips_per_peer"`
	MaxAuthKeys                     int                    `json:"max_auth_keys"`
	QueueSize                       int                    `json:"queue_size"`
}

// DefaultConfig returns the production defaults: observe mode, 7 days of
// learning, cooldown 10 minutes, bounds matching the design.
func DefaultConfig() *Config {
	return &Config{
		Mode:                            ModeObserve,
		LearningPeriod:                  168 * time.Hour,
		Rules:                           defaultRules(),
		MaxPreventionsPerAccountPerHour: 5,
		Cooldown:                        10 * time.Minute,
		SeverityMedium:                  60,
		SeverityHigh:                    80,
		FlapWindow:                      10 * time.Minute,
		FlapThreshold:                   5,
		AuthFailWindow:                  5 * time.Minute,
		AuthFailThreshold:               20,
		AuthSuccessAfterFails:           5,
		SetupKeyIPWindow:                24 * time.Hour,
		SetupKeyIPThreshold:             5,
		SetupKeyRegWindow:               24 * time.Hour,
		SetupKeyRegThreshold:            10,
		ConcurrentGracePeriod:           60 * time.Second,
		MaxPeers:                        50000,
		MaxIPsPerPeer:                   32,
		MaxAuthKeys:                     100000,
		QueueSize:                       256,
	}
}

func defaultRules() map[string]*RuleConfig {
	return map[string]*RuleConfig{
		"peer_new_ip":                 {Enabled: true, Action: ActionAlert},
		"peer_ip_flap":                {Enabled: true, Action: ActionAlert},
		"peer_impossible_travel":      {Enabled: true, Action: ActionAlert},
		"peer_meta_mismatch":          {Enabled: true, Action: ActionAlert},
		"concurrent_connections":      {Enabled: true, Action: ActionRequireApproval},
		"setup_key_misuse":            {Enabled: true, Action: ActionAlert},
		"auth_failure_burst":          {Enabled: true, Action: ActionAlert},
		"auth_success_after_failures": {Enabled: true, Action: ActionAlert},
		"user_new_country":            {Enabled: true, Action: ActionAlert},
		"pat_new_ip":                  {Enabled: true, Action: ActionAlert},
	}
}

func (c *Config) ruleEnabled(name string) bool {
	r, ok := c.Rules[name]
	return ok && r.Enabled
}

func (c *Config) ruleAction(name string) Action {
	if r, ok := c.Rules[name]; ok {
		return r.Action
	}
	return ActionAlert
}

func (c *Config) trusted(ip netip.Addr) bool {
	for _, p := range c.TrustedCIDRs {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// PeerObservation describes a management-visible peer event.
type PeerObservation struct {
	AccountID string
	PeerID    string
	PeerKey   string
	PeerName  string
	SrcIP     netip.Addr
	Meta      PeerMeta
}

// PeerMeta is the machine identity reported by the client.
type PeerMeta struct {
	Hostname  string
	OS        string
	Kernel    string
	Serial    string
	PublicKey string
}

// AuthKind classifies authentication surfaces.
type AuthKind string

const (
	AuthKindPeerLogin AuthKind = "peer_login"
	AuthKindAPI       AuthKind = "api"
	AuthKindSetupKey  AuthKind = "setup_key"
	AuthKindDashboard AuthKind = "dashboard"
)

// AuthFailure is an observable authentication rejection.
type AuthFailure struct {
	Kind      AuthKind
	SourceIP  netip.Addr
	AccountID string
	Subject   string // peer id, user id or key name
}

// AuthSuccess is an accepted authentication.
type AuthSuccess struct {
	Kind      AuthKind
	SourceIP  netip.Addr
	AccountID string
	Subject   string
}

// SetupKeyObservation covers a setup key use.
type SetupKeyObservation struct {
	KeyID     string
	AccountID string
	SourceIP  netip.Addr
	PeerID    string
}
