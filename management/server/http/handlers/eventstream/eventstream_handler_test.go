package eventstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"go.uber.org/mock/gomock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nbcontext "github.com/netbirdio/netbird/management/server/context"
	nbeventstream "github.com/netbirdio/netbird/management/server/eventstream"
	"github.com/netbirdio/netbird/management/server/permissions"
	"github.com/netbirdio/netbird/management/server/permissions/modules"
	"github.com/netbirdio/netbird/management/server/permissions/operations"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/netbirdio/netbird/shared/management/http/api"
)

const (
	accID   = "acc-1"
	adminID = "admin-1"
	userID  = "user-1"
)

// newHarness wires the endpoints with a permissions mock that allows every
// event_streaming operation for adminID and denies it for anyone else.
func newHarness(t *testing.T) (*mux.Router, *nbeventstream.Manager) {
	t.Helper()

	ctrl := gomock.NewController(t)
	perms := permissions.NewMockManager(ctrl)
	perms.EXPECT().
		ValidateUserPermissions(gomock.Any(), gomock.Eq(accID), gomock.Any(), gomock.Eq(modules.EventStreaming), gomock.Any()).
		DoAndReturn(func(ctx context.Context, accountID, uid string, module modules.Module, op operations.Operation) (bool, context.Context, error) {
			return uid == adminID, ctx, nil
		}).
		AnyTimes()

	mgr := nbeventstream.NewManager(t.TempDir())
	router := mux.NewRouter()
	AddEndpoints(perms, mgr, router.PathPrefix("/api").Subrouter())
	return router, mgr
}

func do(r http.Handler, method, path, uid, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if uid != "" {
		req = nbcontext.SetUserAuthInRequest(req, auth.UserAuth{UserId: uid, AccountId: accID})
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestDeniedWithoutPermission(t *testing.T) {
	r, _ := newHarness(t)
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/integrations/event-streaming"},
		{"POST", "/api/integrations/event-streaming"},
		{"PUT", "/api/integrations/event-streaming/1"},
		{"DELETE", "/api/integrations/event-streaming/1"},
	} {
		rec := do(r, tc.method, tc.path, userID, `{}`)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s", tc.method, tc.path)
	}
}

func TestCreateListUpdateDelete(t *testing.T) {
	r, _ := newHarness(t)

	rec := do(r, "POST", "/api/integrations/event-streaming", adminID, `{
		"platform": "generic_http",
		"enabled": true,
		"config": {"url": "https://receiver.internal/events", "headers": "{\"X-API-Key\":\"sekret\"}"}
	}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var created api.EventStream
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, api.EventStreamPlatformGenericHttp, created.Platform)
	require.NotNil(t, created.Config.Headers)
	assert.Contains(t, *created.Config.Headers, "********")
	assert.NotContains(t, *created.Config.Headers, "sekret")

	rec = do(r, "GET", "/api/integrations/event-streaming", adminID, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var list []api.EventStream
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.True(t, list[0].Enabled)
	require.NotNil(t, list[0].Config.Url)
	assert.Equal(t, "https://receiver.internal/events", *list[0].Config.Url)

	off := false
	rec = do(r, "PUT", "/api/integrations/event-streaming/1", adminID, `{"enabled": false}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var updated api.EventStream
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &updated))
	assert.Equal(t, off, updated.Enabled)

	rec = do(r, "DELETE", "/api/integrations/event-streaming/1", adminID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = do(r, "GET", "/api/integrations/event-streaming", adminID, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	assert.Empty(t, list)
}

func TestCreateDatadogMasksAPIKey(t *testing.T) {
	r, _ := newHarness(t)

	rec := do(r, "POST", "/api/integrations/event-streaming", adminID, `{
		"platform": "datadog",
		"enabled": true,
		"config": {"api_key": "dd-secret", "api_url": "https://http-intake.logs.datadoghq.com/api/v2/logs"}
	}`)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var created api.EventStream
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	assert.Equal(t, "********", created.Config.ApiKey)
	assert.Equal(t, "https://http-intake.logs.datadoghq.com/api/v2/logs", created.Config.ApiUrl)
	assert.NotContains(t, rec.Body.String(), "dd-secret")
}

func TestCreateUnsupportedPlatformsReturn501(t *testing.T) {
	r, _ := newHarness(t)

	for _, platform := range []string{"s3", "firehose"} {
		rec := do(r, "POST", "/api/integrations/event-streaming", adminID, `{
			"platform": "`+platform+`",
			"enabled": true,
			"config": {"api_key": "", "api_url": ""}
		}`)
		assert.Equal(t, http.StatusNotImplemented, rec.Code, "platform %s", platform)
		assert.Contains(t, rec.Body.String(), "not supported")
	}
}

func TestUpdateAndDeleteUnknownIDReturn404(t *testing.T) {
	r, _ := newHarness(t)

	rec := do(r, "PUT", "/api/integrations/event-streaming/99", adminID, `{"enabled": true}`)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec = do(r, "DELETE", "/api/integrations/event-streaming/99", adminID, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestInvalidIDAndBodyReturn400(t *testing.T) {
	r, _ := newHarness(t)

	rec := do(r, "PUT", "/api/integrations/event-streaming/abc", adminID, `{}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r, "POST", "/api/integrations/event-streaming", adminID, `not json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	rec = do(r, "POST", "/api/integrations/event-streaming", adminID, `{
		"platform": "generic_http", "enabled": true, "config": {"api_key": "", "api_url": ""}
	}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "config.url")
}

func TestMissingAuthContextRejected(t *testing.T) {
	r, _ := newHarness(t)
	rec := do(r, "GET", "/api/integrations/event-streaming", "", "")
	assert.NotEqual(t, http.StatusOK, rec.Code)
}

func TestPermissionErrorSurfaces(t *testing.T) {
	ctrl := gomock.NewController(t)
	perms := permissions.NewMockManager(ctrl)
	perms.EXPECT().
		ValidateUserPermissions(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(false, context.Background(), errors.New("store down")).
		AnyTimes()

	mgr := nbeventstream.NewManager(t.TempDir())
	router := mux.NewRouter()
	AddEndpoints(perms, mgr, router.PathPrefix("/api").Subrouter())

	rec := do(router, "GET", "/api/integrations/event-streaming", adminID, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}
