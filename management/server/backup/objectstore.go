package backup

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// ObjectInfo describes one stored backup object.
type ObjectInfo struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// ObjectStore is the S3 access seam. The production implementation is
// minioStore; tests substitute an in-memory fake.
type ObjectStore interface {
	// Put stores r under key, returning the stored byte count.
	Put(ctx context.Context, key string, r io.Reader, size int64) (int64, error)
	// Get opens key for reading. The caller closes the returned reader.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
	// List returns every object under the configured prefix.
	List(ctx context.Context) ([]ObjectInfo, error)
	// Stat returns metadata for key.
	Stat(ctx context.Context, key string) (ObjectInfo, error)
}

// minioStore adapts a minio client to ObjectStore.
type minioStore struct {
	client *minio.Client
	bucket string
	prefix string
}

// NewObjectStore builds the S3 client for cfg. The bucket lookup mode is left
// at auto so custom endpoints (MinIO, Backblaze B2, Cloudflare R2) get
// path-style requests while AWS endpoints keep virtual-host style.
func NewObjectStore(cfg Config) (ObjectStore, error) {
	endpoint, secure := parseEndpoint(cfg.S3Endpoint, cfg.S3Region)
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure: secure,
		Region: cfg.S3Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}
	return &minioStore{client: client, bucket: cfg.S3Bucket, prefix: cfg.Prefix()}, nil
}

// parseEndpoint normalizes the configured endpoint into a bare host[:port]
// and a TLS flag. An empty endpoint defaults to the AWS endpoint for the
// region, so a config with only a bucket and credentials works against AWS.
func parseEndpoint(endpoint, region string) (string, bool) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		if region == "" {
			return "s3.amazonaws.com", true
		}
		return fmt.Sprintf("s3.%s.amazonaws.com", region), true
	}
	secure := true
	if strings.HasPrefix(raw, "http://") {
		secure = false
		raw = strings.TrimPrefix(raw, "http://")
	} else {
		raw = strings.TrimPrefix(raw, "https://")
	}
	return strings.TrimSuffix(raw, "/"), secure
}

// Put uploads r to bucket/key. A negative size streams the object with a
// multipart upload, which is what an age-encrypted stream needs because its
// length is not known up front.
func (s *minioStore) Put(ctx context.Context, key string, r io.Reader, size int64) (int64, error) {
	info, err := s.client.PutObject(ctx, s.bucket, key, r, size, minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return 0, fmt.Errorf("failed to upload %s: %w", key, err)
	}
	return info.Size, nil
}

// Get opens the object for reading.
func (s *minioStore) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to download %s: %w", key, err)
	}
	return obj, nil
}

// List returns all objects under the configured prefix.
func (s *minioStore) List(ctx context.Context) ([]ObjectInfo, error) {
	var out []ObjectInfo
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    s.prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", obj.Err)
		}
		out = append(out, ObjectInfo{Key: obj.Key, Size: obj.Size, LastModified: obj.LastModified})
	}
	return out, nil
}

// Stat returns metadata for key.
func (s *minioStore) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		return ObjectInfo{}, fmt.Errorf("failed to stat %s: %w", key, err)
	}
	return ObjectInfo{Key: info.Key, Size: info.Size, LastModified: info.LastModified}, nil
}
