package wguapi

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	testPrivateKey = [32]byte{1}
	testPeerKey    = [32]byte{2}
)

func TestParseDevice(t *testing.T) {
	response := fmt.Sprintf(`private_key=%s
listen_port=51820
fwmark=42
public_key=%s
preshared_key=%s
endpoint=[fe80::1%%eth0]:51820
last_handshake_time_sec=1700000000
last_handshake_time_nsec=123
tx_bytes=100
rx_bytes=200
persistent_keepalive_interval=25
allowed_ip=100.64.0.1/32
protocol_version=1
errno=0

`, hexKey(testPrivateKey), hexKey(testPeerKey), hexKey(testPrivateKey))

	device, err := parseDevice(strings.NewReader(response))
	require.NoError(t, err)

	assert.Equal(t, testPrivateKey, [32]byte(device.PrivateKey))
	assert.Equal(t, 51820, device.ListenPort)
	assert.Equal(t, 42, device.FirewallMark)
	require.Len(t, device.Peers, 1)

	peer := device.Peers[0]
	assert.Equal(t, testPeerKey, [32]byte(peer.PublicKey))
	assert.Equal(t, "fe80::1", peer.Endpoint.IP.String())
	assert.Equal(t, "eth0", peer.Endpoint.Zone)
	assert.Equal(t, 51820, peer.Endpoint.Port)
	assert.Equal(t, time.Unix(1700000000, 123), peer.LastHandshakeTime)
	assert.Equal(t, int64(100), peer.TransmitBytes)
	assert.Equal(t, int64(200), peer.ReceiveBytes)
	assert.Equal(t, 25*time.Second, peer.PersistentKeepaliveInterval)
	require.Len(t, peer.AllowedIPs, 1)
	assert.Equal(t, "100.64.0.1/32", peer.AllowedIPs[0].String())
	assert.Equal(t, 1, peer.ProtocolVersion)
}

func TestParseDeviceReportsErrno(t *testing.T) {
	_, err := parseDevice(strings.NewReader("errno=-22\n\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "-22")
}

func TestParseDeviceRejectsMalformedLine(t *testing.T) {
	_, err := parseDevice(strings.NewReader("not-a-key-value-pair\n\n"))
	require.Error(t, err)
}

func TestClientDevice(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "wgtest0.sock")
	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer func() {
		_ = listener.Close()
	}()

	response := fmt.Sprintf("private_key=%s\nerrno=0\n\n", hexKey(testPrivateKey))
	serveErr := serveOneUAPI(listener, response)

	client := &Client{socketDir: dir, timeout: time.Second}
	device, err := client.Device("wgtest0")
	require.NoError(t, err)
	require.NoError(t, <-serveErr)

	assert.Equal(t, "wgtest0", device.Name)
	assert.Equal(t, testPrivateKey, [32]byte(device.PrivateKey))
}

func TestClientDeviceMissing(t *testing.T) {
	client := &Client{socketDir: t.TempDir(), timeout: time.Second}
	_, err := client.Device("missing0")
	require.Error(t, err)
}

func serveOneUAPI(listener net.Listener, response string) <-chan error {
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer func() {
			_ = conn.Close()
		}()

		reader := bufio.NewReader(conn)
		op, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		if op != "get=1\n" {
			errCh <- fmt.Errorf("unexpected UAPI operation %q", op)
			return
		}
		terminator, err := reader.ReadString('\n')
		if err != nil {
			errCh <- err
			return
		}
		if terminator != "\n" {
			errCh <- fmt.Errorf("unexpected UAPI terminator %q", terminator)
			return
		}
		if _, err := io.WriteString(conn, response); err != nil {
			errCh <- err
			return
		}
		errCh <- nil
	}()
	return errCh
}

func hexKey(key [32]byte) string {
	return hex.EncodeToString(key[:])
}
