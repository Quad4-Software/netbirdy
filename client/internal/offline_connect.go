package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/netbirdio/netbird/client/ssh"
	mgm "github.com/netbirdio/netbird/shared/management/client"
	signal "github.com/netbirdio/netbird/shared/signal/client"
	log "github.com/sirupsen/logrus"
	"time"

	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
	relayClient "github.com/netbirdio/netbird/shared/relay/client"
	wgtypes "golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"path/filepath"
)

// errOfflineLeft is returned by the connect operation after offline mode
// ends so the backoff loop re-enters the normal connect path.
var errOfflineLeft = errors.New("left offline mode")

// waitOfflineDelay sleeps for the configured start delay, aborting early on
// context cancellation. Returns false when cancelled.
func waitOfflineDelay(ctx context.Context) bool {
	d := OfflineStartDelay()
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// runOffline starts the engine from a cached sync response and blocks until
// the engine context ends, mirroring the connected branch of the run loop.
// savedAt is the snapshot's original write time, carried into the engine so
// re-saves do not refresh the staleness clock.
func (c *ConnectClient) runOffline(engineCtx context.Context, cancel context.CancelFunc, snapshot *mgmProto.SyncResponse, savedAt time.Time, runningChan chan struct{}, mobileDependency MobileDependency, logPath string, path string) error {
	defer cancel()

	myPrivateKey, err := wgtypes.ParseKey(c.config.PrivateKey)
	if err != nil {
		return fmt.Errorf("offline mode requires the private key: %w", err)
	}

	// Envelope-format sync responses carry PeerConfig at the top level;
	// legacy NetworkMap syncs carry it under NetworkMap.PeerConfig. Mirror
	// handleSync's fallback so both snapshot shapes boot.
	peerConfig := snapshot.GetPeerConfig()
	if peerConfig == nil {
		peerConfig = snapshot.GetNetworkMap().GetPeerConfig()
	}
	if peerConfig == nil {
		return errors.New("offline snapshot carries no peer config")
	}
	engineConfig, err := createEngineConfig(myPrivateKey, c.config, peerConfig, logPath)
	if err != nil {
		return fmt.Errorf("offline engine config: %w", err)
	}
	engineConfig.TempDir = mobileDependency.TempDir
	engineConfig.CertStore.ProfileOwner = c.profileOwner
	engineConfig.CertStore.OwnerUnknown = c.profileOwnerUnknown
	if path != "" {
		engineConfig.StateDir = filepath.Dir(path)
	}

	relayManager := relayClient.NewManager(engineCtx, nil, myPrivateKey.PublicKey().String(), engineConfig.MTU,
		relayClient.WithNetEvents(c.netMgr))
	c.statusRecorder.SetRelayMgr(relayManager)

	// Signal is best-effort in cached-map mode: the broker authenticates with
	// the WireGuard key alone, so it works while management is down. When it
	// is unreachable too, the engine falls back to the no-op stub and peers
	// rely on cached endpoints and relay credentials.
	signalClient := c.connectOfflineSignal(engineCtx, snapshot, myPrivateKey)
	defer func() {
		if err := signalClient.Close(); err != nil {
			log.Warnf("failed to close offline signal client: %v", err)
		}
	}()

	c.engineMutex.Lock()
	engine := NewEngine(engineCtx, cancel, engineConfig, EngineServices{
		SignalClient:   signalClient,
		MgmClient:      offlineMgmClient{},
		RelayManager:   relayManager,
		StatusRecorder: c.statusRecorder,

		UpdateManager: c.updateManager,
		ClientMetrics: c.clientMetrics,
		MetricsCtx:    c.ctx,
		NetMgr:        c.netMgr,
	}, mobileDependency)
	engine.SetSyncResponsePersistence(c.persistSyncResponse)
	c.engine = engine
	c.engineMutex.Unlock()

	c.engineMutex.Lock()
	c.offlineCancel = cancel
	c.engineMutex.Unlock()

	mgmtURL := c.config.ManagementURL
	if err := engine.StartOffline(snapshot.GetNetbirdConfig(), mgmtURL, snapshot, savedAt); err != nil {
		log.Errorf("offline engine start failed: %s", err)
		return CtxGetState(c.ctx).Wrap(err)
	}

	state := CtxGetState(c.ctx)
	state.Set(StatusOffline)
	log.Warnf("engine running offline from cached network map (management unreachable)")

	if runningChan != nil {
		select {
		case <-runningChan:
		default:
			close(runningChan)
		}
	}

	// Handover loop: probe management until it answers. On success or on an
	// auth refusal we leave offline mode and let the normal run loop take
	// over (a short tunnel flap is expected on reconnect).
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		c.managementReconnectLoop(engineCtx)
	}()

	// The cached session cannot be refreshed while management is down, so its
	// expiry is a hard deadline: past it the peer must not keep running on
	// the stale map. Leaving offline re-enters the normal path, which deletes
	// the snapshot once the expired session is confirmed.
	var deadlineCh <-chan time.Time
	if deadline := offlineSessionDeadline(snapshot); !deadline.IsZero() {
		timer := time.NewTimer(time.Until(deadline))
		defer timer.Stop()
		deadlineCh = timer.C
	}
	select {
	case <-engineCtx.Done():
	case <-deadlineCh:
		log.Warnf("offline session expired, leaving cached mode")
		c.stopOfflineEngine()
	}
	<-engineCtx.Done()
	<-loopDone

	c.engineMutex.Lock()
	c.engine = nil
	c.engineMutex.Unlock()

	if err := engine.Stop(); err != nil {
		log.Errorf("Failed to stop engine: %v", err)
	}
	c.statusRecorder.ClientTeardown()
	return nil
}

// managementReconnectLoop probes management with jittered backoff (2s
// initial, capped at 30s). On a successful connect+login it cancels the
// engine context so runOffline unwinds and the normal path reconnects. On an
// auth-style refusal it deletes the snapshot and does the same — the server
// answered "this peer is gone", the cache must not keep it alive.
func (c *ConnectClient) managementReconnectLoop(ctx context.Context) {
	c.managementReconnectLoopWith(ctx, func(attemptCtx context.Context) error {
		myPrivateKey, err := wgtypes.ParseKey(c.config.PrivateKey)
		if err != nil {
			return fmt.Errorf("private key: %w", err)
		}
		targets := mgmtDialTargets(c.config.ManagementURL.Host, LastMgmtAddrs())
		var client *mgm.GrpcClient
		for _, target := range targets {
			dialOpts := []mgm.Option{mgm.WithNetEvents(c.netMgr)}
			if target != c.config.ManagementURL.Host {
				// Dialing a cached IP: keep the original hostname for TLS
				// verification and gRPC authority.
				dialOpts = append(dialOpts, mgm.WithServerName(c.config.ManagementURL.Hostname()))
			}
			client, err = mgm.NewClient(attemptCtx, target, myPrivateKey, c.config.ManagementURL.Scheme == "https",
				dialOpts...)
			if err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("management dial: %w", err)
		}
		defer client.Close()
		publicSSHKey, err := ssh.GeneratePublicKey([]byte(c.config.SSHKey))
		if err != nil {
			return fmt.Errorf("ssh key: %w", err)
		}
		_, err = loginToManagement(attemptCtx, client, publicSSHKey, c.config)
		return err
	})
}

func (c *ConnectClient) managementReconnectLoopWith(ctx context.Context, attempt func(context.Context) error) {
	interval := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		err := attempt(ctx)
		switch {
		case err == nil:
			log.Warnf("management reachable again, leaving offline mode")
			// tear down the offline engine so the normal path resumes
			c.stopOfflineEngine()
			return
		case !IsTransportErr(err):
			log.Warnf("management refused the peer while offline (%v); dropping cached map", err)
			if err := deleteOfflineSnapshot(c.offlineSnapshotPath); err != nil {
				log.Warnf("failed to delete offline snapshot: %v", err)
			}
			c.stopOfflineEngine()
			return
		default:
			log.Debugf("offline reconnect attempt failed: %v", err)
		}
		// jittered sleep, ctx-aware
		jitter := time.Duration((time.Now().UnixNano() % int64(interval)) / 4)
		t := time.NewTimer(interval + jitter)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if interval < 30*time.Second {
			interval *= 2
			if interval > 30*time.Second {
				interval = 30 * time.Second
			}
		}
	}
}

// connectOfflineSignal dials the signal broker from the cached config. The
// signal service authenticates peers with their WireGuard key alone, so it
// stays usable while management is down and keeps ICE negotiation alive for
// the cached map. Any failure degrades to the no-op stub: the dial is bounded
// by the client's own ~10s connect backoff.
func (c *ConnectClient) connectOfflineSignal(ctx context.Context, snapshot *mgmProto.SyncResponse, key wgtypes.Key) signal.Client {
	cfg := snapshot.GetNetbirdConfig()
	if cfg == nil || cfg.GetSignal() == nil || cfg.GetSignal().GetUri() == "" {
		return offlineSignalClient{}
	}
	client, err := connectToSignal(ctx, cfg, key, c.authKey(), c.netMgr)
	if err != nil {
		log.Warnf("offline mode: signal broker unreachable, continuing without ICE negotiation: %v", err)
		return offlineSignalClient{}
	}
	client.SetConnStateListener(statusRecorderToSignalConnStateNotifier(c.statusRecorder))
	log.Infof("offline mode: signal broker reachable, ICE negotiation stays available")
	return client
}

// maybeStartOffline classifies a management login failure and, when allowed,
// switches the run loop to a cached network map. Returns true when the
// caller's operation should exit (offline engine ran to completion or the
// failure was already handled).
func (c *ConnectClient) maybeStartOffline(engineCtx context.Context, cancel context.CancelFunc, loginErr error, runningChan chan struct{}, mobileDependency MobileDependency, logPath string, path string) bool {
	snapPath := offlineSnapshotPath(path)
	resp, savedAt, snapErr := loadOfflineSnapshot(snapPath, OfflineMaxAge(), time.Now())
	deadline := offlineSessionDeadline(resp)
	var sessExpires *time.Time
	if !deadline.IsZero() {
		sessExpires = &deadline
	}
	c.offlineSnapshotPath = snapPath
	d, _ := shouldStartOffline(loginErr, snapErr, time.Now(), sessExpires)
	log.Debugf("offline decision: %d (loginErr=%v snapErr=%v)", d, loginErr, snapErr)
	switch d {
	case offlineYes:
		if !waitOfflineDelay(engineCtx) {
			return false
		}
		if err := c.runOffline(engineCtx, cancel, resp, savedAt, runningChan, mobileDependency, logPath, path); err != nil {
			log.Errorf("offline mode failed: %v", err)
			return false
		}
		return true
	case offlineDelete:
		if err := deleteOfflineSnapshot(snapPath); err != nil {
			log.Warnf("failed to delete offline snapshot: %v", err)
		}
		return false
	default:
		return false
	}
}

// stopOfflineEngine stops the cached-map engine if one is running.
func (c *ConnectClient) stopOfflineEngine() {
	c.engineMutex.Lock()
	defer c.engineMutex.Unlock()
	if c.offlineCancel != nil {
		c.offlineCancel()
		c.offlineCancel = nil
	}
	if c.engine != nil {
		_ = c.engine.Stop()
		c.engine = nil
	}
}
