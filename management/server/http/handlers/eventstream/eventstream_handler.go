// Package eventstream serves the admin API for activity event streaming
// integrations (generic HTTP webhooks and Datadog).
package eventstream

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"

	nbcontext "github.com/netbirdio/netbird/management/server/context"
	nbeventstream "github.com/netbirdio/netbird/management/server/eventstream"
	"github.com/netbirdio/netbird/management/server/permissions"
	"github.com/netbirdio/netbird/management/server/permissions/modules"
	"github.com/netbirdio/netbird/management/server/permissions/operations"
	"github.com/netbirdio/netbird/shared/management/http/api"
	"github.com/netbirdio/netbird/shared/management/http/util"
	"github.com/netbirdio/netbird/shared/management/status"
)

const (
	// maxBody bounds an integration create or update request.
	maxBody = 1 << 16
)

type handler struct {
	permissionsManager permissions.Manager
	manager            *nbeventstream.Manager
}

// AddEndpoints registers the event streaming endpoints on an authenticated
// router.
func AddEndpoints(permissionsManager permissions.Manager, manager *nbeventstream.Manager, router *mux.Router) {
	h := &handler{permissionsManager: permissionsManager, manager: manager}

	router.HandleFunc("/integrations/event-streaming", h.list).Methods("GET", "OPTIONS")
	router.HandleFunc("/integrations/event-streaming", h.create).Methods("POST", "OPTIONS")
	router.HandleFunc("/integrations/event-streaming/{id}", h.update).Methods("PUT", "OPTIONS")
	router.HandleFunc("/integrations/event-streaming/{id}", h.delete).Methods("DELETE", "OPTIONS")
}

// requirePermission resolves the caller auth and checks the event_streaming
// module permission for the operation, returning the account ID on success.
func (h *handler) requirePermission(w http.ResponseWriter, r *http.Request, op operations.Operation) (string, bool) {
	userAuth, err := nbcontext.GetUserAuthFromContext(r.Context())
	if err != nil {
		util.WriteError(r.Context(), err, w)
		return "", false
	}
	allowed, _, err := h.permissionsManager.ValidateUserPermissions(r.Context(), userAuth.AccountId, userAuth.UserId, modules.EventStreaming, op)
	if err != nil {
		util.WriteError(r.Context(), status.NewPermissionValidationError(err), w)
		return "", false
	}
	if !allowed {
		util.WriteError(r.Context(), status.NewPermissionDeniedError(), w)
		return "", false
	}
	return userAuth.AccountId, true
}

// list returns the account's integrations with secrets masked.
func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	accountID, ok := h.requirePermission(w, r, operations.Read)
	if !ok {
		return
	}

	integrations := h.manager.List(accountID)
	resp := make([]api.EventStream, 0, len(integrations))
	for _, i := range integrations {
		resp = append(resp, toAPI(i))
	}
	util.WriteJSONObject(r.Context(), w, resp)
}

// create validates and stores a new integration.
func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	accountID, ok := h.requirePermission(w, r, operations.Create)
	if !ok {
		return
	}

	var req api.EventStreamRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		util.WriteErrorResponse("invalid request body", http.StatusBadRequest, w)
		return
	}

	in, err := h.manager.Create(r.Context(), accountID, string(req.Platform), req.Enabled, *fromAPIConfig(&req.Config))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	util.WriteJSONObject(r.Context(), w, toAPI(in))
}

// update modifies the config or enabled state of an integration. Empty or
// masked secret fields keep the stored values.
func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	accountID, ok := h.requirePermission(w, r, operations.Update)
	if !ok {
		return
	}

	id, err := pathID(r)
	if err != nil {
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
		return
	}

	var req api.EventStreamUpdateRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBody)).Decode(&req); err != nil {
		util.WriteErrorResponse("invalid request body", http.StatusBadRequest, w)
		return
	}

	in, err := h.manager.Update(r.Context(), accountID, id, req.Enabled, fromAPIConfig(req.Config))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	util.WriteJSONObject(r.Context(), w, toAPI(in))
}

// delete removes an integration.
func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	accountID, ok := h.requirePermission(w, r, operations.Delete)
	if !ok {
		return
	}

	id, err := pathID(r)
	if err != nil {
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
		return
	}

	if err := h.manager.Delete(r.Context(), accountID, id); err != nil {
		h.writeError(w, r, err)
		return
	}
	util.WriteJSONObject(r.Context(), w, util.EmptyObject{})
}

// pathID parses the numeric {id} path variable.
func pathID(r *http.Request) (int64, error) {
	raw := mux.Vars(r)["id"]
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid integration ID")
	}
	return id, nil
}

// writeError maps manager errors to HTTP statuses: unsupported platforms are
// 501, unknown IDs 404, and validation failures 400.
func (h *handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, nbeventstream.ErrPlatformNotSupported):
		util.WriteErrorResponse(err.Error(), http.StatusNotImplemented, w)
	case errors.Is(err, nbeventstream.ErrNotFound):
		util.WriteError(r.Context(), status.Errorf(status.NotFound, "%s", err.Error()), w)
	default:
		util.WriteErrorResponse(err.Error(), http.StatusBadRequest, w)
	}
}

// toAPI maps an integration to its API representation.
func toAPI(i *nbeventstream.Integration) api.EventStream {
	return api.EventStream{
		Id:        i.ID,
		AccountId: i.AccountID,
		Enabled:   i.Enabled,
		Platform:  api.EventStreamPlatform(i.Platform),
		CreatedAt: i.CreatedAt,
		UpdatedAt: i.UpdatedAt,
		Config:    toAPIConfig(i.Config),
	}
}

// toAPIConfig maps the stored config to the API shape. Unset optional fields
// stay absent rather than serialized as empty strings.
func toAPIConfig(c nbeventstream.Config) api.EventStreamConfig {
	out := api.EventStreamConfig{
		ApiKey: c.APIKey,
		ApiUrl: c.APIURL,
	}
	if c.URL != "" {
		out.Url = &c.URL
	}
	if c.Headers != "" {
		out.Headers = &c.Headers
	}
	if c.BodyTemplate != "" {
		out.BodyTemplate = &c.BodyTemplate
	}
	return out
}

// fromAPIConfig maps the request config to the stored shape.
func fromAPIConfig(c *api.EventStreamConfig) *nbeventstream.Config {
	if c == nil {
		return nil
	}
	return &nbeventstream.Config{
		URL:          deref(c.Url),
		Headers:      deref(c.Headers),
		BodyTemplate: deref(c.BodyTemplate),
		APIKey:       c.ApiKey,
		APIURL:       c.ApiUrl,
	}
}

// deref unwraps an optional string field.
func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
