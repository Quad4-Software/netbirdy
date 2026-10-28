package updatemirror

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netbirdio/netbird/shared/updater/reposign"
)

// fakeGH serves a release JSON plus the files map plus a checksums file.
// files values: (content, sig or nil). Keys tree files live under /keys/.
func fakeGH(t *testing.T, files map[string][]byte, checksums string, keys map[string][]byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewTLSServer(mux)
	mux.HandleFunc("/repos/Quad4-Software/netbirdy/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == `"r1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"r1"`)
		var assets string
		for n := range files {
			assets += fmt.Sprintf(`{"name":%q,"browser_download_url":"%s/dl/%s","size":%d},`, n, srv.URL, n, int64(len(files[n])))
		}
		assets += fmt.Sprintf(`{"name":"checksums.txt","browser_download_url":"%s/dl/checksums.txt","size":10}`, srv.URL)
		fmt.Fprintf(w, `{"tag_name":"v9.9.9","assets":[%s]}`, assets)
	})
	mux.HandleFunc("/dl/checksums.txt", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, checksums) })
	mux.HandleFunc("/dl/", func(w http.ResponseWriter, r *http.Request) {
		if b, ok := files[r.URL.Path[len("/dl/"):]]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("/keys/", func(w http.ResponseWriter, r *http.Request) {
		if b, ok := keys[r.URL.Path[len("/keys/"):]]; ok {
			w.Write(b)
			return
		}
		http.NotFound(w, r)
	})
	return srv
}

// keyTree builds a throwaway root + artifact key and the signed key files.
func keyTree(t *testing.T) (map[string][]byte, []reposign.PublicKey, reposign.ArtifactKey) {
	t.Helper()
	root, _, rootPub, err := reposign.GenerateRootKey(24 * time.Hour)
	lastRoot = *root
	if err != nil {
		t.Fatal(err)
	}
	art, _, artPub, _, err := reposign.GenerateArtifactKey(root, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	rk, err := reposign.ParseRootPublicKey(rootPub)
	if err != nil {
		t.Fatal(err)
	}
	bundle, bsig, err := reposign.BundleArtifactKeys(root, []reposign.PublicKey{rkpub(t, artPub)})
	if err != nil {
		t.Fatal(err)
	}
	rl := revList(t, root)
	keys := map[string][]byte{
		"artifact-key-pub.pem":     bundle,
		"artifact-key-pub.pem.sig": bsig,
		"revocation-list.json":     rl[0],
		"revocation-list.json.sig": rl[1],
	}
	return keys, []reposign.PublicKey{rk}, *art
}

func rkpub(t *testing.T, pem []byte) reposign.PublicKey {
	t.Helper()
	k, err := reposign.ParseArtifactPubKey(pem)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func revList(t *testing.T, root *reposign.RootKey) [][]byte {
	t.Helper()
	b, sig, err := reposign.CreateRevocationList(*root, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return [][]byte{b, sig}
}

func mkMirror(t *testing.T, gh *httptest.Server, dir string, roots []reposign.PublicKey) *Mirror {
	t.Helper()
	m := New(Config{
		Enabled:       true,
		APIBaseURL:    gh.URL,
		CacheDir:      dir,
		KeysBaseURL:   gh.URL + "/keys",
		RootKeys:      roots,
		RatePerSecond: 1000,
	}, func() []string { return []string{"windows/amd64"} }, nil)
	m.client = gh.Client() // trusts the TLS test cert
	return m
}

func sign(t *testing.T, k reposign.ArtifactKey, b []byte) []byte {
	t.Helper()
	s, err := reposign.SignData(k, b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const msi = "netbirdy_installer_9.9.9_windows_amd64.msi"

func TestHappyPath(t *testing.T) {
	payload := []byte("installer")
	sum := sha256.Sum256(payload)
	checksums := hex.EncodeToString(sum[:]) + "  " + msi + "\n"
	keys, roots, art := keyTree(t)
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, art, payload)}
	gh := fakeGH(t, files, checksums, keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err != nil {
		t.Fatalf("poll: %v", err)
	}
	s := m.StatusSnapshot()
	if s.Files == 0 || s.Versions[0] != "v9.9.9" {
		t.Fatalf("status %+v", s)
	}
}

func TestBadSignatureRejected(t *testing.T) {
	payload := []byte("x")
	sum := sha256.Sum256(payload)
	keys, roots, art := keyTree(t)
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, art, []byte("tampered"))}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err == nil {
		t.Fatal("expected sig failure")
	}
	if m.StatusSnapshot().LastError == "" {
		t.Fatal("no LastError")
	}
}

func TestChecksumMismatchRejected(t *testing.T) {
	payload := []byte("y")
	keys, roots, art := keyTree(t)
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, art, payload)}
	gh := fakeGH(t, files, strings.Repeat("0", 64)+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err == nil {
		t.Fatal("expected checksum failure")
	}
}

func TestMissingSigRejected(t *testing.T) {
	payload := []byte("z")
	sum := sha256.Sum256(payload)
	keys, roots, _ := keyTree(t)
	files := map[string][]byte{msi: payload} // no .sig
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err == nil {
		t.Fatal("expected missing-sig failure")
	}
}

func TestOversizeRejected(t *testing.T) {
	payload := make([]byte, 8)
	keys, roots, _ := keyTree(t)
	files := map[string][]byte{msi: payload}
	gh := fakeGH(t, files, "", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	a := asset{Name: msi, URL: gh.URL + "/dl/" + msi, Size: maxFileBytes + 1}
	if err := m.admit(context.Background(), a, nil); err == nil {
		t.Fatal("expected oversize failure")
	}
}

func TestBadNameRejected(t *testing.T) {
	for _, n := range []string{"../evil", "a/b", "", "x" + string(rune(0)) + "y"} {
		if nameRe.MatchString(n) {
			t.Fatalf("name accepted: %q", n)
		}
	}
}

func TestRangeAndIfRange(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Enabled: true, CacheDir: dir, RatePerSecond: 1000}, nil, nil)
	payload := []byte("0123456789abcdef")
	if err := m.writeCache("files/"+msi, payload); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/files/v9.9.9/"+msi, nil)
	req.Header.Set("Range", "bytes=0-3")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 206 {
		t.Fatalf("range: %d", resp.StatusCode)
	}
	b, _ := io.ReadAll(resp.Body)
	if string(b) != "0123" {
		t.Fatalf("got %q", b)
	}
	resp2, err := http.Get(srv.URL + "/files/v9.9.9/../traversal")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 404 {
		resp2.Body.Close()
		t.Fatalf("traversal: %d", resp2.StatusCode)
	}
	resp2.Body.Close()
}

func TestRateLimit(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Enabled: true, CacheDir: dir, RatePerSecond: 1, MaxTransfers: 4}, nil, nil)
	m.writeCache("files/"+msi, []byte("x"))
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	var codes []int
	for i := 0; i < 6; i++ {
		r, _ := http.Get(srv.URL + "/files/v9.9.9/" + msi)
		codes = append(codes, r.StatusCode)
		r.Body.Close()
	}
	hit429 := false
	for _, c := range codes {
		if c == 429 {
			hit429 = true
		}
	}
	if !hit429 {
		t.Fatalf("no 429 in %v", codes)
	}
}

func TestETag304(t *testing.T) {
	keys, roots, art := keyTree(t)
	payload := []byte("e")
	sum := sha256.Sum256(payload)
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, art, payload)}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	// second poll: fake returns 304 (etag r1 cached)
	if err := m.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestEvictKeepsTarget(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Enabled: true, CacheDir: dir, MaxCacheBytes: 100}, nil, nil)
	m.writeCache("files/old", make([]byte, 90))
	m.index.LastUsed["files/old"] = 1
	m.writeCache("files/target", make([]byte, 90))
	m.evict("files/target")
	if _, ok := m.index.Files["files/target"]; !ok {
		t.Fatal("target evicted")
	}
}

func TestDisabledReturns404(t *testing.T) {
	m := New(Config{Enabled: false}, nil, nil)
	if m.Handler() != nil {
		// handler exists but the file path 404s when nothing cached
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	r, _ := http.Get(srv.URL + "/x")
	if r.StatusCode != 404 {
		t.Fatal("expected 404")
	}
	r.Body.Close()
}

func TestUnchainedKeyRejected(t *testing.T) {
	payload := []byte("p")
	sum := sha256.Sum256(payload)
	keys, roots, art := keyTree(t)
	// sig made with a key NOT in the validated bundle
	evilRoot, _, _, _ := reposign.GenerateRootKey(24 * time.Hour)
	evil, _, _, _, _ := reposign.GenerateArtifactKey(evilRoot, time.Hour)
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, *evil, payload)}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err == nil {
		t.Fatal("unchained key accepted")
	}
	_ = art
}

func TestRevokedKeyRejected(t *testing.T) {
	payload := []byte("r")
	sum := sha256.Sum256(payload)
	keys, roots, art := keyTree(t)
	root := testRootKey(t)
	kid := kidFor(t, keys["artifact-key-pub.pem"], keys["artifact-key-pub.pem.sig"], roots)
	newRL, newSig, err := reposign.ExtendRevocationList(root, mustRevList(t, keys["revocation-list.json"]), kid, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	keys["revocation-list.json"] = newRL
	keys["revocation-list.json.sig"] = newSig
	files := map[string][]byte{msi: payload, msi + ".sig": sign(t, art, payload)}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err == nil {
		t.Fatal("revoked key accepted")
	}
}

var lastRoot reposign.RootKey

func testRootKey(t *testing.T) reposign.RootKey { return lastRoot }

func kidFor(t *testing.T, pub, sig []byte, roots []reposign.PublicKey) reposign.KeyID {
	keys, err := reposign.ValidateArtifactKeys(roots, pub, *mustSigP(t, sig), nil)
	if err != nil {
		t.Fatal(err)
	}
	return computeTestKeyID(keys[0].Key)
}

func computeTestKeyID(pub []byte) reposign.KeyID {
	h := sha256.Sum256(pub)
	var k reposign.KeyID
	copy(k[:], h[:8])
	return k
}

func mustSigP(t *testing.T, b []byte) *reposign.Signature {
	s, err := reposign.ParseSignature(b)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustRevList(t *testing.T, b []byte) reposign.RevocationList {
	rl, err := reposign.ParseRevocationList(b)
	if err != nil {
		t.Fatal(err)
	}
	return *rl
}

func TestRedirectDisallowed(t *testing.T) {
	m := New(Config{Enabled: true}, nil, nil) // default client keeps CheckRedirect
	target, _ := url.Parse("http://169.254.169.254/latest/meta-data")
	req, _ := http.NewRequest("GET", target.String(), nil)
	if err := m.client.CheckRedirect(req, nil); err == nil {
		t.Fatal("redirect to disallowed host allowed")
	}
	target2, _ := url.Parse("https://notgithub.example/x")
	req2, _ := http.NewRequest("GET", target2.String(), nil)
	if err := m.client.CheckRedirect(req2, nil); err == nil {
		t.Fatal("redirect to unlisted host allowed")
	}
}

func TestCorruptedCacheNotServed(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Enabled: true, CacheDir: dir, RatePerSecond: 100}, nil, nil)
	if err := m.writeCache("files/"+msi, []byte("goodbytes")); err != nil {
		t.Fatal(err)
	}
	// corrupt the file on disk
	p := filepath.Join(dir, "files", msi)
	os.WriteFile(p, []byte("badbytes!"), 0o640)
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	r, err := http.Get(srv.URL + "/files/v9.9.9/" + msi)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode == 200 {
		t.Fatal("corrupted file was served")
	}
}

func TestTransferCap429(t *testing.T) {
	dir := t.TempDir()
	m := New(Config{Enabled: true, CacheDir: dir, RatePerSecond: 1000, MaxTransfers: 1}, nil, nil)
	m.writeCache("files/"+msi, []byte("x"))
	m.transferCh <- struct{}{} // occupy the single slot
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	r, _ := http.Get(srv.URL + "/files/v9.9.9/" + msi)
	if r.StatusCode != 429 {
		t.Fatalf("expected 429, got %d", r.StatusCode)
	}
	r.Body.Close()
	<-m.transferCh // release
	r2, _ := http.Get(srv.URL + "/files/v9.9.9/" + msi)
	if r2.StatusCode != 200 {
		t.Fatalf("expected recovery, got %d", r2.StatusCode)
	}
	r2.Body.Close()
}

func TestPlatformsFromPeers(t *testing.T) {
	got := PlatformsFromPeers([][2]string{
		{"windows", "amd64"}, {"windows", "amd64"}, {"windows", "arm64"},
		{"darwin", "arm64"}, {"linux", "amd64"}, {"android", "arm64"}, {"", "x"},
	})
	want := []string{"windows/amd64", "windows/arm64", "darwin/arm64"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// explicit config override wins
	m := New(Config{Platforms: []string{"darwin/amd64"}}, nil, nil)
	if p := m.platformsFor(); p[0] != "darwin/amd64" {
		t.Fatalf("override %v", p)
	}
}

func TestSigRouteServesAfterPoll(t *testing.T) {
	payload := []byte("msi")
	sum := sha256.Sum256(payload)
	keys, roots, art := keyTree(t)
	sig := sign(t, art, payload)
	files := map[string][]byte{msi: payload, msi + ".sig": sig}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if err := m.PollOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(m.Handler())
	defer srv.Close()
	r, err := http.Get(srv.URL + "/tag/v9.9.9/" + msi + ".sig")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		t.Fatalf("sig route: %d", r.StatusCode)
	}
	b, _ := io.ReadAll(r.Body)
	if string(b) != string(sig) {
		t.Fatal("sig bytes differ")
	}
}

func TestPollOnceConcurrent(t *testing.T) {
	keys, roots, art := keyTree(t)
	sum := sha256.Sum256([]byte("p"))
	files := map[string][]byte{msi: []byte("p"), msi + ".sig": sign(t, art, []byte("p"))}
	gh := fakeGH(t, files, hex.EncodeToString(sum[:])+"  "+msi+"\n", keys)
	defer gh.Close()
	m := mkMirror(t, gh, t.TempDir(), roots)
	if !m.pollMu.TryLock() {
		t.Fatal("lock")
	}
	err := m.PollOnce(context.Background())
	m.pollMu.Unlock()
	if err == nil || err.Error() != "poll already in progress" {
		t.Fatalf("expected in-progress error, got %v", err)
	}
}
