package internal

import (
	"context"
	"net"
	"net/netip"
	"net/url"
	"strings"
	"time"

	nbdns "github.com/netbirdio/netbird/dns"
	mgmProto "github.com/netbirdio/netbird/shared/management/proto"
)

// protectedOfflineHosts returns the hostnames that must never be resolved
// through overlay DNS in offline mode: the management, signal and relay
// endpoints from the cached network map. Without this, a match-all or
// covering domain in the cached DNS config swallows the management hostname
// and the client can never reconnect.
func protectedOfflineHosts(config *mgmProto.NetbirdConfig, mgmtURL *url.URL) map[string]struct{} {
	protected := make(map[string]struct{})
	add := func(raw string) {
		if raw == "" {
			return
		}
		u, err := url.Parse(raw)
		if err != nil {
			return
		}
		h := u.Hostname()
		if h == "" {
			return
		}
		if _, err := netip.ParseAddr(h); err == nil {
			return // IP literals do not go through DNS anyway
		}
		protected[strings.ToLower(strings.TrimSuffix(h, "."))] = struct{}{}
	}
	add(mgmtURL.String())
	if config.GetSignal() != nil {
		add(config.GetSignal().GetUri())
	}
	if r := config.GetRelay(); r != nil {
		for _, ru := range r.GetUrls() {
			add(ru)
		}
	}
	return protected
}

// covers reports whether a DNS route domain ("" or "." match all) would
// intercept the given hostname.
func domainCovers(domain, host string) bool {
	d := strings.ToLower(strings.TrimSuffix(domain, "."))
	if d == "" || d == "." {
		return true
	}
	h := strings.ToLower(host)
	return h == d || strings.HasSuffix(h, "."+d)
}

// filterOfflineNSGroups drops name server groups that would intercept the
// protected management/signal/relay hostnames in offline mode.
func filterOfflineNSGroups(groups []*nbdns.NameServerGroup, protected map[string]struct{}) []*nbdns.NameServerGroup {
	if len(protected) == 0 {
		return groups
	}
	out := groups[:0]
	for _, g := range groups {
		blocked := false
		for host := range protected {
			if len(g.Domains) == 0 {
				// primary group with no domain list matches everything
				blocked = true
				break
			}
			for _, d := range g.Domains {
				if domainCovers(d, host) {
					blocked = true
					break
				}
			}
			if blocked {
				break
			}
		}
		if !blocked {
			out = append(out, g)
		}
	}
	return out
}

// resolveMgmtFallback resolves host with a short timeout, falling back to the
// cached addresses persisted in the snapshot when the system lookup fails
// (for example because offline DNS filtering is not enough or the upstream is
// unreachable). Returns the dial target and the TLS authority to verify
// against. Pure apart from the injectable lookup.
var lookupMgmt = func(ctx context.Context, host string) ([]string, error) {
	r := net.Resolver{}
	return r.LookupHost(ctx, host)
}

// mgmtDialTargets returns the reconnect targets in order: cached management
// IPs first (offline DNS may intercept the hostname), the hostname last.
// The original host must stay the TLS authority for IP targets.
func mgmtDialTargets(hostport string, cached []string) []string {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		h, p = hostport, ""
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return []string{hostport}
	}
	var out []string
	for _, a := range cached {
		if _, err := netip.ParseAddr(a); err != nil {
			continue
		}
		if p != "" {
			out = append(out, net.JoinHostPort(a, p))
		} else {
			out = append(out, a)
		}
	}
	return append(out, hostport)
}

// captureMgmtAddrs resolves the management host once after a successful
// login and stores the addresses for the reconnect fallback: in memory for
// the next snapshot write, and patched into an existing snapshot envelope.
func captureMgmtAddrs(ctx context.Context, host, snapshotPath string) {
	if _, err := netip.ParseAddr(host); err == nil {
		return // literal IP needs no lookup
	}
	lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := lookupMgmt(lctx, host)
	if err != nil || len(addrs) == 0 {
		return
	}
	SetMgmtAddrs(addrs)
	updateMgmtAddrs(snapshotPath, addrs)
}

// resolveMgmtDialTarget returns "ip:port" from the cached addresses when the
// management hostname no longer resolves (offline DNS interception or a dead
// upstream). The caller keeps the original hostname for TLS verification.
// Returns the input unchanged when lookup works or nothing is cached.
func resolveMgmtDialTarget(ctx context.Context, hostport string, cached []string) string {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		h, p = hostport, ""
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return hostport // literal IP needs no lookup
	}
	lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := lookupMgmt(lctx, h); err == nil {
		return hostport
	}
	for _, a := range cached {
		if _, err := netip.ParseAddr(a); err != nil {
			continue
		}
		if p != "" {
			return net.JoinHostPort(a, p)
		}
		return a
	}
	return hostport
}
