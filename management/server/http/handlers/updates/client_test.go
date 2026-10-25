package updates

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHTTPUpdaterRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "ftp://x", "file:///etc/passwd", "http://", "//host", "host:8089", "javascript:alert(1)"} {
		_, err := NewHTTPUpdater(u, "t")
		assert.Error(t, err, u)
	}
	_, err := NewHTTPUpdater("http://netbirdy-updater:8089/", "t")
	assert.NoError(t, err)
}

func TestHTTPUpdaterApplyAndStatus(t *testing.T) {
	var gotAuth, gotBody, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.Method + " " + r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if r.URL.Path == "/v1/status" {
			_, _ = w.Write([]byte(`{"state":"idle"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	u, err := NewHTTPUpdater(srv.URL+"/", "secret")
	require.NoError(t, err)

	raw, err := u.Status(context.Background())
	require.NoError(t, err)
	assert.JSONEq(t, `{"state":"idle"}`, string(raw))
	assert.Equal(t, "GET /v1/status", gotPath)
	assert.Equal(t, "Bearer secret", gotAuth)

	require.NoError(t, u.Apply(context.Background(), "v1.2.3"))
	assert.Equal(t, "POST /v1/update", gotPath)
	assert.JSONEq(t, `{"version":"v1.2.3"}`, gotBody)
}

func TestHTTPUpdaterErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status":
			_, _ = w.Write([]byte(strings.Repeat("x", maxBody+10)))
		case "/v1/update":
			http.Error(w, "no", http.StatusForbidden)
		}
	}))
	defer srv.Close()
	u, err := NewHTTPUpdater(srv.URL, "")
	require.NoError(t, err)

	_, err = u.Status(context.Background())
	assert.ErrorContains(t, err, "too large")
	assert.Error(t, u.Apply(context.Background(), "v1.2.3"))
	assert.Error(t, u.Apply(context.Background(), "latest"))
	assert.Error(t, u.Apply(context.Background(), "v1.2.3;x"))
}

func TestHTTPUpdaterDoesNotFollowRedirects(t *testing.T) {
	hit := false
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()
	u, err := NewHTTPUpdater(srv.URL, "tok")
	require.NoError(t, err)
	_, err = u.Status(context.Background())
	assert.Error(t, err)
	assert.False(t, hit)
}
