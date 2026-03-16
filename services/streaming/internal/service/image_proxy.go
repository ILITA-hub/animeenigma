package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/singleflight"
)

const (
	posterPrefix      = "posters/"
	placeholderPrefix = "posters/placeholder/"
	maxImageSize      = 5 * 1024 * 1024 // 5 MB
	fetchTimeout      = 10 * time.Second
	maxConcurrent     = 50
)

var (
	allowedDomains = []string{
		"shiki.one",
		"shikimori.io",
		"shikimori.one",
		"cdn.myanimelist.net",
	}

	shikimoriAnimeIDRe = regexp.MustCompile(`/uploads/poster/animes/(\d+)/`)
)

type ImageSource string

const (
	SourceCache       ImageSource = "cache_hit"
	SourceShikimori   ImageSource = "shikimori"
	SourceMAL         ImageSource = "mal"
	SourcePlaceholder ImageSource = "placeholder"
)

type ImageResult struct {
	Data        []byte
	ContentType string
	Source      ImageSource
}

type ImageProxyService struct {
	storage    *videoutils.Storage
	httpClient *http.Client
	sfGroup    singleflight.Group
	semaphore  chan struct{}
	log        *logger.Logger
}

func NewImageProxyService(storage *videoutils.Storage, log *logger.Logger) *ImageProxyService {
	return &ImageProxyService{
		storage: storage,
		httpClient: &http.Client{
			Timeout: fetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		semaphore: make(chan struct{}, maxConcurrent),
		log:       log,
	}
}

func (s *ImageProxyService) GetImage(ctx context.Context, rawURL string) (*ImageResult, error) {
	if rawURL == "" {
		return s.placeholderResult(), nil
	}

	if !s.isDomainAllowed(rawURL) {
		return nil, fmt.Errorf("domain not allowed")
	}

	cacheKey := posterPrefix + s.hashURL(rawURL)

	result, err, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		return s.resolveImage(ctx, rawURL, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImageResult), nil
}

func (s *ImageProxyService) resolveImage(ctx context.Context, rawURL, cacheKey string) (*ImageResult, error) {
	// 1. Check MinIO cache
	if result, err := s.fromCache(ctx, cacheKey); err == nil {
		return result, nil
	}

	// Check placeholder cache
	placeholderKey := placeholderPrefix + s.hashURL(rawURL)
	if result, err := s.fromCache(ctx, placeholderKey); err == nil {
		result.Source = SourcePlaceholder
		return result, nil
	}

	// Acquire semaphore for upstream fetch
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 2. Fetch from original URL (Shikimori)
	if data, contentType, err := s.fetchURL(ctx, rawURL); err == nil {
		result := &ImageResult{Data: data, ContentType: contentType, Source: SourceShikimori}
		s.storeInCache(ctx, cacheKey, data, contentType)
		return result, nil
	} else {
		s.log.Warnw("shikimori image fetch failed", "url", rawURL, "error", err)
	}

	// 3. Try MAL fallback via Jikan
	if malResult := s.tryMALFallback(ctx, rawURL, cacheKey); malResult != nil {
		return malResult, nil
	}

	// 4. All failed → store and serve placeholder
	placeholder := s.placeholderResult()
	s.storeInCache(ctx, placeholderKey, placeholder.Data, placeholder.ContentType)
	return placeholder, nil
}

func (s *ImageProxyService) tryMALFallback(ctx context.Context, originalURL, cacheKey string) *ImageResult {
	animeID := s.extractAnimeID(originalURL)
	if animeID == "" {
		return nil
	}

	jikanURL := fmt.Sprintf("https://api.jikan.moe/v4/anime/%s", animeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jikanURL, nil)
	if err != nil {
		return nil
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		s.log.Warnw("jikan API call failed", "anime_id", animeID, "error", err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		s.log.Warnw("jikan API returned non-200", "anime_id", animeID, "status", resp.StatusCode)
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil
	}

	malPosterURL := extractJikanImageURL(body)
	if malPosterURL == "" {
		s.log.Warnw("no image URL in jikan response", "anime_id", animeID)
		return nil
	}

	data, contentType, err := s.fetchURL(ctx, malPosterURL)
	if err != nil {
		s.log.Warnw("MAL image fetch failed", "url", malPosterURL, "error", err)
		return nil
	}

	result := &ImageResult{Data: data, ContentType: contentType, Source: SourceMAL}
	s.storeInCache(ctx, cacheKey, data, contentType)
	return result
}

func (s *ImageProxyService) fromCache(ctx context.Context, key string) (*ImageResult, error) {
	obj, err := s.storage.Client().GetObject(ctx, s.storage.BucketName(), key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer obj.Close()

	info, err := obj.Stat()
	if err != nil {
		return nil, err
	}

	data, err := io.ReadAll(io.LimitReader(obj, maxImageSize))
	if err != nil {
		return nil, err
	}

	contentType := info.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	return &ImageResult{Data: data, ContentType: contentType, Source: SourceCache}, nil
}

func (s *ImageProxyService) storeInCache(ctx context.Context, key string, data []byte, contentType string) {
	if contentType == "" {
		contentType = "image/jpeg"
	}
	reader := bytes.NewReader(data)
	_, err := s.storage.Client().PutObject(ctx, s.storage.BucketName(), key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		s.log.Errorw("failed to cache image in minio", "key", key, "error", err)
	}
}

func (s *ImageProxyService) fetchURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("upstream returned %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return nil, "", fmt.Errorf("unexpected content-type: %s", contentType)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageSize))
	if err != nil {
		return nil, "", err
	}

	return data, contentType, nil
}

func (s *ImageProxyService) isDomainAllowed(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	for _, d := range allowedDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

func (s *ImageProxyService) hashURL(rawURL string) string {
	h := sha256.Sum256([]byte(rawURL))
	return fmt.Sprintf("%x", h)
}

func (s *ImageProxyService) extractAnimeID(posterURL string) string {
	matches := shikimoriAnimeIDRe.FindStringSubmatch(posterURL)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func (s *ImageProxyService) placeholderResult() *ImageResult {
	return &ImageResult{
		Data:        placeholderPNG,
		ContentType: "image/png",
		Source:      SourcePlaceholder,
	}
}

// Minimal 1x1 grey PNG placeholder (67 bytes)
var placeholderPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, 0xde, 0x00, 0x00, 0x00,
	0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0x60, 0x60, 0x60, 0x00,
	0x00, 0x00, 0x04, 0x00, 0x01, 0x27, 0x34, 0x27, 0x0a, 0x00, 0x00, 0x00,
	0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func extractJikanImageURL(body []byte) string {
	s := string(body)
	key := `"large_image_url":"`
	idx := strings.Index(s, key)
	if idx == -1 {
		return ""
	}
	start := idx + len(key)
	end := strings.Index(s[start:], `"`)
	if end == -1 {
		return ""
	}
	u := s[start : start+end]
	u = strings.ReplaceAll(u, `\/`, `/`)
	return u
}
