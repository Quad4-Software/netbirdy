package internal

import (
	"context"
	"errors"
	"fmt"
	"github.com/netbirdio/netbird/client/ssh"
	mgm "github.com/netbirdio/netbird/shared/management/client"
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
func (c *ConnectClient) runOffline(engineCtx context.Context, cancel context.CancelFunc, snapshot *mgmProto.SyncResponse, runningChan chan struct{}, mobileDependency MobileDependency, logPath string, path string) error {
	defer cancel()

	myPrivateKey, err := wgtypes.ParseKey(c.config.PrivateKey)
	if err != nil {
		return fmt.Errorf("offline mode requires the private key: %w", err)
	}

	engineConfig, err := createEngineConfig(myPrivateKey, c.config, snapshot.GetNetworkMap().GetPeerConfig(), logPath)
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

	c.engineMutex.Lock()
	engine := NewEngine(engineCtx, cancel, engineConfig, EngineServices{
		SignalClient:   offlineSignalClient{},
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
	if err := engine.StartOffline(snapshot.GetNetbirdConfig(), mgmtURL, snapshot); err != nil {
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

// maybeStartOffline classifies a management login failure and, when allowed,
// switches the run loop to a cached network map. Returns true when the
// caller's operation should exit (offline engine ran to completion or the
// failure was already handled).
func (c *ConnectClient) maybeStartOffline(engineCtx context.Context, cancel context.CancelFunc, loginErr error, runningChan chan struct{}, mobileDependency MobileDependency, logPath string, path string) bool {
	snapPath := offlineSnapshotPath(path)
	resp, _, snapErr := loadOfflineSnapshot(snapPath, OfflineMaxAge(), time.Now())
	var sessExpires *time.Time
	if resp != nil {
		if t := resp.GetSessionExpiresAt(); t != nil {
			// A zero-valued Timestamp is the wire encoding for "no
			// deadline"; it is non-nil but decodes to the Unix epoch.
			if ts := t.AsTime(); ts.Unix() > 0 {
				sessExpires = &ts
			}
		}
	}
	c.offlineSnapshotPath = snapPath
	d, _ := shouldStartOffline(loginErr, snapErr, time.Now(), sessExpires)
	log.Debugf("offline decision: %d (loginErr=%v snapErr=%v)", d, loginErr, snapErr)
	switch d {
	case offlineYes:
		if !waitOfflineDelay(engineCtx) {
			return false
		}
		if err := c.runOffline(engineCtx, cancel, resp, runningChan, mobileDependency, logPath, path); err != nil {
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
