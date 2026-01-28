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

	"github.com/ILITA-hub/animeenigma/libs/animeparser"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/config"
)

// SourceType represents the type of video source
type SourceType string

const (
	SourceTypeMinio    SourceType = "minio"    // Self-hosted MinIO storage
	SourceTypeAniboom  SourceType = "aniboom"  // Aniboom external API (needs proxy)
	SourceTypeKodik    SourceType = "kodik"    // Kodik external API (iframe embed)
	SourceTypeDirect   SourceType = "direct"   // Direct URL (frontend can play directly)
	SourceTypeExternal SourceType = "external" // Generic external URL (may need proxy)
)

type StreamingService struct {
	storage          *videoutils.Storage
	proxy            *videoutils.VideoProxy
	providerRegistry *animeparser.ProviderRegistry
	cache            *cache.RedisCache
	config           *config.Config
	log              *logger.Logger
}

// NewStreamingService creates a new streaming service with external provider support
func NewStreamingService(
	storage *videoutils.Storage,
	proxy *videoutils.VideoProxy,
	cache *cache.RedisCache,
	cfg *config.Config,
	log *logger.Logger,
) *StreamingService {
	// Initialize provider registry
	registry := animeparser.NewProviderRegistry()

	// Register Kodik provider if configured
	if cfg.Providers.Kodik.APIKey != "" {
		kodikProvider := animeparser.NewKodikProvider(animeparser.KodikConfig{
			APIKey:  cfg.Providers.Kodik.APIKey,
			BaseURL: cfg.Providers.Kodik.BaseURL,
		})
		registry.Register(kodikProvider)
		log.Infow("registered video provider", "provider", "kodik")
	}

	// Register Aniboom provider if configured
	if cfg.Providers.Aniboom.BaseURL != "" {
		aniboomProvider := animeparser.NewAniboomProvider(animeparser.AniboomConfig{
			BaseURL: cfg.Providers.Aniboom.BaseURL,
		})
		registry.Register(aniboomProvider)
		log.Infow("registered video provider", "provider", "aniboom")
	}

	return &StreamingService{
		storage:          storage,
		proxy:            proxy,
		providerRegistry: registry,
		cache:            cache,
		config:           cfg,
		log:              log,
	}
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

// =============================================================================
// External Provider Integration
// =============================================================================

// ExternalVideoSource represents a video source from an external provider
type ExternalVideoSource struct {
	Provider        string    `json:"provider"`
	EmbedURL        string    `json:"embed_url,omitempty"`      // For iframe embed (Kodik)
	StreamURL       string    `json:"stream_url,omitempty"`     // Direct stream URL
	ProxyURL        string    `json:"proxy_url,omitempty"`      // URL to proxy through our backend
	Quality         string    `json:"quality"`
	Translation     string    `json:"translation"`
	TranslationType string    `json:"translation_type"`
	Episode         int       `json:"episode"`
	IsDirect        bool      `json:"is_direct"`        // Can frontend play directly?
	NeedsProxy      bool      `json:"needs_proxy"`      // Should use our proxy endpoint?
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
}

// GetVideoSources fetches video sources for an anime from all registered providers
func (s *StreamingService) GetVideoSources(ctx context.Context, animeTitle, animeTitleJP string) ([]ExternalVideoSource, error) {
	// Try cache first
	cacheKey := fmt.Sprintf("video_sources:%s", animeTitle)
	var cached []ExternalVideoSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil && len(cached) > 0 {
		// Check if any sources have expired
		validSources := make([]ExternalVideoSource, 0, len(cached))
		for _, src := range cached {
			if src.ExpiresAt.IsZero() || src.ExpiresAt.After(time.Now()) {
				validSources = append(validSources, src)
			}
		}
		if len(validSources) > 0 {
			return validSources, nil
		}
	}

	// Fetch from providers
	parserSources, err := s.providerRegistry.SearchAll(ctx, animeTitle, animeTitleJP)
	if err != nil {
		s.log.Warnw("failed to search video providers", "error", err, "title", animeTitle)
	}

	// Convert to our format and generate proxy URLs
	sources := make([]ExternalVideoSource, 0, len(parserSources))
	for _, ps := range parserSources {
		source := ExternalVideoSource{
			Provider:        ps.Provider,
			Quality:         ps.Quality,
			Translation:     ps.Translation,
			TranslationType: ps.TranslationType,
			Episode:         ps.Episode,
			IsDirect:        ps.IsDirect,
			NeedsProxy:      ps.NeedsProxy,
			ExpiresAt:       ps.ExpiresAt,
		}

		// Set appropriate URLs based on provider type
		switch ps.Provider {
		case "kodik":
			// Kodik uses iframe embed, frontend can use directly
			source.EmbedURL = ps.URL
			source.IsDirect = false
			source.NeedsProxy = false
		case "aniboom":
			// Aniboom needs proxy for CORS
			source.StreamURL = ps.URL
			// Generate proxy URL with signed token
			token, _, err := s.GenerateStreamToken(
				fmt.Sprintf("%s-ep%d", animeTitle, ps.Episode),
				videoutils.VideoSourceExternal,
				ps.URL,
				"",
				"",
			)
			if err == nil {
				source.ProxyURL = fmt.Sprintf("/api/v1/stream/proxy?token=%s", token)
			}
		default:
			source.StreamURL = ps.URL
		}

		sources = append(sources, source)
	}

	// Cache for shorter period since external URLs expire
	if len(sources) > 0 {
		_ = s.cache.Set(ctx, cacheKey, sources, 30*time.Minute)
	}

	return sources, nil
}

// GetEpisodeSources fetches video sources for a specific episode
func (s *StreamingService) GetEpisodeSources(ctx context.Context, shikimoriID string, episode int) ([]ExternalVideoSource, error) {
	cacheKey := fmt.Sprintf("episode_sources:%s:%d", shikimoriID, episode)
	var cached []ExternalVideoSource
	if err := s.cache.Get(ctx, cacheKey, &cached); err == nil && len(cached) > 0 {
		return cached, nil
	}

	var allSources []ExternalVideoSource

	// Query each provider
	for name, provider := range map[string]animeparser.VideoSourceProvider{} {
		p, ok := s.providerRegistry.Get(name)
		if !ok {
			continue
		}
		provider = p

		episodes, err := provider.GetEpisodes(ctx, shikimoriID)
		if err != nil {
			s.log.Warnw("failed to get episodes from provider",
				"provider", name,
				"shikimori_id", shikimoriID,
				"error", err)
			continue
		}

		for _, ep := range episodes {
			if ep.Episode != episode {
				continue
			}

			source := ExternalVideoSource{
				Provider:        ep.Provider,
				Quality:         ep.Quality,
				Translation:     ep.Translation,
				TranslationType: ep.TranslationType,
				Episode:         ep.Episode,
				IsDirect:        ep.IsDirect,
				NeedsProxy:      ep.NeedsProxy,
				ExpiresAt:       ep.ExpiresAt,
			}

			switch ep.Provider {
			case "kodik":
				source.EmbedURL = ep.URL
			case "aniboom":
				source.StreamURL = ep.URL
				token, _, err := s.GenerateStreamToken(
					fmt.Sprintf("%s-ep%d", shikimoriID, episode),
					videoutils.VideoSourceExternal,
					ep.URL,
					"",
					"",
				)
				if err == nil {
					source.ProxyURL = fmt.Sprintf("/api/v1/stream/proxy?token=%s", token)
				}
			default:
				source.StreamURL = ep.URL
			}

			allSources = append(allSources, source)
		}
	}

	if len(allSources) > 0 {
		_ = s.cache.Set(ctx, cacheKey, allSources, 30*time.Minute)
	}

	return allSources, nil
}

// ResolveStreamURL resolves and returns a fresh stream URL for a video
func (s *StreamingService) ResolveStreamURL(ctx context.Context, provider, videoID, quality string) (*ExternalVideoSource, error) {
	p, ok := s.providerRegistry.Get(provider)
	if !ok {
		return nil, errors.NotFound(fmt.Sprintf("provider %s not found", provider))
	}

	source, err := p.GetStreamURL(ctx, videoID, quality)
	if err != nil {
		return nil, err
	}

	result := &ExternalVideoSource{
		Provider:   source.Provider,
		StreamURL:  source.URL,
		Quality:    source.Quality,
		IsDirect:   source.IsDirect,
		NeedsProxy: source.NeedsProxy,
		ExpiresAt:  source.ExpiresAt,
	}

	// Generate proxy URL if needed
	if source.NeedsProxy {
		token, _, err := s.GenerateStreamToken(
			videoID,
			videoutils.VideoSourceExternal,
			source.URL,
			"",
			"",
		)
		if err == nil {
			result.ProxyURL = fmt.Sprintf("/api/v1/stream/proxy?token=%s", token)
		}
	}

	return result, nil
}

// GetProviders returns list of registered video providers
func (s *StreamingService) GetProviders() []string {
	providers := []string{}
	for _, name := range []string{"kodik", "aniboom"} {
		if _, ok := s.providerRegistry.Get(name); ok {
			providers = append(providers, name)
		}
	}
	return providers
}
