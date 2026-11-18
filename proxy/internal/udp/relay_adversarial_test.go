package udp

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// udpEchoBackend starts a UDP echo server and returns its address.
func udpEchoBackend(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP(buf[:n], addr)
		}
	}()
	return conn
}

// TestRelay_SlowDialDoesNotBlockOtherSources is a regression test: the
// backend dial used to run inline in the packet loop, so one slow new
// source stalled forwarding for every other client. The dial now runs on
// a session goroutine; an unrelated source must get service while the
// first dial is still parked.
func TestRelay_SlowDialDoesNotBlockOtherSources(t *testing.T) {
	backend := udpEchoBackend(t)
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)

	var calls atomic.Int32
	dialFunc := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if calls.Add(1) == 1 {
			// First dial parks until the relay shuts down.
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return net.DialTimeout(network, addr, 2*time.Second)
	}

	relay := New(context.Background(), RelayConfig{
		Logger:   log.NewEntry(log.StandardLogger()),
		Listener: listener,
		Target:   backend.LocalAddr().String(),
		DialFunc: dialFunc,
	})
	go relay.Serve()
	defer relay.Close()

	slow := udpClient(t, listener.LocalAddr().String())
	fast := udpClient(t, listener.LocalAddr().String())
	defer slow.Close()
	defer fast.Close()

	// The slow source's packet triggers a session whose dial never
	// finishes while the relay lives.
	writeUDP(t, slow, []byte("slow"))
	// Give the dial goroutine a moment to park inside dialFunc.
	require.Eventually(t, func() bool { return calls.Load() >= 1 }, 2*time.Second, 10*time.Millisecond)

	// A second source must still get a full echo while the first dial is
	// blocked. Under the old inline-dial design this packet would sit
	// unread behind the parked dial.
	writeUDP(t, fast, []byte("fast"))
	got := readUDP(t, fast, 3*time.Second)
	assert.Equal(t, []byte("fast"), got, "an unrelated source must be served during a blocked dial")
}

// TestRelay_CloseDuringPendingDial exercises Close while a session dial
// is in flight: the reserved slot must be cleaned up, the goroutine must
// exit, and Close must not hang or leave a phantom session.
func TestRelay_CloseDuringPendingDial(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	require.NoError(t, err)

	dialStarted := make(chan struct{})
	release := make(chan struct{})
	var closeOnce sync.Once
	dialFunc := func(ctx context.Context, network, addr string) (net.Conn, error) {
		closeOnce.Do(func() { close(dialStarted) })
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return net.DialTimeout(network, addr, 2*time.Second)
		}
	}

	relay := New(context.Background(), RelayConfig{
		Logger:   log.NewEntry(log.StandardLogger()),
		Listener: listener,
		Target:   "127.0.0.1:9", // discard port; never actually reached
		DialFunc: dialFunc,
	})
	go relay.Serve()

	client := udpClient(t, listener.LocalAddr().String())
	defer client.Close()
	writeUDP(t, client, []byte("x"))
	select {
	case <-dialStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("dial goroutine never started")
	}

	done := make(chan struct{})
	go func() { relay.Close(); close(done) }()

	// Let the in-flight dial finish after Close began; the placeholder
	// has already been swept, so a completed dial must not resurrect a
	// session entry.
	close(release)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Close hung while a dial was in flight")
	}

	relay.mu.RLock()
	n := len(relay.sessions)
	relay.mu.RUnlock()
	assert.Zero(t, n, "no session may survive Close")
}

func udpClient(t *testing.T, target string) *net.UDPConn {
	t.Helper()
	dst, err := net.ResolveUDPAddr("udp", target)
	require.NoError(t, err)
	conn, err := net.DialUDP("udp", nil, dst)
	require.NoError(t, err)
	return conn
}

func writeUDP(t *testing.T, c *net.UDPConn, data []byte) {
	t.Helper()
	_, err := c.Write(data)
	require.NoError(t, err)
}

func readUDP(t *testing.T, c *net.UDPConn, timeout time.Duration) []byte {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(timeout)))
	buf := make([]byte, 2048)
	n, err := c.Read(buf)
	require.NoError(t, err, "expected an echo reply")
	return buf[:n]
}
