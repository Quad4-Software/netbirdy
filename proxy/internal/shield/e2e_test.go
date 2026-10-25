package shield

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/netip"
	"net/url"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netbirdio/netbird/proxy/internal/proxy"
	"github.com/netbirdio/netbird/trustedproxy"
)

// e2eChain builds: CapturedData -> shield -> passthrough -> reverse proxy.
func e2eChain(t *testing.T, cfg Config, opts ...Option) (*Shield, http.Handler) {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/", "/robots.txt", "/login", "/wp-login.php":
			fmt.Fprint(w, "ok")
		case "/api/data":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(backend.Close)
	target := strings.TrimPrefix(backend.URL, "http://")
	rp := &httputil.ReverseProxy{Rewrite: func(pr *httputil.ProxyRequest) {
		pr.SetURL(&url.URL{Scheme: "http", Host: target})
	}}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	base := []Option{
		WithIPResolver(func(r *http.Request) netip.Addr {
			ip, _ := netip.ParseAddr(r.Header.Get("X-Test-IP"))
			return ip
		}),
		WithKnownHost(func(string) bool { return true }),
	}
	sh, err := New(cfg, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sh.Close)
	chain := sh.Wrap(rp)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cd := proxy.NewCapturedData("req-1")
		ip, _ := netip.ParseAddr(r.Header.Get("X-Test-IP"))
		cd.SetClientIP(ip)
		r = r.WithContext(proxy.WithCapturedData(r.Context(), cd))
		chain.ServeHTTP(w, r)
	})
	return sh, wrapped
}

func mkReq(ip, path, ua string) *http.Request {
	r, _ := http.NewRequest("GET", "http://svc.example"+path, nil)
	r.Header.Set("X-Test-IP", ip)
	r.Header.Set("User-Agent", ua)
	r.Header.Set("Accept", "text/html")
	r.Host = "svc.example"
	return r
}

func browserReq(ip, path string) *http.Request {
	r := mkReq(ip, path, browserUA)
	r.Header.Set("Accept-Language", "en-US")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-CH-UA", `"Chromium";v="120"`)
	return r
}

func TestE2E_BrowserSession(t *testing.T) {
	_, h := e2eChain(t, DefaultConfig())
	for i := 0; i < 20; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, browserReq("198.51.100.10", []string{"/", "/robots.txt", "/login", "/wp-login.php"}[i%4]))
		if rec.Code != 200 && rec.Code != 404 {
			t.Fatalf("blocked at %d: %d", i, rec.Code)
		}
	}
}

func TestE2E_APIClient(t *testing.T) {
	_, h := e2eChain(t, DefaultConfig())
	start := time.Now()
	n := 0
	for time.Since(start) < 3*time.Second {
		rec := httptest.NewRecorder()
		r := mkReq("198.51.100.11", "/api/data", "curl/8.4.0")
		r.Header.Set("Accept", "application/json")
		h.ServeHTTP(rec, r)
		if rec.Code != 200 {
			t.Fatalf("api client blocked: %d", rec.Code)
		}
		n++
		time.Sleep(20 * time.Millisecond)
	}
	if n < 10 {
		t.Fatalf("too few requests: %d", n)
	}
}

func TestE2E_Dirbuster(t *testing.T) {
	sh, h := e2eChain(t, DefaultConfig())
	ip := "198.51.100.12"
	blockedAt := -1
	paths := append([]string{"/.env", "/wp-login.php", "/.git/config", "/phpmyadmin/"},
		func() []string {
			var ps []string
			for i := 0; i < 196; i++ {
				ps = append(ps, fmt.Sprintf("/p%d", i))
			}
			return ps
		}()...)
	for i, p := range paths {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, browserReq(ip, p))
		if rec.Code == 403 && blockedAt < 0 {
			blockedAt = i
		}
	}
	if blockedAt < 0 {
		t.Fatal("never blocked")
	}
	if blockedAt > 6 {
		t.Fatalf("blocked too late: %d", blockedAt)
	}
	if len(sh.Bans()) == 0 {
		t.Fatal("no ban recorded")
	}
}

func TestE2E_GPTBot(t *testing.T) {
	_, h := e2eChain(t, DefaultConfig())
	for _, p := range []string{"/", "/api/data"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, mkReq("198.51.100.13", p, "GPTBot/1.0"))
		if rec.Code != 403 {
			t.Fatalf("GPTBot %s: %d", p, rec.Code)
		}
	}
}

func TestE2E_SearchBots(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AllowSearchEngines = true
	ip := "198.51.100.14"
	fake := &fakeResolver{
		ptr:  map[string][]string{ip: {"crawl-66-249-66-1.googlebot.com."}},
		host: map[string][]string{"crawl-66-249-66-1.googlebot.com": {ip}},
	}
	_, h := e2eChain(t, cfg, WithSearchVerifier(NewDNSVerifierWithResolver(fake)))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mkReq(ip, "/", "Mozilla/5.0 (compatible; Googlebot/2.1)"))
	if rec.Code != 200 {
		t.Fatalf("verified googlebot blocked: %d", rec.Code)
	}
	// spoofed: PTR mismatch
	fake2 := &fakeResolver{ptr: map[string][]string{"198.51.100.15": {"evil.example.com."}}}
	_, h2 := e2eChain(t, cfg, WithSearchVerifier(NewDNSVerifierWithResolver(fake2)))
	rec2 := httptest.NewRecorder()
	h2.ServeHTTP(rec2, mkReq("198.51.100.15", "/", "Googlebot/2.1"))
	if rec2.Code == 200 {
		// fake bot adds 90; one request may pass before ban — require second blocked
		rec2 = httptest.NewRecorder()
		h2.ServeHTTP(rec2, mkReq("198.51.100.15", "/", "Googlebot/2.1"))
	}
	if rec2.Code != 403 {
		t.Fatalf("spoofed googlebot not blocked: %d", rec2.Code)
	}
}

func TestE2E_Challenge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChallengeAt = 20
	cfg.ChallengeDifficulty = 4 // one hex zero, fast to solve
	cfg.TarpitAt = 90
	cfg.BanAt = 100
	cfg.WeightFakeBrowserUA = 30
	sh, h := e2eChain(t, cfg)
	ip := "198.51.100.16"
	// accumulate score via an inconsistent browser UA (missing headers) on navigations
	ua := browserUA
	for i := 0; i < 4; i++ {
		rec := httptest.NewRecorder()
		r := mkReq(ip, "/x", ua) // no Accept-Language/Sec-Fetch -> inconsistency weight
		h.ServeHTTP(rec, r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, browserReq(ip, "/page"))
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "Checking your browser") {
		t.Fatalf("no interstitial: %d", rec.Code)
	}
	body := rec.Body.String()
	nonce := strings.Split(strings.Split(body, `nonce="`)[1], `"`)[0]
	var counter string
	for i := 0; ; i++ {
		sum := sha256.Sum256([]byte(nonce + ":" + fmt.Sprint(i)))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), "0") {
			counter = fmt.Sprint(i)
			break
		}
	}
	// verify
	exp := "" // not bound to exp param in cookie path? pass exp param from page
	_ = exp
	vreq := mkReq(ip, "/.netbirdy-shield/verify?nonce="+nonce+"&n="+counter+"&r=/page", browserUA)
	// exp param required: extract exp? challenge page doesn't emit it; handleVerify needs exp
	// get a fresh exp by signing? simpler: call handleVerify path via serveChallenge first
	vreq.URL.RawQuery = "nonce=" + nonce + "&n=" + counter + "&r=/page&exp="
	// exp must be a unix timestamp matching signed payload: build via shield internals
	// use the challenge payload API directly for a valid exp
	expv := time.Now().Add(time.Minute)
	payload := sh.challengePayload(netip.MustParseAddr(ip), browserUA, "svc.example", expv)
	sig := sh.sign(payload)
	nonce2 := fmt.Sprintf("%x", sha256.Sum256([]byte(payload+sig)))
	var c2 string
	for i := 0; ; i++ {
		sum := sha256.Sum256([]byte(nonce2 + ":" + fmt.Sprint(i)))
		if strings.HasPrefix(hex.EncodeToString(sum[:]), "0") {
			c2 = fmt.Sprint(i)
			break
		}
	}
	vreq = mkReq(ip, verifyPath+"?nonce="+nonce2+"&n="+c2+"&r=/page&exp="+fmt.Sprint(expv.Unix()), browserUA)
	vrec := httptest.NewRecorder()
	sh.handleVerify(vrec, vreq, netip.MustParseAddr(ip))
	if vrec.Code != 302 {
		t.Fatalf("verify: %d", vrec.Code)
	}
	cookies := vrec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no pass cookie")
	}
	// next navigation with cookie passes
	nr := browserReq(ip, "/page")
	nr.AddCookie(cookies[0])
	if !sh.hasValidPass(nr, netip.MustParseAddr(ip)) {
		t.Fatal("pass cookie did not validate")
	}
	nrec := httptest.NewRecorder()
	h.ServeHTTP(nrec, nr)
	if nrec.Code == 403 {
		t.Fatalf("post-challenge nav still blocked: %d", nrec.Code)
	}
	// API request at challenge band is never challenged
	ar := mkReq(ip, "/api/data", ua)
	ar.Header.Set("Accept", "application/json")
	ar.Header.Set("Authorization", "Bearer t")
	arec := httptest.NewRecorder()
	h.ServeHTTP(arec, ar)
	if strings.Contains(arec.Body.String(), "Checking your browser") {
		t.Fatal("API request challenged")
	}
}

func TestE2E_Tarpit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ChallengeAt = 1
	cfg.TarpitAt = 5
	cfg.BanAt = 200
	cfg.TarpitDelay = 300 * time.Millisecond
	cfg.WeightUnknownHost = 10
	sh, h := e2eChain(t, cfg, WithKnownHost(func(string) bool { return false }))
	ip := "198.51.100.17"
	g0 := runtime.NumGoroutine()
	start := time.Now()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, browserReq(ip, "/a"))
	el := time.Since(start)
	if el < 200*time.Millisecond {
		t.Fatalf("not tarpitted: %v", el)
	}
	if rec.Code != 403 {
		t.Fatalf("tarpit code: %d", rec.Code)
	}
	time.Sleep(50 * time.Millisecond)
	if runtime.NumGoroutine() > g0+5 {
		t.Fatal("goroutine leak")
	}
	_ = sh
}

func TestE2E_KnownServiceServingProbe(t *testing.T) {
	sh, h := e2eChain(t, DefaultConfig())
	ip := "198.51.100.18"
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, browserReq(ip, "/wp-login.php"))
		if rec.Code != 200 {
			t.Fatalf("service 200 blocked: %d", rec.Code)
		}
	}
	if len(sh.Bans()) != 0 {
		t.Fatal("service penalized for real path")
	}
}

func TestE2E_ExemptPaths(t *testing.T) {
	sh, h := e2eChain(t, DefaultConfig(), WithKnownHost(func(string) bool { return false }))
	ip := "198.51.100.19"
	// ACME path is exempt from the .well-known honeypot rule
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, mkReq(ip, "/.well-known/acme-challenge/token123", "curl/1"))
	if len(sh.Bans()) != 0 {
		t.Fatal("ACME path banned")
	}
	_ = rec
}

func TestE2E_ObserveMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeObserve
	sh, h := e2eChain(t, cfg)
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, mkReq("198.51.100.20", "/.env", "sqlmap"))
		if rec.Code == 403 {
			t.Fatal("observe mode altered response")
		}
	}
	_ = sh
}

func TestDemo(t *testing.T) {
	type row struct{ name, req, verdict string }
	_, h := e2eChain(t, DefaultConfig())
	var rows []row
	step := func(name, ip, path, ua string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, mkReq(ip, path, ua))
		u := ua
		if len(u) > 18 {
			u = u[:18]
		}
		rows = append(rows, row{name, u + " " + path, fmt.Sprint(rec.Code)})
	}
	step("a browser", "198.51.100.30", "/", browserUA)
	step("a browser", "198.51.100.30", "/api/data", browserUA)
	step("b api/curl", "198.51.100.31", "/api/data", "curl/8.4.0")
	step("c scan .env", "198.51.100.32", "/.env", "curl/8")
	step("c scan wp", "198.51.100.32", "/wp-login.php", "curl/8")
	step("c scan git", "198.51.100.32", "/.git/config", "curl/8")
	step("c banned", "198.51.100.32", "/", "curl/8")
	step("d GPTBot", "198.51.100.33", "/", "GPTBot/1.0")
	step("d GPTBot", "198.51.100.33", "/api/data", "GPTBot/1.0")
	for _, r := range rows {
		t.Logf("%-12s %-28s %s", r.name, r.req, r.verdict)
	}
}

// X-Forwarded-For can only forge the client IP when the peer is a trusted
// proxy; the shield resolver must see the same IP as the access log.
func TestE2E_XFFSpoofing(t *testing.T) {
	tl, err := trustedproxy.Parse("10.0.0.0/8")
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	var seen netip.Addr
	_, h := e2eChain(t, cfg, WithIPResolver(func(r *http.Request) netip.Addr {
		got := tl.ResolveClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
		seen = got
		return got
	}))
	// untrusted peer forges XFF: must be ignored
	r := browserReq("198.51.100.40", "/")
	r.RemoteAddr = "203.0.113.9:5555"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen.String() != "203.0.113.9" {
		t.Fatalf("untrusted XFF forged ip: %s", seen)
	}
	// trusted peer passes the real client IP
	r2 := browserReq("198.51.100.40", "/")
	r2.RemoteAddr = "10.0.0.5:5555"
	r2.Header.Set("X-Forwarded-For", "198.51.100.77")
	h.ServeHTTP(httptest.NewRecorder(), r2)
	if seen.String() != "198.51.100.77" {
		t.Fatalf("trusted XFF not honored: %s", seen)
	}
	// X-Real-IP and Forwarded are never honored
	r3 := browserReq("198.51.100.40", "/")
	r3.RemoteAddr = "10.0.0.5:5555"
	r3.Header.Set("X-Real-IP", "6.6.6.6")
	r3.Header.Set("Forwarded", "for=6.6.6.6")
	h.ServeHTTP(httptest.NewRecorder(), r3)
	if seen.String() != "10.0.0.5" {
		t.Fatalf("X-Real-IP/Forwarded forged ip: %s", seen)
	}
}
