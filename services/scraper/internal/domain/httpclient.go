package domain

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/hashicorp/go-retryablehttp"
	"golang.org/x/net/publicsuffix"
	"golang.org/x/time/rate"
)

// BaseHTTPClient is the single source of truth for upstream HTTP from any
// scraper provider. SCRAPER-FOUND-06: providers receive an injected
// *BaseHTTPClient and MUST NOT hand-roll their own http.Client / retry loop.
//
// Defaults:
//
//   - 10s per-attempt timeout (SCRAPER-NF-01)
//   - 4 retries with 1s→2s→4s→8s exponential backoff via retryablehttp
//     default policy (SCRAPER-NF-03)
//   - cookiejar.Jar scoped to public-suffix etld+1 (DDoS-Guard cookie support)
//   - Mozilla/5.0 Chrome 131 desktop User-Agent
//   - Accept-Language: en-US,en;q=0.9
//   - Accept-Encoding: gzip, deflate, br
//
// Override any default at construction with the With* options. All option
// setters are safe before the first request; mutating limiters after
// construction is not part of the public API.
type BaseHTTPClient struct {
	client   *retryablehttp.Client
	limiters map[string]*rate.Limiter
	mu       sync.Mutex
	jar      *cookiejar.Jar
	log      *logger.Logger
	headers  http.Header
}

// Option configures a BaseHTTPClient at construction time.
type Option func(*BaseHTTPClient)

// WithTimeout overrides the per-attempt HTTP timeout (default 10s).
func WithTimeout(d time.Duration) Option {
	return func(c *BaseHTTPClient) { c.client.HTTPClient.Timeout = d }
}

// WithRetryWaitMin overrides the minimum retry backoff wait (default 1s).
// Used by tests to compress the 1→2→4→8 unit sequence.
func WithRetryWaitMin(d time.Duration) Option {
	return func(c *BaseHTTPClient) { c.client.RetryWaitMin = d }
}

// WithRetryWaitMax overrides the maximum retry backoff wait (default 8s).
func WithRetryWaitMax(d time.Duration) Option {
	return func(c *BaseHTTPClient) { c.client.RetryWaitMax = d }
}

// WithMaxRetries overrides the maximum number of retries (default 4).
func WithMaxRetries(n int) Option {
	return func(c *BaseHTTPClient) { c.client.RetryMax = n }
}

// WithPerHostRPS registers a per-host rate.Limiter. Subsequent calls to the
// same host wait on the limiter before being dispatched.
func WithPerHostRPS(host string, rps float64, burst int) Option {
	return func(c *BaseHTTPClient) {
		c.limiters[host] = rate.NewLimiter(rate.Limit(rps), burst)
	}
}

// WithHeaders replaces the baseline outgoing headers map. Useful for providers
// that need a different UA (e.g. mobile / app-specific).
func WithHeaders(h http.Header) Option {
	return func(c *BaseHTTPClient) { c.headers = h.Clone() }
}

// NewBaseHTTPClient builds a configured *BaseHTTPClient.
func NewBaseHTTPClient(log *logger.Logger, opts ...Option) *BaseHTTPClient {
	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	rc := retryablehttp.NewClient()
	rc.RetryWaitMin = 1 * time.Second
	rc.RetryWaitMax = 8 * time.Second
	rc.RetryMax = 4
	rc.HTTPClient.Timeout = 10 * time.Second
	rc.HTTPClient.Jar = jar
	rc.CheckRetry = retryablehttp.DefaultRetryPolicy
	rc.Backoff = retryablehttp.DefaultBackoff
	rc.Logger = nil // quiet — structured logs come from our middleware downstream

	baseline := http.Header{}
	baseline.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	baseline.Set("Accept-Language", "en-US,en;q=0.9")
	baseline.Set("Accept", "*/*")
	baseline.Set("Accept-Encoding", "gzip, deflate, br")

	c := &BaseHTTPClient{
		client:   rc,
		limiters: make(map[string]*rate.Limiter),
		jar:      jar,
		log:      log,
		headers:  baseline,
	}

	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Timeout returns the per-attempt HTTP timeout currently configured. Used by
// tests and observability to verify the SCRAPER-NF-01 default.
func (c *BaseHTTPClient) Timeout() time.Duration {
	return c.client.HTTPClient.Timeout
}

// applyBaselineHeaders writes baseline headers into req.Header WITHOUT
// overwriting any header the caller already set on the request.
func (c *BaseHTTPClient) applyBaselineHeaders(h http.Header) {
	for k, v := range c.headers {
		if h.Get(k) == "" && len(v) > 0 {
			h.Set(k, v[0])
		}
	}
}

// waitForLimiter blocks until the per-host limiter (if any) issues a token.
// Returns the context error if the wait is cancelled.
func (c *BaseHTTPClient) waitForLimiter(ctx context.Context, host string) error {
	c.mu.Lock()
	lim := c.limiters[host]
	c.mu.Unlock()
	if lim == nil {
		return nil
	}
	return lim.Wait(ctx)
}

// Get issues a GET request to urlStr through the retry + rate-limit + cookie
// pipeline. Caller-supplied ctx propagates through all retry attempts.
func (c *BaseHTTPClient) Get(ctx context.Context, urlStr string) (*http.Response, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	if err := c.waitForLimiter(ctx, u.Host); err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	c.applyBaselineHeaders(req.Header)
	return c.client.Do(req)
}

// Do issues an arbitrary HTTP request through the retry + rate-limit + cookie
// pipeline. Use this when you need POST, custom body, or custom headers
// (caller-set headers take precedence over baseline).
func (c *BaseHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, http.ErrNoLocation
	}
	if err := c.waitForLimiter(ctx, req.URL.Host); err != nil {
		return nil, err
	}

	rreq, err := retryablehttp.FromRequest(req.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	c.applyBaselineHeaders(rreq.Header)
	return c.client.Do(rreq)
}
