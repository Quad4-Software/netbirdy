package updater

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/netbirdio/netbird/shared/updater/reposign"
)

// RootProvider supplies the trust root; default is the embedded keys.
type RootProvider func() ([]reposign.PublicKey, error)

func clientFor(base string) *http.Client {
	return &http.Client{
		CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" || len(via) > 5 {
				return fmt.Errorf("disallowed redirect")
			}
			return nil
		},
	}
}

func getFile(ctx context.Context, c *http.Client, raw string, cap int64) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	local := strings.HasPrefix(raw, "http://127.0.0.1") || strings.HasPrefix(raw, "http://localhost")
	if u.Scheme != "https" && !local {
		return nil, fmt.Errorf("non-https url %s", raw)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %d", raw, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, cap))
}

// fetchSignedImages downloads and verifies the signed images list.
func loadKeys(ctx context.Context, c *http.Client, base string, root []reposign.PublicKey) ([]reposign.PublicKey, error) {
	rl, err := getFile(ctx, c, strings.TrimRight(base, "/")+"/revocation-list.json", 1<<20)
	if err != nil {
		return nil, err
	}
	rs, err := getFile(ctx, c, strings.TrimRight(base, "/")+"/revocation-list.json.sig", 1<<20)
	if err != nil {
		return nil, err
	}
	rsig, err := reposign.ParseSignature(rs)
	if err != nil {
		return nil, err
	}
	rev, err := reposign.ValidateRevocationList(root, rl, *rsig)
	if err != nil {
		return nil, err
	}
	b, err := getFile(ctx, c, strings.TrimRight(base, "/")+"/artifact-key-pub.pem", 1<<20)
	if err != nil {
		return nil, err
	}
	bs, err := getFile(ctx, c, strings.TrimRight(base, "/")+"/artifact-key-pub.pem.sig", 1<<20)
	if err != nil {
		return nil, err
	}
	bsig, err := reposign.ParseSignature(bs)
	if err != nil {
		return nil, err
	}
	return reposign.ValidateArtifactKeys(root, b, *bsig, rev)
}

func fetchSignedImages(ctx context.Context, cfg Config, version string, rootProvider RootProvider) (map[string]string, error) {
	c := clientFor(cfg.ReleaseBase)
	base := strings.TrimRight(cfg.ReleaseBase, "/") + "/" + version
	list, err := getFile(ctx, c, base+"/netbirdy_"+strings.TrimPrefix(version, "v")+"_images.txt", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("images list: %w", err)
	}
	sig, err := getFile(ctx, c, base+"/netbirdy_"+strings.TrimPrefix(version, "v")+"_images.txt.sig", 1<<20)
	if err != nil {
		return nil, fmt.Errorf("images sig: %w", err)
	}
	root, err := rootProvider()
	if err != nil {
		return nil, err
	}
	keys, err := loadKeys(ctx, c, cfg.KeysBase, root)
	if err != nil {
		return nil, err
	}
	sigP, err := reposign.ParseSignature(sig)
	if err != nil {
		return nil, err
	}
	if err := reposign.ValidateArtifact(keys, list, *sigP); err != nil {
		return nil, fmt.Errorf("images list signature: %w", err)
	}
	return ParseImages(list, cfg.ImagePrefix, cfg.Services)
}
