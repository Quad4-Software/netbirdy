// Package updatemirror caches signed client update artifacts fetched from
// GitHub releases and serves them to NetBirdy clients over the management
// HTTPS port. The mirror is an untrusted transport: every file is verified
// against the embedded reposign root and the release checksums before
// admission, and clients verify again independently.
package updatemirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/netbirdio/netbird/shared/updater/reposign"
	"golang.org/x/time/rate"
)

const (
	defaultRepository = "Quad4-Software/netbirdy"
	defaultAPIBaseURL = "https://api.github.com"
	defaultCacheMax   = 2 << 30 // 2 GiB
	defaultPoll       = time.Hour
	maxFileBytes      = 600 << 20 // 600 MiB
	userAgent         = "netbirdy-mirror"
	manifestPath      = "manifest.json"
)

var (
	allowedRedirectHosts = map[string]bool{
		"github.com":                           true,
		"api.github.com":                       true,
		"objects.githubusercontent.com":        true,
		"release-assets.githubusercontent.com": true,
	}
	nameRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)
	versionRe = regexp.MustCompile(`^v?[0-9][A-Za-z0-9._-]{0,63}$`)
)

// Config is the server.update_mirror YAML section.
type Config struct {
	Enabled        bool                 `yaml:"enabled"`
	Repository     string               `yaml:"repository"`
	APIBaseURL     string               `yaml:"api_base_url"` // testing only
	CacheDir       string               `yaml:"cache_dir"`
	MaxCacheBytes  int64                `yaml:"max_cache_bytes"`
	PollInterval   time.Duration        `yaml:"poll_interval"`
	GitHubTokenEnv string               `yaml:"github_token_env"`
	Platforms      []string             `yaml:"platforms"`
	RootKeys       []reposign.PublicKey `yaml:"-"` // test injection
	Prefetch       bool                 `yaml:"prefetch"`
	KeysBaseURL    string               `yaml:"keys_base_url"` // testing only
	RatePerSecond  float64              `yaml:"rate_per_second"`
	MaxTransfers   int                  `yaml:"max_transfers"`
}

func (c *Config) defaults() {
	if c.Repository == "" {
		c.Repository = defaultRepository
	}
	if c.APIBaseURL == "" {
		c.APIBaseURL = defaultAPIBaseURL
	}
	if c.CacheDir == "" {
		c.CacheDir = "updates"
	}
	if c.MaxCacheBytes <= 0 {
		c.MaxCacheBytes = defaultCacheMax
	}
	if c.PollInterval <= 0 {
		c.PollInterval = defaultPoll
	}
	if c.KeysBaseURL == "" {
		c.KeysBaseURL = "https://raw.githubusercontent.com/" + c.Repository + "/main/publickeys/artifact-signatures/keys"
	}
	if c.RatePerSecond <= 0 {
		c.RatePerSecond = 10
	}
	if c.MaxTransfers <= 0 {
		m := 8
		c.MaxTransfers = m
	}
}

// Status mirrors the cache state for the dashboard API.
type Status struct {
	Versions  []string  `json:"versions"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	LastPoll  time.Time `json:"last_poll"`
	NextPoll  time.Time `json:"next_poll"`
	LastError string    `json:"last_error,omitempty"`
}

type cacheIndex struct {
	Files    map[string]int64  `json:"files"` // path -> bytes
	Sums     map[string]string `json:"sums"`  // path -> sha256 hex
	LastUsed map[string]int64  `json:"last_used"`
}

type asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type release struct {
	TagName string  `json:"tag_name"`
	Assets  []asset `json:"assets"`
	ETag    string  `json:"-"`
}

// Mirror is the cache and HTTP handler.
type Mirror struct {
	cfg    Config
	client *http.Client

	mu     sync.RWMutex
	pollMu sync.Mutex
	index  cacheIndex
	st     Status
	etag   map[string]string
	stop   chan struct{}

	platforms  func() []string      // returns "os/arch" pairs
	target     func() string        // resolves the configured version target
	rootKeys   []reposign.PublicKey // nil means use the embedded root
	artKeys    []reposign.PublicKey // validated artifact keys, cached
	artKeysAt  time.Time
	revList    *reposign.RevocationList
	keysTTL    time.Duration
	keysCache  map[string]keysCacheEnt
	limiter    *rate.Limiter
	transferCh chan struct{}
}

// New builds the mirror; platformsFn returns "os/arch" pairs currently in
// use, targetFn resolves AutoUpdateVersion ("", "latest", or "vX.Y.Z").
func New(cfg Config, platformsFn func() []string, targetFn func() string) *Mirror {
	cfg.defaults()
	m := &Mirror{
		cfg:       cfg,
		index:     cacheIndex{Files: map[string]int64{}, Sums: map[string]string{}, LastUsed: map[string]int64{}},
		etag:      map[string]string{},
		keysCache: map[string]keysCacheEnt{},
		stop:      make(chan struct{}),
		platforms: platformsFn,
		target:    targetFn,
	}
	m.client = &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" || !allowedRedirectHosts[req.URL.Hostname()] {
				return errors.New("redirect to disallowed host: " + req.URL.Host)
			}
			if len(via) > 5 {
				return errors.New("too many redirects")
			}
			return nil
		},
	}
	m.limiter = rate.NewLimiter(rate.Limit(cfg.RatePerSecond), int(cfg.RatePerSecond))
	m.transferCh = make(chan struct{}, cfg.MaxTransfers)
	if cfg.RootKeys != nil {
		m.rootKeys = cfg.RootKeys
	}
	_ = m.loadIndex()
	return m
}

func (m *Mirror) indexFile() string { return filepath.Join(m.cfg.CacheDir, "index.json") }

func (m *Mirror) loadIndex() error {
	b, err := os.ReadFile(m.indexFile())
	if err != nil {
		return err
	}
	return json.Unmarshal(b, &m.index)
}

func (m *Mirror) saveIndexLocked() {
	b, _ := json.Marshal(m.index)
	tmp := fmt.Sprintf("%s.tmp-%d", m.indexFile(), time.Now().UnixNano())
	if err := os.WriteFile(tmp, b, 0o640); err == nil {
		_ = os.Rename(tmp, m.indexFile())
	}
}

func (m *Mirror) get(ctx context.Context, raw string, etagKey string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/vnd.github+json")
	if m.cfg.GitHubTokenEnv != "" {
		if tok := os.Getenv(m.cfg.GitHubTokenEnv); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	m.mu.Lock()
	if e := m.etag[etagKey]; etagKey != "" && e != "" {
		req.Header.Set("If-None-Match", e)
	}
	m.mu.Unlock()
	if u, err := url.Parse(raw); err != nil || u.Scheme != "https" {
		return nil, fmt.Errorf("refusing non-https url: %s", raw)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusOK && etagKey != "" {
		if e := resp.Header.Get("ETag"); e != "" {
			m.mu.Lock()
			m.etag[etagKey] = e
			m.mu.Unlock()
		}
	}
	return resp, nil
}

// PlatformsFromPeers maps peer (GoOS, arch) pairs to update platforms:
// windows and darwin only; linux/android/ios/unknown are ignored.
func PlatformsFromPeers(metas [][2]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range metas {
		osx, arch := m[0], m[1]
		if arch == "" {
			arch = "amd64"
		}
		if osx != "windows" && osx != "darwin" {
			continue
		}
		k := osx + "/" + arch
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// platformsFor resolves the wanted "os/arch" pairs.
func (m *Mirror) platformsFor() []string {
	if len(m.cfg.Platforms) > 0 {
		return m.cfg.Platforms
	}
	if m.platforms != nil {
		if p := m.platforms(); len(p) > 0 {
			return p
		}
	}
	return []string{"windows/amd64", "darwin/arm64"}
}

// assetNames returns the release file names wanted for one platform.
func assetNames(version, osx, arch string) []string {
	switch osx {
	case "windows":
		return []string{
			fmt.Sprintf("netbirdy_installer_%s_windows_%s.msi", version, arch),
			fmt.Sprintf("netbirdy_installer_%s_windows_%s.exe", version, arch),
		}
	case "darwin":
		return []string{fmt.Sprintf("netbirdy_%s_darwin_%s.pkg", version, arch)}
	}
	return nil
}

func (m *Mirror) fetchRelease(ctx context.Context) (*release, bool, error) {
	u := strings.TrimSuffix(m.cfg.APIBaseURL, "/") + "/repos/" + m.cfg.Repository + "/releases/latest"
	resp, err := m.get(ctx, u, "latest")
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("releases api: %d", resp.StatusCode)
	}
	var r release
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, false, err
	}
	return &r, false, nil
}

func (m *Mirror) fetchChecksums(ctx context.Context, assets []asset) (map[string]string, error) {
	for _, a := range assets {
		if !strings.HasSuffix(a.Name, "checksums.txt") {
			continue
		}
		resp, err := m.get(ctx, a.URL, a.Name)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("checksums: %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		out := map[string]string{}
		for _, line := range strings.Split(string(body), "\n") {
			f := strings.Fields(line)
			if len(f) == 2 && len(f[0]) == 64 {
				out[path.Base(f[1])] = strings.ToLower(f[0])
			}
		}
		return out, nil
	}
	return nil, errors.New("checksums file not found in release")
}

func (m *Mirror) admit(ctx context.Context, a asset, want map[string]string) error {
	if !nameRe.MatchString(a.Name) {
		return fmt.Errorf("bad name %q", a.Name)
	}
	if a.Size > maxFileBytes {
		return fmt.Errorf("oversize %q", a.Name)
	}
	resp, err := m.get(ctx, a.URL, a.Name)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %q: %d", a.Name, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFileBytes+1))
	if err != nil {
		return err
	}
	if int64(len(body)) > maxFileBytes {
		return fmt.Errorf("oversize body %q", a.Name)
	}
	sum := sha256.Sum256(body)
	if want[a.Name] != hex.EncodeToString(sum[:]) {
		return fmt.Errorf("checksum mismatch %q", a.Name)
	}
	sigBytes, err := m.verifyArtifact(ctx, a.URL+".sig", body)
	if err != nil {
		return fmt.Errorf("signature %q: %w", a.Name, err)
	}
	if err := m.writeCache(path.Join("files", a.Name), body); err != nil {
		return err
	}
	if err := m.writeCache(path.Join("files", a.Name+".sig"), sigBytes); err != nil {
		return err
	}
	return nil
}

func (m *Mirror) writeCache(rel string, body []byte) error {
	rel = filepath.Clean(rel)
	if strings.Contains(rel, "..") {
		return errors.New("bad path")
	}
	dst := filepath.Join(m.cfg.CacheDir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o750); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", dst, time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640)
	if err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	m.mu.Lock()
	sum := sha256.Sum256(body)
	m.index.Files[rel] = int64(len(body))
	m.index.Sums[rel] = hex.EncodeToString(sum[:])
	m.index.LastUsed[rel] = time.Now().Unix()
	m.saveIndexLocked()
	m.mu.Unlock()
	return nil
}

func (m *Mirror) evict(target string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, b := range m.index.Files {
		total += b
	}
	if total <= m.cfg.MaxCacheBytes {
		return
	}
	type e struct {
		rel string
		at  int64
	}
	var es []e
	for rel := range m.index.Files {
		if target != "" && strings.Contains(filepath.Base(rel), strings.TrimPrefix(target, "v")) {
			continue
		}
		es = append(es, e{rel, m.index.LastUsed[rel]})
	}
	// LRU first
	for i := 0; i < len(es); i++ {
		for j := i + 1; j < len(es); j++ {
			if es[j].at < es[i].at {
				es[i], es[j] = es[j], es[i]
			}
		}
	}
	for _, it := range es {
		if total <= m.cfg.MaxCacheBytes {
			break
		}
		_ = os.Remove(filepath.Join(m.cfg.CacheDir, it.rel))
		total -= m.index.Files[it.rel]
		delete(m.index.Files, it.rel)
		delete(m.index.Sums, it.rel)
		delete(m.index.LastUsed, it.rel)
	}
	m.saveIndexLocked()
}

// root returns the trust root: injected or the embedded production root.
func (m *Mirror) root() ([]reposign.PublicKey, error) {
	if m.rootKeys != nil {
		return m.rootKeys, nil
	}
	return reposign.EmbeddedRootKeys()
}

// keysFile fetches one key-tree file from the keys base URL.
func (m *Mirror) keysFile(ctx context.Context, name string) ([]byte, error) {
	u := strings.TrimSuffix(m.cfg.KeysBaseURL, "/") + "/" + name
	resp, err := m.get(ctx, u, "keys:"+name)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("keys file %q: %d", name, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 1<<20))
}

type keysCacheEnt struct {
	data []byte
	at   time.Time
}

// keysCached serves key tree files from a short lived in memory cache so the
// unauthenticated keys route cannot be used to amplify upstream fetches.
func (m *Mirror) keysCached(ctx context.Context, name string) ([]byte, error) {
	m.mu.Lock()
	e, ok := m.keysCache[name]
	if ok && time.Since(e.at) < 10*time.Minute {
		m.mu.Unlock()
		return e.data, nil
	}
	m.mu.Unlock()
	b, err := m.keysFile(ctx, name)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.keysCache[name] = keysCacheEnt{data: b, at: time.Now()}
	m.mu.Unlock()
	return b, nil
}

// trustedArtifactKeys validates the revocation list and artifact key
// bundle against the root and caches them for the poll cycle.
func (m *Mirror) trustedArtifactKeys(ctx context.Context) ([]reposign.PublicKey, error) {
	ttl := m.keysTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	if m.artKeys != nil && m.revList != nil &&
		time.Since(m.artKeysAt) < ttl &&
		(m.revList.ExpiresAt.IsZero() || time.Now().Before(m.revList.ExpiresAt)) {
		return m.artKeys, nil
	}
	root, err := m.root()
	if err != nil {
		return nil, err
	}
	rl, err := m.keysFile(ctx, "revocation-list.json")
	if err != nil {
		return nil, err
	}
	rs, err := m.keysFile(ctx, "revocation-list.json.sig")
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
	bundle, err := m.keysFile(ctx, "artifact-key-pub.pem")
	if err != nil {
		return nil, err
	}
	bs, err := m.keysFile(ctx, "artifact-key-pub.pem.sig")
	if err != nil {
		return nil, err
	}
	bsig, err := reposign.ParseSignature(bs)
	if err != nil {
		return nil, err
	}
	keys, err := reposign.ValidateArtifactKeys(root, bundle, *bsig, rev)
	if err != nil {
		return nil, err
	}
	if rev != nil && !rev.ExpiresAt.IsZero() && time.Now().After(rev.ExpiresAt) {
		return nil, errors.New("revocation list expired")
	}
	m.revList = rev
	m.artKeys = keys
	m.artKeysAt = time.Now()
	return keys, nil
}

// verifyArtifact fetches <asset>.sig and validates the artifact bytes.
func (m *Mirror) verifyArtifact(ctx context.Context, sigURL string, body []byte) ([]byte, error) {
	keys, err := m.trustedArtifactKeys(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := m.get(ctx, sigURL, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sig fetch: %d", resp.StatusCode)
	}
	sigBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	sig, err := reposign.ParseSignature(sigBytes)
	if err != nil {
		return nil, err
	}
	if err := reposign.ValidateArtifact(keys, body, *sig); err != nil {
		return nil, err
	}
	return sigBytes, nil
}

// PollOnce performs one fetch cycle and returns its error (for tests).
func (m *Mirror) PollOnce(ctx context.Context) error {
	if !m.pollMu.TryLock() {
		return errors.New("poll already in progress")
	}
	defer m.pollMu.Unlock()
	rel, notmod, err := m.fetchRelease(ctx)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.st.LastPoll = time.Now()
	m.st.NextPoll = m.st.LastPoll.Add(m.cfg.PollInterval)
	m.mu.Unlock()
	if notmod {
		return nil
	}
	tgt := ""
	if m.target != nil {
		tgt = m.target()
	}
	want, err := m.fetchChecksums(ctx, rel.Assets)
	if err != nil {
		m.setErr(err)
		return err
	}
	for _, p := range m.platformsFor() {
		osx, arch, _ := strings.Cut(p, "/")
		for _, name := range assetNames(strings.TrimPrefix(rel.TagName, "v"), osx, arch) {
			for _, a := range rel.Assets {
				if a.Name != name {
					continue
				}
				if err := m.admit(ctx, a, want); err != nil {
					m.setErr(err)
					return err
				}
			}
		}
	}
	m.evict(tgt)
	m.mu.Lock()
	m.st.Versions = append([]string{rel.TagName}, m.st.Versions...)
	m.st.Files = len(m.index.Files)
	m.st.Bytes = 0
	for _, b := range m.index.Files {
		m.st.Bytes += b
	}
	m.st.LastError = ""
	m.mu.Unlock()
	return nil
}

func (m *Mirror) setErr(err error) {
	m.mu.Lock()
	m.st.LastError = err.Error()
	m.mu.Unlock()
}

// StatusSnapshot returns the current status.
func (m *Mirror) StatusSnapshot() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.st
}

// Run polls until Close.
func (m *Mirror) Run(ctx context.Context) {
	t := time.NewTimer(0)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case <-t.C:
		}
		if err := m.PollOnce(ctx); err != nil {
			log.Warnf("update mirror: %v", err)
		}
		j := m.cfg.PollInterval / 10
		t.Reset(m.cfg.PollInterval + time.Duration(time.Now().UnixNano()%int64(j+1)))
	}
}

// Close stops the loop.
func (m *Mirror) Close() { close(m.stop) }

// --- HTTP handler -------------------------------------------------------

func (m *Mirror) serveVerified(w http.ResponseWriter, r *http.Request, rel string) {
	m.mu.RLock()
	_, ok := m.index.Files[rel]
	m.mu.RUnlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	// integrity: reject a cache entry whose bytes changed since admission
	f2, err := os.Open(filepath.Join(m.cfg.CacheDir, rel))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	h := sha256.New()
	_, err = io.Copy(h, f2)
	f2.Close()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	m.mu.RLock()
	exp, ok2 := m.index.Sums[rel]
	m.mu.RUnlock()
	if !ok2 || exp != hex.EncodeToString(h.Sum(nil)) {
		// corrupted or pre-index file: drop it and refuse
		m.mu.Lock()
		delete(m.index.Files, rel)
		delete(m.index.Sums, rel)
		m.mu.Unlock()
		http.NotFound(w, r)
		return
	}
	serveFile(w, r, filepath.Join(m.cfg.CacheDir, rel))
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "public, max-age=300")
	http.ServeContent(w, r, filepath.Base(path), st.ModTime(), f)
}

// Handler returns http.Handler for /updates/v1/.
func (m *Mirror) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /manifest.json", func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		defer m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_ = json.NewEncoder(w).Encode(m.st)
	})
	mux.HandleFunc("GET /files/{ver}/{name}", m.file())
	mux.HandleFunc("GET /tag/{ver}/{name}", m.sig(true))
	mux.HandleFunc("GET /keys/{name}", m.sig(false))
	return mux
}

func (m *Mirror) file() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if !versionRe.MatchString(r.PathValue("ver")) || !nameRe.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		if !m.limiter.Allow() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		select {
		case m.transferCh <- struct{}{}:
			defer func() { <-m.transferCh }()
		default:
			http.Error(w, "busy", http.StatusTooManyRequests)
			return
		}
		rel := filepath.Join("files", name)
		m.serveVerified(w, r, rel)
	}
}

func (m *Mirror) sig(isTag bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		if isTag {
			if !versionRe.MatchString(r.PathValue("ver")) || !nameRe.MatchString(name) {
				http.NotFound(w, r)
				return
			}
			if !m.limiter.Allow() {
				http.Error(w, "rate limited", http.StatusTooManyRequests)
				return
			}
			if !strings.HasSuffix(name, ".sig") {
				http.NotFound(w, r)
				return
			}
			rel := filepath.Join("files", name)
			p := filepath.Join(m.cfg.CacheDir, rel)
			m.mu.RLock()
			_, ok := m.index.Files[rel]
			m.mu.RUnlock()
			if !ok {
				http.NotFound(w, r)
				return
			}
			serveFile(w, r, p)
			return
		}
		if !nameRe.MatchString(name) {
			http.NotFound(w, r)
			return
		}
		if !m.limiter.Allow() {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
			return
		}
		b, err := m.keysCached(r.Context(), name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(b)
	}
}
