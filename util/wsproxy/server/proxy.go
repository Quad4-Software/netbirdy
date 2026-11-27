package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"

	"github.com/coder/websocket"
	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/util/wsproxy"
)

// Config contains the configuration for the WebSocket proxy.
type Config struct {
	Handler         http.Handler
	Path            string
	MetricsRecorder MetricsRecorder
}

// Proxy handles WebSocket to gRPC handler proxying.
type Proxy struct {
	config  Config
	metrics MetricsRecorder
}

// New creates a new WebSocket proxy instance with optional configuration
func New(handler http.Handler, opts ...Option) *Proxy {
	config := Config{
		Handler:         handler,
		Path:            wsproxy.ProxyPath,
		MetricsRecorder: NoOpMetricsRecorder{}, // Default to no-op
	}

	for _, opt := range opts {
		opt(&config)
	}

	return &Proxy{
		config:  config,
		metrics: config.MetricsRecorder,
	}
}

// Handler returns an http.Handler that proxies WebSocket connections to the local gRPC server.
func (p *Proxy) Handler() http.Handler {
	return &proxyHandler{
		metrics: p.config.MetricsRecorder,
		handler: p.config.Handler,
	}
}

type proxyHandler struct {
	metrics MetricsRecorder
	handler http.Handler
	conn    atomic.Pointer[wsConnAdapter]
}

type singleConnListener struct {
	conn      net.Conn
	taken     atomic.Bool
	closed    chan struct{}
	closeOnce sync.Once
}

func newSingleConnListener(conn net.Conn) *singleConnListener {
	return &singleConnListener{conn: conn, closed: make(chan struct{})}
}

func (l *singleConnListener) Accept() (net.Conn, error) {
	if !l.taken.Swap(true) {
		return l.conn, nil
	}
	<-l.closed
	return nil, net.ErrClosed
}

func (l *singleConnListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.closed)
	})
	return nil
}

func (l *singleConnListener) Addr() net.Addr {
	return l.conn.LocalAddr()
}

func (ph *proxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	ph.metrics.RecordConnection(ctx)
	defer ph.metrics.RecordDisconnection(ctx)

	log.Debugf("WebSocket proxy handling connection from %s, forwarding to internal gRPC handler", r.RemoteAddr)
	acceptOptions := &websocket.AcceptOptions{
		OriginPatterns: []string{"*"},
	}

	wsConn, err := websocket.Accept(w, r, acceptOptions)
	if err != nil {
		ph.metrics.RecordError(ctx, "websocket_accept_failed")
		log.Errorf("WebSocket upgrade failed from %s: %v", r.RemoteAddr, err)
		return
	}
	serverConn := (&wsConnAdapter{
		ctx:        ctx,
		conn:       wsConn,
		metrics:    ph.metrics,
		clientAddr: r.RemoteAddr,
	})

	defer func() {
		_ = serverConn.Close()
	}()

	ph.conn.Store(serverConn) // used in tests only

	log.Debugf("WebSocket proxy established: %s -> gRPC handler", r.RemoteAddr)

	listener := newSingleConnListener(serverConn)
	protocols := new(http.Protocols)
	protocols.SetUnencryptedHTTP2(true)
	server := &http.Server{
		Handler:   ph.handler,
		Protocols: protocols,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		ConnState: func(_ net.Conn, state http.ConnState) {
			if state == http.StateClosed || state == http.StateHijacked {
				_ = listener.Close()
			}
		},
		// b/c we are wrapping a ws connection, read and write connection deadlines normally set
		// via ReadTimeout and WriteTimeout http.Server fields aren't available to us. The ws
		// library doesn't expose connection deadline timer config, and we ignore these calls in "wsConnAdapter".
		//
		// Another issue is that serving a single connection bypasses setting of connection deadlines altogether,
		// ReadTimeout and Writetimeout set here would only apply to h2 streams, i.e. after a HEADERS frame
		// arrival and processing, turning ReadTimeout into a request body read deadline, and WriteTimeout into
		// a response deadline (the latter not useful for streaming requests).
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Debugf("WebSocket proxy HTTP/2 server closed with error: %v", err)
	}

	log.Debugf("WebSocket proxy closing: %s -> gRPC handler", r.RemoteAddr)
}
