package proxy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/proxy/auth"
)

// TestStripSessionCookie_PreservesMalformed is a regression test: the
// previous implementation rebuilt the Cookie header via
// Request.Cookies(), which silently dropped pairs the stdlib cannot
// parse (bare flags, values containing spaces). The raw-filter
// implementation must keep every non-session pair byte-for-byte.
func TestStripSessionCookie_PreservesMalformed(t *testing.T) {
	pr := newProxyRequest(t, "http://svc.example/", "10.0.0.1:443")
	pr.In.Header["Cookie"] = []string{
		"a=1; legacy_flag; " + auth.SessionCookieName + "=SECRET; b=2; spaced =va lu e",
	}
	pr.Out.Header = pr.In.Header.Clone()

	stripSessionCookie(pr)

	got := pr.Out.Header["Cookie"]
	require.Len(t, got, 1)
	assert.NotContains(t, got[0], auth.SessionCookieName)
	assert.Contains(t, got[0], "a=1")
	assert.Contains(t, got[0], "legacy_flag", "unparsable pairs must survive")
	assert.Contains(t, got[0], "spaced =va lu e", "malformed values must survive")
	assert.Contains(t, got[0], "b=2")
}

func TestStripSessionCookie_MultipleHeaderLines(t *testing.T) {
	pr := newProxyRequest(t, "http://svc.example/", "10.0.0.1:443")
	pr.In.Header["Cookie"] = []string{
		"first=1; " + auth.SessionCookieName + "=X",
		"second=2; " + auth.SessionCookieName + "=Y; third=3",
	}
	pr.Out.Header = pr.In.Header.Clone()

	stripSessionCookie(pr)

	got := pr.Out.Header["Cookie"]
	require.Len(t, got, 2)
	assert.Equal(t, "first=1", got[0])
	assert.Equal(t, "second=2; third=3", got[1])
}

func TestStripSessionCookie_OnlySessionDropsHeader(t *testing.T) {
	pr := newProxyRequest(t, "http://svc.example/", "10.0.0.1:443")
	pr.In.Header["Cookie"] = []string{auth.SessionCookieName + "=SECRET"}
	pr.Out.Header = pr.In.Header.Clone()

	stripSessionCookie(pr)
	assert.Empty(t, pr.Out.Header["Cookie"], "a lone session cookie must remove the header entirely")
}

func TestStripSessionCookie_BareSessionNameDropped(t *testing.T) {
	pr := newProxyRequest(t, "http://svc.example/", "10.0.0.1:443")
	pr.In.Header["Cookie"] = []string{"keep=1; " + auth.SessionCookieName + "; other=2"}
	pr.Out.Header = pr.In.Header.Clone()

	stripSessionCookie(pr)
	got := pr.Out.Header["Cookie"]
	require.Len(t, got, 1)
	assert.NotContains(t, got[0], auth.SessionCookieName)
	assert.Contains(t, got[0], "keep=1")
	assert.Contains(t, got[0], "other=2")
}

func TestStripSessionCookie_NoCookiesNoop(t *testing.T) {
	pr := newProxyRequest(t, "http://svc.example/", "10.0.0.1:443")
	stripSessionCookie(pr)
	assert.Empty(t, pr.Out.Header["Cookie"])
}
