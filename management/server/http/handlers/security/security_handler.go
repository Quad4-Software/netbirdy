// Package security serves the admin and owner API for the anomaly-detection
// engine: status, the security event log and the runtime mode switch.
package security

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"

	"github.com/netbirdio/netbird/management/server/account"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	nbsecurity "github.com/netbirdio/netbird/management/server/security"
	"github.com/netbirdio/netbird/shared/management/http/api"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/status"
)

const (
	// maxBody bounds a mode update request.
	maxBody = 1 << 16
	// defaultEventLimit applies when the limit query parameter is absent.
	defaultEventLimit = 200
	// maxEventLimit caps the number of events one request can return.
	maxEventLimit = 1000
)

// Options carries the dependencies the security endpoints need from the
// boot layer.
type Options struct {
	// Engine returns the running anomaly engine, or nil when security is off.
	Engine func() *nbsecurity.Engine
	// LogFile is the security event log path tailed by the events endpoint.
	LogFile string
	// SetMode applies a mode change, starting the engine lazily and
	// persisting the choice so it survives restarts.
	SetMode func(mode string) error
}

type handler struct {
	accountManager account.Manager
	opts           Options
}

// AddEndpoints registers the security endpoints on an authenticated router.
func AddEndpoints(accountManager account.Manager, opts Options, router *mux.Router) {
	h := &handler{accountManager: accountManager, opts: opts}

	router.HandleFunc("/security/status", h.getStatus).Methods("GET", "OPTIONS")
	router.HandleFunc("/security/events", h.getEvents).Methods("GET", "OPTIONS")
	router.HandleFunc("/security/mode", h.updateMode).Methods("PUT", "OPTIONS")
}

// requireAdmin gates every security endpoint on the admin or owner role,
// since these endpoints expose security findings and change enforcement.
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

// getStatus reports the engine mode, learning period, rule state and
// findings counters. A nil engine reports mode "off".
func (h *handler) getStatus(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	resp := api.SecurityStatus{
		Mode:           api.SecurityStatusModeOff,
		LearningPeriod: "",
		Rules:          []api.SecurityRule{},
		FindingsByRule: map[string]int64{},
	}
	engine := h.opts.Engine()
	if engine == nil {
		util.WriteJSONObject(r.Context(), w, resp)
		return
	}
	st := engine.Status()
	resp.Mode = api.SecurityStatusMode(st.Mode)
	resp.LearningPeriod = st.LearningPeriod.String()
	resp.FindingsTotal = st.FindingsTotal
	for _, rule := range st.Rules {
		resp.Rules = append(resp.Rules, api.SecurityRule{
			Name:    rule.Name,
			Enabled: rule.Enabled,
			Action:  api.SecurityRuleAction(rule.Action),
		})
	}
	for name, n := range st.FindingsByRule {
		resp.FindingsByRule[name] = n
	}
	util.WriteJSONObject(r.Context(), w, resp)
}

// getEvents returns the tail of the security event log, newest first.
func (h *handler) getEvents(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	limit := defaultEventLimit
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			util.WriteErrorResponse("invalid limit parameter", http.StatusBadRequest, w)
			return
		}
		if n > maxEventLimit {
			n = maxEventLimit
		}
		limit = n
	}

	events, err := nbsecurity.ReadEventLog(h.opts.LogFile, limit)
	if err != nil {
		util.WriteError(r.Context(), fmt.Errorf("read security event log: %w", err), w)
		return
	}
	resp := api.SecurityEventList{Events: []api.SecurityEvent{}}
	for _, ev := range events {
		resp.Events = append(resp.Events, toAPIEvent(ev))
	}
	util.WriteJSONObject(r.Context(), w, resp)
}

// updateMode switches the engine mode. A running engine takes the change
// immediately; without one the mode is persisted into the management config
// file and applies after restart.
func (h *handler) updateMode(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	var req api.SecurityModeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		util.WriteErrorResponse("invalid request body", http.StatusBadRequest, w)
		return
	}
	mode := nbsecurity.Mode(req.Mode)
	switch mode {
	case nbsecurity.ModeOff, nbsecurity.ModeObserve, nbsecurity.ModePrevent:
	default:
		util.WriteErrorResponse("invalid mode: must be off, observe or prevent", http.StatusBadRequest, w)
		return
	}

	if h.opts.SetMode == nil {
		util.WriteError(r.Context(), fmt.Errorf("security mode updates are not configured"), w)
		return
	}
	if err := h.opts.SetMode(string(mode)); err != nil {
		util.WriteError(r.Context(), fmt.Errorf("set security mode: %w", err), w)
		return
	}
	util.WriteJSONObject(r.Context(), w, api.SecurityModeResponse{
		Mode:    api.SecurityModeResponseMode(mode),
		Applied: true,
		Note:    "persisted to security-state.json",
	})
}

// toAPIEvent maps a security.Event to its API representation.
func toAPIEvent(ev nbsecurity.Event) api.SecurityEvent {
	return api.SecurityEvent{
		Time:         ev.Timestamp.UTC().Format(time.RFC3339),
		Type:         ev.Type,
		Rule:         ev.Rule,
		Severity:     api.SecurityEventSeverity(ev.Severity),
		Action:       string(ev.ActionTaken),
		AccountId:    optionalString(ev.AccountID),
		PeerId:       optionalString(ev.PeerID),
		PeerName:     optionalString(ev.PeerName),
		UserId:       optionalString(ev.UserID),
		SrcIp:        optionalString(ev.SrcIP),
		Reason:       optionalString(ev.Reason),
		WouldPrevent: ev.WouldPrevent,
	}
}

func optionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// LogFileForConfig resolves the event log path the way the engine does:
// the configured override or <dataDir>/security-events.jsonl.
func LogFileForConfig(logFile, dataDir string) string {
	if logFile != "" {
		return logFile
	}
	return filepath.Join(dataDir, "security-events.jsonl")
}
