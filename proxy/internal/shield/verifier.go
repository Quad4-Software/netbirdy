package shield

import (
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
)

// Resolver is the DNS surface the verifier needs.
type Resolver interface {
	LookupAddr(ctx context.Context, addr string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
}

type netResolver struct{ r *net.Resolver }

func (n netResolver) LookupAddr(ctx context.Context, a string) ([]string, error) {
	return n.r.LookupAddr(ctx, a)
}
func (n netResolver) LookupHost(ctx context.Context, h string) ([]string, error) {
	return n.r.LookupHost(ctx, h)
}

// botSuffixes maps claimed bot tokens to allowed PTR suffixes.
var botSuffixes = map[string][]string{
	"googlebot":   {".googlebot.com", ".google.com"},
	"bingbot":     {".search.msn.com"},
	"duckduckbot": {".duckduckgo.com"},
	"baiduspider": {".crawl.baidu.com", ".crawl.baidu.jp"},
	"yandex":      {".yandex.com", ".yandex.net", ".yandex.ru"},
	"applebot":    {".applebot.apple.com"},
}

type verifyEntry struct {
	ok      bool
	expires time.Time
}

// DNSVerifier implements SearchVerifier with forward-confirmed reverse DNS,
// a bounded cache and in-flight dedupe.
type DNSVerifier struct {
	res      Resolver
	mu       sync.Mutex
	cache    map[string]verifyEntry
	inflight map[string]chan bool
	now      func() time.Time
	max      int
}

// NewDNSVerifier builds a verifier on the system resolver.
func NewDNSVerifier() *DNSVerifier {
	return &DNSVerifier{
		res:      netResolver{r: &net.Resolver{PreferGo: true}},
		cache:    map[string]verifyEntry{},
		inflight: map[string]chan bool{},
		now:      time.Now,
		max:      8192,
	}
}

// NewDNSVerifierWithResolver injects a resolver for tests.
func NewDNSVerifierWithResolver(r Resolver) *DNSVerifier {
	v := NewDNSVerifier()
	v.res = r
	return v
}

// Verify returns true when ip is confirmed for the bot claimed in ua.
// The first lookup resolves synchronously with a 2s timeout; results are
// cached (6h positive, 10m negative) so repeats are free.
func (v *DNSVerifier) Verify(ctx context.Context, ip netip.Addr, ua string) bool {
	key := ip.String()
	l := strings.ToLower(ua)
	var suffixes []string
	for tok, sf := range botSuffixes {
		if strings.Contains(l, strings.ToLower(tok)) {
			suffixes = sf
			break
		}
	}
	if len(suffixes) == 0 {
		return false
	}
	v.mu.Lock()
	if e, ok := v.cache[key]; ok && e.expires.After(v.now()) {
		v.mu.Unlock()
		return e.ok
	}
	if ch, ok := v.inflight[key]; ok {
		v.mu.Unlock()
		select {
		case r := <-ch:
			return r
		case <-ctx.Done():
			return false
		case <-time.After(2 * time.Second):
			return false
		}
	}
	ch := make(chan bool, 1)
	v.inflight[key] = ch
	v.mu.Unlock()

	ok := v.check(ctx, key, suffixes)

	v.mu.Lock()
	if len(v.cache) >= v.max {
		v.cache = map[string]verifyEntry{}
	}
	ttl := 6 * time.Hour
	if !ok {
		ttl = 10 * time.Minute
	}
	v.cache[key] = verifyEntry{ok: ok, expires: v.now().Add(ttl)}
	delete(v.inflight, key)
	v.mu.Unlock()
	ch <- ok
	return ok
}

func (v *DNSVerifier) check(ctx context.Context, ip string, suffixes []string) bool {
	c, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	names, err := v.res.LookupAddr(c, ip)
	if err != nil || len(names) == 0 {
		return false
	}
	var host string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSuffix(n, "."))
		for _, sf := range suffixes {
			if strings.HasSuffix(n, sf) {
				host = n
				break
			}
		}
		if host != "" {
			break
		}
	}
	if host == "" {
		return false
	}
	addrs, err := v.res.LookupHost(c, host)
	if err != nil {
		return false
	}
	for _, a := range addrs {
		if a == ip {
			return true
		}
	}
	return false
}
