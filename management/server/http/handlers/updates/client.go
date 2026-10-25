package updates

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HTTPUpdater talks to the sidecar updater program over HTTP.
type HTTPUpdater struct {
	base   string
	token  string
	client *http.Client
}

// NewHTTPUpdater returns a client for the updater at rawURL. The URL must use http or https.
func NewHTTPUpdater(rawURL, token string) (*HTTPUpdater, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("updater url must be an absolute http or https url")
	}
	return &HTTPUpdater{
		base:   strings.TrimRight(u.String(), "/"),
		token:  token,
		client: &http.Client{Timeout: updaterTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
	}, nil
}

func (u *HTTPUpdater) do(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u.base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if u.token != "" {
		req.Header.Set("Authorization", "Bearer "+u.token)
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if len(data) > maxBody {
		return nil, resp.StatusCode, errors.New("updater response too large")
	}
	return data, resp.StatusCode, nil
}

// Status returns the updater status document.
func (u *HTTPUpdater) Status(ctx context.Context) (json.RawMessage, error) {
	data, code, err := u.do(ctx, http.MethodGet, "/v1/status", nil)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("updater status returned %d", code)
	}
	return data, nil
}

// Apply asks the updater to move the stack to version.
func (u *HTTPUpdater) Apply(ctx context.Context, ver string) error {
	if !versionRe.MatchString(ver) {
		return fmt.Errorf("invalid version %q", ver)
	}
	body, err := json.Marshal(applyRequest{Version: ver})
	if err != nil {
		return err
	}
	_, code, err := u.do(ctx, http.MethodPost, "/v1/update", body)
	if err != nil {
		return err
	}
	if code != http.StatusOK && code != http.StatusAccepted {
		return fmt.Errorf("updater returned %d", code)
	}
	return nil
}
