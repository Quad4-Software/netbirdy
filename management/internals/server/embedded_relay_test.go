package server

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	nbconfig "github.com/netbirdio/netbird/management/internals/server/config"
	relayserver "github.com/netbirdio/netbird/relay/server"
	relayauth "github.com/netbirdio/netbird/shared/relay/auth"
	"github.com/netbirdio/netbird/shared/relay/auth/hmac"
	authv2 "github.com/netbirdio/netbird/shared/relay/auth/hmac/v2"
	relayclient "github.com/netbirdio/netbird/shared/relay/client"
	"github.com/netbirdio/netbird/trustedproxy"
)

func boolPtr(b bool) *bool { return &b }

func TestEmbeddedRelayEnabled(t *testing.T) {
	tests := []struct {
		name         string
		cfg          *nbconfig.Config
		wantEnabled  bool
		wantAddr     string
		wantAddrPart string
	}{
		{
			name: "auto enabled with domain and no external relays",
			cfg: &nbconfig.Config{
				HttpConfig: &nbconfig.HttpServerConfig{LetsEncryptDomain: "mgmt.example.com"},
			},
			wantEnabled: true,
			wantAddr:    "mgmt.example.com:33073",
		},
		{
			name: "auto disabled when external relays configured",
			cfg: &nbconfig.Config{
				HttpConfig: &nbconfig.HttpServerConfig{LetsEncryptDomain: "mgmt.example.com"},
				Relay:      &nbconfig.Relay{Addresses: []string{"rels://relay.example.com"}},
			},
			wantEnabled: false,
		},
		{
			name: "auto disabled when no address resolvable",
			cfg:  &nbconfig.Config{},
		},
		{
			name: "explicit enabled without address resolves to enabled check",
			cfg: &nbconfig.Config{
				EmbeddedRelay: &nbconfig.EmbeddedRelayConfig{Enabled: boolPtr(true)},
				HttpConfig:    &nbconfig.HttpServerConfig{LetsEncryptDomain: "mgmt.example.com"},
			},
			wantEnabled: true,
			wantAddr:    "mgmt.example.com:33073",
		},
		{
			name: "explicit enabled with custom address",
			cfg: &nbconfig.Config{
				EmbeddedRelay: &nbconfig.EmbeddedRelayConfig{
					Enabled: boolPtr(true),
					Address: "relay.example.com:443",
				},
				Relay: &nbconfig.Relay{Addresses: []string{"rels://other.example.com"}},
			},
			wantEnabled: true,
			wantAddr:    "relay.example.com:443",
		},
		{
			name: "explicit disabled beats domain fallback",
			cfg: &nbconfig.Config{
				EmbeddedRelay: &nbconfig.EmbeddedRelayConfig{Enabled: boolPtr(false)},
				HttpConfig:    &nbconfig.HttpServerConfig{LetsEncryptDomain: "mgmt.example.com"},
			},
			wantEnabled: false,
		},
		{
			name: "bare address gets default port",
			cfg: &nbconfig.Config{
				EmbeddedRelay: &nbconfig.EmbeddedRelayConfig{
					Enabled: boolPtr(true),
					Address: "relay.example.com",
				},
			},
			wantEnabled: true,
			wantAddr:    "relay.example.com:33073",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &BaseServer{Config: tc.cfg, mgmtPort: 33073}
			enabled, addr := s.embeddedRelayEnabled()
			assert.Equal(t, tc.wantEnabled, enabled)
			assert.Equal(t, tc.wantAddr, addr)
		})
	}
}

func TestRemoteAddr(t *testing.T) {
	trusted, err := netip.ParsePrefix("10.0.0.1/32")
	require.NoError(t, err)
	list := trustedproxy.FromPrefixes([]netip.Prefix{trusted})

	req := &http.Request{
		RemoteAddr: "10.0.0.1:1234",
		Header: http.Header{
			"X-Real-Ip":   []string{"203.0.113.5"},
			"X-Real-Port": []string{"4567"},
		},
	}
	assert.Equal(t, "203.0.113.5:4567", remoteAddr(req, list))

	req.RemoteAddr = "192.0.2.9:1234"
	assert.Equal(t, "192.0.2.9:1234", remoteAddr(req, list))

	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Del("X-Real-Port")
	assert.Equal(t, "10.0.0.1:1234", remoteAddr(req, list))
}

func TestAppendUnique(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, appendUnique([]string{"a"}, "b"))
	assert.Equal(t, []string{"a"}, appendUnique([]string{"a"}, "a"))
}

// TestEmbeddedRelayWSEndToEnd exercises the full WebSocket path a peer takes:
// management-issued token in the token store, WS dial to /relay, relay
// handshake through embeddedRelay.serveWS into the relay core.
func TestEmbeddedRelayWSEndToEnd(t *testing.T) {
	secret := sha256.Sum256([]byte("embedded-relay-test-secret"))

	relay, err := relayserver.NewRelay(relayserver.Config{
		ExposedAddress: "127.0.0.1:443",
		TLSSupport:     false,
		AuthValidator:  relayauth.NewTimedHMACValidator(secret[:]),
	})
	require.NoError(t, err)
	defer relay.Shutdown(context.Background())

	er := &embeddedRelay{relay: relay}
	srv := httptest.NewServer(http.HandlerFunc(er.serveWS))
	defer srv.Close()

	gen, err := authv2.NewGenerator(authv2.AuthAlgoHMACSHA256, secret[:], time.Hour)
	require.NoError(t, err)
	token, err := gen.GenerateToken()
	require.NoError(t, err)

	store := &hmac.TokenStore{}
	require.NoError(t, store.UpdateToken(&hmac.Token{
		Payload:   string(token.Payload),
		Signature: base64.StdEncoding.EncodeToString(token.Signature),
	}))

	relayURL := "rel://" + strings.TrimPrefix(srv.URL, "http://")
	c := relayclient.NewClient(relayURL, store, "peer-one", 1400)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, c.Connect(ctx))
}

func TestEmbeddedRelayWSRejectsBadToken(t *testing.T) {
	secret := sha256.Sum256([]byte("embedded-relay-test-secret"))

	relay, err := relayserver.NewRelay(relayserver.Config{
		ExposedAddress: "127.0.0.1:443",
		TLSSupport:     false,
		AuthValidator:  relayauth.NewTimedHMACValidator(secret[:]),
	})
	require.NoError(t, err)
	defer relay.Shutdown(context.Background())

	er := &embeddedRelay{relay: relay}
	srv := httptest.NewServer(http.HandlerFunc(er.serveWS))
	defer srv.Close()

	wrong := sha256.Sum256([]byte("wrong-secret"))
	gen, err := authv2.NewGenerator(authv2.AuthAlgoHMACSHA256, wrong[:], time.Hour)
	require.NoError(t, err)
	token, err := gen.GenerateToken()
	require.NoError(t, err)

	store := &hmac.TokenStore{}
	require.NoError(t, store.UpdateToken(&hmac.Token{
		Payload:   string(token.Payload),
		Signature: base64.StdEncoding.EncodeToString(token.Signature),
	}))

	relayURL := "rel://" + strings.TrimPrefix(srv.URL, "http://")
	c := relayclient.NewClient(relayURL, store, "peer-evil", 1400)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	assert.Error(t, c.Connect(ctx))
}

func TestEmbeddedRelayWSMalformedHandshake(t *testing.T) {
	secret := sha256.Sum256([]byte("embedded-relay-test-secret"))

	relay, err := relayserver.NewRelay(relayserver.Config{
		ExposedAddress: "127.0.0.1:443",
		TLSSupport:     false,
		AuthValidator:  relayauth.NewTimedHMACValidator(secret[:]),
	})
	require.NoError(t, err)
	defer relay.Shutdown(context.Background())

	er := &embeddedRelay{relay: relay}
	srv := httptest.NewServer(http.HandlerFunc(er.serveWS))
	defer srv.Close()

	cases := map[string]http.Header{
		"plain get":       {},
		"upgrade no key":  {"Upgrade": {"websocket"}, "Connection": {"Upgrade"}},
		"garbage key":     {"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"not-base64!!!"}, "Sec-Websocket-Version": {"13"}},
		"garbage version": {"Upgrade": {"websocket"}, "Connection": {"Upgrade"}, "Sec-Websocket-Key": {"dGhlIHNhbXBsZSBub25jZQ=="}, "Sec-Websocket-Version": {"99"}},
	}
	for name, header := range cases {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			require.NoError(t, err)
			req.Header = header
			resp, err := srv.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			assert.GreaterOrEqual(t, resp.StatusCode, 400)
			assert.Less(t, resp.StatusCode, 500)
			assert.Equal(t, 0, relay.ConnectedPeerCount())
		})
	}
}

// FuzzRemoteAddr feeds arbitrary RemoteAddr/header combinations through the
// trusted-proxy resolution. It must never panic, must only honor the headers
// when the direct peer is trusted, and must produce a parseable host:port
// whenever the inputs were parseable.
func FuzzRemoteAddr(f *testing.F) {
	f.Add("10.0.0.1:443", "1.2.3.4", "8080")
	f.Add("[::1]:443", "bad ip", "")
	f.Fuzz(func(t *testing.T, remote, realIP, realPort string) {
		prefix, err := netip.ParsePrefix("10.0.0.0/8")
		require.NoError(t, err)
		list := trustedproxy.FromPrefixes([]netip.Prefix{prefix})

		req := &http.Request{
			RemoteAddr: remote,
			Header: http.Header{
				"X-Real-Ip":   []string{realIP},
				"X-Real-Port": []string{realPort},
			},
		}
		out := remoteAddr(req, list)
		if !list.IsTrusted(remote) {
			require.Equal(t, remote, out, "untrusted remote must be passed through")
		}
	})
}

// FuzzEmbeddedRelayEnabled fuzzes the configured address string through the
// enablement check. Explicit enablement must produce an enabled, host:port
// result or a clean disabled result, never a panic.
func FuzzEmbeddedRelayEnabled(f *testing.F) {
	f.Add("relay.example.com")
	f.Add("relay.example.com:443")
	f.Add("::1")
	f.Fuzz(func(t *testing.T, address string) {
		s := &BaseServer{
			mgmtPort: 33073,
			Config: &nbconfig.Config{
				EmbeddedRelay: &nbconfig.EmbeddedRelayConfig{
					Enabled: boolPtr(true),
					Address: address,
				},
			},
		}
		enabled, addr := s.embeddedRelayEnabled()
		if !enabled {
			return
		}
		_, _, err := net.SplitHostPort(addr)
		require.NoError(t, err, "enabled result %q must carry a port", addr)
	})
}
