package grpc

import (
	"context"
	"testing"

	"github.com/netbirdio/netbird/shared/management/proto"
)

type staticShieldSource struct{ cfg *proto.ShieldConfig }

func (s staticShieldSource) ShieldProto() *proto.ShieldConfig { return s.cfg }

func newShieldTestServer(t *testing.T) *ProxyServiceServer {
	t.Helper()
	s := NewProxyServiceServer(nil, nil, nil, ProxyOIDCConfig{}, nil, nil, nil, nil, nil)
	t.Cleanup(s.Close)
	return s
}

func TestShieldProtoNilWithoutSource(t *testing.T) {
	s := newShieldTestServer(t)
	if got := s.shieldProto(); got != nil {
		t.Fatalf("expected nil shield config, got %+v", got)
	}
}

func TestShieldProtoFromSource(t *testing.T) {
	s := newShieldTestServer(t)
	s.SetShieldSource(staticShieldSource{cfg: &proto.ShieldConfig{Mode: "enforce"}})
	got := s.shieldProto()
	if got == nil || got.GetMode() != "enforce" {
		t.Fatalf("shield config %+v", got)
	}
}

func TestRecordAndReadShieldStatus(t *testing.T) {
	s := newShieldTestServer(t)
	s.recordShieldStatus("p1", nil)
	if len(s.ShieldStatuses()) != 0 {
		t.Fatal("nil status recorded")
	}
	s.recordShieldStatus("p1", &proto.ShieldStatus{Mode: "enforce", Banned: 3})
	st := s.ShieldStatuses()["p1"]
	if st == nil || st.GetMode() != "enforce" || st.GetBanned() != 3 {
		t.Fatalf("status %+v", st)
	}
}

func TestSendShieldCommandQueuesForLegacyProxy(t *testing.T) {
	s := newShieldTestServer(t)
	conn := &proxyConnection{
		proxyID:  "p1",
		sendChan: make(chan *proto.GetMappingUpdateResponse, 1),
		ctx:      context.Background(),
	}
	s.connectedProxies.Store("p1", conn)

	// A legacy-stream proxy cannot take a live push; the command is queued.
	if pushed := s.SendShieldCommand(&proto.ShieldCommand{UnbanIp: "203.0.113.9"}); pushed != 0 {
		t.Fatalf("pushed %d", pushed)
	}
	cmds := s.drainShieldCommands("p1")
	if len(cmds) != 1 || cmds[0].GetUnbanIp() != "203.0.113.9" {
		t.Fatalf("queued commands %+v", cmds)
	}
	if cmds := s.drainShieldCommands("p1"); len(cmds) != 0 {
		t.Fatal("commands not drained")
	}
}

func TestPushShieldConfigNudgesSendChan(t *testing.T) {
	s := newShieldTestServer(t)
	conn := &proxyConnection{
		proxyID:  "p1",
		sendChan: make(chan *proto.GetMappingUpdateResponse, 1),
		ctx:      context.Background(),
	}
	s.connectedProxies.Store("p1", conn)

	if pushed := s.PushShieldConfig(); pushed != 1 {
		t.Fatalf("pushed %d", pushed)
	}
	select {
	case <-conn.sendChan:
	default:
		t.Fatal("no nudge on send channel")
	}
}
