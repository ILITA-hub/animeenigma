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
//   - Accept-Encoding: (unset — Go transport auto-injects gzip and decodes)
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
	// provider is the stream-provider this client serves (set via WithProvider).
	// Each BaseHTTPClient is single-provider (scraper main builds one per
	// provider), so the tag is baked at construction (RESEARCH A3) and threaded
	// onto every outbound request's context so the recording transport can pivot
	// streaming egress by provider+host (D-02/D-09). Empty → no provider tag.
	provider string
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

// WithTransport overrides the underlying http.RoundTripper used by the
// retryablehttp client, preserving the BaseHTTPClient's retry + rate-limit +
// cookie-jar pipeline.
//
// PRODUCTION (Phase 02, AR-EGRESS-03): this is the egress-recording seam.
// scraper main wraps the recording transport (tracing.WrapTransport /
// tracing.WrapRecording) and injects it here at each per-provider construction
// site, so every upstream request a provider makes emits exactly one egress
// effect row (host/status/bytes/duration), pivotable by provider+host via the
// WithProvider tag (D-02/D-09).
//
// TESTS: also used to route all outgoing traffic to an httptest.Server. The
// Phase 18 orchestrator failover integration test
// (services/scraper/internal/service/orchestrator_phase18_test.go) exercises a
// REAL gogoanime.Provider against an offline httptest.Server through this seam
// without hand-rolling a separate http.Client (which would defeat the
// SCRAPER-FOUND-06 "no hand-rolled clients" invariant) or exposing the
// internal client field (breaks encapsulation). Phase 19 (AnimeKai) reuses it.
func WithTransport(rt http.RoundTripper) Option {
	return func(c *BaseHTTPClient) { c.client.HTTPClient.Transport = rt }
}

// WithProvider bakes the stream-provider tag into this client (RESEARCH A3:
// each BaseHTTPClient is single-provider). Every outbound request issued via
// Get/Do has its context tagged with the provider so the recording transport
// records `target = provider + host` for streaming egress (D-02/D-09). General
// (non-streaming) egress leaves this empty and carries no provider (D-01).
func WithProvider(name string) Option {
	return func(c *BaseHTTPClient) { c.provider = name }
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
	// Do NOT set Accept-Encoding. Go's net/http transport auto-injects
	// "Accept-Encoding: gzip" and transparently decodes responses. Setting
	// our own value (e.g. "gzip, deflate, br") DISABLES the auto-decoder,
	// leaving callers to read raw compressed bytes — which broke gogoanime's
	// HTML scraper against anitaku.to's Cloudflare edge (it serves Brotli).
	// We don't ship a Brotli decoder, and gzip alone covers all upstreams.

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

// Jar returns the cookie jar attached to this client. Providers use it to
// inspect cookies set by the upstream (e.g. AnimePahe's DDoS-Guard bypass
// cookie — see services/scraper/internal/providers/animepahe).
// Returns the live jar instance; callers MUST treat it as read-mostly
// (the http stack writes to it on every response).
func (c *BaseHTTPClient) Jar() http.CookieJar { return c.jar }

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
	ctx = ProviderContext(ctx, c.provider)
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
	ctx = ProviderContext(ctx, c.provider)
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
