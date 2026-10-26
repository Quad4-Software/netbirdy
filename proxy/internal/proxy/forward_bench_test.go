package proxy_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/netbirdio/netbird/proxy/internal/proxy"
)

type bodyTransport struct{ body string }

func (t bodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	h := make(http.Header, 2)
	h.Set("Content-Type", "text/plain")
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     h,
		Body:       io.NopCloser(strings.NewReader(t.body)),
	}, nil
}

type discardWriter struct{ h http.Header }

func (w *discardWriter) Header() http.Header         { return w.h }
func (w *discardWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *discardWriter) WriteHeader(int)             {}

func newForwardBenchProxy(body string) *proxy.ReverseProxy {
	rp := proxy.NewReverseProxy(bodyTransport{body: body}, "https", nil, nil)
	rp.AddMapping(proxy.Mapping{
		ID:        "svc",
		AccountID: "acc",
		Host:      "app.example.com",
		Paths: map[string]*proxy.PathTarget{
			"/": {URL: &url.URL{Scheme: "http", Host: "10.0.0.1:8080"}},
		},
	})
	return rp
}

func BenchmarkForwardGET(b *testing.B) {
	rp := newForwardBenchProxy("hello")
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		req := httptest.NewRequest(http.MethodGet, "https://app.example.com/api/items?x=1", nil)
		req.Host = "app.example.com"
		req.RemoteAddr = "203.0.113.50:12345"
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		for pb.Next() {
			rp.ServeHTTP(&discardWriter{h: make(http.Header, 4)}, req)
		}
	})
}

func BenchmarkForwardGETSerial(b *testing.B) {
	rp := newForwardBenchProxy("hello")
	req := httptest.NewRequest(http.MethodGet, "https://app.example.com/api/items?x=1", nil)
	req.Host = "app.example.com"
	req.RemoteAddr = "203.0.113.50:12345"
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36")
	b.ReportAllocs()
	for b.Loop() {
		rp.ServeHTTP(&discardWriter{h: make(http.Header, 4)}, req)
	}
}

func BenchmarkForwardLargeBody(b *testing.B) {
	rp := newForwardBenchProxy(strings.Repeat("x", 1<<20))
	req := httptest.NewRequest(http.MethodGet, "https://app.example.com/big", nil)
	req.Host = "app.example.com"
	req.RemoteAddr = "203.0.113.50:12345"
	b.ReportAllocs()
	b.SetBytes(1 << 20)
	for b.Loop() {
		rp.ServeHTTP(&discardWriter{h: make(http.Header, 4)}, req)
	}
}
