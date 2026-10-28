package updates

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/internals/modules/updatemirror"
	"github.com/netbirdio/netbird/management/server/activity"
	nbcontext "github.com/netbirdio/netbird/management/server/context"
	"github.com/netbirdio/netbird/management/server/http/middleware/bypass"
	"github.com/netbirdio/netbird/management/server/mock_server"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/shared/auth"
)

const (
	accID = "acc-1"
	admin = "admin-1"
	user  = "user-1"
)

type fakeMirror struct {
	mu     sync.Mutex
	status updatemirror.Status
	polls  int
	block  chan struct{}
}

func (f *fakeMirror) StatusSnapshot() updatemirror.Status { return f.status }

func (f *fakeMirror) PollOnce(ctx context.Context) error {
	f.mu.Lock()
	f.polls++
	f.mu.Unlock()
	if f.block != nil {
		<-f.block
	}
	return nil
}

func (f *fakeMirror) pollCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.polls
}

type fakeUpdater struct {
	applied []string
	err     error
	status  json.RawMessage
	statErr error
}

func (f *fakeUpdater) Status(context.Context) (json.RawMessage, error) { return f.status, f.statErr }

func (f *fakeUpdater) Apply(_ context.Context, v string) error {
	f.applied = append(f.applied, v)
	return f.err
}

type events struct {
	mu   sync.Mutex
	list []activity.ActivityDescriber
	meta []map[string]any
}

func newHarness(t *testing.T, opts Options) (*mux.Router, *events, *handler) {
	t.Helper()
	ev := &events{}
	am := &mock_server.MockAccountManager{
		GetUserByIDFunc: func(_ context.Context, id string) (*types.User, error) {
			switch id {
			case admin:
				return &types.User{Id: id, AccountID: accID, Role: types.UserRoleAdmin}, nil
			case user:
				return &types.User{Id: id, AccountID: accID, Role: types.UserRoleUser}, nil
			case "other-account-admin":
				return &types.User{Id: id, AccountID: "other", Role: types.UserRoleOwner}, nil
			}
			return nil, errors.New("not found")
		},
		GetAccountSettingsFunc: func(context.Context, string, string) (*types.Settings, error) {
			return &types.Settings{AutoUpdateVersion: "latest", AutoUpdateAlways: true}, nil
		},
		StoreEventFunc: func(_ context.Context, _, _, _ string, a activity.ActivityDescriber, meta map[string]any) {
			ev.mu.Lock()
			defer ev.mu.Unlock()
			ev.list = append(ev.list, a)
			ev.meta = append(ev.meta, meta)
		},
	}
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	h := newHandler(am, opts)
	h.register(api)
	return r, ev, h
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

func TestNonAdminForbiddenOnEveryRoute(t *testing.T) {
	r, ev, _ := newHarness(t, Options{Mirror: &fakeMirror{}, Updater: &fakeUpdater{}})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/updates"}, {"POST", "/api/updates/check"}, {"POST", "/api/updates/server"},
	} {
		for _, uid := range []string{user, "other-account-admin"} {
			rec := do(r, tc.method, tc.path, uid, `{"version":"v1.2.3"}`)
			assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s as %s", tc.method, tc.path, uid)
		}
	}
	assert.Empty(t, ev.list)
}

func TestMissingAuthContextRejected(t *testing.T) {
	r, _, _ := newHarness(t, Options{Mirror: &fakeMirror{}, Updater: &fakeUpdater{}})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/updates"}, {"POST", "/api/updates/check"}, {"POST", "/api/updates/server"},
	} {
		rec := do(r, tc.method, tc.path, "", `{"version":"v1.2.3"}`)
		assert.NotEqual(t, http.StatusOK, rec.Code)
		assert.NotEqual(t, http.StatusAccepted, rec.Code)
	}
}

func TestUpdatePathsAreNotAuthBypassed(t *testing.T) {
	for _, p := range []string{"/api/updates", "/api/updates/check", "/api/updates/server"} {
		matched := false
		hit := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { matched = true })
		rec := httptest.NewRecorder()
		got := bypass.ShouldBypass(p, hit, rec, httptest.NewRequest("GET", p, nil))
		assert.False(t, got, p)
		assert.False(t, matched, p)
	}
}

func TestGetFullShape(t *testing.T) {
	m := &fakeMirror{status: updatemirror.Status{Versions: []string{"0.9.0", "1.2.0", "1.10.0"}, Files: 4, Bytes: 1234, LastError: "boom"}}
	up := &fakeUpdater{status: json.RawMessage(`{"state":"idle"}`)}
	r, _, _ := newHarness(t, Options{Mirror: m, Updater: up, Repository: "Quad4-Software/netbirdy", MaxCacheBytes: 99})
	rec := do(r, "GET", "/api/updates", admin, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Mirror.Enabled)
	assert.Equal(t, "1.10.0", got.LatestVersion)
	assert.Equal(t, "https://github.com/Quad4-Software/netbirdy/releases/tag/v1.10.0", got.LatestReleaseURL)
	assert.Equal(t, int64(99), got.Mirror.MaxBytes)
	assert.Equal(t, "boom", got.Mirror.LastError)
	assert.True(t, got.Updater.Available)
	assert.JSONEq(t, `{"state":"idle"}`, string(got.Updater.Status))
	assert.Equal(t, "latest", got.ClientRollout.AutoUpdateVersion)
	assert.True(t, got.ClientRollout.AutoUpdateAlways)
	assert.NotEmpty(t, got.ServerVersion)
}

func TestGetDisabledAndUnreachableUpdater(t *testing.T) {
	r, _, _ := newHarness(t, Options{})
	rec := do(r, "GET", "/api/updates", admin, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got Response
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.False(t, got.Mirror.Enabled)
	assert.NotNil(t, got.Mirror.Versions)
	assert.Empty(t, got.Mirror.Versions)
	assert.False(t, got.Updater.Available)
	assert.Empty(t, got.LatestReleaseURL)

	r, _, _ = newHarness(t, Options{Updater: &fakeUpdater{statErr: errors.New("down")}})
	rec = do(r, "GET", "/api/updates", admin, "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.True(t, got.Updater.Available)
	assert.Equal(t, "unreachable", got.Updater.Error)
}

func TestCheckDisabledConflict(t *testing.T) {
	r, _, _ := newHarness(t, Options{})
	assert.Equal(t, http.StatusConflict, do(r, "POST", "/api/updates/check", admin, "").Code)
}

func TestCheckSingleFlightAndInterval(t *testing.T) {
	m := &fakeMirror{block: make(chan struct{})}
	r, ev, h := newHarness(t, Options{Mirror: m})
	assert.Equal(t, http.StatusAccepted, do(r, "POST", "/api/updates/check", admin, "").Code)
	assert.Equal(t, http.StatusTooManyRequests, do(r, "POST", "/api/updates/check", admin, "").Code)
	close(m.block)
	assert.Eventually(t, func() bool { h.mu.Lock(); defer h.mu.Unlock(); return !h.checking }, time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, m.pollCount())
	require.Len(t, ev.list, 1)
	assert.Equal(t, activity.UpdateCheckRequested, ev.list[0])
}

func TestCheckRateLimitedWithinInterval(t *testing.T) {
	m := &fakeMirror{}
	r, _, _ := newHarness(t, Options{Mirror: m})
	assert.Equal(t, http.StatusAccepted, do(r, "POST", "/api/updates/check", admin, "").Code)
	assert.Equal(t, http.StatusTooManyRequests, do(r, "POST", "/api/updates/check", admin, "").Code)
}

func TestServerUpdateUnconfigured(t *testing.T) {
	r, ev, _ := newHarness(t, Options{})
	rec := do(r, "POST", "/api/updates/server", admin, `{"version":"v1.2.3"}`)
	assert.Equal(t, http.StatusNotImplemented, rec.Code)
	assert.Empty(t, ev.list)
}

func TestServerUpdateValidation(t *testing.T) {
	up := &fakeUpdater{}
	r, ev, _ := newHarness(t, Options{Updater: up})
	for _, body := range []string{``, `{}`, `{"version":""}`, `{"version":"latest"}`, `{"version":"1.2"}`, `{"version":"v1.2.3; rm -rf /"}`, `{"version":"../../x"}`, `not json`} {
		assert.Equal(t, http.StatusBadRequest, do(r, "POST", "/api/updates/server", admin, body).Code, body)
	}
	assert.Empty(t, up.applied)
	assert.Empty(t, ev.list)
}

func TestServerUpdateHappyAndFailure(t *testing.T) {
	up := &fakeUpdater{}
	r, ev, _ := newHarness(t, Options{Updater: up})
	assert.Equal(t, http.StatusAccepted, do(r, "POST", "/api/updates/server", admin, `{"version":"v1.2.3"}`).Code)
	assert.Equal(t, []string{"v1.2.3"}, up.applied)
	require.Len(t, ev.list, 1)
	assert.Equal(t, activity.ServerUpdateRequested, ev.list[0])
	assert.Equal(t, map[string]any{"version": "v1.2.3"}, ev.meta[0])

	up.err = errors.New("nope")
	rec := do(r, "POST", "/api/updates/server", admin, `{"version":"v1.2.4"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Len(t, ev.list, 1)
}

func TestLatestOf(t *testing.T) {
	assert.Equal(t, "", latestOf(nil))
	assert.Equal(t, "2.0.0", latestOf([]string{"1.9.9", "2.0.0", "junk", "1.10.0"}))
	assert.Equal(t, "v1.2.0", latestOf([]string{"v1.2.0", "v1.1.0"}))
}
