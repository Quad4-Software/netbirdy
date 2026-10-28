// Package updates serves the owner and admin API for server and client updates.
package updates

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/mux"
	goversion "github.com/hashicorp/go-version"
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/management/internals/modules/updatemirror"
	"github.com/netbirdio/netbird/management/server/account"
	"github.com/netbirdio/netbird/management/server/activity"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/status"
	"github.com/netbirdio/netbird/version"
)

const (
	checkInterval  = 30 * time.Second
	checkTimeout   = 5 * time.Minute
	updaterTimeout = 10 * time.Second
	maxBody        = 1 << 20
	defaultRepo    = "Quad4-Software/netbirdy"
)

var versionRe = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.]+)?$`)

// MirrorSource is the part of the update mirror the API needs.
type MirrorSource interface {
	StatusSnapshot() updatemirror.Status
	PollOnce(ctx context.Context) error
}

// Updater is the sidecar updater client.
type Updater interface {
	Status(ctx context.Context) (json.RawMessage, error)
	Apply(ctx context.Context, version string) error
}

// Options configures the handler.
type Options struct {
	Mirror        MirrorSource
	Updater       Updater
	Repository    string
	MaxCacheBytes int64
}

type handler struct {
	accountManager account.Manager
	opts           Options

	mu        sync.Mutex
	lastCheck time.Time
	checking  bool
	now       func() time.Time
}

// AddEndpoints registers the update endpoints on an authenticated router.
func AddEndpoints(accountManager account.Manager, router *mux.Router, opts Options) {
	newHandler(accountManager, opts).register(router)
}

func newHandler(accountManager account.Manager, opts Options) *handler {
	if opts.Repository == "" {
		opts.Repository = defaultRepo
	}
	return &handler{accountManager: accountManager, opts: opts, now: time.Now}
}

func (h *handler) register(router *mux.Router) {
	router.HandleFunc("/updates", h.get).Methods("GET", "OPTIONS")
	router.HandleFunc("/updates/check", h.check).Methods("POST", "OPTIONS")
	router.HandleFunc("/updates/server", h.applyServer).Methods("POST", "OPTIONS")
}

type mirrorInfo struct {
	Enabled   bool      `json:"enabled"`
	Versions  []string  `json:"versions"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	MaxBytes  int64     `json:"max_bytes"`
	LastPoll  time.Time `json:"last_poll"`
	NextPoll  time.Time `json:"next_poll"`
	LastError string    `json:"last_error,omitempty"`
}

type updaterInfo struct {
	Available bool            `json:"available"`
	Status    json.RawMessage `json:"status,omitempty"`
	Error     string          `json:"error,omitempty"`
}

type rollout struct {
	AutoUpdateVersion string `json:"auto_update_version"`
	AutoUpdateAlways  bool   `json:"auto_update_always"`
}

// Response is the body of GET /api/updates.
type Response struct {
	ServerVersion    string      `json:"server_version"`
	LatestVersion    string      `json:"latest_version"`
	LatestReleaseURL string      `json:"latest_release_url"`
	Mirror           mirrorInfo  `json:"mirror"`
	Updater          updaterInfo `json:"updater"`
	ClientRollout    rollout     `json:"client_rollout"`
}

func (h *handler) requireAdmin(r *http.Request) (accountID, userID string, err error) {
	auth, err := nbcontext.GetUserAuthFromContext(r.Context())
	if err != nil {
		return "", "", err
	}
	user, err := h.accountManager.GetUserByID(r.Context(), auth.UserId)
	if err != nil {
		return "", "", err
	}
	if user.AccountID != auth.AccountId || !user.HasAdminPower() {
		return "", "", status.NewAdminPermissionError()
	}
	return auth.AccountId, auth.UserId, nil
}

func latestOf(versions []string) string {
	var best *goversion.Version
	bestRaw := ""
	for _, v := range versions {
		pv, err := goversion.NewVersion(v)
		if err != nil {
			continue
		}
		if best == nil || pv.GreaterThan(best) {
			best, bestRaw = pv, v
		}
	}
	return bestRaw
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	accountID, userID, err := h.requireAdmin(r)
	if err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	resp := Response{ServerVersion: version.NetbirdVersion(), Mirror: mirrorInfo{Versions: []string{}}}
	if h.opts.Mirror != nil {
		st := h.opts.Mirror.StatusSnapshot()
		resp.Mirror = mirrorInfo{
			Enabled: true, Versions: append([]string{}, st.Versions...), Files: st.Files, Bytes: st.Bytes,
			MaxBytes: h.opts.MaxCacheBytes, LastPoll: st.LastPoll, NextPoll: st.NextPoll, LastError: st.LastError,
		}
		resp.LatestVersion = latestOf(st.Versions)
	}
	if resp.LatestVersion != "" {
		tag := "v" + strings.TrimPrefix(resp.LatestVersion, "v")
		resp.LatestReleaseURL = fmt.Sprintf("https://github.com/%s/releases/tag/%s", h.opts.Repository, tag)
	}

	if settings, err := h.accountManager.GetAccountSettings(r.Context(), accountID, userID); err == nil && settings != nil {
		resp.ClientRollout = rollout{AutoUpdateVersion: settings.AutoUpdateVersion, AutoUpdateAlways: settings.AutoUpdateAlways}
	}

	if h.opts.Updater != nil {
		resp.Updater.Available = true
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		raw, err := h.opts.Updater.Status(ctx)
		switch {
		case err != nil:
			resp.Updater.Error = "unreachable"
		case json.Valid(raw):
			resp.Updater.Status = raw
		default:
			resp.Updater.Error = "invalid status"
		}
	}

	util.WriteJSONObject(r.Context(), w, resp)
}

func (h *handler) check(w http.ResponseWriter, r *http.Request) {
	accountID, userID, err := h.requireAdmin(r)
	if err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}
	if h.opts.Mirror == nil {
		util.WriteErrorResponse("update mirror is not enabled", http.StatusConflict, w)
		return
	}

	h.mu.Lock()
	if h.checking || h.now().Sub(h.lastCheck) < checkInterval {
		h.mu.Unlock()
		util.WriteErrorResponse("an update check ran recently, try again shortly", http.StatusTooManyRequests, w)
		return
	}
	h.checking = true
	h.lastCheck = h.now()
	h.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		if err := h.opts.Mirror.PollOnce(ctx); err != nil {
			log.Warnf("manual update check failed: %v", err)
		}
		h.mu.Lock()
		h.checking = false
		h.mu.Unlock()
	}()

	h.accountManager.StoreEvent(r.Context(), userID, accountID, accountID, activity.UpdateCheckRequested, nil)
	w.WriteHeader(http.StatusAccepted)
}

type applyRequest struct {
	Version string `json:"version"`
}

func (h *handler) applyServer(w http.ResponseWriter, r *http.Request) {
	accountID, userID, err := h.requireAdmin(r)
	if err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}
	if h.opts.Updater == nil {
		util.WriteErrorResponse("no updater is configured, see the updating guide", http.StatusNotImplemented, w)
		return
	}

	var req applyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil || !versionRe.MatchString(req.Version) {
		util.WriteErrorResponse("a valid version such as v1.2.3 is required", http.StatusBadRequest, w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), updaterTimeout)
	defer cancel()
	if err := h.opts.Updater.Apply(ctx, req.Version); err != nil {
		log.WithContext(r.Context()).Warnf("server update request failed: %v", err)
		util.WriteErrorResponse("the updater rejected or could not start the update", http.StatusBadGateway, w)
		return
	}

	h.accountManager.StoreEvent(r.Context(), userID, accountID, accountID, activity.ServerUpdateRequested, map[string]any{"version": req.Version})
	w.WriteHeader(http.StatusAccepted)
}
