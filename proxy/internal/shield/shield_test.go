package shield

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var testIP = netip.MustParseAddr("203.0.113.7")

func newShield(t *testing.T, mut func(*Config), opts ...Option) *Shield {
	t.Helper()
	cfg := DefaultConfig()
	if mut != nil {
		mut(&cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	base := []Option{WithIPResolver(func(*http.Request) netip.Addr { return testIP })}
	sh, err := New(cfg, append(base, opts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sh.Close)
	return sh
}

func req(path, ua string) *http.Request {
	r := httptest.NewRequest("GET", "http://svc.example"+path, nil)
	r.Header.Set("User-Agent", ua)
	r.Header.Set("Accept", "text/html")
	r.Header.Set("Accept-Language", "en-US,en;q=0.9")
	r.Header.Set("Sec-Fetch-Mode", "navigate")
	r.Header.Set("Sec-CH-UA", `"Chromium";v="120"`)
	r.RemoteAddr = "203.0.113.7:1234"
	return r
}

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36"

func TestCleanRequestPasses(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	rec := httptest.NewRecorder()
	called := false
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true })).ServeHTTP(rec, req("/", browserUA))
	if !called || rec.Code != 200 {
		t.Fatalf("blocked clean request: %d", rec.Code)
	}
}

func TestHoneypotBlocks(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return false }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	})).ServeHTTP(rec, req("/.env", browserUA))
	// first honeypot hit passes to service (post-response scoring), next banned
	rec2 := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })).ServeHTTP(rec2, req("/.env", browserUA))
	if rec2.Code != 403 {
		t.Fatalf("expected 403 after ban, got %d", rec2.Code)
	}
	if len(sh.Bans()) == 0 {
		t.Fatal("expected a ban")
	}
}

func TestHoneypotServedByServiceIsNotPunished(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.WeightHoneypot = 100 }, WithKnownHost(func(string) bool { return true }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req("/.env", browserUA))
	_, st := sh.store.get(testIP)
	if st.score >= sh.cfg.BanAt {
		t.Fatalf("score %v reached ban threshold", st.score)
	}
}

func TestScannerUA(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req("/", "sqlmap/1.8"))
	_, st := sh.store.get(testIP)
	if st.score < 80 {
		t.Fatalf("scanner ua not scored: %v", st.score)
	}
}

func TestAICrawlerDenied(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req("/", "GPTBot/1.0"))
	if rec.Code != 403 {
		t.Fatalf("AI crawler not denied: %d", rec.Code)
	}
}

func TestCurlOnKnownHostZero(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req("/api", "curl/8.0"))
	_, st := sh.store.get(testIP)
	if st.score > 0 {
		t.Fatalf("curl scored %v on known host", st.score)
	}
}

func TestSpoofedGooglebot(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.AllowSearchEngines = true },
		WithKnownHost(func(string) bool { return true }),
		WithSearchVerifier(fakeVerifier{ok: false}))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})).ServeHTTP(rec, req("/", "Googlebot/2.1"))
	_, st := sh.store.get(testIP)
	if st.score < sh.cfg.WeightFakeSearchBot {
		t.Fatalf("fake googlebot not scored: %v", st.score)
	}
}

type fakeVerifier struct{ ok bool }

func (f fakeVerifier) Verify(_ context.Context, _ netip.Addr, _ string) bool { return f.ok }

func TestDecay(t *testing.T) {
	now := time.Now()
	clk := func() time.Time { return now }
	sh := newShield(t, nil, WithClock(clk), WithKnownHost(func(string) bool { return false }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })).ServeHTTP(rec, req("/.env", browserUA))
	_, st := sh.store.get(testIP)
	sc0 := st.score
	now = now.Add(10 * time.Minute)
	sh.store.decay(st)
	if st.score > sc0/1.9 {
		t.Fatalf("decay failed: %v -> %v", sc0, st.score)
	}
}

func TestBanEscalationAndExpiry(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.BanBaseTTL = time.Minute; c.BanEscalation = 2; c.BanMaxTTL = 10 * time.Minute })
	_, st := sh.store.get(testIP)
	t1 := sh.store.ban(st)
	if t1 != time.Minute {
		t.Fatalf("first ban %v", t1)
	}
	st.banUntil = st.banUntil.Add(-time.Minute) // expired recently
	t2 := sh.store.ban(st)
	if t2 != 2*time.Minute {
		t.Fatalf("escalated ban %v", t2)
	}
}

func TestMemoryCap(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.MaxEntries = 64 * 10 })
	for i := 0; i < 64*100; i++ {
		ip := netip.MustParseAddr("10.0.0.1").Next()
		_, _ = sh.store.get(ip.Next())
	}
	total := 0
	for _, s := range sh.store.shards {
		s.mu.Lock()
		total += len(s.entries)
		s.mu.Unlock()
	}
	if total > 64*10 {
		t.Fatalf("cap exceeded: %d", total)
	}
}

func TestStateFileRoundTrip(t *testing.T) {
	f := t.TempDir() + "/bans.json"
	sh := newShield(t, func(c *Config) { c.StateFile = f }, WithKnownHost(func(string) bool { return false }))
	rec := httptest.NewRecorder()
	sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })).ServeHTTP(rec, req("/.env", browserUA))
	if err := sh.store.saveBans(); err != nil {
		t.Fatal(err)
	}
	sh2 := newShield(t, func(c *Config) { c.StateFile = f })
	if len(sh2.Bans()) == 0 {
		t.Fatal("bans not restored")
	}
}

func TestChallengeVerifyFlow(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.ChallengeDifficulty = 4 })
	// craft a valid solve at difficulty 4 (one hex zero)
	ip := testIP
	ua := browserUA
	host := "svc.example"
	exp := time.Now().Add(time.Minute)
	payload := sh.challengePayload(ip, ua, host, exp)
	sig := sh.sign(payload)
	nonce := sha256Hex(payload + sig)
	// find a counter with 1 leading hex zero
	var n string
	for i := 0; i < 100; i++ {
		sum := sha256Hex(nonce + ":" + itoa(i))
		if sum[0] == '0' {
			n = itoa(i)
			break
		}
	}
	if n == "" {
		t.Fatal("no nonce found")
	}
	r := httptest.NewRequest("GET", verifyPath+"?nonce="+nonce+"&n="+n+"&exp="+itoa64(exp.Unix())+"&r=/", nil)
	r.Header.Set("User-Agent", ua)
	r.Host = host
	rec := httptest.NewRecorder()
	sh.handleVerify(rec, r, ip)
	if rec.Code != 302 {
		t.Fatalf("verify failed: %d %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) == 0 {
		t.Fatal("no pass cookie")
	}
}

func TestChallengeTamperedCookie(t *testing.T) {
	sh := newShield(t, nil)
	r := req("/", browserUA)
	r.AddCookie(&http.Cookie{Name: passCookie, Value: "MTAwMC4xMjM0"})
	if sh.hasValidPass(r, testIP) {
		t.Fatal("tampered cookie accepted")
	}
}

func TestObserveModeNeverBlocks(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.Mode = ModeObserve })
	for i := 0; i < 50; i++ {
		rec := httptest.NewRecorder()
		sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) })).ServeHTTP(rec, req("/.env", "sqlmap"))
		if rec.Code != 404 {
			t.Fatalf("observe mode blocked: %d", rec.Code)
		}
	}
}

func TestUnban(t *testing.T) {
	sh := newShield(t, nil)
	_, st := sh.store.get(testIP)
	st.banUntil = time.Now().Add(time.Hour)
	if !sh.Unban(testIP) {
		t.Fatal("unban failed")
	}
	if sh.Unban(testIP) {
		t.Fatal("double unban")
	}
}

func TestObserve(t *testing.T) {
	sh := newShield(t, nil)
	sh.Observe(testIP, Event{Kind: EventAuthFailure})
	_, st := sh.store.get(testIP)
	if st.score <= 0 {
		t.Fatal("observe did not score")
	}
}

func TestTarpitCap(t *testing.T) {
	sh := newShield(t, func(c *Config) { c.MaxTarpits = 1 })
	sh.store.activeTarpits.Store(5) // simulate overflow
	rec := httptest.NewRecorder()
	r := req("/", browserUA)
	if sh.tarpit(rec, r) {
		t.Fatal("tarpit should have overflowed to block")
	}
}

func TestConcurrency(t *testing.T) {
	sh := newShield(t, nil, WithKnownHost(func(string) bool { return true }))
	done := make(chan struct{})
	h := sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	for g := 0; g < 16; g++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			for i := 0; i < 200; i++ {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req("/x"+itoa(n), browserUA))
			}
		}(g)
	}
	for g := 0; g < 16; g++ {
		<-done
	}
}

func BenchmarkCleanRequest(b *testing.B) {
	cfg := DefaultConfig()
	cfg.RatePerSecond = 1e15
	cfg.RateBurst = 1 << 30
	cfg.Mode = ModeEnforce
	sh, err := New(cfg, WithIPResolver(func(*http.Request) netip.Addr { return testIP }), WithKnownHost(func(string) bool { return true }))
	if err != nil {
		b.Fatal(err)
	}
	defer sh.Close()
	h := sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := req("/", browserUA)
	w := &noopWriter{h: http.Header{}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, r)
	}
}

type noopWriter struct{ h http.Header }

func (w *noopWriter) Header() http.Header         { return w.h }
func (w *noopWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *noopWriter) WriteHeader(int)             {}

func BenchmarkCleanRequestParallel(b *testing.B) {
	cfg := DefaultConfig()
	cfg.RatePerSecond = 1e15
	cfg.RateBurst = 1 << 30
	var ctr atomic.Int32
	sh, err := New(cfg,
		WithIPResolver(func(r *http.Request) netip.Addr {
			return netip.MustParseAddr(r.RemoteAddr[:strings.IndexByte(r.RemoteAddr, ':')])
		}),
		WithKnownHost(func(string) bool { return true }))
	if err != nil {
		b.Fatal(err)
	}
	defer sh.Close()
	h := sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		n := ctr.Add(1)
		r := req("/", browserUA)
		r.RemoteAddr = fmt.Sprintf("10.9.%d.%d:4433", n/250+1, n%250+1)
		w := &noopWriter{h: http.Header{}}
		for pb.Next() {
			h.ServeHTTP(w, r)
		}
	})
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func itoa(i int) string     { return strconv.Itoa(i) }
func itoa64(i int64) string { return strconv.FormatInt(i, 10) }

func BenchmarkBare(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Mode = ModeOff
	cfg.RateBurst = 1 << 30
	sh, _ := New(cfg)
	defer sh.Close()
	h := sh.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := req("/", browserUA)
	w := &noopWriter{h: http.Header{}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.ServeHTTP(w, r)
	}
}

type fakeResolver struct {
	ptr   map[string][]string
	host  map[string][]string
	err   error
	dly   time.Duration
	calls int
}

func (f *fakeResolver) LookupAddr(_ context.Context, a string) ([]string, error) {
	f.calls++
	if f.dly > 0 {
		time.Sleep(f.dly)
	}
	return f.ptr[a], f.err
}
func (f *fakeResolver) LookupHost(_ context.Context, h string) ([]string, error) {
	f.calls++
	if f.dly > 0 {
		time.Sleep(f.dly)
	}
	return f.host[h], f.err
}

func TestDNSVerifier(t *testing.T) {
	ip := netip.MustParseAddr("66.249.66.1")
	fr := &fakeResolver{
		ptr:  map[string][]string{ip.String(): {"crawl-66-249-66-1.googlebot.com."}},
		host: map[string][]string{"crawl-66-249-66-1.googlebot.com": {ip.String()}},
	}
	v := NewDNSVerifierWithResolver(fr)
	if !v.Verify(context.Background(), ip, "Mozilla/5.0 (compatible; Googlebot/2.1)") {
		t.Fatal("expected verified")
	}
	// cache hit: second call uses cache
	n := fr.calls
	v.Verify(context.Background(), ip, "Googlebot/2.1")
	if fr.calls != n {
		t.Fatal("expected cache hit")
	}
	// spoofed PTR suffix
	fr2 := &fakeResolver{ptr: map[string][]string{ip.String(): {"evil.example.com."}}}
	v2 := NewDNSVerifierWithResolver(fr2)
	if v2.Verify(context.Background(), ip, "Googlebot/2.1") {
		t.Fatal("spoofed PTR accepted")
	}
	// forward mismatch
	fr3 := &fakeResolver{
		ptr:  map[string][]string{ip.String(): {"crawl-66-249-66-1.googlebot.com."}},
		host: map[string][]string{"crawl-66-249-66-1.googlebot.com": {"1.2.3.4"}},
	}
	v3 := NewDNSVerifierWithResolver(fr3)
	if v3.Verify(context.Background(), ip, "Googlebot/2.1") {
		t.Fatal("forward mismatch accepted")
	}
	// resolver error -> false, then negative cached
	fre := &fakeResolver{err: fmt.Errorf("nx")}
	v4 := NewDNSVerifierWithResolver(fre)
	if v4.Verify(context.Background(), ip, "Googlebot/2.1") {
		t.Fatal("resolver error accepted")
	}
	v4.Verify(context.Background(), ip, "Googlebot/2.1")
	if fre.calls > 2 {
		t.Fatal("negative cache not used")
	}
}
