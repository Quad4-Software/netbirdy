package backup

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/server/account"
	nbbackup "github.com/netbirdio/netbird/management/server/backup"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	"github.com/netbirdio/netbird/management/server/mock_server"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/shared/auth"
	"github.com/netbirdio/netbird/shared/management/http/api"
)

const (
	accID      = "acc-1"
	adminID    = "admin-1"
	regularID  = "user-1"
	otherAdmin = "other-account-admin"
)

func newHarness(t *testing.T) (*mux.Router, *nbbackup.Manager, string) {
	t.Helper()
	dir := t.TempDir()
	am := &mock_server.MockAccountManager{
		GetUserByIDFunc: func(_ context.Context, id string) (*types.User, error) {
			switch id {
			case adminID:
				return &types.User{Id: id, AccountID: accID, Role: types.UserRoleAdmin}, nil
			case regularID:
				return &types.User{Id: id, AccountID: accID, Role: types.UserRoleUser}, nil
			case otherAdmin:
				return &types.User{Id: id, AccountID: "other", Role: types.UserRoleOwner}, nil
			}
			return nil, errors.New("not found")
		},
	}
	mgr := nbbackup.NewManager(dir, &nbbackup.UnsupportedSnapshotter{Err: errors.New("no snapshotter")})
	var _ account.Manager = am
	router := mux.NewRouter()
	AddEndpoints(am, mgr, router.PathPrefix("/api").Subrouter())
	return router, mgr, dir
}

func do(r http.Handler, method, path, userID, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if userID != "" {
		req = nbcontext.SetUserAuthInRequest(req, auth.UserAuth{UserId: userID, AccountId: accID})
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func routes() []struct{ method, path string } {
	return []struct{ method, path string }{
		{"GET", "/api/server/backup"},
		{"PUT", "/api/server/backup"},
		{"POST", "/api/server/backup/run"},
		{"GET", "/api/server/backup/snapshots"},
	}
}

func TestNonAdminForbiddenOnEveryRoute(t *testing.T) {
	r, _, _ := newHarness(t)
	for _, tc := range routes() {
		for _, uid := range []string{regularID, otherAdmin} {
			rec := do(r, tc.method, tc.path, uid, `{}`)
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s as %s", tc.method, tc.path, uid)
		}
	}
}

func TestMissingAuthContextRejected(t *testing.T) {
	r, _, _ := newHarness(t)
	for _, tc := range routes() {
		rec := do(r, tc.method, tc.path, "", `{}`)
		assert.NotEqual(t, http.StatusOK, rec.Code, "%s %s without auth", tc.method, tc.path)
	}
}

func TestGetStateMasksSecrets(t *testing.T) {
	r, mgr, _ := newHarness(t)
	require.NoError(t, mgr.UpdateConfig(context.Background(), nbbackup.Config{
		S3Bucket:      "b",
		S3AccessKey:   "ak",
		S3SecretKey:   "sk",
		EncryptionKey: "ek",
		Interval:      "6h",
	}))

	rec := do(r, "GET", "/api/server/backup", adminID, "")
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ServerBackupState
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.True(t, resp.Status.Enabled)
	assert.Equal(t, "b", resp.Config.S3Bucket)
	assert.Equal(t, "6h", resp.Config.Interval)
	for _, secret := range []string{resp.Config.S3AccessKey, resp.Config.S3SecretKey, resp.Config.EncryptionKey} {
		assert.Equal(t, "********", secret)
	}
	assert.NotContains(t, rec.Body.String(), `"sk"`)
	assert.NotContains(t, rec.Body.String(), `"ek"`)
}

func TestUpdateConfigKeepsSecretsAndPersists(t *testing.T) {
	r, mgr, dir := newHarness(t)
	require.NoError(t, mgr.UpdateConfig(context.Background(), nbbackup.Config{
		S3Bucket:      "b",
		S3SecretKey:   "sk",
		EncryptionKey: "ek",
	}))

	rec := do(r, "PUT", "/api/server/backup", adminID, `{"s3_bucket":"b2","s3_region":"eu-1","interval":"12h"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp api.ServerBackupState
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, "b2", resp.Config.S3Bucket)
	assert.Equal(t, "eu-1", resp.Config.S3Region)
	assert.Equal(t, "12h", resp.Config.Interval)
	// Empty secret fields in the request kept the stored secrets, which the
	// persisted config file still holds in cleartext.
	persisted, err := os.ReadFile(filepath.Join(dir, "backup.json"))
	require.NoError(t, err)
	var stored nbbackup.Config
	require.NoError(t, json.Unmarshal(persisted, &stored))
	assert.Equal(t, "sk", stored.S3SecretKey)
	assert.Equal(t, "ek", stored.EncryptionKey)
}

func TestUpdateConfigRejectsBadInterval(t *testing.T) {
	r, _, _ := newHarness(t)
	rec := do(r, "PUT", "/api/server/backup", adminID, `{"s3_bucket":"b","interval":"every day"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestRunWithoutConfigurationIsBadRequest(t *testing.T) {
	r, _, _ := newHarness(t)
	rec := do(r, "POST", "/api/server/backup/run", adminID, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "not configured")
}

func TestRunWithoutEncryptionKeyIsBadRequest(t *testing.T) {
	r, mgr, _ := newHarness(t)
	require.NoError(t, mgr.UpdateConfig(context.Background(), nbbackup.Config{S3Bucket: "b"}))
	rec := do(r, "POST", "/api/server/backup/run", adminID, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), nbbackup.EnvEncryptionKey)
}
