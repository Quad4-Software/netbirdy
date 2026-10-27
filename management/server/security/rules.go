package security

import (
	"fmt"
	"net/netip"
	"strings"
	"time"
)

// scoring weights for the combined new-ip finding
const (
	weightNewIP      = 20
	weightNewSubnet  = 15 // /24 or /48 not previously seen
	weightNewASN     = 25
	weightNewCountry = 30
)

// cap total score at 100
func capScore(s int) int {
	if s > 100 {
		return 100
	}
	return s
}

func sameSubnet(a, b netip.Addr, v4, v6 int) bool {
	if a.Is4() && b.Is4() {
		return netip.PrefixFrom(a, v4).Contains(b)
	}
	return netip.PrefixFrom(a, v6).Contains(b)
}

// peerNewIP scores a source IP not in the peer's learned set, escalating
// with how far the change is (subnet, ASN, country).
func (e *Engine) peerNewIP(obs PeerObservation, b *peerBaseline, now time.Time) (score int, reason string) {
	if b.knownIP(obs.SrcIP) {
		return 0, ""
	}
	if len(b.ips) == 0 {
		return 0, "" // first observation seeds the baseline silently
	}
	score += weightNewIP
	newSubnet := true
	for _, e2 := range b.ips {
		if sameSubnet(e2.ip, obs.SrcIP, 24, 48) {
			newSubnet = false
			break
		}
	}
	if newSubnet {
		score += weightNewSubnet
	}
	if e.geo != nil {
		country, asn := e.geo.Lookup(obs.SrcIP)
		if asn != 0 && len(b.asns) > 0 {
			if _, ok := b.asns[asn]; !ok {
				score += weightNewASN
			}
		}
		if country != "" && len(b.countries) > 0 {
			if _, ok := b.countries[country]; !ok {
				score += weightNewCountry
			}
		}
	}
	return capScore(score), "source IP not in peer baseline"
}

func (e *Engine) peerIPFlap(obs PeerObservation, now time.Time) (score int, reason string) {
	key := obs.AccountID + "/" + obs.PeerID
	n := e.ipFlapCounts.add(key, obs.SrcIP.String(), now, e.cfg().FlapWindow, e.cfg().MaxAuthKeys)
	if n > e.cfg().FlapThreshold {
		return 70, fmt.Sprintf("%d distinct IPs in %s", n, e.cfg().FlapWindow)
	}
	return 0, ""
}

func (e *Engine) peerMetaMismatch(obs PeerObservation, b *peerBaseline) (score int, reason string) {
	base := b.meta
	if base.Hostname == "" && base.OS == "" && base.Serial == "" {
		return 0, "" // no baseline yet
	}
	var diffs []string
	if obs.Meta.Serial != "" && base.Serial != "" && obs.Meta.Serial != base.Serial {
		diffs = append(diffs, "serial")
	}
	if obs.Meta.PublicKey != "" && base.PublicKey != "" && obs.Meta.PublicKey != base.PublicKey {
		diffs = append(diffs, "machine key")
	}
	if obs.Meta.OS != "" && base.OS != "" && obs.Meta.OS != base.OS {
		diffs = append(diffs, "os")
	}
	if obs.Meta.Kernel != "" && base.Kernel != "" && obs.Meta.Kernel != base.Kernel {
		diffs = append(diffs, "kernel")
	}
	if len(diffs) == 0 {
		if obs.Meta.Hostname != "" && base.Hostname != "" && obs.Meta.Hostname != base.Hostname {
			// hostname-only change is weak evidence: a re-provisioned
			// host looks identical otherwise
			return 30, "hostname changed"
		}
		return 0, ""
	}
	return capScore(50 + 15*len(diffs)), strings.Join(diffs, ",") + " changed"
}

// concurrentConnection fires when a second live sync stream for the same
// peer key comes from a different source IP.
func (e *Engine) concurrentConnection(key, srcIP string, now time.Time) (score int, reason string, existingIP string) {
	ips := e.liveStreams[key]
	if len(ips) == 0 {
		return 0, "", ""
	}
	for ip, opened := range ips {
		if ip != srcIP && now.Sub(opened) > e.cfg().ConcurrentGracePeriod {
			return 85, fmt.Sprintf("same peer key live from %s and %s", ip, srcIP), ip
		}
	}
	return 0, "", ""
}

func (e *Engine) authFailureBurst(f AuthFailure, now time.Time) (score int, reason string) {
	key := string(f.Kind) + "/" + f.SourceIP.String()
	n := e.failCounts.hit(key, now, e.cfg().AuthFailWindow, e.cfg().MaxAuthKeys)
	if n > e.cfg().AuthFailThreshold {
		return 70, fmt.Sprintf("%d failures from %s in %s", n, f.SourceIP, e.cfg().AuthFailWindow)
	}
	// per-subject too (credential stuffing against one identity)
	if f.Subject != "" {
		subj := string(f.Kind) + "/subj/" + f.Subject
		if e.failCounts.hit(subj, now, e.cfg().AuthFailWindow, e.cfg().MaxAuthKeys) > e.cfg().AuthFailThreshold {
			return 75, fmt.Sprintf("%d failures against %s in %s", n, f.Subject, e.cfg().AuthFailWindow)
		}
	}
	return 0, ""
}

func (e *Engine) authSuccessAfterFailures(s AuthSuccess, now time.Time) (score int, reason string) {
	if s.Subject == "" {
		return 0, ""
	}
	subj := string(s.Kind) + "/subj/" + s.Subject
	if e.failCounts.count(subj, now, e.cfg().AuthFailWindow) >= e.cfg().AuthSuccessAfterFails {
		return 55, "successful auth after a failure burst"
	}
	return 0, ""
}

func (e *Engine) setupKeyMisuse(o SetupKeyObservation, now time.Time) (score int, reason string) {
	ips := e.setupKeyIPs.add(o.KeyID, o.SourceIP.String(), now, e.cfg().SetupKeyIPWindow, e.cfg().MaxAuthKeys)
	regs := e.setupKeyRegs.hit(o.KeyID, now, e.cfg().SetupKeyRegWindow, e.cfg().MaxAuthKeys)
	if regs > e.cfg().SetupKeyRegThreshold {
		return 75, fmt.Sprintf("%d registrations via setup key in %s", regs, e.cfg().SetupKeyRegWindow)
	}
	if ips > e.cfg().SetupKeyIPThreshold {
		return 60, fmt.Sprintf("setup key used from %d distinct IPs in %s", ips, e.cfg().SetupKeyIPWindow)
	}
	return 0, ""
}

func (e *Engine) userNewCountry(accountID, userID, country string, now time.Time) (score int, reason string) {
	if country == "" {
		return 0, ""
	}
	key := accountID + "/" + userID
	e.mu.Lock()
	set := e.userCountries[key]
	if set == nil {
		set = make(map[string]struct{})
		e.userCountries[key] = set
	}
	_, known := set[country]
	set[country] = struct{}{}
	e.mu.Unlock()
	if !known && len(set) > 1 { // first country seeds the baseline silently
		return 65, "dashboard login from a new country"
	}
	return 0, ""
}

func (e *Engine) patNewIP(accountID, tokenID string, src netip.Addr, now time.Time) (score int, reason string) {
	key := accountID + "/" + tokenID
	n := e.patIPs.add(key, src.String(), now, 30*24*time.Hour, e.cfg().MaxAuthKeys)
	if n > 1 { // first use seeds silently
		return 55, "PAT used from a new IP"
	}
	return 0, ""
}
