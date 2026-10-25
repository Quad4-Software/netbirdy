package h3

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

func testTLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	cert := srv.TLS.Certificates[0]
	return &tls.Config{Certificates: []tls.Certificate{cert}}
}

func h3Client(t *testing.T) *http3.Transport {
	t.Helper()
	return &http3.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test cert
		QUICConfig:      &quic.Config{},
	}
}

func TestH3_RequestOverQUIC(t *testing.T) {
	tlsCfg := testTLSConfig(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Handler", "real")
		io.WriteString(w, "h3 ok")
	})
	srv, err := New("127.0.0.1:0", handler, tlsCfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	go srv.Serve()

	// wait for the UDP socket
	time.Sleep(100 * time.Millisecond)
	port := srv.udp.LocalAddr().String()
	tr := h3Client(t)
	defer tr.Close()

	req, err := http.NewRequest("GET", "https://"+port+"/path", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bs, _ := io.ReadAll(resp.Body)
	if resp.ProtoMajor != 3 {
		t.Fatalf("expected HTTP/3, got %s", resp.Proto)
	}
	if string(bs) != "h3 ok" || resp.Header.Get("X-Handler") != "real" {
		t.Fatalf("handler chain not reached: %q", bs)
	}
}

func TestH3_LargeBodyAndShutdown(t *testing.T) {
	tlsCfg := testTLSConfig(t)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(w, r.Body)
	})
	srv, err := New("127.0.0.1:0", handler, tlsCfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	time.Sleep(100 * time.Millisecond)
	tr := h3Client(t)
	defer tr.Close()

	body := strings.Repeat("x", 1<<20) // 1MB
	req, _ := http.NewRequest("POST", "https://"+srv.udp.LocalAddr().String()+"/up", strings.NewReader(body))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if len(got) != 1<<20 {
		t.Fatalf("echoed %d bytes", len(got))
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestH3_AltSvcMiddleware(t *testing.T) {
	var got string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})
	h := AltSvcMiddleware(`h3=":443"; ma=86400`)(inner)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	got = rec.Header().Get("Alt-Svc")
	if !strings.Contains(got, "h3=") {
		t.Fatalf("alt-svc missing: %q", got)
	}
}
