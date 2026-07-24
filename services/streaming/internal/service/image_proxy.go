package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"image"
	_ "image/gif" // register decoder for upstream GIF posters
	"image/jpeg"
	_ "image/png" // register decoder for upstream PNG posters
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
	"github.com/minio/minio-go/v7"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register decoder for upstream WebP posters
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
		"myanimelist.net",
	}

	shikimoriAnimeIDRe = regexp.MustCompile(`/uploads/poster/animes/(\d+)/`)

	// gachaImagePattern matches the relative gacha art URLs the frontend sends
	// for resizing. Mirrors the key validation in
	// services/gacha/internal/handler/images.go's validKeyPattern. This is the
	// ONLY gate for relative URLs in GetImage — it anchors ^...$, allows only
	// the cards/ and banners/ prefixes, and restricts keys to
	// [A-Za-z0-9._-]+ (no `/`, so no path traversal is expressible).
	gachaImagePattern = regexp.MustCompile(`^/api/gacha/images/((?:cards|banners)/[A-Za-z0-9._-]+)$`)

	// Allowed resize widths — a fixed bucket set bounds MinIO cache
	// fragmentation to at most len(allowedWidths) variants per poster.
	// Mirrored in frontend/web/src/composables/useImageProxy.ts.
	allowedWidths = []int{128, 256, 384, 512, 640}
)

const resizeJPEGQuality = 80

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

// degradationLevelReader is the minimal slice of *cache.DegradationWatcher the
// image proxy needs: the current platform degradation level. It is declared as
// an interface (rather than the concrete *cache.DegradationWatcher) so the
// resize-shed decision is unit-testable with a trivial fake — no Redis required
// — while the real watcher satisfies it. Fail-open: a nil reader reads as
// level 0 and never sheds. NOTE: unlike a concrete *DegradationWatcher (whose
// Level() is nil-receiver-safe), a nil INTERFACE is not method-callable, so
// callers must nil-check the field before calling Level().
type degradationLevelReader interface {
	Level() int
}

type ImageProxyService struct {
	storage      *videoutils.Storage
	httpClient   *http.Client
	sfGroup      singleflight.Group
	semaphore    chan struct{}
	degradation  degradationLevelReader
	log          *logger.Logger
	gachaBaseURL string
}

func NewImageProxyService(storage *videoutils.Storage, degradation degradationLevelReader, log *logger.Logger, gachaBaseURL string) *ImageProxyService {
	return &ImageProxyService{
		storage: storage,
		httpClient: &http.Client{
			Timeout: fetchTimeout,
			// SSRF guard (finding #64): the image proxy fetches only public
			// poster hosts (Shikimori / MyAnimeList) — MinIO reads use the SDK
			// client, not this httpClient — so every dial may be blocked from
			// reaching a private/loopback/link-local address. This closes both a
			// redirect-to-internal hop and a rebinding upstream host.
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
					Control:   netguard.DenyPrivateControl,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          10,
				IdleConnTimeout:       30 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				// Re-validate the redirect target host/scheme up front (finding
				// #64); the dial guard above is the authoritative rebind-safe layer.
				if err := netguard.ValidatePublicURL(req.URL.String()); err != nil {
					return fmt.Errorf("redirect blocked: %w", err)
				}
				return nil
			},
		},
		semaphore:    make(chan struct{}, maxConcurrent),
		degradation:  degradation,
		log:          log,
		gachaBaseURL: gachaBaseURL,
	}
}

// rewriteGachaURL maps a relative gacha image URL onto the internal gacha
// service base. SSRF-safe: fixed base (from config only) + strict key regex
// (no traversal chars) — the regex is the sole gate for relative URLs.
func (s *ImageProxyService) rewriteGachaURL(rawURL string) (string, bool) {
	m := gachaImagePattern.FindStringSubmatch(rawURL)
	if m == nil {
		return "", false
	}
	return s.gachaBaseURL + "/api/gacha/images/" + m[1], true
}

// GetImage returns the proxied image. width <= 0 serves the upstream
// original; width > 0 is snapped up to the nearest allowed bucket and the
// image is downscaled server-side (a 56px row thumbnail downloads ~5-20KB
// instead of the 300-530KB Shikimori original).
func (s *ImageProxyService) GetImage(ctx context.Context, rawURL string, width int) (*ImageResult, error) {
	if rawURL == "" {
		return s.placeholderResult(), nil
	}

	if rewritten, ok := s.rewriteGachaURL(rawURL); ok {
		rawURL = rewritten
	} else if !s.isDomainAllowed(rawURL) {
		return nil, fmt.Errorf("domain not allowed")
	}

	width = snapWidth(width)

	cacheKey := posterPrefix + s.hashURL(rawURL)
	sfKey := cacheKey
	if width > 0 {
		sfKey = fmt.Sprintf("%s|w=%d", cacheKey, width)
	}

	result, err, _ := s.sfGroup.Do(sfKey, func() (interface{}, error) {
		if width > 0 {
			return s.resolveResized(ctx, rawURL, cacheKey, width)
		}
		return s.resolveImage(ctx, rawURL, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	return result.(*ImageResult), nil
}

// snapWidth snaps a requested width UP to the nearest allowed bucket
// (largest bucket when the request exceeds it); <= 0 means "original size".
func snapWidth(width int) int {
	if width <= 0 {
		return 0
	}
	for _, w := range allowedWidths {
		if width <= w {
			return w
		}
	}
	return allowedWidths[len(allowedWidths)-1]
}

// resolveResized serves posters/w{width}/{hash} from MinIO, deriving it from
// the full-size image (which itself goes through the regular cache → upstream
// → MAL-fallback chain) on a miss.
func (s *ImageProxyService) resolveResized(ctx context.Context, rawURL, cacheKey string, width int) (*ImageResult, error) {
	resizedKey := fmt.Sprintf("%sw%d/%s", posterPrefix, width, s.hashURL(rawURL))

	if result, err := s.fromCache(ctx, resizedKey); err == nil {
		return result, nil
	}

	// Share the full-size resolution with concurrent full-size/other-width
	// requests via the same singleflight key.
	fullI, err, _ := s.sfGroup.Do(cacheKey, func() (interface{}, error) {
		return s.resolveImage(ctx, rawURL, cacheKey)
	})
	if err != nil {
		return nil, err
	}
	full := fullI.(*ImageResult)
	// Don't try to resize the 1x1 placeholder
	if full.Source == SourcePlaceholder {
		return full, nil
	}

	result, cacheable := s.resizeOrShed(full, width)
	if cacheable {
		// Only the genuinely-downscaled bytes are worth caching; the shed
		// (Critical degradation) and resize-failure paths return the original
		// unchanged, so normal sizes regenerate once healthy.
		s.storeInCache(ctx, resizedKey, result.Data, result.ContentType)
	}
	return result, nil
}

// resizeOrShed downscales full to width, or serves the un-resized original when
// the platform is under Critical degradation (level >= 2). The bool reports
// whether the returned variant should be cached — false on both the shed path
// and the resize-failure path (each returns full unchanged).
//
// Fail-open by design: a nil reader, or a *cache.DegradationWatcher whose
// governor is down/undeployed, both read level 0 → normal resize. The explicit
// nil check is required because s.degradation is an interface (a nil interface
// is not method-callable).
func (s *ImageProxyService) resizeOrShed(full *ImageResult, width int) (*ImageResult, bool) {
	if s.degradation != nil && s.degradation.Level() >= 2 {
		// Critical platform degradation: skip the CPU-heavy decode+scale+encode
		// and serve the un-resized original (already resolved above). Mirrors
		// the proven resize-failure fallback below. The resized variant is
		// intentionally NOT cached on this path, so normal sizes regenerate once
		// pressure clears.
		return full, false
	}

	data, contentType, err := downscaleImage(full.Data, width)
	if err != nil {
		// Unsupported/corrupt format — serve the full-size original instead
		s.log.Warnw("image resize failed, serving original", "width", width, "error", err)
		return full, false
	}

	return &ImageResult{Data: data, ContentType: contentType, Source: full.Source}, true
}

// downscaleImage scales src down to targetWidth (never upscales) and
// re-encodes as JPEG. Returns the encoded bytes and content type.
func downscaleImage(src []byte, targetWidth int) ([]byte, string, error) {
	img, _, err := image.Decode(bytes.NewReader(src))
	if err != nil {
		return nil, "", fmt.Errorf("decode: %w", err)
	}

	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return nil, "", fmt.Errorf("invalid dimensions %dx%d", srcW, srcH)
	}

	outW, outH := srcW, srcH
	if srcW > targetWidth {
		outW = targetWidth
		outH = (srcH*targetWidth + srcW/2) / srcW
		if outH < 1 {
			outH = 1
		}
	}

	dst := image.NewRGBA(image.Rect(0, 0, outW, outH))
	if outW == srcW && outH == srcH {
		draw.Copy(dst, image.Point{}, img, bounds, draw.Src, nil)
	} else {
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: resizeJPEGQuality}); err != nil {
		return nil, "", fmt.Errorf("encode: %w", err)
	}
	return buf.Bytes(), "image/jpeg", nil
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
