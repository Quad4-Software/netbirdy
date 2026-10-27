package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/gorilla/mux"
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/server/activity"
	"github.com/netbirdio/netbird/management/server/geolocation"
	securityhandlers "github.com/netbirdio/netbird/management/server/http/handlers/security"
	nbpeer "github.com/netbirdio/netbird/management/server/peer"
	"github.com/netbirdio/netbird/management/server/security"
	"github.com/netbirdio/netbird/management/server/store"
	"github.com/netbirdio/netbird/management/server/types"
)

// geoAdapter feeds the engine the geolite country when a DB is loaded.
type geoAdapter struct {
	geo geolocation.Geolocation
}

func (g geoAdapter) Lookup(ip netip.Addr) (string, uint32) {
	if g.geo == nil {
		return "", 0
	}
	rec, err := g.geo.Lookup(net.IP(ip.AsSlice()))
	if err != nil || rec == nil {
		return "", 0
	}
	return rec.Country.ISOCode, 0 // geolite country DB has no ASN
}

// activitySinkAdapter writes each security event to the activity store so
// findings show up in the audit log, event-streaming and webhooks.
type activitySinkAdapter struct {
	storeEvent func(ctx context.Context, initiator, target, accountID string, act activity.Activity, meta map[string]any)
}

var ruleToActivity = map[string]activity.Activity{
	"peer_new_ip":                 activity.PeerNewIPDetected,
	"concurrent_connections":      activity.PeerConcurrentConnection,
	"peer_ip_flap":                activity.PeerIPFlapping,
	"peer_meta_mismatch":          activity.PeerMetaMismatch,
	"auth_failure_burst":          activity.AuthFailureBurst,
	"auth_success_after_failures": activity.AuthFailureBurst,
	"setup_key_misuse":            activity.SetupKeyMisuseDetected,
	"user_new_country":            activity.UserLoginAnomaly,
	"pat_new_ip":                  activity.PATAnomaly,
	"peer_impossible_travel":      activity.PeerNewIPDetected,
}

const securityInitiator = "system/security"

func (a activitySinkAdapter) Emit(ev security.Event) {
	act, ok := ruleToActivity[ev.Rule]
	if !ok || a.storeEvent == nil {
		return
	}
	meta := map[string]any{
		"src_ip": ev.SrcIP, "rule": ev.Rule, "score": ev.Score,
		"severity": string(ev.Severity), "mode": string(ev.Mode),
		"would_prevent": ev.WouldPrevent, "reason": ev.Reason,
	}
	target := ev.PeerID
	if target == "" {
		target = ev.UserID
	}
	a.storeEvent(context.Background(), securityInitiator, target, ev.AccountID, act, meta)
}

// noopActions is the v1 prevention placeholder: actions land as errors the
// engine logs; nothing is fatal. Used only for tests that must not act.
type noopActions struct{}

func (noopActions) RequireApproval(ctx context.Context, accountID, peerID string) error {
	return errors.New("prevention not enabled yet")
}
func (noopActions) ExpireLogin(ctx context.Context, accountID, peerID string) error {
	return errors.New("prevention not enabled yet")
}
func (noopActions) BlockPeer(ctx context.Context, accountID, peerID string) error {
	return errors.New("prevention not enabled yet")
}

// managementActions implements security.Actions over the account manager.
// Safeguards: never act in non-prevent mode (defensive), never act on a peer
// owned by an account owner unless the action is require_approval, every
// action is idempotent and writes an audit event with the security
// initiator.
// actionsBackend is the narrow surface managementActions needs; the real
// account.Manager satisfies it, tests use fakes.
type actionsBackend interface {
	GetPeer(ctx context.Context, accountID, peerID, userID string) (*nbpeer.Peer, error)
	GetAccountSettings(ctx context.Context, accountID, userID string) (*types.Settings, error)
	GetUserByID(ctx context.Context, id string) (*types.User, error)
	GetStore() store.Store
	StoreEvent(ctx context.Context, initiatorID, targetID, accountID string, activityID activity.ActivityDescriber, meta map[string]any)
	UpdateAccountPeers(ctx context.Context, accountID string, reason types.UpdateReason)
}

type managementActions struct {
	am actionsBackend
}

func (a managementActions) RequireApproval(ctx context.Context, accountID, peerID string) error {
	if err := a.setRequiresApproval(ctx, accountID, peerID); err != nil {
		return err
	}
	return nil
}

func (a managementActions) ExpireLogin(ctx context.Context, accountID, peerID string) error {
	return a.markLoginExpired(ctx, accountID, peerID)
}

func (a managementActions) BlockPeer(ctx context.Context, accountID, peerID string) error {
	// Peer blocking in management is expressed as login expiry: the next
	// login attempt is refused and the peer cannot rejoin.
	return a.markLoginExpired(ctx, accountID, peerID)
}

// setRequiresApproval marks the peer as needing admin approval. When peer
// approval is disabled in account settings the flag has no effect, so the
// adapter falls back to login expiry (reported in the event meta).
func (a managementActions) setRequiresApproval(ctx context.Context, accountID, peerID string) error {
	if err := a.ownerGuard(ctx, accountID, peerID, "require_approval"); err != nil {
		return err
	}
	settings, err := a.am.GetAccountSettings(ctx, accountID, securityInitiator)
	if err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	approvalOn := settings.Extra.PeerApprovalEnabled
	if !approvalOn {
		return a.markLoginExpired(ctx, accountID, peerID)
	}
	peer, err := a.am.GetPeer(ctx, accountID, peerID, securityInitiator)
	if err != nil {
		return fmt.Errorf("peer lookup: %w", err)
	}
	if peer.Status != nil && peer.Status.RequiresApproval {
		return nil // idempotent
	}
	st := *peer.Status
	st.RequiresApproval = true
	if err := a.am.GetStore().ExecuteInTransaction(ctx, func(tx store.Store) error {
		return tx.SavePeerStatus(ctx, accountID, peerID, st)
	}); err != nil {
		return fmt.Errorf("save peer status: %w", err)
	}
	a.emitActionEvent(ctx, accountID, peerID, activity.PeerNewIPDetected, "require_approval")
	go a.am.UpdateAccountPeers(context.WithoutCancel(ctx), accountID, types.UpdateReason{Resource: types.UpdateResourcePeer, Operation: types.UpdateOperationUpdate})
	log.WithContext(ctx).Warnf("security action require_approval account=%s peer=%s", accountID, peerID)
	return nil
}

// markLoginExpired expires the peer's login, forcing re-authentication and
// refusing it (offline snapshot sees the auth refusal and drops too).
func (a managementActions) markLoginExpired(ctx context.Context, accountID, peerID string) error {
	if err := a.ownerGuard(ctx, accountID, peerID, "expire_login"); err != nil {
		return err
	}
	peer, err := a.am.GetPeer(ctx, accountID, peerID, securityInitiator)
	if err != nil {
		return fmt.Errorf("peer lookup: %w", err)
	}
	if peer.Status != nil && peer.Status.LoginExpired {
		return nil // idempotent
	}
	st := *peer.Status
	st.LoginExpired = true
	if err := a.am.GetStore().ExecuteInTransaction(ctx, func(tx store.Store) error {
		return tx.SavePeerStatus(ctx, accountID, peerID, st)
	}); err != nil {
		return fmt.Errorf("save peer status: %w", err)
	}
	a.emitActionEvent(ctx, accountID, peerID, activity.PeerNewIPDetected, "expire_login")
	go a.am.UpdateAccountPeers(context.WithoutCancel(ctx), accountID, types.UpdateReason{Resource: types.UpdateResourcePeer, Operation: types.UpdateOperationUpdate})
	log.WithContext(ctx).Warnf("security action expire_login account=%s peer=%s", accountID, peerID)
	return nil
}

// ownerGuard refuses to act automatically on an account owner's peer for
// anything stronger than require_approval.
func (a managementActions) ownerGuard(ctx context.Context, accountID, peerID, action string) error {
	if action == "require_approval" {
		return nil
	}
	peer, err := a.am.GetPeer(ctx, accountID, peerID, securityInitiator)
	if err != nil {
		return err
	}
	if peer.UserID == "" {
		return nil
	}
	user, err := a.am.GetUserByID(ctx, peer.UserID)
	if err != nil {
		return err
	}
	if user != nil && user.Role == types.UserRoleOwner {
		return fmt.Errorf("refused: owner peer for action %s", action)
	}
	return nil
}

func (a managementActions) emitActionEvent(ctx context.Context, accountID, peerID string, act activity.Activity, action string) {
	a.am.StoreEvent(ctx, securityInitiator, peerID, accountID, act, map[string]any{
		"security_action": action, "mode": string(security.ModePrevent),
	})
}

// securityHolder carries the anomaly engine so the dashboard can start it
// lazily when a mode is enabled at runtime.
type securityHolder struct {
	mu          sync.Mutex
	engine      *security.Engine
	initialized bool
}

func (s *BaseServer) securityHolder() *securityHolder {
	return Create(s, func() *securityHolder { return &securityHolder{} })
}

// SecurityEngine is the anomaly-detection singleton; nil when no mode is
// configured and none was enabled at runtime.
func (s *BaseServer) SecurityEngine() *security.Engine {
	h := s.securityHolder()
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.initialized {
		h.initialized = true
		h.engine = s.buildSecurityEngine("")
	}
	return h.engine
}

// SetSecurityMode switches the anomaly mode at runtime, building the
// engine on first enable, and persists the choice to security-state.json
// in the datadir so it survives restarts without touching the read-only
// management config file.
func (s *BaseServer) SetSecurityMode(mode string) error {
	switch security.Mode(mode) {
	case security.ModeOff, security.ModeObserve, security.ModePrevent:
	default:
		return fmt.Errorf("invalid mode %q", mode)
	}
	h := s.securityHolder()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.engine == nil && mode != string(security.ModeOff) {
		h.engine = s.buildSecurityEngine(mode)
		if h.engine == nil {
			return fmt.Errorf("failed to start the security engine")
		}
		h.initialized = true
	} else if !h.initialized {
		h.initialized = true
		h.engine = s.buildSecurityEngine("")
	}
	if h.engine != nil {
		if err := h.engine.SetMode(security.Mode(mode)); err != nil {
			return err
		}
	}
	return s.writeSecurityState(mode)
}

func (s *BaseServer) securityStatePath() string {
	return filepath.Join(s.Config.Datadir, "security-state.json")
}

func (s *BaseServer) securityStateMode() string {
	b, err := os.ReadFile(s.securityStatePath())
	if err != nil {
		return ""
	}
	var st struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return ""
	}
	return st.Mode
}

func (s *BaseServer) writeSecurityState(mode string) error {
	b, err := json.Marshal(struct {
		Mode string `json:"mode"`
	}{Mode: mode})
	if err != nil {
		return err
	}
	tmp := s.securityStatePath() + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.securityStatePath())
}

// buildSecurityEngine constructs the engine from the config file, the
// runtime state file and an optional explicit mode. Returns nil when the
// effective mode is off.
func (s *BaseServer) buildSecurityEngine(modeOverride string) *security.Engine {
	sc := s.Config.Security
	mode := modeOverride
	if mode == "" {
		mode = s.securityStateMode()
	}
	if mode == "" && sc != nil {
		mode = sc.Mode
	}
	if mode == "" || mode == string(security.ModeOff) {
		return nil
	}
	{
		cfg := security.DefaultConfig()
		cfg.Mode = security.Mode(mode)
		if sc != nil {
			if sc.LearningPeriod != "" {
				if d, err := time.ParseDuration(sc.LearningPeriod); err == nil {
					cfg.LearningPeriod = d
				}
			}
			for _, c := range sc.TrustedCIDRs {
				if p, err := netip.ParsePrefix(c); err == nil {
					cfg.TrustedCIDRs = append(cfg.TrustedCIDRs, p)
				}
			}
			for name, rc := range sc.Rules {
				if r, ok := cfg.Rules[name]; ok {
					if rc.Enabled != nil {
						r.Enabled = *rc.Enabled
					}
					if rc.Action != "" {
						r.Action = security.Action(rc.Action)
					}
				}
			}
		}
		logPath := ""
		if sc != nil {
			logPath = sc.LogFile
		}
		if logPath == "" {
			logPath = filepath.Join(s.Config.Datadir, "security-events.jsonl")
		}
		fileSink, err := security.NewFileSink(logPath, 0, 0)
		if err != nil {
			log.Warnf("security log file %s unavailable: %v", logPath, err)
		}
		var sink security.Sink
		sinks := security.MultiSink{}
		if fileSink != nil {
			sinks = append(sinks, fileSink)
		}
		if sc != nil && sc.Syslog != nil && sc.Syslog.Enabled {
			ss, err := security.NewSyslogSink(security.SyslogConfig{
				Network: sc.Syslog.Network, Address: sc.Syslog.Address,
				Facility: sc.Syslog.Facility, CAFile: sc.Syslog.CAFile,
				ServerName: sc.Syslog.ServerName, Framing: sc.Syslog.Framing,
			})
			if err != nil {
				log.Warnf("security syslog sink: %v", err)
			} else {
				sinks = append(sinks, ss)
			}
		}
		sinks = append(sinks, activitySinkAdapter{storeEvent: func(ctx context.Context, initiator, target, accountID string, act activity.Activity, meta map[string]any) {
			s.AccountManager().StoreEvent(ctx, initiator, target, accountID, act, meta)
		}})
		sink = sinks

		var geo security.GeoResolver
		if g := s.GeoLocationManager(); g != nil {
			geo = geoAdapter{geo: g}
		}
		auditPath := ""
		if sc != nil {
			auditPath = sc.AuditLogFile
		}
		e := security.NewEngine(cfg, security.SystemClock{}, geo, sink, managementActions{am: s.AccountManager()})
		baselinePath := filepath.Join(s.Config.Datadir, "security-baseline.json")
		if err := e.LoadSnapshot(baselinePath); err != nil {
			log.Warnf("security baseline load: %v", err)
		}
		// periodic save every 5 minutes; stopped with the server
		go e.RunSnapshotLoop(baselinePath, 5*time.Minute)
		log.Infof("security monitoring enabled: mode=%s learning=%s log=%s audit=%s", cfg.Mode, cfg.LearningPeriod, logPath, auditPath)
		return e
	}
}

// registerSecurityAPI mounts the authenticated admin and owner security
// endpoints. The engine getter stays lazy so a nil engine (mode off) still
// serves status and events.
func (s *BaseServer) registerSecurityAPI(router *mux.Router) {
	var logFile string
	if sc := s.Config.Security; sc != nil {
		logFile = sc.LogFile
	}
	securityhandlers.AddEndpoints(s.AccountManager(), securityhandlers.Options{
		Engine:  s.SecurityEngine,
		LogFile: securityhandlers.LogFileForConfig(logFile, s.Config.Datadir),
		SetMode: s.SetSecurityMode,
	}, router)
}
