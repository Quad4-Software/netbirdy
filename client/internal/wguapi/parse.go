package wguapi

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func parseDevice(r io.Reader) (*wgtypes.Device, error) {
	var dp deviceParser
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			break
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid UAPI key/value pair %q", line)
		}
		dp.parse(key, value)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read UAPI device: %w", err)
	}
	return dp.finish()
}

type deviceParser struct {
	device wgtypes.Device
	err    error

	inPeer    bool
	hsSec     int64
	hsNsec    int64
	hsSecSet  bool
	hsNsecSet bool
}

func (dp *deviceParser) finish() (*wgtypes.Device, error) {
	if dp.err != nil {
		return nil, dp.err
	}
	dp.device.PublicKey = dp.device.PrivateKey.PublicKey()
	return &dp.device, nil
}

func (dp *deviceParser) parse(key, value string) {
	if key == "errno" {
		if errno := dp.parseInt(value); errno != 0 {
			dp.err = fmt.Errorf("UAPI errno %d", errno)
		}
		return
	}

	if key == "public_key" {
		dp.inPeer = true
		dp.hsSec = 0
		dp.hsNsec = 0
		dp.hsSecSet = false
		dp.hsNsecSet = false
		dp.device.Peers = append(dp.device.Peers, wgtypes.Peer{
			PublicKey: dp.parseKey(value),
		})
		return
	}

	if dp.inPeer {
		dp.parsePeer(key, value)
		return
	}

	switch key {
	case "private_key":
		dp.device.PrivateKey = dp.parseKey(value)
	case "listen_port":
		dp.device.ListenPort = int(dp.parseInt(value))
	case "fwmark":
		dp.device.FirewallMark = int(dp.parseInt(value))
	}
}

func (dp *deviceParser) parsePeer(key, value string) {
	peer := &dp.device.Peers[len(dp.device.Peers)-1]
	switch key {
	case "preshared_key":
		peer.PresharedKey = dp.parseKey(value)
	case "endpoint":
		peer.Endpoint = dp.parseAddr(value)
	case "last_handshake_time_sec":
		dp.hsSec = dp.parseInt(value)
		dp.hsSecSet = true
		dp.updateHandshake(peer)
	case "last_handshake_time_nsec":
		dp.hsNsec = dp.parseInt(value)
		dp.hsNsecSet = true
		dp.updateHandshake(peer)
	case "tx_bytes":
		peer.TransmitBytes = dp.parseInt(value)
	case "rx_bytes":
		peer.ReceiveBytes = dp.parseInt(value)
	case "persistent_keepalive_interval":
		peer.PersistentKeepaliveInterval = time.Duration(dp.parseInt(value)) * time.Second
	case "allowed_ip":
		_, cidr, err := net.ParseCIDR(value)
		if err != nil {
			dp.setErr(err)
			return
		}
		peer.AllowedIPs = append(peer.AllowedIPs, *cidr)
	case "protocol_version":
		peer.ProtocolVersion = int(dp.parseInt(value))
	}
}

func (dp *deviceParser) updateHandshake(peer *wgtypes.Peer) {
	if !dp.hsSecSet || !dp.hsNsecSet || dp.hsSec == 0 {
		return
	}
	peer.LastHandshakeTime = time.Unix(dp.hsSec, dp.hsNsec)
}

func (dp *deviceParser) parseKey(value string) wgtypes.Key {
	if dp.err != nil {
		return wgtypes.Key{}
	}

	b, err := hex.DecodeString(value)
	if err != nil {
		dp.setErr(fmt.Errorf("parse UAPI key: %w", err))
		return wgtypes.Key{}
	}
	key, err := wgtypes.NewKey(b)
	if err != nil {
		dp.setErr(err)
		return wgtypes.Key{}
	}
	return key
}

func (dp *deviceParser) parseInt(value string) int64 {
	if dp.err != nil {
		return 0
	}

	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		dp.setErr(fmt.Errorf("parse UAPI integer %q: %w", value, err))
		return 0
	}
	return parsed
}

func (dp *deviceParser) parseAddr(value string) *net.UDPAddr {
	if dp.err != nil {
		return nil
	}

	addr, err := net.ResolveUDPAddr("udp", value)
	if err != nil {
		dp.setErr(fmt.Errorf("parse UAPI endpoint %q: %w", value, err))
		return nil
	}
	return addr
}

func (dp *deviceParser) setErr(err error) {
	if dp.err == nil {
		dp.err = err
	}
}
