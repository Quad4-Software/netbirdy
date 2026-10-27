package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netbirdio/netbird/management/internals/shared/db"
	"github.com/netbirdio/netbird/management/server/types"
)

// TestSqliteSnapshotterRoundtrip verifies VACUUM INTO against a real sqlite
// store and that the resulting archive extracts a readable database file.
func TestSqliteSnapshotterRoundtrip(t *testing.T) {
	dataDir := t.TempDir()
	conn, err := db.OpenSqliteFile(context.Background(), dataDir, "store.db")
	require.NoError(t, err)
	defer conn.Close()

	require.NoError(t, conn.DB(nil).Exec("CREATE TABLE widgets (id INTEGER PRIMARY KEY, name TEXT)").Error)
	require.NoError(t, conn.DB(nil).Exec("INSERT INTO widgets (name) VALUES ('alpha'), ('beta')").Error)

	snap, err := NewSnapshotter(types.SqliteStoreEngine, conn)
	require.NoError(t, err)

	var buf bytes.Buffer
	manifest, err := snap.Snapshot(context.Background(), &buf)
	require.NoError(t, err)
	require.NotNil(t, manifest)
	assert.Equal(t, string(types.SqliteStoreEngine), manifest.Engine)
	assert.Equal(t, []string{"store.db"}, manifest.Files)
	assert.Equal(t, manifestVersion, manifest.Version)

	outDir := t.TempDir()
	got, files, err := extractArchive(bytes.NewReader(buf.Bytes()), outDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"store.db"}, files)
	assert.Equal(t, manifest.Timestamp, got.Timestamp.UTC())

	// The extracted file is a real database holding the snapshot's rows.
	restored, err := db.OpenSqliteFile(context.Background(), outDir, "store.db")
	require.NoError(t, err)
	defer restored.Close()
	var count int
	require.NoError(t, restored.DB(nil).Raw("SELECT COUNT(*) FROM widgets").Scan(&count).Error)
	assert.Equal(t, 2, count)
}

// TestNewSnapshotterUnsupportedEngine verifies non-sqlite engines fail loudly.
func TestNewSnapshotterUnsupportedEngine(t *testing.T) {
	_, err := NewSnapshotter(types.PostgresStoreEngine, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not supported")

	_, err = NewSnapshotter(types.SqliteStoreEngine, nil)
	require.Error(t, err)
}

// TestSqliteFileNameEnvOverride verifies NB_STORE_ENGINE_SQLITE_FILE wins.
func TestSqliteFileNameEnvOverride(t *testing.T) {
	t.Setenv(sqliteFileEnv, "custom.db?cache=shared")
	assert.Equal(t, "custom.db", sqliteFileName())
	assert.Equal(t, "custom.db", filepath.Base(SqliteStorePath(t.TempDir())))

	abs := filepath.Join(t.TempDir(), "nested", "abs.db")
	t.Setenv(sqliteFileEnv, abs)
	assert.Equal(t, abs, SqliteStorePath(t.TempDir()))
}

// TestSqliteStorePathDefault verifies the default store path resolution.
func TestSqliteStorePathDefault(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, filepath.Join(dir, db.SqliteFileName), SqliteStorePath(dir))
}

// TestExtractArchiveRejectsTraversal verifies entries are reduced to base
// names so an archive cannot write outside the target directory.
func TestExtractArchiveRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0600, Size: 3}))
	_, err := tw.Write([]byte("bad"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	outDir := t.TempDir()
	manifest, _, err := extractArchive(bytes.NewReader(buf.Bytes()), outDir)
	// No manifest was added, so the extract fails, and escape.txt must
	// never reach the parent directory.
	require.Error(t, err)
	assert.Nil(t, manifest)
	_, statErr := os.Stat(filepath.Join(outDir, "..", "escape.txt"))
	assert.True(t, os.IsNotExist(statErr))
}
