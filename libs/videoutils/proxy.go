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
	parsed, err := url.Parse(sourceURL)
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
