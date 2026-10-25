// Package h3 runs HTTP/3 on QUIC for the proxy's main listener. UDP binds
// the same port as the TLS listener; certificates, SNI and ACME keep using
// the shared tls.Config via http3.ConfigureTLSConfig, so ACME tls-alpn-01
// on the TCP side is unaffected.
package h3

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// Server serves the proxy handler chain over HTTP/3.
type Server struct {
	server *http3.Server
	udp    net.PacketConn
}

// New binds a UDP socket on the same host:port as the TCP listener and
// prepares HTTP/3 with conservative limits: no 0-RTT (replay risk), idle
// timeout 30s, 250 incoming streams, no datagrams.
func New(addr string, handler http.Handler, tlsConfig *tls.Config, portOverride int) (*Server, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("parse addr %q: %w", addr, err)
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if portOverride > 0 {
		port = fmt.Sprint(portOverride)
	}
	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, fmt.Errorf("resolve udp %s:%s: %w", host, port, err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("listen udp %s: %w", udpAddr, err)
	}
	s := &http3.Server{
		Handler:   handler,
		TLSConfig: http3.ConfigureTLSConfig(tlsConfig),
		QUICConfig: &quic.Config{
			MaxIdleTimeout:     30 * time.Second,
			MaxIncomingStreams: 250,
			Allow0RTT:          false,
			EnableDatagrams:    false,
			// quic-go validates client addresses with Retry tokens during
			// the handshake by default, which bounds UDP amplification abuse.

		},
	}
	return &Server{server: s, udp: conn}, nil
}

// AltSvc returns the Alt-Svc header value advertising this h3 endpoint.
func (s *Server) AltSvc() string {
	port := s.udp.LocalAddr().(*net.UDPAddr).Port
	return fmt.Sprintf(`h3=":%d"; ma=86400`, port)
}

// Serve runs the HTTP/3 server until Close.
func (s *Server) Serve() error {
	return s.server.Serve(s.udp)
}

// Close shuts the server down.
func (s *Server) Close() error {
	return s.server.Close()
}

// AltSvcMiddleware adds the Alt-Svc header so TCP clients learn about h3.
// Advisory only: WebSocket and extended-CONNECT clients stay on h1/h2.
func AltSvcMiddleware(headerValue string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Alt-Svc", headerValue)
			next.ServeHTTP(w, r)
		})
	}
}
