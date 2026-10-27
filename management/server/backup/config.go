// Package backup implements encrypted, S3-uploaded snapshots of the
// management store. Snapshots are tar.gz archives containing the store file
// and a manifest, encrypted with age passphrase encryption and uploaded to a
// configurable S3-compatible bucket either on a schedule or on demand.
package backup

import (
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
)

// Environment variables that seed the backup configuration at startup. A
// persisted backup.json in the datadir, written when the config is updated
// over the API, takes precedence over these.
const (
	// EnvS3Endpoint is the S3 endpoint host or URL (for example
	// "s3.us-east-1.amazonaws.com" or "https://minio.internal:9000").
	EnvS3Endpoint = "NB_BACKUP_S3_ENDPOINT"
	// EnvS3Region is the S3 region used for request signing.
	EnvS3Region = "NB_BACKUP_S3_REGION"
	// EnvS3Bucket is the destination bucket. Empty disables the feature.
	EnvS3Bucket = "NB_BACKUP_S3_BUCKET"
	// EnvS3AccessKey is the S3 access key.
	EnvS3AccessKey = "NB_BACKUP_S3_ACCESS_KEY"
	// EnvS3SecretKey is the S3 secret key.
	//nolint:gosec // G101 false positive: an env var name, not a credential
	EnvS3SecretKey = "NB_BACKUP_S3_SECRET_KEY"
	// EnvS3Prefix is the object key prefix snapshots are stored under.
	EnvS3Prefix = "NB_BACKUP_S3_PREFIX"
	// EnvInterval is a Go duration string for the schedule. Empty or 0 means
	// manual runs only.
	EnvInterval = "NB_BACKUP_INTERVAL"
	// EnvEncryptionKey is the passphrase used for age scrypt encryption of
	// every uploaded snapshot.
	EnvEncryptionKey = "NB_BACKUP_ENCRYPTION_KEY"
)

const (
	// DefaultPrefix is the object key prefix used when none is configured.
	DefaultPrefix = "netbirdy-backups/"
	// maskedSecret replaces secret values in API responses, status snapshots
	// and logs.
	maskedSecret = "********"
	// configFileName is the persisted runtime config file inside the datadir.
	configFileName = "backup.json"
	// archiveSuffix is the object key suffix of an encrypted snapshot.
	archiveSuffix = ".tar.gz.age"
)

// Config holds every runtime setting of the backup manager. It is what the
// API reads and updates and what is persisted to backup.json.
type Config struct {
	// S3Endpoint is the S3 endpoint as host[:port] or an http(s) URL. Empty
	// defaults to the AWS endpoint resolved from the region.
	S3Endpoint string `json:"s3_endpoint"`
	// S3Region is the signing region, for example "us-east-1".
	S3Region string `json:"s3_region,omitempty"`
	// S3Bucket is the destination bucket. Empty means the feature is
	// disabled.
	S3Bucket string `json:"s3_bucket"`
	// S3AccessKey is the S3 access key.
	S3AccessKey string `json:"s3_access_key,omitempty"`
	// S3SecretKey is the S3 secret key.
	S3SecretKey string `json:"s3_secret_key,omitempty"`
	// S3Prefix is the object key prefix, defaulting to DefaultPrefix.
	S3Prefix string `json:"s3_prefix,omitempty"`
	// Interval is a Go duration string ("24h", "30m"). Empty or "0" means
	// scheduled runs are off and only manual runs happen.
	Interval string `json:"interval,omitempty"`
	// EncryptionKey is the age passphrase. Backups refuse to run without it.
	EncryptionKey string `json:"encryption_key,omitempty"`
}

// ConfigFromEnv builds a Config from the NB_BACKUP_* environment variables.
func ConfigFromEnv() Config {
	return Config{
		S3Endpoint:    strings.TrimSpace(os.Getenv(EnvS3Endpoint)),
		S3Region:      strings.TrimSpace(os.Getenv(EnvS3Region)),
		S3Bucket:      strings.TrimSpace(os.Getenv(EnvS3Bucket)),
		S3AccessKey:   strings.TrimSpace(os.Getenv(EnvS3AccessKey)),
		S3SecretKey:   os.Getenv(EnvS3SecretKey),
		S3Prefix:      os.Getenv(EnvS3Prefix),
		Interval:      strings.TrimSpace(os.Getenv(EnvInterval)),
		EncryptionKey: os.Getenv(EnvEncryptionKey),
	}
}

// Enabled reports whether the backup feature is configured. The bucket is the
// single switch: without it nothing runs or uploads.
func (c Config) Enabled() bool {
	return c.S3Bucket != ""
}

// IntervalDuration parses the configured interval. Empty or "0" parses to 0,
// meaning the scheduler does not fire and only manual runs are possible.
func (c Config) IntervalDuration() (time.Duration, error) {
	v := strings.TrimSpace(c.Interval)
	if v == "" || v == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid backup interval %q: %w", c.Interval, err)
	}
	return d, nil
}

// Prefix returns the configured object prefix, normalized to end with "/" and
// never start with one, so object keys stay relative.
func (c Config) Prefix() string {
	p := strings.TrimSpace(c.S3Prefix)
	if p == "" {
		return DefaultPrefix
	}
	p = strings.TrimPrefix(p, "/")
	if !strings.HasSuffix(p, "/") {
		p += "/"
	}
	return p
}

// Validate checks the configuration for errors that would fail a run anyway.
// A disabled config (no bucket) is valid.
func (c Config) Validate() error {
	if _, err := c.IntervalDuration(); err != nil {
		return err
	}
	if c.S3Endpoint != "" {
		if err := validateEndpoint(c.S3Endpoint); err != nil {
			return err
		}
	}
	if strings.ContainsAny(c.S3Bucket, " /\t\n") {
		return fmt.Errorf("invalid bucket name %q", c.S3Bucket)
	}
	return nil
}

// Masked returns a copy of the config with every secret replaced by a mask.
// The masked copy is what API responses and status report; it is never valid
// to run a backup with.
func (c Config) Masked() Config {
	masked := c
	if masked.S3AccessKey != "" {
		masked.S3AccessKey = maskedSecret
	}
	if masked.S3SecretKey != "" {
		masked.S3SecretKey = maskedSecret
	}
	if masked.EncryptionKey != "" {
		masked.EncryptionKey = maskedSecret
	}
	return masked
}

// MergeSecrets keeps existing secret values when the incoming config leaves
// them empty or echoes back the mask, letting a client PUT the masked config
// it read without destroying the stored secrets.
func (c Config) MergeSecrets(in Config) Config {
	merged := in
	if isKeptSecret(in.S3AccessKey) {
		merged.S3AccessKey = c.S3AccessKey
	}
	if isKeptSecret(in.S3SecretKey) {
		merged.S3SecretKey = c.S3SecretKey
	}
	if isKeptSecret(in.EncryptionKey) {
		merged.EncryptionKey = c.EncryptionKey
	}
	return merged
}

// isKeptSecret reports whether an incoming secret field means "keep the
// existing value": either empty or the echoed mask.
func isKeptSecret(v string) bool {
	return v == "" || v == maskedSecret
}

// validateEndpoint rejects endpoint strings that would smuggle in a path or a
// non-http(s) scheme. A bare host or host:port is accepted and treated as
// https.
func validateEndpoint(endpoint string) error {
	raw := strings.TrimSpace(endpoint)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid S3 endpoint %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("invalid S3 endpoint scheme %q: only http and https are supported", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("invalid S3 endpoint %q: missing host", endpoint)
	}
	if u.Path != "" && u.Path != "/" {
		return fmt.Errorf("invalid S3 endpoint %q: paths are not supported", endpoint)
	}
	return nil
}
