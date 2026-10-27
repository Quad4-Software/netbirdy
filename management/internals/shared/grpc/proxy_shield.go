package grpc

import (
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/shared/management/proto"
)

// maxPendingShieldCommands bounds the per-proxy shield command queue so a
// proxy that never connects via the sync stream cannot accumulate commands
// without limit.
const maxPendingShieldCommands = 64

// ShieldConfigSource supplies the management-controlled shield config
// pushed to proxies over the sync stream. A nil proto means no managed
// config and proxies keep their local shield settings.
type ShieldConfigSource interface {
	ShieldProto() *proto.ShieldConfig
}

// shieldStatusEntry is the last shield report received from a proxy.
type shieldStatusEntry struct {
	status     *proto.ShieldStatus
	receivedAt time.Time
}

// shieldControl tracks the pushed shield config, queued commands and the
// per-proxy status cache. Embedded in ProxyServiceServer.
type shieldControl struct {
	mu       sync.Mutex
	source   ShieldConfigSource
	cmds     map[string][]*proto.ShieldCommand
	statuses sync.Map // proxyID -> shieldStatusEntry
}

// SetShieldSource wires the config provider. Pass nil to stop pushing a
// managed shield config; proxies then keep their local settings.
func (s *ProxyServiceServer) SetShieldSource(src ShieldConfigSource) {
	s.shield.mu.Lock()
	s.shield.source = src
	s.shield.mu.Unlock()
}

// shieldProto returns the current managed shield config, or nil when none
// is managed.
func (s *ProxyServiceServer) shieldProto() *proto.ShieldConfig {
	s.shield.mu.Lock()
	src := s.shield.source
	s.shield.mu.Unlock()
	if src == nil {
		return nil
	}
	return src.ShieldProto()
}

// drainShieldCommands removes and returns the commands queued for proxyID.
// Called from the sender path, which owns the stream's Send side.
func (s *ProxyServiceServer) drainShieldCommands(proxyID string) []*proto.ShieldCommand {
	s.shield.mu.Lock()
	defer s.shield.mu.Unlock()
	cmds := s.shield.cmds[proxyID]
	if len(cmds) > 0 {
		delete(s.shield.cmds, proxyID)
	}
	return cmds
}

// recordShieldStatus stores the latest shield report a proxy attached to a
// sync-stream ack.
func (s *ProxyServiceServer) recordShieldStatus(proxyID string, st *proto.ShieldStatus) {
	if st == nil {
		return
	}
	s.shield.statuses.Store(proxyID, shieldStatusEntry{status: st, receivedAt: time.Now()})
}

// ShieldStatuses returns the last reported shield status per proxy id.
func (s *ProxyServiceServer) ShieldStatuses() map[string]*proto.ShieldStatus {
	out := map[string]*proto.ShieldStatus{}
	s.shield.statuses.Range(func(key, value any) bool {
		out[key.(string)] = value.(shieldStatusEntry).status
		return true
	})
	return out
}

// PushShieldConfig nudges every connected proxy so the next message on its
// stream carries the current shield config immediately instead of waiting
// for the next mapping update. Returns the number of proxies nudged.
func (s *ProxyServiceServer) PushShieldConfig() int {
	pushed := 0
	s.connectedProxies.Range(func(_, value any) bool {
		conn := value.(*proxyConnection)
		select {
		case conn.sendChan <- &proto.GetMappingUpdateResponse{}:
			pushed++
		default:
			log.WithContext(conn.ctx).Debugf("shield config push to proxy %s skipped: send queue full", conn.proxyID)
		}
		return true
	})
	return pushed
}

// SendShieldCommand queues cmd for every connected proxy and nudges live
// sync-stream proxies so the command ships immediately. Commands for
// proxies that cannot carry them right now (legacy stream, full queue)
// stay queued and drain on their next sync-stream message. Returns the
// number of proxies pushed immediately.
func (s *ProxyServiceServer) SendShieldCommand(cmd *proto.ShieldCommand) int {
	pushed := 0
	s.connectedProxies.Range(func(_, value any) bool {
		conn := value.(*proxyConnection)
		s.shield.mu.Lock()
		if s.shield.cmds == nil {
			s.shield.cmds = map[string][]*proto.ShieldCommand{}
		}
		q := s.shield.cmds[conn.proxyID]
		q = append(q, cmd)
		if len(q) > maxPendingShieldCommands {
			q = q[len(q)-maxPendingShieldCommands:]
		}
		s.shield.cmds[conn.proxyID] = q
		s.shield.mu.Unlock()

		if conn.syncStream == nil {
			return true
		}
		select {
		case conn.sendChan <- &proto.GetMappingUpdateResponse{}:
			pushed++
		default:
			log.WithContext(conn.ctx).Debugf("shield command push to proxy %s skipped: send queue full", conn.proxyID)
		}
		return true
	})
	return pushed
}
