package server

import (
	"testing"
)

// FuzzGetInstanceURL feeds arbitrary exposed addresses through the URL
// normalizer. On success the scheme must be rel or rels and a TLS relay must
// never produce a rel:// URL.
func FuzzGetInstanceURL(f *testing.F) {
	f.Add("example.com", true)
	f.Add("rel://example.com", false)
	f.Add("example.com:443", true)
	f.Fuzz(func(t *testing.T, addr string, tlsSupport bool) {
		u, err := getInstanceURL(addr, tlsSupport)
		if err != nil {
			return
		}
		if u.Scheme != SchemeREL && u.Scheme != SchemeRELS {
			t.Fatalf("unexpected scheme %q for input %q", u.Scheme, addr)
		}
		if tlsSupport && u.Scheme == SchemeREL {
			t.Fatalf("non-TLS scheme for TLS relay on input %q", addr)
		}
	})
}
