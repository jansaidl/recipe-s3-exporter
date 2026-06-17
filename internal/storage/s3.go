// Package storage wraps an S3-compatible object store (via minio-go) for a
// single configured export target.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Object describes a stored object for the explore view.
type Object struct {
	Key          string
	Size         int64
	LastModified time.Time
}

// Store is a thin wrapper around a minio client bound to one bucket.
type Store struct {
	client *minio.Client
	bucket string
	prefix string
}

// Config holds the connection parameters for a target (secrets decrypted).
type Config struct {
	Endpoint     string
	Region       string
	Bucket       string
	Prefix       string
	AccessKey    string
	SecretKey    string
	UsePathStyle bool
	UseSSL       bool
}

// New builds a Store from a decrypted target configuration.
func New(cfg Config) (*Store, error) {
	endpoint := normalizeEndpoint(cfg.Endpoint)
	lookup := minio.BucketLookupAuto
	if cfg.UsePathStyle {
		lookup = minio.BucketLookupPath
	}
	cl, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure:       cfg.UseSSL,
		Region:       cfg.Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("init s3 client: %w", err)
	}
	return &Store{client: cl, bucket: cfg.Bucket, prefix: strings.Trim(cfg.Prefix, "/")}, nil
}

// Key joins the configured prefix with the given parts.
func (s *Store) Key(parts ...string) string {
	all := append([]string{}, s.prefix)
	all = append(all, parts...)
	clean := make([]string, 0, len(all))
	for _, p := range all {
		if p = strings.Trim(p, "/"); p != "" {
			clean = append(clean, p)
		}
	}
	return strings.Join(clean, "/")
}

// Test verifies connectivity and bucket access.
func (s *Store) Test(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("connect/auth failed: %w", err)
	}
	if !exists {
		return fmt.Errorf("bucket %q does not exist or is not accessible", s.bucket)
	}
	return nil
}

// Exists reports whether an object already exists at key.
func (s *Store) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if minio.ToErrorResponse(err).Code == "NoSuchKey" || minio.ToErrorResponse(err).StatusCode == 404 {
		return false, nil
	}
	return false, err
}

// uploadPartSize bounds memory per concurrent upload when the object size is
// unknown (streaming multipart). 16 MiB parts allow objects up to ~160 GiB.
const uploadPartSize = 16 * 1024 * 1024

// Upload streams reader (of the given size) to key. Pass size = -1 if unknown;
// in that case a bounded-memory streaming multipart upload is used.
func (s *Store) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	opts := minio.PutObjectOptions{ContentType: contentType}
	if size < 0 {
		// Cap the part size so unknown-length streams don't allocate the
		// minio-go default (~512 MiB) per part.
		opts.PartSize = uploadPartSize
	}
	_, err := s.client.PutObject(ctx, s.bucket, key, reader, size, opts)
	return err
}

// List returns objects under the configured prefix (optionally a sub-prefix).
func (s *Store) List(ctx context.Context, sub string) ([]Object, error) {
	prefix := s.Key(sub)
	if prefix != "" {
		prefix += "/"
	}
	var out []Object
	for obj := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, Object{Key: obj.Key, Size: obj.Size, LastModified: obj.LastModified})
	}
	return out, nil
}

// PresignGet returns a temporary download URL for an object key.
func (s *Store) PresignGet(ctx context.Context, key string, expiry time.Duration) (string, error) {
	u, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, url.Values{})
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// normalizeEndpoint strips scheme and trailing slash; minio wants host[:port].
func normalizeEndpoint(ep string) string {
	ep = strings.TrimSpace(ep)
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return strings.TrimRight(ep, "/")
}
