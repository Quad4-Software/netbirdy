package bodytap

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestScanRoutingFields_ReplayIdentity is the metamorphic invariant:
// whatever the scanner consumes, the restored body must replay the exact
// original byte stream to the upstream.
func TestScanRoutingFields_ReplayIdentity(t *testing.T) {
	bodies := map[string]string{
		"empty object":      "{}",
		"scalar json":       `"hello"`,
		"not json":          "this is definitely not json \x00\xff",
		"truncated mid-key": `{"model":"claude-opus-4-8","mess`,
		"nested deep":       `{"a":{"b":{"c":{"d":{"model":"x"}}}},"model":"m"}`,
		"huge non-json":     strings.Repeat("z", 2<<20),
		"model then tail":   `{"model":"gpt-4o","stream":true,"messages":[]}` + strings.Repeat(" ", 4096),
		"duplicate model":   `{"model":"first","model":"second"}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "https://x/v1/messages", strings.NewReader(body))
			_, _, _, release := ScanRoutingFields(req, MaxRoutingScanBytes, NewBudget(0))
			defer release()
			got, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.Equal(t, body, string(got), "replayed body must equal the input byte for byte")
		})
	}
}

// TestScanRoutingFields_ReplayIdentityBeyondCeiling covers the case where
// the scan gives up at the ceiling: the prefix plus the unread tail must
// still reconstruct the full body.
func TestScanRoutingFields_ReplayIdentityBeyondCeiling(t *testing.T) {
	body := strings.Repeat(`{"a":"b","`, 1<<20) // ~2.5 MiB, mostly scanned
	req := httptest.NewRequest("POST", "https://x/v1/messages", strings.NewReader(body))
	_, _, _, release := ScanRoutingFields(req, 1<<20, NewBudget(0))
	defer release()
	got, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
}

// TestScanRoutingFields_BudgetCharging verifies the retained scan buffer
// is charged to the shared capture budget.
func TestScanRoutingFields_BudgetCharging(t *testing.T) {
	body := `{"model":"gpt-4o","messages":[{"role":"user","content":"` + strings.Repeat("x", 4096) + `"}]}`
	b := NewBudget(1 << 20)

	req := httptest.NewRequest("POST", "https://x/v1/messages", strings.NewReader(body))
	_, _, ok, release := ScanRoutingFields(req, MaxRoutingScanBytes, b)
	assert.True(t, ok)

	// Budget must be holding roughly the consumed prefix, and release
	// must return it fully.
	release()
	req2 := httptest.NewRequest("POST", "https://x/v1/messages", strings.NewReader(strings.Repeat("y", 1<<19)))
	_, _, _, release2 := ScanRoutingFields(req2, MaxRoutingScanBytes, b)
	defer release2()
	// A second scan acquiring ~512KiB must succeed only if the first
	// release actually returned its share.
	bb := NewBudget(1)
	req3 := httptest.NewRequest("POST", "https://x/v1/messages", strings.NewReader(`{"model":"m","x":"`+strings.Repeat("x", 4096)+`"}`))
	_, _, ok3, release3 := ScanRoutingFields(req3, MaxRoutingScanBytes, bb)
	defer release3()
	assert.False(t, ok3, "a budget too small for the consumed prefix must fail the scan")
	got, _ := io.ReadAll(req3.Body)
	assert.Contains(t, string(got), `"model":"m"`, "body must still replay when the budget check fails")
}
