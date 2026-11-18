package auth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCredentialFormValue_OversizedBodyNotParsed is a regression test for
// the pre-auth body bound: PostFormValue buffers the entire request body
// (and multipart parsing can spill to temp files), so an oversized or
// unknown-length body must yield no credential rather than being parsed.
func TestCredentialFormValue_OversizedBodyNotParsed(t *testing.T) {
	form := "pin=123456&pad=" + strings.Repeat("a", maxCredentialFormBytes)
	r := httptest.NewRequest(http.MethodPost, "http://x/login", strings.NewReader(form))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assert.Empty(t, credentialFormValue(r, "pin"),
		"a body over the cap must not be parsed for credentials")
}

func TestCredentialFormValue_ChunkedBodyNotParsed(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://x/login", nil)
	r.Body = io.NopCloser(strings.NewReader("pin=123456"))
	r.ContentLength = -1 // chunked framing: length unknown until read
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assert.Empty(t, credentialFormValue(r, "pin"),
		"an unknown-length body must not be parsed for credentials")
}

func TestCredentialFormValue_SmallFormParses(t *testing.T) {
	body := "pin=123456&pad=" + strings.Repeat("a", 1<<10)
	r := httptest.NewRequest(http.MethodPost, "http://x/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assert.Equal(t, "123456", credentialFormValue(r, "pin"))
}

func TestCredentialFormValue_AtCapParses(t *testing.T) {
	pad := strings.Repeat("a", maxCredentialFormBytes-len("pin=123456&pad="))
	r := httptest.NewRequest(http.MethodPost, "http://x/login", strings.NewReader("pin=123456&pad="+pad))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assert.Equal(t, "123456", credentialFormValue(r, "pin"),
		"a body exactly at the cap is still a legitimate login")
}

func TestCredentialFormValue_NonPostIgnored(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://x/login?pin=1", nil)
	assert.Empty(t, credentialFormValue(r, "pin"))
}
