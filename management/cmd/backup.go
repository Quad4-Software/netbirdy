package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/netbirdio/netbird/management/server/backup"
)

var (
	backupEndpoint      string
	backupRegion        string
	backupBucket        string
	backupAccessKey     string
	backupSecretKey     string
	backupPrefix        string
	backupObjectKey     string
	backupEncryptionKey string
	backupOutput        string
	backupDataDir       string
)

// backupCmd groups the offline backup commands. They talk to the bucket
// directly and never touch a running management server.
var backupCmd = &cobra.Command{
	Use:          "backup",
	Short:        "Manage encrypted store backups stored in S3",
	SilenceUsage: true,
}

var backupListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List backup snapshots in the configured bucket",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := resolveBackupConfig()
		if err != nil {
			return err
		}
		store, err := backup.NewObjectStore(cfg)
		if err != nil {
			return err
		}
		objects, err := store.List(cmd.Context())
		if err != nil {
			return err
		}
		sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "KEY\tSIZE\tLAST MODIFIED")
		for _, obj := range objects {
			_, _ = fmt.Fprintf(w, "%s\t%d\t%s\n", obj.Key, obj.Size, obj.LastModified.UTC().Format(time.RFC3339))
		}
		return w.Flush()
	},
}

var backupRestoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Download, decrypt and restore a store backup",
	Long: `Download a backup snapshot from the configured bucket, decrypt it with
the backup encryption key and write the store file to --output (the configured
store path by default).

WARNING: run this only while the management server is STOPPED. Restoring over
a live store file corrupts the running server.`,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackupRestore(cmd)
	},
}

func init() {
	backupCmd.PersistentFlags().StringVar(&backupEndpoint, "endpoint", "", "S3 endpoint host[:port] or http(s) URL (env "+backup.EnvS3Endpoint+")")
	backupCmd.PersistentFlags().StringVar(&backupRegion, "region", "", "S3 region (env "+backup.EnvS3Region+")")
	backupCmd.PersistentFlags().StringVar(&backupBucket, "bucket", "", "S3 bucket (env "+backup.EnvS3Bucket+")")
	backupCmd.PersistentFlags().StringVar(&backupAccessKey, "access-key", "", "S3 access key (env "+backup.EnvS3AccessKey+")")
	backupCmd.PersistentFlags().StringVar(&backupSecretKey, "secret-key", "", "S3 secret key (env "+backup.EnvS3SecretKey+")")
	backupCmd.PersistentFlags().StringVar(&backupPrefix, "prefix", "", "object key prefix (env "+backup.EnvS3Prefix+", default "+backup.DefaultPrefix+")")
	backupCmd.PersistentFlags().StringVar(&backupDataDir, "datadir", defaultMgmtDataDir, "server data directory; used to read backup.json and resolve the default restore path")

	backupRestoreCmd.Flags().StringVar(&backupObjectKey, "key", "latest", "object key of the snapshot to restore, or \"latest\"")
	backupRestoreCmd.Flags().StringVar(&backupEncryptionKey, "encryption-key", "", "age passphrase the snapshot was encrypted with (env "+backup.EnvEncryptionKey+")")
	backupRestoreCmd.Flags().StringVar(&backupOutput, "output", "", "path to write the restored store file (default: the configured store path in --datadir)")

	backupCmd.AddCommand(backupListCmd)
	backupCmd.AddCommand(backupRestoreCmd)
	rootCmd.AddCommand(backupCmd)
}

// resolveBackupConfig merges S3 settings from flags, the NB_BACKUP_* env vars
// and the persisted backup.json in the datadir, in that order of precedence.
func resolveBackupConfig() (backup.Config, error) {
	cfg := backup.ConfigFromEnv()

	if persisted, err := loadPersistedBackupConfig(backupDataDir); err == nil && persisted != nil {
		cfg = overlayConfig(cfg, *persisted)
	}

	flagCfg := backup.Config{
		S3Endpoint:    backupEndpoint,
		S3Region:      backupRegion,
		S3Bucket:      backupBucket,
		S3AccessKey:   backupAccessKey,
		S3SecretKey:   backupSecretKey,
		S3Prefix:      backupPrefix,
		EncryptionKey: backupEncryptionKey,
	}
	cfg = overlayConfig(cfg, flagCfg)

	if cfg.S3Bucket == "" {
		return cfg, fmt.Errorf("no S3 bucket configured: pass --bucket, set %s or update the backup config via the API", backup.EnvS3Bucket)
	}
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// loadPersistedBackupConfig reads the runtime config the management server
// persists next to the store, letting the CLI work without NB_* env vars.
func loadPersistedBackupConfig(dataDir string) (*backup.Config, error) {
	data, err := os.ReadFile(filepath.Join(dataDir, "backup.json"))
	if err != nil {
		return nil, err
	}
	var cfg backup.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// overlayConfig returns base with every non-empty field of over applied.
func overlayConfig(base, over backup.Config) backup.Config {
	out := base
	if over.S3Endpoint != "" {
		out.S3Endpoint = over.S3Endpoint
	}
	if over.S3Region != "" {
		out.S3Region = over.S3Region
	}
	if over.S3Bucket != "" {
		out.S3Bucket = over.S3Bucket
	}
	if over.S3AccessKey != "" {
		out.S3AccessKey = over.S3AccessKey
	}
	if over.S3SecretKey != "" {
		out.S3SecretKey = over.S3SecretKey
	}
	if over.S3Prefix != "" {
		out.S3Prefix = over.S3Prefix
	}
	if over.Interval != "" {
		out.Interval = over.Interval
	}
	if over.EncryptionKey != "" {
		out.EncryptionKey = over.EncryptionKey
	}
	return out
}

func runBackupRestore(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintln(out, "!!! WARNING: the management server must be STOPPED before restoring a backup !!!")
	_, _ = fmt.Fprintln(out, "!!! restoring over a live store file corrupts the running server        !!!")

	cfg, err := resolveBackupConfig()
	if err != nil {
		return err
	}
	if cfg.EncryptionKey == "" {
		return fmt.Errorf("no encryption key configured: pass --encryption-key or set %s", backup.EnvEncryptionKey)
	}

	store, err := backup.NewObjectStore(cfg)
	if err != nil {
		return err
	}

	key := backupObjectKey
	if key == "" || key == "latest" {
		objects, err := store.List(cmd.Context())
		if err != nil {
			return err
		}
		key, err = backup.LatestSnapshot(objects)
		if err != nil {
			return err
		}
	}

	tmpDir, err := os.MkdirTemp("", "netbirdy-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	manifest, files, err := backup.DownloadAndExtract(cmd.Context(), store, key, cfg.EncryptionKey, tmpDir)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "restoring snapshot %s (engine=%s, taken=%s, netbirdy=%s)\n",
		key, manifest.Engine, manifest.Timestamp.UTC().Format(time.RFC3339), manifest.NetbirdVersion)

	if len(files) == 0 {
		return fmt.Errorf("snapshot %s contains no store files", key)
	}

	output := backupOutput
	if output == "" {
		output = backup.SqliteStorePath(backupDataDir)
	}

	for _, name := range files {
		dest := output
		if name != filepath.Base(output) && len(files) > 1 {
			dest = filepath.Join(filepath.Dir(output), name)
		}
		if err := copyRestoredFile(filepath.Join(tmpDir, name), dest); err != nil {
			return err
		}
		_, _ = fmt.Fprintf(out, "wrote %s\n", dest)
	}

	_, _ = fmt.Fprintln(out, "restore complete. Start the management server only after verifying the restored files.")
	return nil
}

// copyRestoredFile installs src at dest, keeping a timestamped copy of any
// existing destination so a bad restore is reversible.
func copyRestoredFile(src, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		backupCopy := fmt.Sprintf("%s.pre-restore-%s", dest, time.Now().UTC().Format("20060102T150405Z"))
		if err := copyFile(dest, backupCopy); err != nil {
			return fmt.Errorf("failed to preserve existing file %s: %w", dest, err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create directory for %s: %w", dest, err)
	}
	if err := copyFile(src, dest); err != nil {
		return fmt.Errorf("failed to write %s: %w", dest, err)
	}
	return nil
}

// copyFile copies src to dest with owner-only permissions.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
