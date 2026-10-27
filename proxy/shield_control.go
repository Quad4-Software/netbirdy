package proxy

import (
	"github.com/netbirdio/netbird/proxy/internal/shield"
	"github.com/netbirdio/netbird/shared/management/proto"
)

// applyShieldConfig merges a management-pushed shield config onto the
// running shield configuration. When the proxy runs without a local shield
// the push is ignored: the request chain was built without the shield
// middleware, so a runtime enable would have no effect anyway.
func (s *Server) applyShieldConfig(sc *proto.ShieldConfig) {
	if sc == nil {
		return
	}
	if s.shield == nil {
		s.Logger.Debug("ignoring pushed shield config: shield not enabled locally")
		return
	}
	cfg := s.shield.Config()
	if sc.GetMode() != "" {
		cfg.Mode = shield.Mode(sc.GetMode())
	}
	if sc.GetAiCrawlers() != "" {
		cfg.AICrawlers = shield.AICrawlerPolicy(sc.GetAiCrawlers())
	}
	// allow_cidrs is a full snapshot: an empty list clears the local ranges.
	cfg.AllowCIDRs = sc.GetAllowCidrs()
	cfg.AllowSearchEngines = sc.GetAllowSearchEngines()
	allowLocal := sc.GetAllowLocal()
	cfg.AllowLocal = &allowLocal
	if err := s.shield.ApplyConfig(cfg); err != nil {
		s.Logger.WithError(err).Warn("rejected pushed shield config")
	}
}

// applyShieldCommands runs imperative shield actions pushed by management
// over the sync stream.
func (s *Server) applyShieldCommands(cmds []*proto.ShieldCommand) {
	for _, cmd := range cmds {
		ip := cmd.GetUnbanIp()
		if ip == "" {
			continue
		}
		if s.shield == nil {
			s.Logger.Debug("ignoring shield unban command: shield not enabled locally")
			continue
		}
		if s.shield.UnbanIP(ip) {
			s.Logger.Debugf("shield: unbanned %s on management command", ip)
		}
	}
}

// shieldStatusProto snapshots the local shield state for the sync-stream
// ack. A proxy without a local shield reports mode "off" so management can
// still display it.
func (s *Server) shieldStatusProto() *proto.ShieldStatus {
	st := &proto.ShieldStatus{Mode: string(shield.ModeOff)}
	if s.shield == nil {
		return st
	}
	stats := s.shield.Stats()
	st.Mode = s.shield.Mode()
	st.Banned = int64(stats.Banned)
	st.ActiveTarpits = int64(stats.ActiveTarpits)
	st.TrackedSources = int64(stats.Tracked)
	for _, b := range s.shield.Bans() {
		st.BannedIps = append(st.BannedIps, b.IP)
	}
	return st
}
