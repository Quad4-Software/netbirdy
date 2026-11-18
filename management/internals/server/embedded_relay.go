package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/coder/websocket"
	log "github.com/sirupsen/logrus"

	nbconfig "github.com/netbirdio/netbird/management/internals/server/config"
	"github.com/netbirdio/netbird/management/server/http/middleware/bypass"
	relayserver "github.com/netbirdio/netbird/relay/server"
	quiclistener "github.com/netbirdio/netbird/relay/server/listener/quic"
	wslistener "github.com/netbirdio/netbird/relay/server/listener/ws"
	"github.com/netbirdio/netbird/shared/management/http/util"
	sharedrelay "github.com/netbirdio/netbird/shared/relay"
	relayauth "github.com/netbirdio/netbird/shared/relay/auth"
	quictls "github.com/netbirdio/netbird/shared/relay/tls"
	"github.com/netbirdio/netbird/trustedproxy"
)

// embeddedRelay serves the relay protocol inside the management service. The
// WebSocket transport rides the existing HTTPS handler so it works behind a
// reverse proxy, and QUIC listens on its own UDP socket.
type embeddedRelay struct {
	relay          *relayserver.Relay
	quic           *quiclistener.Listener
	addr           string
	trustedProxies *trustedproxy.List
}

// embeddedRelayEnabled resolves the flag + auto-fallback enablement rule: an
// explicit Enabled wins; otherwise the relay is enabled whenever no external
// Relay addresses are configured and a public address can be resolved.
func (s *BaseServer) embeddedRelayEnabled() (bool, string) {
	cfg := s.Config.EmbeddedRelay

	addr := ""
	if cfg != nil {
		addr = cfg.Address
	}
	if addr == "" && s.Config.HttpConfig != nil && s.Config.HttpConfig.LetsEncryptDomain != "" {
		addr = s.Config.HttpConfig.LetsEncryptDomain
	}
	if addr != "" {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, fmt.Sprintf("%d", s.mgmtPort))
		}
		if !validAdvertiseAddress(addr) {
			addr = ""
		}
	}

	if cfg != nil && cfg.Enabled != nil {
		if !*cfg.Enabled {
			return false, ""
		}
		return addr != "", addr
	}
	if addr == "" {
		return false, ""
	}
	auto := s.Config.Relay == nil || len(s.Config.Relay.Addresses) == 0
	if !auto {
		return false, ""
	}
	return true, addr
}

// startEmbeddedRelay wires the relay core into the HTTP router and the QUIC
// listener. It runs after setupTLS so transports can reuse the resolved
// certificate, and before the root handler is assembled so /relay is mounted
// in time.
func (s *BaseServer) startEmbeddedRelay(ctx context.Context) error {
	enabled, addr := s.embeddedRelayEnabled()
	if !enabled {
		if s.Config.EmbeddedRelay != nil && s.Config.EmbeddedRelay.Enabled != nil && *s.Config.EmbeddedRelay.Enabled && addr == "" {
			return fmt.Errorf("embedded relay enabled but no public address could be resolved; set EmbeddedRelay.Address")
		}
		return nil
	}

	if s.Config.Relay == nil {
		s.Config.Relay = &nbconfig.Relay{}
	}
	if s.Config.Relay.Secret == "" {
		s.Config.Relay.Secret = generateRelaySecret()
		log.WithContext(ctx).Warn("embedded relay: generated an in-memory relay secret; set Relay.Secret in config so relay tokens survive restarts")
	}

	hashedSecret := sha256.Sum256([]byte(s.Config.Relay.Secret))
	tlsEnabled := s.tlsConfig != nil || s.certManager != nil

	relay, err := relayserver.NewRelay(relayserver.Config{
		Meter:          s.Metrics().GetMeter(),
		ExposedAddress: addr,
		TLSSupport:     tlsEnabled,
		AuthValidator:  relayauth.NewTimedHMACValidator(hashedSecret[:]),
	})
	if err != nil {
		return fmt.Errorf("embedded relay: %w", err)
	}
	instanceURL := relay.InstanceURL()
	exposed := instanceURL.String()
	er := &embeddedRelay{
		relay:          relay,
		addr:           addr,
		trustedProxies: trustedproxy.FromPrefixes(s.Config.ReverseProxy.TrustedHTTPProxies),
	}

	if err := bypass.AddBypassPath(sharedrelay.WebSocketURLPath); err != nil {
		return fmt.Errorf("embedded relay: %w", err)
	}
	s.Router().HandleFunc(sharedrelay.WebSocketURLPath, er.serveWS)
	s.Router().HandleFunc("/api/relay/status", er.serveStatus).Methods(http.MethodGet, http.MethodOptions)

	s.Config.Relay.Addresses = appendUnique(s.Config.Relay.Addresses, exposed)
	log.WithContext(ctx).Infof("embedded relay: serving WebSocket on %s and advertising %s", sharedrelay.WebSocketURLPath, exposed)

	if tlsEnabled && (s.Config.EmbeddedRelay == nil || s.Config.EmbeddedRelay.QUICListenAddress != "disabled") {
		if err := er.bindQUIC(s); err != nil {
			log.WithContext(ctx).Warnf("embedded relay: QUIC transport unavailable: %v", err)
		} else {
			go func() {
				if err := er.quic.Serve(relay.Accept); err != nil {
					log.WithContext(ctx).Warnf("embedded relay: QUIC listener stopped: %v", err)
				}
			}()
		}
	}

	s.OnStop(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if er.quic != nil {
			_ = er.quic.Shutdown(shutdownCtx)
		}
		relay.Shutdown(shutdownCtx)
	})

	s.embeddedRelay = er
	return nil
}

// bindQUIC resolves the QUIC listen address and TLS config, then binds the
// UDP socket. The TLS source mirrors setupTLS: an injected or file config
// wins, then the cert manager.
func (er *embeddedRelay) bindQUIC(s *BaseServer) error {
	listenAddr := ""
	if s.Config.EmbeddedRelay != nil {
		listenAddr = s.Config.EmbeddedRelay.QUICListenAddress
	}
	if listenAddr == "" {
		_, port, err := net.SplitHostPort(er.addr)
		if err != nil {
			return fmt.Errorf("derive QUIC port: %w", err)
		}
		listenAddr = ":" + port
	}

	tlsConfig := s.tlsConfig
	if tlsConfig == nil && s.certManager != nil {
		tlsConfig = s.certManager.TLSConfig()
	}
	if tlsConfig == nil {
		return fmt.Errorf("no TLS configuration available")
	}

	quicTLS, err := quictls.ServerQUICTLSConfig(tlsConfig)
	if err != nil {
		return fmt.Errorf("QUIC TLS: %w", err)
	}

	l := &quiclistener.Listener{Address: listenAddr, TLSConfig: quicTLS}
	if err := l.Bind(); err != nil {
		return err
	}
	er.quic = l
	log.Infof("embedded relay: QUIC listening on udp %s", listenAddr)
	return nil
}

// serveWS accepts a WebSocket upgrade on the management HTTP server and hands
// the connection to the relay core. It mirrors the standalone listener's
// onAccept including trusted-proxy remote address resolution.
func (er *embeddedRelay) serveWS(w http.ResponseWriter, r *http.Request) {
	wsConn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		log.Errorf("embedded relay: failed to accept ws connection from %s: %s", r.RemoteAddr, err)
		return
	}

	rAddr, err := net.ResolveTCPAddr("tcp", remoteAddr(r, er.trustedProxies))
	if err != nil {
		_ = wsConn.Close(websocket.StatusInternalError, "internal error")
		return
	}

	er.relay.Accept(wslistener.NewConn(wsConn, rAddr))
}

// embeddedRelayStatus is the API response for GET /api/relay/status.
type embeddedRelayStatus struct {
	Enabled        bool     `json:"enabled"`
	Address        string   `json:"address"`
	Transports     []string `json:"transports"`
	ConnectedPeers int      `json:"connected_peers"`
}

// serveStatus reports embedded relay state on the authenticated API.
func (er *embeddedRelay) serveStatus(w http.ResponseWriter, r *http.Request) {
	transports := []string{"websocket"}
	if er.quic != nil {
		transports = append(transports, "quic")
	}
	instanceURL := er.relay.InstanceURL()
	util.WriteJSONObject(r.Context(), w, embeddedRelayStatus{
		Enabled:        true,
		Address:        instanceURL.String(),
		Transports:     transports,
		ConnectedPeers: er.relay.ConnectedPeerCount(),
	})
}

// remoteAddr resolves the client address, honoring X-Real-Ip/X-Real-Port only
// when the direct peer is a configured trusted proxy.
func remoteAddr(r *http.Request, trusted *trustedproxy.List) string {
	if trusted != nil && !trusted.Empty() && trusted.IsTrusted(r.RemoteAddr) {
		realIP := r.Header.Get("X-Real-Ip")
		realPort := r.Header.Get("X-Real-Port")
		if realIP != "" && realPort != "" {
			return net.JoinHostPort(realIP, realPort)
		}
	}
	return r.RemoteAddr
}

func generateRelaySecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// validAdvertiseAddress reports whether addr is a usable host:port for the
// advertised relay URL. SplitHostPort alone accepts "]:443"-style garbage that
// later breaks URL parsing, so the port must also be numeric and in range.
func validAdvertiseAddress(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || host == "" {
		return false
	}
	p, err := strconv.Atoi(port)
	return err == nil && p > 0 && p <= 65535
}

func appendUnique(list []string, v string) []string {
	for _, item := range list {
		if item == v {
			return list
		}
	}
	return append(list, v)
}
