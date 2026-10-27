package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStore is an in-memory ObjectStore for tests.
type fakeStore struct {
	objects map[string][]byte
	putErr  error
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string][]byte{}}
}

func (f *fakeStore) Put(_ context.Context, key string, r io.Reader, _ int64) (int64, error) {
	if f.putErr != nil {
		return 0, f.putErr
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return 0, err
	}
	f.objects[key] = data
	return int64(len(data)), nil
}

func (f *fakeStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	data, ok := f.objects[key]
	if !ok {
		return nil, errNotFound
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (f *fakeStore) List(context.Context) ([]ObjectInfo, error) {
	var out []ObjectInfo
	for k, v := range f.objects {
		out = append(out, ObjectInfo{Key: k, Size: int64(len(v))})
	}
	return out, nil
}

func (f *fakeStore) Stat(_ context.Context, key string) (ObjectInfo, error) {
	v, ok := f.objects[key]
	if !ok {
		return ObjectInfo{}, errNotFound
	}
	return ObjectInfo{Key: key, Size: int64(len(v))}, nil
}

var errNotFound = errTest("not found")

type errTest string

func (e errTest) Error() string { return string(e) }

// fakeSnapshotter writes a small tar.gz snapshot like the sqlite one.
type fakeSnapshotter struct {
	err      error
	snapshot func(w io.Writer)
}

func (f *fakeSnapshotter) Snapshot(_ context.Context, w io.Writer) (*Manifest, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.snapshot != nil {
		f.snapshot(w)
	}
	return &Manifest{Version: manifestVersion, Timestamp: time.Now().UTC(), Engine: "sqlite"}, nil
}

func testConfig() Config {
	return Config{
		S3Endpoint:    "minio.test:9000",
		S3Bucket:      "backups",
		S3AccessKey:   "AKID",
		S3SecretKey:   "secret",
		EncryptionKey: "passphrase",
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	plain := []byte(strings.Repeat("netbirdy-store-contents", 100))

	var buf bytes.Buffer
	n, err := encrypt(&buf, bytes.NewReader(plain), "correct horse battery staple")
	require.NoError(t, err)
	assert.Equal(t, int64(len(plain)), n)
	assert.NotEqual(t, plain, buf.Bytes())

	r, err := decrypt(bytes.NewReader(buf.Bytes()), "correct horse battery staple")
	require.NoError(t, err)
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, plain, out)
}

func TestDecryptWrongPassphrase(t *testing.T) {
	var buf bytes.Buffer
	_, err := encrypt(&buf, strings.NewReader("payload"), "one")
	require.NoError(t, err)

	_, err = decrypt(bytes.NewReader(buf.Bytes()), "two")
	assert.Error(t, err)
}

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvS3Endpoint, "minio:9000")
	t.Setenv(EnvS3Bucket, "b")
	t.Setenv(EnvS3SecretKey, "sk")
	t.Setenv(EnvInterval, "6h")

	cfg := ConfigFromEnv()
	assert.Equal(t, "minio:9000", cfg.S3Endpoint)
	assert.Equal(t, "b", cfg.S3Bucket)
	assert.Equal(t, "sk", cfg.S3SecretKey)
	assert.Equal(t, "6h", cfg.Interval)
}

func TestConfigMasked(t *testing.T) {
	cfg := testConfig().Masked()
	assert.Equal(t, maskedSecret, cfg.S3AccessKey)
	assert.Equal(t, maskedSecret, cfg.S3SecretKey)
	assert.Equal(t, maskedSecret, cfg.EncryptionKey)
	assert.Equal(t, "backups", cfg.S3Bucket)
	assert.Equal(t, "minio.test:9000", cfg.S3Endpoint)
}

func TestConfigMergeSecrets(t *testing.T) {
	old := testConfig()

	// Empty incoming secrets keep the stored values.
	merged := old.MergeSecrets(Config{S3Bucket: "new"})
	assert.Equal(t, "new", merged.S3Bucket)
	assert.Equal(t, old.S3AccessKey, merged.S3AccessKey)
	assert.Equal(t, old.S3SecretKey, merged.S3SecretKey)
	assert.Equal(t, old.EncryptionKey, merged.EncryptionKey)

	// An echoed mask also keeps the stored values.
	merged = old.MergeSecrets(Config{S3AccessKey: maskedSecret, S3SecretKey: maskedSecret, EncryptionKey: maskedSecret})
	assert.Equal(t, old.S3AccessKey, merged.S3AccessKey)
	assert.Equal(t, old.S3SecretKey, merged.S3SecretKey)
	assert.Equal(t, old.EncryptionKey, merged.EncryptionKey)

	// A new secret replaces the stored one.
	merged = old.MergeSecrets(Config{S3SecretKey: "rotated"})
	assert.Equal(t, "rotated", merged.S3SecretKey)
}

func TestConfigIntervalDuration(t *testing.T) {
	d, err := Config{Interval: ""}.IntervalDuration()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)

	d, err = Config{Interval: "0"}.IntervalDuration()
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), d)

	d, err = Config{Interval: "24h"}.IntervalDuration()
	require.NoError(t, err)
	assert.Equal(t, 24*time.Hour, d)

	_, err = Config{Interval: "daily"}.IntervalDuration()
	assert.Error(t, err)
	assert.Error(t, Config{Interval: "daily"}.Validate())
}

func TestConfigValidateEndpoint(t *testing.T) {
	assert.NoError(t, Config{S3Bucket: "b", S3Endpoint: "minio:9000"}.Validate())
	assert.NoError(t, Config{S3Bucket: "b", S3Endpoint: "https://minio:9000"}.Validate())
	assert.Error(t, Config{S3Bucket: "b", S3Endpoint: "ftp://minio"}.Validate())
	assert.Error(t, Config{S3Bucket: "b", S3Endpoint: "https://minio/evil/path"}.Validate())
	assert.Error(t, Config{S3Bucket: "bad bucket"}.Validate())
}

func TestConfigPrefix(t *testing.T) {
	assert.Equal(t, DefaultPrefix, Config{}.Prefix())
	assert.Equal(t, "backups/", Config{S3Prefix: "backups"}.Prefix())
	assert.Equal(t, "a/b/", Config{S3Prefix: "/a/b/"}.Prefix())
}

func TestParseEndpoint(t *testing.T) {
	host, secure := parseEndpoint("", "us-west-2")
	assert.Equal(t, "s3.us-west-2.amazonaws.com", host)
	assert.True(t, secure)

	host, secure = parseEndpoint("", "")
	assert.Equal(t, "s3.amazonaws.com", host)
	assert.True(t, secure)

	host, secure = parseEndpoint("https://minio:9000", "")
	assert.Equal(t, "minio:9000", host)
	assert.True(t, secure)

	host, secure = parseEndpoint("http://minio:9000", "")
	assert.Equal(t, "minio:9000", host)
	assert.False(t, secure)

	host, secure = parseEndpoint("minio:9000", "")
	assert.Equal(t, "minio:9000", host)
	assert.True(t, secure)
}

func TestWriteExtractArchiveRoundtrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.db")
	require.NoError(t, writeFile(src, strings.NewReader("db-bytes")))

	manifest := &Manifest{
		Version:        manifestVersion,
		Timestamp:      time.Now().UTC().Truncate(time.Second),
		NetbirdVersion: "0.0.0-test",
		Engine:         "sqlite",
		Files:          []string{"store.db"},
	}
	var buf bytes.Buffer
	require.NoError(t, writeArchive(&buf, manifest, map[string]string{"store.db": src}))

	outDir := t.TempDir()
	got, files, err := extractArchive(bytes.NewReader(buf.Bytes()), outDir)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, manifest.Version, got.Version)
	assert.Equal(t, manifest.Engine, got.Engine)
	assert.Equal(t, manifest.Files, got.Files)
	assert.Equal(t, []string{"store.db"}, files)

	content, err := os.ReadFile(filepath.Join(outDir, "store.db"))
	require.NoError(t, err)
	assert.Equal(t, "db-bytes", string(content))
}

func TestManifestJSONRoundtrip(t *testing.T) {
	dir := t.TempDir()
	manifest := &Manifest{
		Version:        manifestVersion,
		Timestamp:      time.Now().UTC().Truncate(time.Second),
		NetbirdVersion: "1.2.3",
		Engine:         "sqlite",
		Files:          []string{"store.db"},
	}
	var buf bytes.Buffer
	require.NoError(t, writeArchive(&buf, manifest, nil))

	got, _, err := extractArchive(bytes.NewReader(buf.Bytes()), dir)
	require.NoError(t, err)
	assert.Equal(t, manifest.NetbirdVersion, got.NetbirdVersion)
	assert.Equal(t, manifest.Timestamp, got.Timestamp.UTC())
}

func TestLatestSnapshot(t *testing.T) {
	now := time.Now()
	key, err := LatestSnapshot([]ObjectInfo{
		{Key: "p/old.tar.gz.age", LastModified: now.Add(-time.Hour)},
		{Key: "p/new.tar.gz.age", LastModified: now},
		{Key: "p/ignore.txt", LastModified: now.Add(time.Hour)},
	})
	require.NoError(t, err)
	assert.Equal(t, "p/new.tar.gz.age", key)

	_, err = LatestSnapshot(nil)
	assert.Error(t, err)
}

func TestManagerRunNowUploadsEncrypted(t *testing.T) {
	store := newFakeStore()
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{snapshot: func(w io.Writer) {
		require.NoError(t, writeArchive(w, &Manifest{Version: manifestVersion}, map[string]string{}))
	}})
	mgr.newStore = func(Config) (ObjectStore, error) { return store, nil }
	require.NoError(t, mgr.UpdateConfig(context.Background(), testConfig()))

	result, err := mgr.RunNow(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, result.Key)
	assert.True(t, strings.HasPrefix(result.Key, DefaultPrefix))
	assert.True(t, strings.HasSuffix(result.Key, archiveSuffix))

	// The stored object decrypts back to an archive containing a manifest.
	manifest, _, err := DownloadAndExtract(context.Background(), store, result.Key, testConfig().EncryptionKey, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, manifestVersion, manifest.Version)

	status := mgr.Status()
	assert.True(t, status.Enabled)
	assert.Equal(t, result.Key, status.LastSnapshot)
	assert.Empty(t, status.LastError)
	assert.False(t, status.LastRun.IsZero())
	assert.Equal(t, maskedSecret, status.Config.S3SecretKey)
	assert.Equal(t, maskedSecret, status.Config.EncryptionKey)
}

func TestManagerRunRefusesWithoutKey(t *testing.T) {
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{})
	cfg := testConfig()
	cfg.EncryptionKey = ""
	require.NoError(t, mgr.UpdateConfig(context.Background(), cfg))

	_, err := mgr.RunNow(context.Background())
	assert.ErrorIs(t, err, ErrNoEncryptionKey)
	assert.Equal(t, ErrNoEncryptionKey.Error(), mgr.Status().LastError)
}

func TestManagerRunRefusesWithoutBucket(t *testing.T) {
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{})
	require.NoError(t, mgr.UpdateConfig(context.Background(), Config{EncryptionKey: "k"}))

	_, err := mgr.RunNow(context.Background())
	assert.ErrorIs(t, err, ErrNotConfigured)
}

func TestManagerRunConcurrentGuard(t *testing.T) {
	release := make(chan struct{})
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{snapshot: func(w io.Writer) {
		<-release
	}})
	mgr.newStore = func(Config) (ObjectStore, error) { return newFakeStore(), nil }
	require.NoError(t, mgr.UpdateConfig(context.Background(), testConfig()))

	done := make(chan error, 1)
	go func() {
		_, err := mgr.RunNow(context.Background())
		done <- err
	}()
	// Wait until the first run holds the guard.
	require.Eventually(t, func() bool { return mgr.Status().Running }, 2*time.Second, 10*time.Millisecond)

	_, err := mgr.RunNow(context.Background())
	assert.Error(t, err)
	close(release)
	require.NoError(t, <-done)
}

func TestManagerConfigPersistsAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager(dir, &fakeSnapshotter{})
	require.NoError(t, mgr.UpdateConfig(context.Background(), testConfig()))

	restarted := NewManager(dir, &fakeSnapshotter{})
	status := restarted.Status()
	assert.Equal(t, "backups", status.Config.S3Bucket)
	assert.Equal(t, "minio.test:9000", status.Config.S3Endpoint)
	assert.Equal(t, maskedSecret, status.Config.S3SecretKey)

	// The persisted file must be owner-only because it holds secrets.
	info, err := os.Stat(filepath.Join(dir, configFileName))
	require.NoError(t, err)
	assert.Equal(t, 0600, int(info.Mode().Perm()))
}

func TestManagerUpdateConfigKeepsSecrets(t *testing.T) {
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{})
	require.NoError(t, mgr.UpdateConfig(context.Background(), testConfig()))

	// A PUT echoing the masked config must not destroy the stored secrets.
	require.NoError(t, mgr.UpdateConfig(context.Background(), mgr.Status().Config))
	assert.Equal(t, testConfig().EncryptionKey, mgr.config().EncryptionKey)
	assert.Equal(t, testConfig().S3SecretKey, mgr.config().S3SecretKey)
}

func TestManagerSchedulerRunsOnInterval(t *testing.T) {
	store := newFakeStore()
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{snapshot: func(w io.Writer) {
		require.NoError(t, writeArchive(w, &Manifest{Version: manifestVersion}, map[string]string{}))
	}})
	mgr.newStore = func(Config) (ObjectStore, error) { return store, nil }

	cfg := testConfig()
	cfg.Interval = "50ms"
	require.NoError(t, mgr.UpdateConfig(context.Background(), cfg))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	require.Eventually(t, func() bool {
		return len(store.objects) > 0
	}, 15*time.Second, 25*time.Millisecond)
	assert.NotEmpty(t, mgr.Status().LastSnapshot)
}

func TestManagerManualOnlyWithoutInterval(t *testing.T) {
	mgr := NewManager(t.TempDir(), &fakeSnapshotter{})
	require.NoError(t, mgr.UpdateConfig(context.Background(), testConfig()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mgr.Start(ctx)
	defer mgr.Stop()

	assert.True(t, mgr.Status().NextRun.IsZero())
}
