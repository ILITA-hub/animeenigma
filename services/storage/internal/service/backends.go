package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	libserrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/config"
	"github.com/ILITA-hub/animeenigma/services/storage/internal/domain"
)

// PresignExpiry is the lifetime of every presigned PUT/GET this service
// hands out. Exported so the handler derives the wire-level `expires_in`
// from this same constant — a single source of truth, so the reported
// value can never drift from the real URL lifetime.
const PresignExpiry = time.Hour

// backend wraps one *minio.Client with its bucket + endpoint metadata
// needed to build canonical URLs (BaseURLs).
type backend struct {
	client   *minio.Client
	bucket   string
	endpoint string
	useSSL   bool
}

// Backends is the production implementation of handler.Backends — real
// MinIO/S3 clients keyed by storage id ("minio" | "s3"). It owns both
// buckets and every placement-agnostic object operation; service.Placement
// decides *which* id a request lands on, Backends does the actual work.
type Backends struct {
	byID map[string]*backend
	log  *logger.Logger
}

// NewBackends constructs the minio client (always) and the s3 client (only
// when cfg.S3.Endpoint is non-empty — an absent S3 endpoint is a supported
// dev configuration). Call EnsureBuckets once at startup before serving
// traffic.
func NewBackends(cfg *config.Config, log *logger.Logger) (*Backends, error) {
	b := &Backends{byID: map[string]*backend{}, log: log}

	minioClient, err := newClient(cfg.Minio)
	if err != nil {
		return nil, fmt.Errorf("init minio client: %w", err)
	}
	b.byID[domain.BackendMinio] = &backend{
		client: minioClient, bucket: cfg.Minio.Bucket, endpoint: cfg.Minio.Endpoint, useSSL: cfg.Minio.UseSSL,
	}

	if cfg.S3.Endpoint != "" {
		s3Client, err := newClient(cfg.S3)
		if err != nil {
			return nil, fmt.Errorf("init s3 client: %w", err)
		}
		b.byID[domain.BackendS3] = &backend{
			client: s3Client, bucket: cfg.S3.Bucket, endpoint: cfg.S3.Endpoint, useSSL: cfg.S3.UseSSL,
		}
	}

	return b, nil
}

func newClient(cfg config.BackendConfig) (*minio.Client, error) {
	return minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
}

// HasS3 reports whether the s3 backend was configured (STORAGE_S3_ENDPOINT
// non-empty). Used to decide the s3Absent flag Placement needs.
func (b *Backends) HasS3() bool {
	_, ok := b.byID[domain.BackendS3]
	return ok
}

func (b *Backends) get(storage string) (*backend, error) {
	be, ok := b.byID[storage]
	if !ok {
		return nil, libserrors.InvalidInput("unknown storage: " + storage)
	}
	return be, nil
}

// EnsureBuckets creates each configured backend's bucket if absent.
// Idempotent: swallows "already exists" races so concurrent instances or a
// restart never fail boot. Mirrors
// services/library/internal/minio/writer.go:122-140.
func (b *Backends) EnsureBuckets(ctx context.Context) error {
	for id, be := range b.byID {
		exists, err := be.client.BucketExists(ctx, be.bucket)
		if err == nil && exists {
			continue
		}
		if err != nil && b.log != nil {
			b.log.Warnw("bucket exists probe failed; attempting MakeBucket", "storage", id, "bucket", be.bucket, "error", err)
		}
		mkErr := be.client.MakeBucket(ctx, be.bucket, minio.MakeBucketOptions{})
		if mkErr == nil || isBucketRaceError(mkErr) {
			continue
		}
		return fmt.Errorf("ensure bucket for %s: %w", id, mkErr)
	}
	return nil
}

// isBucketRaceError reports whether err is one of the "bucket already
// exists" responses that should be swallowed rather than treated as a boot
// failure.
func isBucketRaceError(err error) bool {
	if err == nil {
		return false
	}
	var er minio.ErrorResponse
	if libserrors.As(err, &er) {
		return er.Code == "BucketAlreadyOwnedByYou" || er.Code == "BucketAlreadyExists"
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "already own") || strings.Contains(msg, "already exists")
}

// IngestURLs presigns one PUT URL per file under prefix.
func (b *Backends) IngestURLs(ctx context.Context, storage, prefix string, files []string) ([]domain.PutURL, error) {
	be, err := b.get(storage)
	if err != nil {
		return nil, err
	}

	urls := make([]domain.PutURL, 0, len(files))
	for _, name := range files {
		u, err := be.client.PresignedPutObject(ctx, be.bucket, prefix+name, PresignExpiry)
		if err != nil {
			return nil, fmt.Errorf("presign put %s: %w", name, err)
		}
		urls = append(urls, domain.PutURL{Name: name, URL: u.String()})
	}
	return urls, nil
}

// DownloadURLs presigns one GET URL for every object found under prefix.
// name is the object key relative to prefix.
func (b *Backends) DownloadURLs(ctx context.Context, storage, prefix string) ([]domain.GetURL, error) {
	be, err := b.get(storage)
	if err != nil {
		return nil, err
	}

	var urls []domain.GetURL
	for obj := range be.client.ListObjects(ctx, be.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		u, err := be.client.PresignedGetObject(ctx, be.bucket, obj.Key, PresignExpiry, nil)
		if err != nil {
			return nil, fmt.Errorf("presign get %s: %w", obj.Key, err)
		}
		urls = append(urls, domain.GetURL{Name: strings.TrimPrefix(obj.Key, prefix), URL: u.String()})
	}
	return urls, nil
}

// Move server-side copies every object under fromPrefix to the
// corresponding key under toPrefix, then removes the source — within one
// backend (the pending/<job> -> linked flow).
func (b *Backends) Move(ctx context.Context, storage, fromPrefix, toPrefix string) (int, error) {
	be, err := b.get(storage)
	if err != nil {
		return 0, err
	}

	moved := 0
	for obj := range be.client.ListObjects(ctx, be.bucket, minio.ListObjectsOptions{Prefix: fromPrefix, Recursive: true}) {
		if obj.Err != nil {
			return moved, obj.Err
		}
		newKey := toPrefix + strings.TrimPrefix(obj.Key, fromPrefix)
		if _, err := be.client.CopyObject(ctx,
			minio.CopyDestOptions{Bucket: be.bucket, Object: newKey},
			minio.CopySrcOptions{Bucket: be.bucket, Object: obj.Key},
		); err != nil {
			return moved, fmt.Errorf("copy %s -> %s: %w", obj.Key, newKey, err)
		}
		if err := be.client.RemoveObject(ctx, be.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return moved, fmt.Errorf("remove %s: %w", obj.Key, err)
		}
		moved++
	}
	return moved, nil
}

// Copy streams every object under prefix from fromStorage to toStorage
// (cross-backend — used by migration). Returns the count and total bytes
// copied.
func (b *Backends) Copy(ctx context.Context, fromStorage, toStorage, prefix string) (int, int64, error) {
	src, err := b.get(fromStorage)
	if err != nil {
		return 0, 0, err
	}
	dst, err := b.get(toStorage)
	if err != nil {
		return 0, 0, err
	}

	copied := 0
	var totalBytes int64
	for obj := range src.client.ListObjects(ctx, src.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return copied, totalBytes, obj.Err
		}

		n, err := b.copyOne(ctx, src, dst, obj.Key)
		if err != nil {
			return copied, totalBytes, err
		}
		copied++
		totalBytes += n
	}
	return copied, totalBytes, nil
}

// copyOne streams a single object from src to dst, closing the source
// reader regardless of outcome.
func (b *Backends) copyOne(ctx context.Context, src, dst *backend, key string) (int64, error) {
	reader, err := src.client.GetObject(ctx, src.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return 0, fmt.Errorf("get %s: %w", key, err)
	}
	defer reader.Close()

	stat, err := reader.Stat()
	if err != nil {
		return 0, fmt.Errorf("stat %s: %w", key, err)
	}

	if _, err := dst.client.PutObject(ctx, dst.bucket, key, reader, stat.Size, minio.PutObjectOptions{ContentType: stat.ContentType}); err != nil {
		return 0, fmt.Errorf("put %s: %w", key, err)
	}
	return stat.Size, nil
}

// DeletePrefix removes every object under prefix — eviction/cleanup.
func (b *Backends) DeletePrefix(ctx context.Context, storage, prefix string) (int, error) {
	be, err := b.get(storage)
	if err != nil {
		return 0, err
	}

	deleted := 0
	for obj := range be.client.ListObjects(ctx, be.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return deleted, obj.Err
		}
		if err := be.client.RemoveObject(ctx, be.bucket, obj.Key, minio.RemoveObjectOptions{}); err != nil {
			return deleted, fmt.Errorf("remove %s: %w", obj.Key, err)
		}
		deleted++
	}
	return deleted, nil
}

// List returns every object under prefix (bucket-relative key + size).
func (b *Backends) List(ctx context.Context, storage, prefix string) ([]domain.Object, error) {
	be, err := b.get(storage)
	if err != nil {
		return nil, err
	}

	var objects []domain.Object
	for obj := range be.client.ListObjects(ctx, be.bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		objects = append(objects, domain.Object{Key: obj.Key, Size: obj.Size})
	}
	return objects, nil
}

// BaseURLs returns the canonical base URL for every configured backend
// (scheme derived from UseSSL) — the only place that knows
// http://minio:9000/raw-library vs https://s3.firstvds.ru/raw-library.
func (b *Backends) BaseURLs() map[string]string {
	out := make(map[string]string, len(b.byID))
	for id, be := range b.byID {
		scheme := "http"
		if be.useSSL {
			scheme = "https"
		}
		out[id] = fmt.Sprintf("%s://%s/%s", scheme, be.endpoint, be.bucket)
	}
	return out
}

// Health probes every backend (BucketExists) and always reports both
// "minio" and "s3" keys — "down" for a backend that isn't configured at
// all (e.g. no STORAGE_S3_ENDPOINT), so callers get a stable shape.
func (b *Backends) Health(ctx context.Context) map[string]string {
	out := map[string]string{domain.BackendMinio: "down", domain.BackendS3: "down"}
	for id, be := range b.byID {
		if ok, err := be.client.BucketExists(ctx, be.bucket); err == nil && ok {
			out[id] = "up"
		}
	}
	return out
}
