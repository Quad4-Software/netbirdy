// Package backup serves the admin and owner API for encrypted S3 backups of
// the management store.
package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/netbirdio/netbird/management/server/account"
	nbbackup "github.com/netbirdio/netbird/management/server/backup"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	"github.com/netbirdio/netbird/shared/management/http/api"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/status"
)

const (
	// maxBody bounds a config update request.
	maxBody = 1 << 16
	// runTimeout bounds a manual backup run triggered over the API.
	runTimeout = 30 * time.Minute
)

type handler struct {
	accountManager account.Manager
	backup         *nbbackup.Manager
}

// AddEndpoints registers the backup endpoints on an authenticated router.
func AddEndpoints(accountManager account.Manager, backupManager *nbbackup.Manager, router *mux.Router) {
	h := &handler{accountManager: accountManager, backup: backupManager}

	router.HandleFunc("/server/backup", h.getState).Methods("GET", "OPTIONS")
	router.HandleFunc("/server/backup", h.updateConfig).Methods("PUT", "OPTIONS")
	router.HandleFunc("/server/backup/run", h.run).Methods("POST", "OPTIONS")
	router.HandleFunc("/server/backup/snapshots", h.listSnapshots).Methods("GET", "OPTIONS")
}

// requireAdmin gates every backup endpoint on the admin or owner role, since
// these endpoints manage server-level state and can trigger uploads.
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

// getState returns the effective backup config, secrets masked, and the
// scheduler status.
func (h *handler) getState(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}
	util.WriteJSONObject(r.Context(), w, toState(h.backup.Status()))
}

// updateConfig validates and persists a new backup config. Empty or masked
// secret fields keep the stored values.
func (h *handler) updateConfig(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	var req api.ServerBackupConfig
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		util.WriteErrorResponse("invalid request body", http.StatusBadRequest, w)
		return
	}

	cfg := nbbackup.Config{
		S3Endpoint:    req.S3Endpoint,
		S3Region:      req.S3Region,
		S3Bucket:      req.S3Bucket,
		S3AccessKey:   req.S3AccessKey,
		S3SecretKey:   req.S3SecretKey,
		S3Prefix:      req.S3Prefix,
		Interval:      req.Interval,
		EncryptionKey: req.EncryptionKey,
	}
	if err := h.backup.UpdateConfig(r.Context(), cfg); err != nil {
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
		return
	}

	util.WriteJSONObject(r.Context(), w, toState(h.backup.Status()))
}

// run triggers an immediate backup run and returns the stored object key.
func (h *handler) run(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), runTimeout)
	defer cancel()

	result, err := h.backup.RunNow(ctx)
	if err != nil {
		writeRunError(r, err, w)
		return
	}
	util.WriteJSONObject(r.Context(), w, api.ServerBackupRunResult{Key: result.Key, Size: result.Size})
}

// listSnapshots returns every snapshot object under the configured prefix.
func (h *handler) listSnapshots(w http.ResponseWriter, r *http.Request) {
	if err := h.requireAdmin(r); err != nil {
		util.WriteError(r.Context(), err, w)
		return
	}

	objects, err := h.backup.ListSnapshots(r.Context())
	if err != nil {
		writeRunError(r, err, w)
		return
	}

	resp := api.ServerBackupSnapshotList{Snapshots: []api.ServerBackupSnapshot{}}
	for _, obj := range objects {
		resp.Snapshots = append(resp.Snapshots, api.ServerBackupSnapshot{
			Key:          obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified.UTC().Format(time.RFC3339),
		})
	}
	util.WriteJSONObject(r.Context(), w, resp)
}

// toState maps the manager status to the API response shape.
func toState(st nbbackup.Status) api.ServerBackupState {
	cfg := st.Config
	return api.ServerBackupState{
		Config: api.ServerBackupConfig{
			S3Endpoint:    cfg.S3Endpoint,
			S3Region:      cfg.S3Region,
			S3Bucket:      cfg.S3Bucket,
			S3AccessKey:   cfg.S3AccessKey,
			S3SecretKey:   cfg.S3SecretKey,
			S3Prefix:      cfg.S3Prefix,
			Interval:      cfg.Interval,
			EncryptionKey: cfg.EncryptionKey,
		},
		Status: api.ServerBackupStatus{
			Enabled:      st.Enabled,
			Running:      st.Running,
			LastRun:      formatTime(st.LastRun),
			LastError:    st.LastError,
			LastSnapshot: st.LastSnapshot,
			NextRun:      formatTime(st.NextRun),
		},
	}
}

// formatTime renders t as RFC 3339 or empty when zero.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// writeRunError maps run failures to an HTTP status: configuration problems
// are client errors, storage and snapshot failures are server errors.
func writeRunError(r *http.Request, err error, w http.ResponseWriter) {
	switch {
	case errors.Is(err, nbbackup.ErrNotConfigured), errors.Is(err, nbbackup.ErrNoEncryptionKey):
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
	case errors.Is(err, nbbackup.ErrRunInProgress):
		util.WriteErrorResponse(err.Error(), http.StatusConflict, w)
	default:
		util.WriteErrorResponse(err.Error(), http.StatusInternalServerError, w)
	}
}
