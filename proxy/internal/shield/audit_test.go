package shield

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rePageVar = func(name string) *regexp.Regexp {
	return regexp.MustCompile(`const ` + name + `="([^"]*)"`)
}

// TestChallengePageCarriesServerExpiry is a regression test: the page
// previously embedded the client-supplied exp query parameter (empty on
// the normal flow), so the verify endpoint could never mint a pass
// cookie. Now the page must carry the server's own expiry and the full
// challenge-verify-cookie roundtrip must succeed.
func TestChallengePageCarriesServerExpiry(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.ChallengeDifficulty = 4 })

	r := req("/page", browserUA)
	rec := httptest.NewRecorder()
	require.True(t, sh.serveChallenge(rec, r, testIP))
	page := rec.Body.String()

	nonce := rePageVar("nonce").FindStringSubmatch(page)
	diff := rePageVar("diff").FindStringSubmatch(page)
	exp := rePageVar("exp").FindStringSubmatch(page)
	require.NotEmpty(t, nonce, "page must embed the nonce")
	require.NotEmpty(t, diff, "page must embed the difficulty prefix")
	require.NotEmpty(t, exp, "page must embed the server-issued expiry")

	expUnix, err := strconv.ParseInt(exp[1], 10, 64)
	require.NoError(t, err, "embedded expiry must be a unix timestamp")
	assert.Positive(t, expUnix)

	var counter string
	for i := 0; i < 1_000_000; i++ {
		sum := sha256.Sum256([]byte(nonce[1] + ":" + strconv.Itoa(i)))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), diff[1]) {
			counter = strconv.Itoa(i)
			break
		}
	}
	require.NotEmpty(t, counter, "PoW must be solvable at test difficulty")

	vr := httptest.NewRequest(http.MethodGet,
		"http://svc.example"+verifyPath+"?nonce="+nonce[1]+"&n="+counter+"&exp="+exp[1]+"&r=/page", nil)
	vr.Header.Set("User-Agent", browserUA)
	vrec := httptest.NewRecorder()
	sh.handleVerify(vrec, vr, testIP)
	require.Equal(t, http.StatusFound, vrec.Code, "a solved challenge must pass")

	var pass *http.Cookie
	for _, c := range vrec.Result().Cookies() {
		if c.Name == passCookie {
			pass = c
		}
	}
	require.NotNil(t, pass, "verify must issue the pass cookie")

	pr := req("/", browserUA)
	pr.AddCookie(pass)
	assert.True(t, sh.hasValidPass(pr, testIP), "issued pass cookie must validate")
}

// TestDNSVerifier_CrossBotCacheIsolation is a regression test: the
// verdict cache was keyed by IP only, so a cached googlebot result was
// returned for a later bingbot claim from the same address.
func TestDNSVerifier_CrossBotCacheIsolation(t *testing.T) {
	ip := netip.MustParseAddr("66.249.66.1")
	fr := &fakeResolver{
		ptr:  map[string][]string{ip.String(): {"crawl-66-249-66-1.googlebot.com."}},
		host: map[string][]string{"crawl-66-249-66-1.googlebot.com": {ip.String()}},
	}
	v := NewDNSVerifierWithResolver(fr)
	require.True(t, v.Verify(context.Background(), ip, "Googlebot/2.1"))

	calls := fr.calls
	assert.False(t, v.Verify(context.Background(), ip, "bingbot/2.0"),
		"a different bot claim must not inherit the cached googlebot verdict")
	assert.Greater(t, fr.calls, calls, "a different bot claim must trigger a fresh lookup")
}

// TestObserveConcurrentWithServe exercises the shared per-source state
// from the out-of-band signal path and the request path at the same
// time. Run with -race: the unlocked mutation this guards against is
// invisible without the detector.
func TestObserveConcurrentWithServe(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	h := sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 60 {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req("/.env", browserUA))
			}
		}()
		go func() {
			defer wg.Done()
			for range 60 {
				sh.Observe(testIP, Event{Kind: EventAuthFailure})
			}
		}()
	}
	wg.Wait()

	st := sh.Stats()
	assert.Greater(t, st.Tracked, 0)
}
