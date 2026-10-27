package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/netbirdio/netbird/management/internals/shared/db"
	"github.com/netbirdio/netbird/management/server/types"
	"github.com/netbirdio/netbird/version"
)

const (
	// manifestName is the archive entry describing the snapshot.
	manifestName = "manifest.json"
	// manifestVersion is the current manifest schema version.
	manifestVersion = 1
	// sqliteFileEnv overrides the sqlite store file name, matching the store.
	sqliteFileEnv = "NB_STORE_ENGINE_SQLITE_FILE"
	// objectNameFormat produces object keys like
	// netbirdy-backup-20261027T014523Z.tar.gz.age.
	objectNameFormat = "netbirdy-backup-%s" + archiveSuffix
	// timestampFormat is the UTC timestamp layout embedded in object keys.
	timestampFormat = "20060102T150405Z"
)

// Manifest describes a snapshot archive. It travels inside the archive as
// manifest.json so a restore can inspect what it is applying.
type Manifest struct {
	// Version is the manifest schema version.
	Version int `json:"version"`
	// Timestamp is when the snapshot was taken, in UTC.
	Timestamp time.Time `json:"timestamp"`
	// NetbirdVersion is the management server version that wrote the
	// snapshot.
	NetbirdVersion string `json:"netbird_version"`
	// Engine is the store engine the snapshot was taken from.
	Engine string `json:"engine"`
	// Files lists the store file names archived alongside the manifest.
	Files []string `json:"files"`
}

// Snapshotter produces a consistent snapshot of the management store as a
// tar.gz stream written to w. The stream contains manifest.json plus every
// store file.
type Snapshotter interface {
	Snapshot(ctx context.Context, w io.Writer) (*Manifest, error)
}

// NewSnapshotter builds the Snapshotter for the store engine. Only the SQLite
// engine is supported; other engines return an error so callers fail loudly
// instead of silently skipping the backup.
func NewSnapshotter(engine types.Engine, conn *db.Conn) (Snapshotter, error) {
	if engine != types.SqliteStoreEngine {
		return nil, fmt.Errorf("backup of store engine %q is not supported: only the %q engine can be snapshotted", engine, types.SqliteStoreEngine)
	}
	if conn == nil {
		return nil, fmt.Errorf("backup requires an open store connection")
	}
	return &sqliteSnapshotter{conn: conn, dbName: sqliteFileName()}, nil
}

// sqliteSnapshotter snapshots the SQLite store with VACUUM INTO, which yields
// a single consistent database file without copying the -wal and -shm
// sidecars.
type sqliteSnapshotter struct {
	conn   *db.Conn
	dbName string
}

// sqliteFileName resolves the store file base name the way the store does:
// NB_STORE_ENGINE_SQLITE_FILE wins, stripped of any URI query parameters,
// otherwise the default store.db.
func sqliteFileName() string {
	name := db.SqliteFileName
	if envFile, ok := os.LookupEnv(sqliteFileEnv); ok && envFile != "" {
		file, _, _ := strings.Cut(envFile, "?")
		name = filepath.Base(file)
	}
	return name
}

// SqliteStorePath resolves the absolute path of the live sqlite store file.
// Restore uses it as the default output path.
func SqliteStorePath(dataDir string) string {
	name := db.SqliteFileName
	if envFile, ok := os.LookupEnv(sqliteFileEnv); ok && envFile != "" {
		file, _, _ := strings.Cut(envFile, "?")
		if filepath.IsAbs(file) {
			return file
		}
		name = file
	}
	return filepath.Join(dataDir, name)
}

// Snapshot runs VACUUM INTO a temporary file inside the data directory and
// streams a tar.gz containing the database and manifest.json into w.
// VACUUM INTO serializes with writers on its own, so the copy is consistent
// without locking the store from the Go side.
func (s *sqliteSnapshotter) Snapshot(ctx context.Context, w io.Writer) (*Manifest, error) {
	tmpDir, err := os.MkdirTemp("", "netbirdy-backup-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot workspace: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dbCopy := filepath.Join(tmpDir, s.dbName)
	vacuum := fmt.Sprintf("VACUUM INTO '%s'", strings.ReplaceAll(dbCopy, "'", "''"))
	if err := s.conn.DB(nil).WithContext(ctx).Exec(vacuum).Error; err != nil {
		return nil, fmt.Errorf("failed to snapshot sqlite store: %w", err)
	}

	manifest := &Manifest{
		Version:        manifestVersion,
		Timestamp:      time.Now().UTC(),
		NetbirdVersion: version.NetbirdVersion(),
		Engine:         string(types.SqliteStoreEngine),
		Files:          []string{s.dbName},
	}
	if err := writeArchive(w, manifest, map[string]string{s.dbName: dbCopy}); err != nil {
		return nil, err
	}
	return manifest, nil
}

// UnsupportedSnapshotter reports Err on every run, keeping a Manager on an
// engine that cannot be snapshotted functional for status and config
// management.
type UnsupportedSnapshotter struct {
	Err error
}

// Snapshot always fails on engines that cannot be snapshotted.
func (s *UnsupportedSnapshotter) Snapshot(context.Context, io.Writer) (*Manifest, error) {
	if s.Err != nil {
		return nil, s.Err
	}
	return nil, fmt.Errorf("%w", ErrSnapshotUnsupported)
}

// writeArchive streams a tar.gz holding the manifest and every file named by
// files (tar entry name -> source path) into w.
func writeArchive(w io.Writer, manifest *Manifest, files map[string]string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode manifest: %w", err)
	}
	if err := writeTarEntry(tw, manifestName, manifestBytes); err != nil {
		return err
	}

	for name, path := range files {
		if err := writeTarFile(tw, name, path); err != nil {
			return err
		}
	}

	if err := tw.Close(); err != nil {
		return fmt.Errorf("failed to finalize archive: %w", err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("failed to finalize archive compression: %w", err)
	}
	return nil
}

// writeTarEntry writes one regular file entry with the given bytes.
func writeTarEntry(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0600,
		Size:    int64(len(data)),
		ModTime: time.Now(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write archive header for %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("failed to write archive entry %s: %w", name, err)
	}
	return nil
}

// writeTarFile streams the file at path into the archive as name.
func writeTarFile(tw *tar.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %s for archiving: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat %s: %w", path, err)
	}
	hdr := &tar.Header{
		Name:    name,
		Mode:    0600,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("failed to write archive header for %s: %w", name, err)
	}
	if _, err := io.Copy(tw, f); err != nil {
		return fmt.Errorf("failed to archive %s: %w", path, err)
	}
	return nil
}

// extractArchive unpacks a tar.gz stream into dir and returns its manifest
// and the names of the extracted store files. Entry names are reduced to
// their base name so a hostile archive cannot escape dir.
func extractArchive(r io.Reader, dir string) (*Manifest, []string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read archive: %w", err)
	}
	defer gz.Close()

	var manifest *Manifest
	var files []string
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read archive: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		name := filepath.Base(hdr.Name)
		if name == manifestName {
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read manifest: %w", err)
			}
			var m Manifest
			if err := json.Unmarshal(data, &m); err != nil {
				return nil, nil, fmt.Errorf("failed to parse manifest: %w", err)
			}
			manifest = &m
			continue
		}
		out := filepath.Join(dir, name)
		if err := writeFile(out, tr); err != nil {
			return nil, nil, err
		}
		files = append(files, name)
	}
	if manifest == nil {
		return nil, nil, fmt.Errorf("archive does not contain %s", manifestName)
	}
	return manifest, files, nil
}

// writeFile writes r to path with owner-only permissions.
func writeFile(path string, r io.Reader) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// objectName returns the object key for a snapshot taken at t.
func objectName(prefix string, t time.Time) string {
	return prefix + fmt.Sprintf(objectNameFormat, t.UTC().Format(timestampFormat))
}

// LatestSnapshot returns the key of the most recently modified object, which
// is what "latest" resolves to on restore.
func LatestSnapshot(objects []ObjectInfo) (string, error) {
	var latest ObjectInfo
	for _, obj := range objects {
		if !strings.HasSuffix(obj.Key, archiveSuffix) {
			continue
		}
		if obj.LastModified.After(latest.LastModified) ||
			(obj.LastModified.Equal(latest.LastModified) && obj.Key > latest.Key) {
			latest = obj
		}
	}
	if latest.Key == "" {
		return "", fmt.Errorf("no backup snapshots found")
	}
	return latest.Key, nil
}
