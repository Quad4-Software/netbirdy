// Package shield serves the admin and owner API for the management-controlled
// proxy shield: the pushed configuration, per-proxy status and unban commands.
package shield

import (
	"encoding/json"
	"io"
	"net/http"
	"net/netip"

	"github.com/gorilla/mux"

	"github.com/netbirdio/netbird/management/server/account"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	nbshield "github.com/netbirdio/netbird/management/server/shield"
	"github.com/netbirdio/netbird/shared/management/http/api"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/proto"
	"github.com/netbirdio/netbird/shared/management/status"
)

// maxBody bounds a shield config update request.
const maxBody = 1 << 16

// StreamPusher is the slice of the proxy gRPC server the shield endpoints
// need: pushing config and commands onto open sync streams and reading the
// per-proxy status cache.
type StreamPusher interface {
	// PushShieldConfig nudges every connected proxy to receive the current
	// shield config immediately. Returns the number of proxies nudged.
	PushShieldConfig() int
	// SendShieldCommand queues cmd for every connected proxy and pushes it
	// live where possible. Returns the number of proxies pushed immediately.
	SendShieldCommand(cmd *proto.ShieldCommand) int
	// ShieldStatuses returns the last reported shield status per proxy id.
	ShieldStatuses() map[string]*proto.ShieldStatus
	// GetConnectedProxies returns the ids of all connected proxies.
	GetConnectedProxies() []string
}

type handler struct {
	accountManager account.Manager
	manager        *nbshield.Manager
	pusher         StreamPusher
}

// AddEndpoints registers the proxy shield endpoints on an authenticated
// router.
func AddEndpoints(accountManager account.Manager, manager *nbshield.Manager, pusher StreamPusher, router *mux.Router) {
	h := &handler{accountManager: accountManager, manager: manager, pusher: pusher}

	router.HandleFunc("/proxy/shield", h.getState).Methods("GET", "OPTIONS")
	router.HandleFunc("/proxy/shield", h.updateConfig).Methods("PUT", "OPTIONS")
	router.HandleFunc("/proxy/shield/unban", h.unban).Methods("POST", "OPTIONS")
}

// requireAdmin gates every shield endpoint on the admin or owner role, since
// these endpoints change traffic enforcement on every connected proxy.
func (h *handler) requireAdmin(r *http.Request) error {
	auth, err := nbcontext.GetUserAuthFromContext(r.Context())
	if err != nil {
		return err
	}
	user, err := h.accountManager.GetUserByID(r.Context(), auth.UserId)
	if err != nil {
		return err
	}
	if user.AccountID != auth.AccountId || !user.HasAdminPower() {
		return status.NewAdminPermissionError()
	}
	return nil
}

// getState returns the managed shield config and the last shield status
// each connected proxy reported.
func (h *handler) getState(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	resp := api.ProxyShieldState{
		Managed: h.manager.Configured(),
		Proxies: []api.ProxyShieldProxy{},
	}
	if cfg := h.manager.Get(); cfg != nil {
		resp.Config = &api.ProxyShieldConfig{
			Mode:               api.ProxyShieldConfigMode(cfg.Mode),
			AllowCidrs:         cfg.AllowCIDRs,
			AiCrawlers:         api.ProxyShieldConfigAiCrawlers(cfg.AICrawlers),
			AllowSearchEngines: cfg.AllowSearchEngines,
			AllowLocal:         cfg.AllowLocal,
		}
	}
	statuses := h.pusher.ShieldStatuses()
	for _, id := range h.pusher.GetConnectedProxies() {
		resp.Proxies = append(resp.Proxies, api.ProxyShieldProxy{
			ProxyId: id,
			Status:  toAPIStatus(statuses[id]),
		})
	}
	util.WriteJSONObject(r.Context(), w, resp)
}

// updateConfig validates, persists and pushes a new shield config to every
// connected proxy.
func (h *handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	var req api.ProxyShieldConfig
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		util.WriteErrorResponse("invalid request body", http.StatusBadRequest, w)
		return
	}
	cfg := &nbshield.Config{
		Mode:               string(req.Mode),
		AllowCIDRs:         req.AllowCidrs,
		AICrawlers:         string(req.AiCrawlers),
		AllowSearchEngines: req.AllowSearchEngines,
		AllowLocal:         req.AllowLocal,
	}
	if err := h.manager.Set(cfg); err != nil {
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
		return
	}

	pushed := h.pusher.PushShieldConfig()
	util.WriteJSONObject(r.Context(), w, api.ProxyShieldUpdateResult{
		Config: req,
		Pushed: pushed,
	})
}

// unban pushes an unban command for one source IP to every connected proxy.
func (h *handler) unban(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	ip := r.URL.Query().Get("ip")
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		util.WriteErrorResponse("invalid ip parameter", http.StatusBadRequest, w)
		return
	}

	pushed := h.pusher.SendShieldCommand(&proto.ShieldCommand{UnbanIp: addr.String()})
	util.WriteJSONObject(r.Context(), w, api.ProxyShieldUnbanResult{
		Ip:     addr.String(),
		Pushed: pushed,
	})
}

// toAPIStatus maps a proto shield status to its API representation; nil in,
// nil out.
func toAPIStatus(st *proto.ShieldStatus) *api.ProxyShieldStatus {
	if st == nil {
		return nil
	}
	return &api.ProxyShieldStatus{
		Mode:           st.GetMode(),
		Banned:         st.GetBanned(),
		ActiveTarpits:  st.GetActiveTarpits(),
		TrackedSources: st.GetTrackedSources(),
		BannedIps:      orEmpty(st.GetBannedIps()),
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
