package videoutils

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// StorageConfig holds MinIO configuration
type StorageConfig struct {
	Endpoint        string `json:"endpoint" yaml:"endpoint"`
	AccessKeyID     string `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" yaml:"secret_access_key"`
	UseSSL          bool   `json:"use_ssl" yaml:"use_ssl"`
	BucketName      string `json:"bucket_name" yaml:"bucket_name"`
	Region          string `json:"region" yaml:"region"`
}

// Storage provides video storage operations
type Storage struct {
	client     *minio.Client
	bucketName string
	endpoint   string // host:port of the MinIO server, used to recognize own URLs
}

// Client returns the underlying MinIO client
func (s *Storage) Client() *minio.Client {
	return s.client
}

// BucketName returns the configured bucket name
func (s *Storage) BucketName() string {
	return s.bucketName
}

// Endpoint returns the configured MinIO endpoint (host[:port]). Used to mark
// MinIO as a first-party host for the HLS proxy's SSRF dial guard, since it
// resolves to a Docker-private IP the proxy must still reach.
func (s *Storage) Endpoint() string {
	return s.endpoint
}

// NewStorage creates a new MinIO storage client
func NewStorage(cfg StorageConfig) (*Storage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	return &Storage{
		client:     client,
		bucketName: cfg.BucketName,
		endpoint:   cfg.Endpoint,
	}, nil
}

// presignTTL bounds how long a presigned GET stays valid. The proxy
// re-presigns on EVERY upstream fetch (manifest + each segment), so this
// only needs to outlive a single request round-trip; 15m is generous.
const presignTTL = 15 * time.Minute

// PresignURL recognizes URLs that point at THIS MinIO server and rewrites
// them into short-lived presigned GET URLs so a credential-less HTTP client
// (the HLS proxy) can fetch private-bucket objects without the bucket being
// public. URLs for any other host are left untouched: it returns ("", false)
// so the caller fetches them unchanged.
//
// This is the seam that lets the streaming HLS proxy serve self-hosted
// library (`ae` provider) HLS from a PRIVATE MinIO bucket: the proxy still
// gates entry on our own HMAC signature / provenance tokens, then presigns
// the actual upstream MinIO read here.
// IsOwnHost reports whether rawURL points at THIS MinIO server. Used by the
// HLS proxy to label self-hosted (`ae` provider) playback traffic distinctly
// from external-CDN traffic in metrics.
func (s *Storage) IsOwnHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	return err == nil && u.Host == s.endpoint
}

func (s *Storage) PresignURL(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host != s.endpoint {
		return "", false
	}
	// Path is /{bucket}/{object...}; split off the first segment as bucket.
	p := strings.TrimPrefix(u.Path, "/")
	slash := strings.IndexByte(p, '/')
	if slash <= 0 || slash == len(p)-1 {
		return "", false
	}
	bucket, object := p[:slash], p[slash+1:]
	object, err = url.PathUnescape(object)
	if err != nil {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	signed, err := s.client.PresignedGetObject(ctx, bucket, object, presignTTL, nil)
	if err != nil {
		return "", false
	}
	return signed.String(), true
}

// MultiStorage composes several Storage backends (e.g. local MinIO + an
// external S3-compatible host) and routes presign requests to whichever one
// owns the URL's host. This is the seam that lets the HLS proxy presign
// upstream GETs for library episodes that may live on EITHER backend
// depending on which one ingested them — catalog signs stream URLs for both
// hosts identically, so the proxy just needs to know which Storage to ask.
//
// Construction never dials: it only holds references to already-built
// *Storage values (see NewStorage:51-66).
type MultiStorage struct {
	storages []*Storage
}

// NewMultiStorage wraps one or more Storage backends. Nil entries are
// skipped, so callers can pass an optional backend that may be absent
// (e.g. external S3 when unconfigured) without a manual nil check:
// NewMultiStorage(minioStorage, s3Storage) where s3Storage may be nil.
func NewMultiStorage(ss ...*Storage) *MultiStorage {
	m := &MultiStorage{}
	for _, s := range ss {
		if s != nil {
			m.storages = append(m.storages, s)
		}
	}
	return m
}

// PresignURL routes rawURL to the first wrapped Storage whose IsOwnHost
// matches and returns its presigned GET URL. A URL matching none of the
// wrapped hosts returns ("", false), same contract as Storage.PresignURL,
// so the caller fetches it unchanged.
func (m *MultiStorage) PresignURL(rawURL string) (string, bool) {
	for _, s := range m.storages {
		if s.IsOwnHost(rawURL) {
			return s.PresignURL(rawURL)
		}
	}
	return "", false
}

// IsOwnHost reports whether rawURL points at ANY of the wrapped storage
// backends. Used by the HLS proxy to label self-hosted (`ae` provider)
// playback traffic distinctly from external-CDN traffic in metrics,
// regardless of which backend serves the episode.
func (m *MultiStorage) IsOwnHost(rawURL string) bool {
	for _, s := range m.storages {
		if s.IsOwnHost(rawURL) {
			return true
		}
	}
	return false
}

// Hosts returns the endpoint host[:port] of every wrapped Storage, in
// registration order.
func (m *MultiStorage) Hosts() []string {
	hosts := make([]string, 0, len(m.storages))
	for _, s := range m.storages {
		hosts = append(hosts, s.endpoint)
	}
	return hosts
}

// EnsureBucket creates the bucket if it doesn't exist
func (s *Storage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucketName)
	if err != nil {
		return fmt.Errorf("check bucket exists: %w", err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.bucketName, minio.MakeBucketOptions{})
		if err != nil {
			return fmt.Errorf("create bucket: %w", err)
		}
	}

	return nil
}

// VideoFile represents a video file in storage
type VideoFile struct {
	Key          string    `json:"key"`
	Size         int64     `json:"size"`
	ContentType  string    `json:"content_type"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag"`
}

// Upload uploads a video file
func (s *Storage) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (*VideoFile, error) {
	info, err := s.client.PutObject(ctx, s.bucketName, key, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return nil, fmt.Errorf("upload file: %w", err)
	}

	return &VideoFile{
		Key:         key,
		Size:        info.Size,
		ContentType: contentType,
		ETag:        info.ETag,
	}, nil
}

// Download downloads a video file
func (s *Storage) Download(ctx context.Context, key string) (io.ReadCloser, *VideoFile, error) {
	obj, err := s.client.GetObject(ctx, s.bucketName, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("get object: %w", err)
	}

	info, err := obj.Stat()
	if err != nil {
		obj.Close()
		return nil, nil, fmt.Errorf("stat object: %w", err)
	}

	return obj, &VideoFile{
		Key:          key,
		Size:         info.Size,
		ContentType:  info.ContentType,
		LastModified: info.LastModified,
		ETag:         info.ETag,
	}, nil
}

// GetPresignedURL generates a presigned URL for direct download
func (s *Storage) GetPresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	reqParams := make(url.Values)
	presignedURL, err := s.client.PresignedGetObject(ctx, s.bucketName, key, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("generate presigned url: %w", err)
	}
	return presignedURL.String(), nil
}

// GetPresignedPutURL generates a presigned URL for a direct client-side upload
// (HTTP PUT). The client PUTs the object body straight to MinIO/S3 with this URL,
// bypassing the streaming service for the bytes.
func (s *Storage) GetPresignedPutURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	presignedURL, err := s.client.PresignedPutObject(ctx, s.bucketName, key, expiry)
	if err != nil {
		return "", fmt.Errorf("generate presigned put url: %w", err)
	}
	return presignedURL.String(), nil
}

// Delete removes a video file
func (s *Storage) Delete(ctx context.Context, key string) error {
	return s.client.RemoveObject(ctx, s.bucketName, key, minio.RemoveObjectOptions{})
}

// Exists checks if a file exists
func (s *Storage) Exists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, s.bucketName, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// List lists video files with optional prefix
func (s *Storage) List(ctx context.Context, prefix string) ([]*VideoFile, error) {
	var files []*VideoFile

	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}

	for obj := range s.client.ListObjects(ctx, s.bucketName, opts) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		files = append(files, &VideoFile{
			Key:          obj.Key,
			Size:         obj.Size,
			ContentType:  obj.ContentType,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
		})
	}

	return files, nil
}

// GenerateVideoKey generates a storage key for a video
func GenerateVideoKey(animeID, episodeNum string, quality string) string {
	return path.Join("videos", animeID, fmt.Sprintf("ep%s_%s.mp4", episodeNum, quality))
}

// GenerateThumbnailKey generates a storage key for a thumbnail
func GenerateThumbnailKey(animeID, episodeNum string) string {
	return path.Join("thumbnails", animeID, fmt.Sprintf("ep%s.jpg", episodeNum))
}

// GeneratePosterKey generates a storage key for an anime poster
func GeneratePosterKey(animeID string) string {
	return path.Join("posters", animeID+".jpg")
}
