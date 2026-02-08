package videoutils

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// VideoSource represents a source for video streaming
type VideoSource string

const (
	SourceMinio    VideoSource = "minio"    // Self-hosted MinIO storage
	SourceExternal VideoSource = "external" // External API/CDN
	SourceYouTube  VideoSource = "youtube"  // YouTube (for openings/endings)
)

// VideoStreamInfo contains information about a video stream
type VideoStreamInfo struct {
	Source      VideoSource `json:"source"`
	URL         string      `json:"url"`
	Quality     string      `json:"quality"`
	ContentType string      `json:"content_type"`
	Size        int64       `json:"size,omitempty"`
	Duration    int         `json:"duration,omitempty"` // seconds
}

// ProxyConfig configures the video proxy
type ProxyConfig struct {
	UserAgent       string        `json:"user_agent" yaml:"user_agent"`
	Timeout         time.Duration `json:"timeout" yaml:"timeout"`
	MaxBufferSize   int64         `json:"max_buffer_size" yaml:"max_buffer_size"`
	AllowedDomains  []string      `json:"allowed_domains" yaml:"allowed_domains"`
	RefererOverride string        `json:"referer_override" yaml:"referer_override"`
}

// DefaultProxyConfig returns sensible defaults
func DefaultProxyConfig() ProxyConfig {
	return ProxyConfig{
		UserAgent:     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
		Timeout:       30 * time.Second,
		MaxBufferSize: 10 * 1024 * 1024, // 10MB buffer
		AllowedDomains: []string{
			"storage.googleapis.com",
			"cdn.myanimelist.net",
			"shikimori.one",
			"nyaa.si",
			// Add more trusted domains
		},
	}
}

// VideoProxy handles proxying video streams from external sources
type VideoProxy struct {
	client *http.Client
	config ProxyConfig
}

// NewVideoProxy creates a new video proxy
func NewVideoProxy(cfg ProxyConfig) *VideoProxy {
	return &VideoProxy{
		client: &http.Client{
			Timeout: cfg.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				// Follow redirects but preserve headers
				if len(via) >= 10 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
		config: cfg,
	}
}

// ProxyStream proxies a video stream from an external URL
func (p *VideoProxy) ProxyStream(ctx context.Context, sourceURL string, w http.ResponseWriter, r *http.Request) error {
	// Validate URL is from allowed domain
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source url: %w", err)
	}

	if !p.isDomainAllowed(parsed.Host) {
		return fmt.Errorf("domain not allowed: %s", parsed.Host)
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers for upstream request
	req.Header.Set("User-Agent", p.config.UserAgent)
	if p.config.RefererOverride != "" {
		req.Header.Set("Referer", p.config.RefererOverride)
	}

	// Handle range requests for seeking
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// Make upstream request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Set CORS headers for frontend access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		// Client disconnected, not an error
		if strings.Contains(err.Error(), "broken pipe") ||
			strings.Contains(err.Error(), "connection reset") {
			return nil
		}
		return fmt.Errorf("stream copy: %w", err)
	}

	return nil
}

// GetStreamInfo fetches information about a video stream without downloading
func (p *VideoProxy) GetStreamInfo(ctx context.Context, sourceURL string) (*VideoStreamInfo, error) {
	_, err := url.Parse(sourceURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, sourceURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", p.config.UserAgent)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("head request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned status: %d", resp.StatusCode)
	}

	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)

	return &VideoStreamInfo{
		Source:      SourceExternal,
		URL:         sourceURL,
		ContentType: resp.Header.Get("Content-Type"),
		Size:        size,
	}, nil
}

// isDomainAllowed checks if a domain is in the allowed list
func (p *VideoProxy) isDomainAllowed(host string) bool {
	// Allow all domains if list is empty
	if len(p.config.AllowedDomains) == 0 {
		return true
	}

	host = strings.ToLower(host)
	for _, allowed := range p.config.AllowedDomains {
		allowed = strings.ToLower(allowed)
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// StreamResult represents a resolved video stream
type StreamResult struct {
	Info      *VideoStreamInfo
	StreamFn  func(w http.ResponseWriter, r *http.Request) error
	ExpiresAt time.Time
}

// VideoResolver resolves video sources to streamable URLs
type VideoResolver interface {
	// Resolve finds the best available stream for a video
	Resolve(ctx context.Context, animeID, episodeNum string) (*StreamResult, error)

	// ResolveExternal resolves an external video URL
	ResolveExternal(ctx context.Context, externalURL string) (*StreamResult, error)
}

// HLSProxyAllowedDomains contains domains allowed for HLS proxying
// Note: MegaCloud CDNs use dynamically generated domain names, so we allow
// common TLDs used by streaming CDNs. Rate limiting protects against abuse.
var HLSProxyAllowedDomains = []string{
	// Known streaming domains
	"megacloud.tv",
	"megacloud.blog",
	"megacloud.club",
	"rapid-cloud.co",
	"rapidcloud.live",
	"vidstream.pro",
	"vidstreamz.online",
	"mcloud.to",
	"mcloud2.to",
	"mgstatics.xyz",
	"netmagcdn.com", // MegaCloud HLS CDN
	"owocdn.top",    // AnimePahe/Kwik CDN
	"kwik.cx",       // AnimePahe CDN
}

// HLSProxyAllowedTLDs contains TLDs commonly used by streaming CDNs
// These CDNs use random domain names like sunburst93.live, haildrop77.pro
var HLSProxyAllowedTLDs = []string{
	".live",
	".pro",
	".xyz",
	".club",
	".tv",
	".to",
	".online",
	".wiki",
}

// ProxyWithReferer proxies a stream with a custom Referer header
// This is needed for HLS streams that require specific referer for authentication
func (p *VideoProxy) ProxyWithReferer(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) error {
	// Validate URL
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("invalid source url: %w", err)
	}

	// Check if domain is allowed for HLS proxy
	if !isHLSDomainAllowed(parsed.Host) {
		return fmt.Errorf("domain not allowed for HLS proxy: %s", parsed.Host)
	}

	// Create upstream request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers for upstream request
	req.Header.Set("User-Agent", p.config.UserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
		// Also set Origin which some CDNs check
		if parsedReferer, err := url.Parse(referer); err == nil {
			req.Header.Set("Origin", parsedReferer.Scheme+"://"+parsedReferer.Host)
		}
	}

	// Handle range requests for seeking
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	// Make upstream request
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()

	// Check if this is an M3U8 file that needs URL rewriting
	contentType := resp.Header.Get("Content-Type")
	isM3U8 := strings.Contains(contentType, "mpegurl") ||
		strings.Contains(contentType, "x-mpegurl") ||
		strings.HasSuffix(strings.ToLower(parsed.Path), ".m3u8")

	// Set CORS headers for frontend access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Accept-Ranges")

	if isM3U8 && resp.StatusCode == http.StatusOK {
		// Read and rewrite M3U8 content
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("read m3u8 body: %w", err)
		}

		// Rewrite URLs in the M3U8
		rewritten := rewriteM3U8URLs(string(body), sourceURL, referer)

		// Set headers (skip Content-Length as it changed)
		for key, values := range resp.Header {
			if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Length" {
				continue
			}
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}

		w.Header().Set("Content-Length", strconv.Itoa(len(rewritten)))
		w.WriteHeader(resp.StatusCode)
		w.Write([]byte(rewritten))
		return nil
	}

	// Copy response headers for non-M3U8 content
	for key, values := range resp.Header {
		// Skip hop-by-hop headers and Content-Type (we'll set it ourselves)
		if key == "Connection" || key == "Keep-Alive" || key == "Transfer-Encoding" || key == "Content-Type" {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Fix Content-Type for HLS segments - some CDNs return wrong types (like image/jpeg)
	// to obfuscate the content. We need to set the correct type for hls.js to work.
	correctContentType := getCorrectHLSContentType(parsed.Path, resp.Header.Get("Content-Type"))
	w.Header().Set("Content-Type", correctContentType)

	// Write status code
	w.WriteHeader(resp.StatusCode)

	// Stream the response with rate limiting (5MB/s max per stream)
	rateLimitedCopy(w, resp.Body, 5*1024*1024) // 5MB/s

	return nil
}

// rewriteM3U8URLs rewrites URLs in an M3U8 playlist to go through the proxy
// Also fixes unsupported audio codecs (mp4a.40.1 -> mp4a.40.2)
func rewriteM3U8URLs(content, baseURL, referer string) string {
	// Fix unsupported audio codec: AAC Main Profile (mp4a.40.1) -> AAC-LC (mp4a.40.2)
	// Chrome/Edge don't support mp4a.40.1 but the actual audio is usually AAC-LC compatible
	if strings.Contains(content, "mp4a.40.1") {
		content = strings.ReplaceAll(content, "mp4a.40.1", "mp4a.40.2")
	}

	// Parse base URL for resolving relative URLs
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return content
	}

	// Get the base path for relative URLs (directory of the M3U8 file)
	basePath := parsedBase.Scheme + "://" + parsedBase.Host
	if lastSlash := strings.LastIndex(parsedBase.Path, "/"); lastSlash > 0 {
		basePath += parsedBase.Path[:lastSlash+1]
	} else {
		basePath += "/"
	}

	var result strings.Builder
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments (except URI attributes)
		if trimmed == "" || (strings.HasPrefix(trimmed, "#") && !strings.Contains(trimmed, "URI=\"")) {
			result.WriteString(line)
			result.WriteString("\n")
			continue
		}

		// Handle lines with URI="..." attributes (like #EXT-X-KEY)
		if strings.Contains(trimmed, "URI=\"") {
			rewritten := rewriteURIAttribute(line, basePath, referer)
			result.WriteString(rewritten)
			result.WriteString("\n")
			continue
		}

		// This is a URL line (not a comment)
		if !strings.HasPrefix(trimmed, "#") {
			rewrittenURL := rewriteHLSURL(trimmed, basePath, referer)
			result.WriteString(rewrittenURL)
			result.WriteString("\n")
			continue
		}

		result.WriteString(line)
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}

// rewriteHLSURL rewrites a single HLS URL to go through the proxy
func rewriteHLSURL(urlStr, basePath, referer string) string {
	// Skip if already a proxy URL (check both encoded and decoded)
	if strings.Contains(urlStr, "/api/streaming/hls-proxy") ||
		strings.Contains(urlStr, "%2Fapi%2Fstreaming%2Fhls-proxy") ||
		strings.Contains(urlStr, "hls-proxy") {
		return urlStr
	}

	// Already absolute URL
	var absoluteURL string
	if strings.HasPrefix(urlStr, "http://") || strings.HasPrefix(urlStr, "https://") {
		absoluteURL = urlStr
	} else if strings.HasPrefix(urlStr, "/") {
		// Root-relative URL - but skip if it's our proxy path
		if strings.HasPrefix(urlStr, "/api/streaming/hls-proxy") {
			return urlStr
		}
		parsedBase, _ := url.Parse(basePath)
		absoluteURL = parsedBase.Scheme + "://" + parsedBase.Host + urlStr
	} else {
		// Relative URL
		absoluteURL = basePath + urlStr
	}

	// Build proxy URL
	proxyURL := "/api/streaming/hls-proxy?url=" + url.QueryEscape(absoluteURL)
	if referer != "" {
		proxyURL += "&referer=" + url.QueryEscape(referer)
	}

	return proxyURL
}

// rewriteURIAttribute rewrites URI="..." attributes in M3U8 tags
func rewriteURIAttribute(line, basePath, referer string) string {
	// Find URI="..." and rewrite the URL inside
	uriStart := strings.Index(line, "URI=\"")
	if uriStart == -1 {
		return line
	}
	uriStart += 5 // len("URI=\"")

	uriEnd := strings.Index(line[uriStart:], "\"")
	if uriEnd == -1 {
		return line
	}

	originalURI := line[uriStart : uriStart+uriEnd]
	rewrittenURI := rewriteHLSURL(originalURI, basePath, referer)

	return line[:uriStart] + rewrittenURI + line[uriStart+uriEnd:]
}

// getCorrectHLSContentType returns the correct Content-Type for HLS content
// Some CDNs return incorrect types (like image/jpeg) to obfuscate video content
func getCorrectHLSContentType(path, upstreamContentType string) string {
	pathLower := strings.ToLower(path)

	// M3U8 playlists
	if strings.HasSuffix(pathLower, ".m3u8") {
		return "application/vnd.apple.mpegurl"
	}

	// MPEG-TS segments
	if strings.HasSuffix(pathLower, ".ts") {
		return "video/mp2t"
	}

	// Common segment extensions used by various CDNs
	// Many CDNs use custom extensions or no extension for encrypted segments
	if strings.HasSuffix(pathLower, ".seg") ||
		strings.HasSuffix(pathLower, ".segment") ||
		strings.HasSuffix(pathLower, ".frag") {
		return "video/mp2t"
	}

	// If the upstream says it's an image but the content length suggests video
	// (video segments are typically > 100KB), treat it as video
	if strings.HasPrefix(upstreamContentType, "image/") {
		// Assume it's a video segment - CDNs often lie about Content-Type
		return "video/mp2t"
	}

	// Key files (AES-128 encryption keys are exactly 16 bytes)
	if strings.Contains(pathLower, "key") || strings.Contains(pathLower, "enc") {
		return "application/octet-stream"
	}

	// If upstream already has a valid media type, use it
	if strings.Contains(upstreamContentType, "mpegurl") ||
		strings.Contains(upstreamContentType, "mp2t") ||
		strings.Contains(upstreamContentType, "octet-stream") ||
		strings.Contains(upstreamContentType, "video/") ||
		strings.Contains(upstreamContentType, "audio/") {
		return upstreamContentType
	}

	// Default to octet-stream for unknown types
	return "application/octet-stream"
}

// isHLSDomainAllowed checks if a domain is allowed for HLS proxying
func isHLSDomainAllowed(host string) bool {
	host = strings.ToLower(host)

	// Strip port number if present
	if colonIdx := strings.LastIndex(host, ":"); colonIdx != -1 {
		host = host[:colonIdx]
	}

	// Check exact domain matches
	for _, allowed := range HLSProxyAllowedDomains {
		allowed = strings.ToLower(allowed)
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}

	// Check allowed TLDs (for dynamically generated CDN domains)
	for _, tld := range HLSProxyAllowedTLDs {
		if strings.HasSuffix(host, tld) {
			return true
		}
	}

	return false
}

// rateLimitedCopy copies data with a rate limit
func rateLimitedCopy(dst io.Writer, src io.Reader, bytesPerSecond int64) error {
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64
	startTime := time.Now()

	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = fmt.Errorf("invalid write result")
				}
			}
			written += int64(nw)

			// Calculate expected time based on bytes written
			expectedDuration := time.Duration(float64(written) / float64(bytesPerSecond) * float64(time.Second))
			elapsed := time.Since(startTime)

			// Sleep if we're ahead of the rate limit
			if expectedDuration > elapsed {
				time.Sleep(expectedDuration - elapsed)
			}

			if ew != nil {
				// Client disconnected, not an error
				if strings.Contains(ew.Error(), "broken pipe") ||
					strings.Contains(ew.Error(), "connection reset") {
					return nil
				}
				return ew
			}
			if nr != nw {
				return io.ErrShortWrite
			}
		}
		if er != nil {
			if er == io.EOF {
				return nil
			}
			return er
		}
	}
}
