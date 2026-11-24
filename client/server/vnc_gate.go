package server

import (
	"context"

	"github.com/netbirdio/netbird/client/internal/ipcauth"
	"github.com/netbirdio/netbird/client/internal/profilemanager"
	"github.com/netbirdio/netbird/client/mdm"
)

// The VNC half of the privilege gate described in ssh_gate.go. The daemon runs
// as root/LocalSystem and the VNC server it hosts attaches to the console
// session, so both of these cross the user-to-root boundary:
//
//   - Enabling the VNC server publishes the console desktop, keyboard and
//     mouse to whoever the account authorizes. On a multi-user host that
//     desktop belongs to another user, and the profile the caller owns is not a
//     privilege they hold over it.
//   - Disabling the approval prompt removes the console user's per-connection
//     consent, turning an authorized VNC session into a silent one.
func requirePrivilegeForVNCChange(ctx context.Context, stored *profilemanager.Config, change privilegedConfigChange) error {
	if enables(storedFlag(stored, func(c *profilemanager.Config) *bool { return c.DisableVNCApproval }), change.disableVNCApproval) {
		return denyPrivileged(ctx, "disabling VNC connection approval", ipcauth.UpCommand("--disable-vnc-approval"))
	}

	if enables(storedFlag(stored, func(c *profilemanager.Config) *bool { return c.ServerVNCAllowed }), change.serverVNCAllowed) {
		return denyPrivileged(ctx, "enabling the NetBirdy VNC server", ipcauth.UpCommand("--allow-server-vnc"))
	}

	return nil
}

// vncServerEnabled reports whether the profile currently runs the VNC server.
//
// Unlike the SSH server, VNC defaults to off: the flag was introduced with the
// server itself, so a nil value is a config written before VNC existed and
// means the server is not running.
func vncServerEnabled(cfg *profilemanager.Config) bool {
	if cfg == nil || cfg.ServerVNCAllowed == nil {
		return false
	}
	return *cfg.ServerVNCAllowed
}

// enabledRemoteAccessServer names a remote-access server the profile currently
// runs, and whether it runs any. It decides whether the management-binding and
// deregistration guards apply, since either server hands authorization
// decisions to the management identity the profile points at. SSH is reported
// first when both are on: it is the more privileged of the two.
func enabledRemoteAccessServer(cfg *profilemanager.Config) (string, bool) {
	switch {
	case sshServerEnabled(cfg):
		return "SSH", true
	case vncServerEnabled(cfg):
		return "VNC", true
	default:
		return "", false
	}
}

// enabledRemoteAccessServerWithPolicy applies runtime MDM overrides, which are
// not written to the profile file used by the privilege gate. An override wins
// over the profile flag for its own server only.
func enabledRemoteAccessServerWithPolicy(cfg *profilemanager.Config, policy *mdm.Policy) (string, bool) {
	sshOn, vncOn := sshServerEnabled(cfg), vncServerEnabled(cfg)
	if policy != nil {
		if enabled, ok := policy.GetBool(mdm.KeyAllowServerSSH); ok {
			sshOn = enabled
		}
		if enabled, ok := policy.GetBool(mdm.KeyAllowServerVNC); ok {
			vncOn = enabled
		}
	}
	switch {
	case sshOn:
		return "SSH", true
	case vncOn:
		return "VNC", true
	default:
		return "", false
	}
}
