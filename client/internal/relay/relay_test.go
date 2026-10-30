package relay

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/pion/stun/v3"
	"github.com/pion/turn/v4"
	"github.com/stretchr/testify/require"
)

func TestProbeSTUNLocalServer(t *testing.T) {
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Errorf("close STUN listener: %v", err)
		}
	})

	serverErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 1500)
		n, remoteAddr, err := listener.ReadFrom(buf)
		if err != nil {
			serverErr <- err
			return
		}

		request := &stun.Message{Raw: append([]byte(nil), buf[:n]...)}
		if err := request.Decode(); err != nil {
			serverErr <- fmt.Errorf("decode binding request: %w", err)
			return
		}

		remoteUDPAddr, ok := remoteAddr.(*net.UDPAddr)
		if !ok {
			serverErr <- fmt.Errorf("unexpected remote address type %T", remoteAddr)
			return
		}
		response, err := stun.Build(stun.BindingSuccess, &stun.XORMappedAddress{
			IP:   remoteUDPAddr.IP,
			Port: remoteUDPAddr.Port,
		})
		if err != nil {
			serverErr <- fmt.Errorf("build binding response: %w", err)
			return
		}
		if err := request.AddTo(response); err != nil {
			serverErr <- fmt.Errorf("copy transaction ID: %w", err)
			return
		}
		if _, err := listener.WriteTo(response.Raw, remoteAddr); err != nil {
			serverErr <- fmt.Errorf("write binding response: %w", err)
			return
		}
		serverErr <- nil
	}()

	serverAddr, ok := listener.LocalAddr().(*net.UDPAddr)
	require.True(t, ok, "listener address should be UDP")
	uri := &stun.URI{
		Scheme: stun.SchemeTypeSTUN,
		Host:   serverAddr.IP.String(),
		Port:   serverAddr.Port,
		Proto:  stun.ProtoTypeUDP,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)

	addr, err := NewStunTurnProbe(time.Second).probeSTUN(ctx, uri)
	require.NoError(t, err)
	require.NotEmpty(t, addr, "STUN binding response should return a mapped address")
	require.NoError(t, <-serverErr)
}

func TestProbeTURNLocalServer(t *testing.T) {
	for _, protocol := range []stun.ProtoType{stun.ProtoTypeUDP, stun.ProtoTypeTCP} {
		t.Run(protocol.String(), func(t *testing.T) {
			uri := startLocalTURNServer(t, protocol)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			t.Cleanup(cancel)

			addr, err := NewStunTurnProbe(time.Second).probeTURN(ctx, uri)
			require.NoError(t, err)
			host, portString, err := net.SplitHostPort(addr)
			require.NoError(t, err)
			require.Equal(t, "127.0.0.1", host)
			port, err := strconv.Atoi(portString)
			require.NoError(t, err)
			require.Positive(t, port, "TURN allocation should return a relay port")
		})
	}
}

func startLocalTURNServer(t *testing.T, protocol stun.ProtoType) *stun.URI {
	t.Helper()

	const relayIP = "127.0.0.1"
	var (
		serverAddress string
		closeListener func() error
		config        turn.ServerConfig
	)

	relayAddressGenerator := &turn.RelayAddressGeneratorStatic{
		RelayAddress: net.ParseIP(relayIP),
		Address:      relayIP,
	}
	switch protocol {
	case stun.ProtoTypeUDP:
		listener, err := net.ListenPacket("udp4", net.JoinHostPort(relayIP, "0"))
		require.NoError(t, err)
		serverAddress = listener.LocalAddr().String()
		closeListener = listener.Close
		config.PacketConnConfigs = []turn.PacketConnConfig{{
			PacketConn:            listener,
			RelayAddressGenerator: relayAddressGenerator,
		}}
	case stun.ProtoTypeTCP:
		listener, err := net.Listen("tcp4", net.JoinHostPort(relayIP, "0"))
		require.NoError(t, err)
		serverAddress = listener.Addr().String()
		closeListener = listener.Close
		config.ListenerConfigs = []turn.ListenerConfig{{
			Listener:              listener,
			RelayAddressGenerator: relayAddressGenerator,
		}}
	default:
		t.Fatalf("unsupported TURN transport %s", protocol)
	}

	config.Realm = "relay-test"
	config.AuthHandler = func(username, realm string, _ net.Addr) ([]byte, bool) {
		if username != "test-user" || realm != config.Realm {
			return nil, false
		}
		return turn.GenerateAuthKey(username, realm, "test-password"), true
	}

	server, err := turn.NewServer(config)
	if err != nil {
		if closeErr := closeListener(); closeErr != nil {
			t.Errorf("close TURN listener after server setup error: %v", closeErr)
		}
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Errorf("close TURN server: %v", err)
		}
	})

	host, portString, err := net.SplitHostPort(serverAddress)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)
	return &stun.URI{
		Scheme:   stun.SchemeTypeTURN,
		Host:     host,
		Port:     port,
		Username: "test-user",
		Password: "test-password",
		Proto:    protocol,
	}
}
