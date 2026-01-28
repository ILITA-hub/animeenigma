package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/config"
)

type StreamingService struct {
	storage *videoutils.Storage
	proxy   *videoutils.VideoProxy
	cache   *cache.RedisCache
	config  *config.Config
	log     *logger.Logger
}

func NewStreamingService(
	storage *videoutils.Storage,
	proxy *videoutils.VideoProxy,
	cache *cache.RedisCache,
	cfg *config.Config,
	log *logger.Logger,
) *StreamingService {
	return &StreamingService{
		storage: storage,
		proxy:   proxy,
		cache:   cache,
		config:  cfg,
		log:     log,
	}
}

// StreamToken represents a signed token for video access
type StreamToken struct {
	VideoID    string            `json:"vid"`
	SourceType videoutils.VideoSource `json:"st"`
	SourceURL  string            `json:"url,omitempty"`
	StorageKey string            `json:"key,omitempty"`
	UserID     string            `json:"uid,omitempty"`
	ExpiresAt  int64             `json:"exp"`
}

// StreamInfo contains information for streaming a video
type StreamInfo struct {
	AnimeID       string         `json:"anime_id"`
	AnimeName     string         `json:"anime_name"`
	EpisodeNumber int            `json:"episode_number"`
	EpisodeName   string         `json:"episode_name,omitempty"`
	Duration      int            `json:"duration,omitempty"`
	Sources       []StreamSource `json:"sources"`
	ThumbnailURL  string         `json:"thumbnail_url,omitempty"`
	NextEpisode   *int           `json:"next_episode,omitempty"`
	PrevEpisode   *int           `json:"previous_episode,omitempty"`
}

type StreamSource struct {
	ID           string `json:"id"`
	Type         string `json:"type"`
	Quality      string `json:"quality"`
	Language     string `json:"language"`
	URL          string `json:"url"`
	ExpiresAt    string `json:"expires_at"`
	RequiresProxy bool  `json:"requires_proxy"`
}

// GenerateStreamToken creates a signed token for video access
func (s *StreamingService) GenerateStreamToken(videoID string, sourceType videoutils.VideoSource, sourceURL, storageKey, userID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(s.config.Stream.TokenTTL)

	token := StreamToken{
		VideoID:    videoID,
		SourceType: sourceType,
		SourceURL:  sourceURL,
		StorageKey: storageKey,
		UserID:     userID,
		ExpiresAt:  expiresAt.Unix(),
	}

	tokenBytes, err := json.Marshal(token)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal token: %w", err)
	}

	// Sign the token
	mac := hmac.New(sha256.New, []byte(s.config.Stream.TokenSecret))
	mac.Write(tokenBytes)
	signature := mac.Sum(nil)

	// Combine token and signature
	signed := append(tokenBytes, signature...)
	encoded := base64.URLEncoding.EncodeToString(signed)

	return encoded, expiresAt, nil
}

// ValidateStreamToken validates and decodes a stream token
func (s *StreamingService) ValidateStreamToken(tokenStr string) (*StreamToken, error) {
	decoded, err := base64.URLEncoding.DecodeString(tokenStr)
	if err != nil {
		return nil, errors.InvalidInput("invalid token encoding")
	}

	if len(decoded) < 32 {
		return nil, errors.InvalidInput("invalid token format")
	}

	tokenBytes := decoded[:len(decoded)-32]
	signature := decoded[len(decoded)-32:]

	// Verify signature
	mac := hmac.New(sha256.New, []byte(s.config.Stream.TokenSecret))
	mac.Write(tokenBytes)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSig) {
		return nil, errors.Unauthorized("invalid token signature")
	}

	var token StreamToken
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return nil, errors.InvalidInput("invalid token data")
	}

	// Check expiration
	if time.Now().Unix() > token.ExpiresAt {
		return nil, errors.Unauthorized("token expired")
	}

	return &token, nil
}

// ProxyExternalStream proxies video content from an external URL
func (s *StreamingService) ProxyExternalStream(ctx context.Context, token *StreamToken, w http.ResponseWriter, r *http.Request) error {
	if token.SourceURL == "" {
		return errors.InvalidInput("no source URL in token")
	}

	return s.proxy.ProxyStream(ctx, token.SourceURL, w, r)
}

// StreamFromStorage streams video directly from MinIO
func (s *StreamingService) StreamFromStorage(ctx context.Context, token *StreamToken, w http.ResponseWriter, r *http.Request) error {
	if token.StorageKey == "" {
		return errors.InvalidInput("no storage key in token")
	}

	reader, fileInfo, err := s.storage.Download(ctx, token.StorageKey)
	if err != nil {
		return fmt.Errorf("download from storage: %w", err)
	}
	defer reader.Close()

	// Set headers
	w.Header().Set("Content-Type", fileInfo.ContentType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "public, max-age=86400")

	// Handle range requests for seeking
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		// TODO: Implement proper range request handling
		w.Header().Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", fileInfo.Size-1, fileInfo.Size))
		w.WriteHeader(http.StatusPartialContent)
	}

	_, err = io.Copy(w, reader)
	return err
}

// GetPresignedURL generates a presigned URL for direct MinIO access
func (s *StreamingService) GetPresignedURL(ctx context.Context, storageKey string, expiry time.Duration) (string, error) {
	return s.storage.GetPresignedURL(ctx, storageKey, expiry)
}

// Upload uploads a video file to storage
func (s *StreamingService) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (*videoutils.VideoFile, error) {
	if size > s.config.Stream.MaxUploadSize {
		return nil, errors.InvalidInput(fmt.Sprintf("file size exceeds maximum of %d bytes", s.config.Stream.MaxUploadSize))
	}

	return s.storage.Upload(ctx, key, reader, size, contentType)
}

// GetUploadURL generates a presigned URL for direct upload
func (s *StreamingService) GetUploadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	// For now, use the download URL generation
	// In production, you'd want a separate presigned PUT URL
	return s.storage.GetPresignedURL(ctx, key, expiry)
}

// Delete removes a video from storage
func (s *StreamingService) Delete(ctx context.Context, storageKey string) error {
	return s.storage.Delete(ctx, storageKey)
}
