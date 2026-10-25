package internal

import (
	"context"
	"net"
	"net/netip"
	"time"

	"errors"
	"fmt"
	log "github.com/sirupsen/logrus"
	"net/url"

	"github.com/netbirdio/netbird/client/system"
	mgm "github.com/netbirdio/netbird/shared/management/client"
	"github.com/netbirdio/netbird/shared/management/domain"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
	sigClient "github.com/netbirdio/netbird/shared/signal/client"
	sigProto "github.com/netbirdio/netbird/shared/signal/proto"
)

var errOfflineUnavailable = errors.New("control plane unreachable: engine in offline mode")

// offlineMgmClient satisfies the management client interface while the engine
// runs from a cached network map. Every call reports an unavailable error so
// nothing can silently depend on a live control plane.
type offlineMgmClient struct{}

func (offlineMgmClient) Close() error { return nil }
func (offlineMgmClient) Sync(context.Context, func(context.Context) *system.Info, func(*mgmProto.SyncResponse) error) error {
	return errOfflineUnavailable
}
func (offlineMgmClient) Job(context.Context, func(*mgmProto.JobRequest) *mgmProto.JobResponse) error {
	return errOfflineUnavailable
}
func (offlineMgmClient) Register(string, string, *system.Info, []byte, domain.List) (*mgmProto.LoginResponse, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) Login(*system.Info, []byte, domain.List) (*mgmProto.LoginResponse, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) ExtendAuthSession(*system.Info, string) (*mgmProto.ExtendAuthSessionResponse, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) GetDeviceAuthorizationFlow() (*mgmProto.DeviceAuthorizationFlow, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) GetPKCEAuthorizationFlow(bool) (*mgmProto.PKCEAuthorizationFlow, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) GetServerURL() string        { return "" }
func (offlineMgmClient) IsHealthy() bool             { return false }
func (offlineMgmClient) HealthCheck() error          { return errOfflineUnavailable }
func (offlineMgmClient) SyncMeta(*system.Info) error { return errOfflineUnavailable }
func (offlineMgmClient) Logout() error               { return nil }
func (offlineMgmClient) CreateExpose(context.Context, mgm.ExposeRequest) (*mgm.ExposeResponse, error) {
	return nil, errOfflineUnavailable
}
func (offlineMgmClient) RenewExpose(context.Context, string) error { return errOfflineUnavailable }
func (offlineMgmClient) StopExpose(context.Context, string) error  { return errOfflineUnavailable }

// offlineSignalClient satisfies the signal client interface as a no-op.
type offlineSignalClient struct{}

func (offlineSignalClient) Close() error                { return nil }
func (offlineSignalClient) StreamConnected() bool       { return false }
func (offlineSignalClient) GetStatus() sigClient.Status { return sigClient.Status("offline") }
func (offlineSignalClient) Receive(context.Context, func(*sigProto.Message) error) error {
	return errOfflineUnavailable
}
func (offlineSignalClient) Ready() bool                         { return false }
func (offlineSignalClient) IsHealthy() bool                     { return false }
func (offlineSignalClient) WaitStreamConnected(context.Context) {}
func (offlineSignalClient) SendToStream(*sigProto.EncryptedMessage) error {
	return nil
}
func (offlineSignalClient) Send(*sigProto.Message) error                     { return nil }
func (offlineSignalClient) SetOnReconnectedListener(func())                  {}
func (offlineSignalClient) SetConnStateListener(sigClient.ConnStateNotifier) {}

// SetOffline puts the engine in cached-map mode: no Sync stream, no signal
// negotiation, no relay credential refresh.
func (e *Engine) SetOffline(offline bool) {
	e.offline = offline
}

// StartOffline boots the engine and applies the cached sync response through
// the same handleSync path a live map would take.
func (e *Engine) StartOffline(netbirdConfig *mgmProto.NetbirdConfig, mgmtURL *url.URL, snapshot *mgmProto.SyncResponse) error {
	e.SetOffline(true)
	e.offlineProtectedHosts = protectedOfflineHosts(netbirdConfig, mgmtURL)
	if err := e.Start(netbirdConfig, mgmtURL); err != nil {
		return err
	}
	if snapshot != nil {
		if err := e.handleSync(snapshot); err != nil {
			return fmt.Errorf("apply cached sync response: %w", err)
		}
		e.applyOfflineEndpoints(LastEndpoints())
	}
	return nil
}

// applyOfflineEndpoints reconfigures cached peers with their last-known
// direct endpoints and a persistent keepalive so WireGuard can handshake
// without signal. Peers with no stored endpoint stay inert until stage 3.
func (e *Engine) applyOfflineEndpoints(endpoints map[string]string) {
	if len(endpoints) == 0 {
		return
	}
	for key, ep := range endpoints {
		ap, err := netip.ParseAddrPort(ep)
		if err != nil {
			continue
		}
		allowed, ok := e.peerStore.AllowedIPs(key)
		if !ok || len(allowed) == 0 {
			continue
		}
		addr := &net.UDPAddr{IP: ap.Addr().AsSlice(), Port: int(ap.Port())}
		if err := e.wgInterface.UpdatePeer(key, allowed, 25*time.Second, addr, nil); err != nil {
			log.Debugf("offline endpoint apply for peer failed: %v", err)
		}
	}
}
