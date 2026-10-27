// Package eventstream implements per-account activity event streaming
// integrations: it stores the integration configuration and forwards account
// activity events to external HTTP endpoints (generic webhooks and Datadog).
package eventstream

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"text/template"
	"time"
)

// Platform names understood by the API. Only generic_http and datadog deliver
// events on self-hosted installs. The AWS platforms need request signing and
// are rejected with ErrPlatformNotSupported.
const (
	PlatformGenericHTTP = "generic_http"
	PlatformDatadog     = "datadog"
	PlatformS3          = "s3"
	PlatformFirehose    = "firehose"
)

// maskedValue replaces stored secrets in API responses.
const maskedValue = "********"

var (
	// ErrPlatformNotSupported is returned for platforms that cannot deliver
	// events on a self-hosted install.
	ErrPlatformNotSupported = errors.New("platform is not supported on self-hosted")
	// ErrNotFound is returned when no integration with the requested ID exists
	// on the account.
	ErrNotFound = errors.New("event streaming integration not found")
)

// secretHeaderNames are request header names whose values are masked in API
// responses, matched case-insensitively.
var secretHeaderNames = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"x-api-key":           {},
	"dd-api-key":          {},
}

// Config carries the platform specific settings of an integration. The JSON
// shape matches the dashboard contract.
type Config struct {
	// URL is the delivery endpoint for the generic_http platform.
	URL string `json:"url,omitempty"`
	// Headers is a JSON encoded object of request headers for generic_http.
	Headers string `json:"headers,omitempty"`
	// BodyTemplate is an optional Go template rendered over the event JSON.
	BodyTemplate string `json:"body_template,omitempty"`
	// APIKey is the Datadog API key, mapped to the DD-API-KEY header.
	APIKey string `json:"api_key"`
	// APIURL is the Datadog logs intake URL.
	APIURL string `json:"api_url"`
}

// Integration is one configured event streaming destination of an account.
type Integration struct {
	// ID is the account unique numeric identifier.
	ID int64 `json:"id"`
	// AccountID owns the integration.
	AccountID string `json:"account_id"`
	// Enabled marks the integration as the active destination. At most one
	// integration per account is enabled at a time.
	Enabled bool `json:"enabled"`
	// Platform selects the delivery implementation.
	Platform string `json:"platform"`
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the last modification timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// Config holds the platform specific settings.
	Config Config `json:"config"`
}

// validatePlatform rejects unknown platforms and the AWS platforms that need
// request signing.
func validatePlatform(platform string) error {
	switch platform {
	case PlatformGenericHTTP, PlatformDatadog:
		return nil
	case PlatformS3, PlatformFirehose:
		return fmt.Errorf("%s: %w", platform, ErrPlatformNotSupported)
	default:
		return fmt.Errorf("unknown platform %q", platform)
	}
}

// validateHTTPURL requires an absolute http or https URL.
func validateHTTPURL(rawURL, field string) error {
	if rawURL == "" {
		return fmt.Errorf("config.%s is required", field)
	}
	u, err := url.Parse(rawURL)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return fmt.Errorf("config.%s is not a valid URL", field)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("config.%s must use http or https", field)
	}
	return nil
}

// parseHeaders decodes the JSON encoded header object. An empty string yields
// an empty map.
func parseHeaders(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil, fmt.Errorf("config.headers is not a JSON object: %w", err)
	}
	for k, v := range headers {
		if strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("config.headers contains an empty header name")
		}
		if strings.ContainsAny(v, "\r\n") {
			return nil, fmt.Errorf("config.headers contains a header value with a newline")
		}
	}
	return headers, nil
}

// encodeHeaders serializes a header map back to the JSON string form. An empty
// map yields an empty string.
func encodeHeaders(headers map[string]string) (string, error) {
	if len(headers) == 0 {
		return "", nil
	}
	data, err := json.Marshal(headers)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// isSecretHeader reports whether the header name carries a credential that is
// masked in API responses.
func isSecretHeader(name string) bool {
	_, ok := secretHeaderNames[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// validateConfig checks the platform specific fields of cfg.
func validateConfig(platform string, cfg Config) error {
	switch platform {
	case PlatformGenericHTTP:
		if err := validateHTTPURL(cfg.URL, "url"); err != nil {
			return err
		}
		if _, err := parseHeaders(cfg.Headers); err != nil {
			return err
		}
		if cfg.BodyTemplate != "" {
			if _, err := template.New("body").Parse(cfg.BodyTemplate); err != nil {
				return fmt.Errorf("config.body_template is not a valid template: %w", err)
			}
		}
		return nil
	case PlatformDatadog:
		if err := validateHTTPURL(cfg.APIURL, "api_url"); err != nil {
			return err
		}
		if cfg.APIKey == "" {
			return fmt.Errorf("config.api_key is required")
		}
		return nil
	default:
		return validatePlatform(platform)
	}
}

// Masked returns a copy of the integration with secret fields masked for API
// responses.
func (i *Integration) Masked() *Integration {
	out := *i
	out.Config.APIKey = maskSecret(i.Config.APIKey)
	out.Config.Headers = maskHeaders(i.Config.Headers)
	return &out
}

// maskSecret masks a non-empty secret, keeping empty values empty.
func maskSecret(secret string) string {
	if secret == "" {
		return ""
	}
	return maskedValue
}

// maskHeaders re-encodes the header JSON with secret header values replaced by
// the mask. Unparseable header JSON passes through unchanged rather than
// leaking values.
func maskHeaders(raw string) string {
	headers, err := parseHeaders(raw)
	if err != nil || len(headers) == 0 {
		return raw
	}
	masked := make(map[string]string, len(headers))
	for k, v := range headers {
		if isSecretHeader(k) && v != "" {
			masked[k] = maskedValue
			continue
		}
		masked[k] = v
	}
	out, err := encodeHeaders(masked)
	if err != nil {
		return raw
	}
	return out
}

// mergeSecrets fills empty or masked secret fields of update with the stored
// values, so a masked GET payload can be sent back unchanged.
func (c Config) mergeSecrets(stored Config) (Config, error) {
	if c.APIKey == "" || c.APIKey == maskedValue {
		c.APIKey = stored.APIKey
	}

	incoming, err := parseHeaders(c.Headers)
	if err != nil {
		return c, err
	}
	// Stored headers that fail to parse are treated as absent rather than
	// failing the update. They were validated on write.
	prev, _ := parseHeaders(stored.Headers)
	merged := false
	for k, v := range incoming {
		if isSecretHeader(k) && (v == "" || v == maskedValue) {
			if old, ok := prev[headerKey(prev, k)]; ok {
				incoming[k] = old
				merged = true
			}
		}
	}
	if merged || c.Headers != stored.Headers {
		encoded, err := encodeHeaders(incoming)
		if err != nil {
			return c, err
		}
		c.Headers = encoded
	}
	return c, nil
}

// headerKey finds the stored map key matching name case-insensitively.
func headerKey(headers map[string]string, name string) string {
	for k := range headers {
		if strings.EqualFold(k, name) {
			return k
		}
	}
	return name
}
