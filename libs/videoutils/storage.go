package videoutils

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"path"
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
	}, nil
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
