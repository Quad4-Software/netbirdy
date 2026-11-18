package restrict

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFilter_AllInvalidAllowlistFailsClosed is a regression test:
// an allowlist whose every entry fails to parse must deny, not silently
// degrade to allow-all.
func TestParseFilter_AllInvalidAllowlistFailsClosed(t *testing.T) {
	f := ParseFilter(FilterConfig{AllowedCIDRs: []string{"garbage", "999.0.0.0/8", ""}})
	require.NotNil(t, f, "a filter is still built when the allowlist is configured")
	assert.True(t, f.HasRestrictions())

	for _, s := range []string{"10.1.2.3", "192.168.0.1", "8.8.8.8", "100.64.0.1", "::1"} {
		assert.Equal(t, DenyCIDR, f.Check(netip.MustParseAddr(s), nil), "unparseable allowlist must deny %s", s)
		assert.Equal(t, DenyCIDR, f.CheckCIDR(netip.MustParseAddr(s)), "CheckCIDR path must also deny %s", s)
	}
}

func TestParseFilter_PartiallyInvalidAllowlistUsesValidEntries(t *testing.T) {
	f := ParseFilter(FilterConfig{AllowedCIDRs: []string{"garbage", "10.0.0.0/8"}})
	require.NotNil(t, f)
	assert.Equal(t, Allow, f.Check(netip.MustParseAddr("10.9.9.9"), nil),
		"a valid allowlist entry still admits its range")
	assert.Equal(t, DenyCIDR, f.Check(netip.MustParseAddr("192.168.1.1"), nil),
		"the valid entries still exclude everything else")
}

func TestParseFilter_AllInvalidBlocklistFailsOpen(t *testing.T) {
	// A blocklist is a denylist: unparsable entries mean nothing extra is
	// blocked, which is the documented best-effort direction.
	f := ParseFilter(FilterConfig{BlockedCIDRs: []string{"garbage"}})
	require.NotNil(t, f)
	assert.Equal(t, Allow, f.Check(netip.MustParseAddr("10.0.0.1"), nil))
}
