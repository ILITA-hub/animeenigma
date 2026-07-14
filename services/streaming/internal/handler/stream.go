package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	"github.com/ILITA-hub/animeenigma/libs/videoutils"
	"github.com/ILITA-hub/animeenigma/services/streaming/internal/service"
	"github.com/go-chi/chi/v5"
	"golang.org/x/sync/semaphore"
)

// HLS proxy configuration
const (
	maxHLSProxyConnections = 50 // Maximum concurrent HLS proxy streams
)

// Global state for HLS proxy connection limiting
var (
	hlsProxySemaphore    = semaphore.NewWeighted(maxHLSProxyConnections)
	hlsActiveConnections atomic.Int32
)

type StreamHandler struct {
	streamingService *service.StreamingService
	videoProxy       *videoutils.VideoProxy
	// ownStorages recognizes upstream URLs served from ANY of our storage
	// backends (local MinIO + optional external S3) so self-hosted `ae`
	// playback is labeled distinctly in metrics. May be nil (no storages).
	ownStorages *videoutils.MultiStorage
	hlsSessions *service.HLSSessions // egress aggregator (may be nil — no-op)
	log         *logger.Logger
}

func NewStreamHandler(streamingService *service.StreamingService, log *logger.Logger) *StreamHandler {
	return NewStreamHandlerWithSessions(streamingService, nil, nil, log)
}

// parseSolodcdnEdges splits STREAMING_SOLODCDN_EDGES ("p12,p13,p14") into edge
// labels, trimming blanks. An empty/unset value yields nil so videoutils applies
// its built-in default pool (defaultSolodcdnEdges).
func parseSolodcdnEdges(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var edges []string
	for _, e := range strings.Split(raw, ",") {
		if e = strings.TrimSpace(e); e != "" {
			edges = append(edges, e)
		}
	}
	return edges
}

// storageHost strips the optional port from a storage endpoint
// ("minio:9000" -> "minio"; a bare "minio" or "s3.firstvds.ru" is returned
// unchanged). Used for both the local MinIO and optional external S3 hosts.
func storageHost(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(endpoint); err == nil {
		return host
	}
	return endpoint
}

// NewStreamHandlerWithSessions wires the HLS egress aggregator (AR-EGRESS-04).
// A nil aggregator degrades to no egress accounting (proxy behavior unchanged).
//
// s3Storage is the OPTIONAL external S3-compatible backend library episodes
// may also live on, alongside local MinIO (streamingService's Storage()).
// A nil s3Storage degrades to MinIO-only presigning, unchanged from before
// this parameter existed.
func NewStreamHandlerWithSessions(streamingService *service.StreamingService, s3Storage *videoutils.Storage, hlsSessions *service.HLSSessions, log *logger.Logger) *StreamHandler {
	// Create video proxy with default config for HLS proxying. No
	// AllowedDomains override: the HLS trust gate is `preauth OR first-party
	// OR provenance-signed` (the static allowlist was retired 2026-07-14);
	// AllowedDomains only gates the separate legacy token path, which this
	// handler's proxy never routes through.
	proxyCfg := videoutils.DefaultProxyConfig()

	var minioStorage *videoutils.Storage
	if streamingService != nil {
		minioStorage = streamingService.Storage()
	}
	ownStorages, firstPartyHosts := storageProxyWiring(minioStorage, s3Storage)
	proxyCfg.FirstPartyHosts = firstPartyHosts
	// Self-hosted library (`ae` provider) HLS lives in a PRIVATE bucket on
	// EITHER local MinIO or an external S3-compatible host — catalog signs
	// stream URLs for both identically, so the proxy routes each upstream
	// GET through MultiStorage, which presigns against whichever backend
	// actually owns the URL's host. The proxy gates entry on our HMAC sig /
	// provenance tokens first, then presigns the actual storage read here
	// so neither bucket ever needs to be public. Only URLs whose host is one
	// of the wrapped storages' endpoints are rewritten; every external-CDN
	// fetch is left untouched.
	if len(ownStorages.Hosts()) > 0 {
		proxyCfg.UpstreamSigner = ownStorages.PresignURL
	}

	// Layer A — solodcdn edge failover (extends AUTO-562 playback self-healing).
	// The sibling-edge pool comes from STREAMING_SOLODCDN_EDGES (default
	// p12,p13,p14); the shared proxy's edge telemetry is folded into Prometheus
	// here, keeping libs/videoutils Prometheus-free (the auto-registration trap:
	// single-emitter metrics live in the emitting service, fed via callbacks).
	proxyCfg.SolodcdnEdges = parseSolodcdnEdges(os.Getenv("STREAMING_SOLODCDN_EDGES"))
	proxyCfg.OnEdgeRotation = func(from, to, outcome string) {
		metrics.ProxyEdgeRotations.WithLabelValues(from, to, outcome).Inc()
	}
	proxyCfg.OnEdgeAttempt = func(edge, outcome string, ms int64) {
		metrics.ProxyEdgeAttemptSeconds.WithLabelValues(edge, outcome).Observe(float64(ms) / 1000)
	}
	proxyCfg.OnEdgeServed = func(edge string) {
		metrics.ProxyEdgeSelected.WithLabelValues(edge).Inc()
	}

	return &StreamHandler{
		streamingService: streamingService,
		videoProxy:       videoutils.NewVideoProxy(proxyCfg),
		ownStorages:      ownStorages,
		hlsSessions:      hlsSessions,
		log:              log,
	}
}

// storageProxyWiring builds the MultiStorage signer over both storage
// backends plus the HLS proxy's first-party host exemptions.
//
// FirstPartyHosts exists ONLY to exempt Docker-private hosts (stealth-scraper,
// minio) from the SSRF dial guard's private-IP + redirect checks (#64/#65) —
// they resolve to private IPs the proxy must still reach. The external S3
// host is deliberately NOT listed: it resolves public, so the guarded dialer
// already allows it with no exemption, and listing it would only strip
// DNS-rebind protection for that host. Presigning for it still works — that's
// the MultiStorage's job, independent of the dial-guard exemptions.
func storageProxyWiring(minioStorage, s3Storage *videoutils.Storage) (*videoutils.MultiStorage, []string) {
	firstParty := []string{"stealth-scraper"}
	if minioStorage != nil {
		if host := storageHost(minioStorage.Endpoint()); host != "" {
			firstParty = append(firstParty, host)
		}
	}
	return videoutils.NewMultiStorage(minioStorage, s3Storage), firstParty
}

// ProxyStream handles proxying external video streams
func (h *StreamHandler) ProxyStream(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		httputil.BadRequest(w, "token is required")
		return
	}

	token, err := h.streamingService.ValidateStreamToken(tokenStr)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// Wrap writer to count bytes transferred
	cw := &metrics.CountingResponseWriter{
		ResponseWriter: w,
		Counter:        metrics.ProxyBytesTransferredTotal.WithLabelValues("stream"),
	}

	switch token.SourceType {
	case videoutils.SourceExternal:
		if err := h.streamingService.ProxyExternalStream(r.Context(), token, cw, r); err != nil {
			h.log.Errorw("failed to proxy stream", "error", err, "video_id", token.VideoID, "user_id", token.UserID)
			// Don't send error response if we've already started writing
		}

	case videoutils.SourceMinio:
		if err := h.streamingService.StreamFromStorage(r.Context(), token, cw, r); err != nil {
			h.log.Errorw("failed to stream from storage", "error", err, "video_id", token.VideoID, "user_id", token.UserID)
		}

	default:
		httputil.Error(w, apperrors.InvalidInput("unsupported source type"))
	}
}

// DirectStream handles direct streaming from MinIO storage
func (h *StreamHandler) DirectStream(w http.ResponseWriter, r *http.Request) {
	tokenStr := r.URL.Query().Get("token")
	if tokenStr == "" {
		httputil.BadRequest(w, "token is required")
		return
	}

	token, err := h.streamingService.ValidateStreamToken(tokenStr)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	if token.SourceType != videoutils.SourceMinio {
		httputil.Error(w, apperrors.InvalidInput("token is not for direct streaming"))
		return
	}

	cw := &metrics.CountingResponseWriter{
		ResponseWriter: w,
		Counter:        metrics.ProxyBytesTransferredTotal.WithLabelValues("storage"),
	}
	if err := h.streamingService.StreamFromStorage(r.Context(), token, cw, r); err != nil {
		h.log.Errorw("failed to stream from storage", "error", err, "video_id", token.VideoID, "user_id", token.UserID)
	}
}

// GenerateToken generates a stream token (internal API)
func (h *StreamHandler) GenerateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		VideoID    string `json:"video_id"`
		SourceType string `json:"source_type"`
		SourceURL  string `json:"source_url,omitempty"`
		StorageKey string `json:"storage_key,omitempty"`
		UserID     string `json:"user_id,omitempty"`
	}

	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}

	var sourceType videoutils.VideoSource
	switch req.SourceType {
	case "minio":
		sourceType = videoutils.SourceMinio
	case "external":
		sourceType = videoutils.SourceExternal
	default:
		httputil.BadRequest(w, "invalid source_type")
		return
	}

	token, expiresAt, err := h.streamingService.GenerateStreamToken(
		req.VideoID, sourceType, req.SourceURL, req.StorageKey, req.UserID)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"token":      token,
		"expires_at": expiresAt,
	})
}

// HLSProxy proxies HLS streams with proper Referer headers
// This endpoint allows the frontend to play HLS streams that require Referer authentication
func (h *StreamHandler) HLSProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.WriteHeader(http.StatusOK)
		return
	}

	sourceURL := r.URL.Query().Get("url")
	referer := r.URL.Query().Get("referer")

	if sourceURL == "" {
		httputil.BadRequest(w, "url parameter is required")
		return
	}

	h.serveProxy(w, r, sourceURL, referer, false)
}

// MaskedProxy serves the Track A opaque path-token form
// /api/v1/m/<token>/<leaf> (public: /api/streaming/m/...). The sealed AES-GCM
// token carries {url, referer, exp, type} and IS the authorization — no
// allowlist or exp/sig query pair on this path (spec 2026-07-10 §3).
func (h *StreamHandler) MaskedProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight (same policy as HLSProxy)
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.WriteHeader(http.StatusOK)
		return
	}

	payload, err := videoutils.DecodeStreamToken(chi.URLParam(r, "token"), time.Now())
	if err != nil {
		h.log.Warnw("masked proxy: rejected token", "error", err)
		http.Error(w, "invalid stream token", http.StatusForbidden)
		return
	}

	// Metrics/log hygiene: the raw path embeds a high-cardinality token and
	// libs/metrics normalizePath labels r.URL.Path AFTER the handler runs —
	// collapse it to a stable value.
	r.URL.Path = "/api/v1/m"

	// The mp4/webm content-type override rides inside the token on this path;
	// surface it as the query param the shared pipeline already reads
	// (proxy.go's `type` switch).
	if payload.Type != "" {
		q := r.URL.Query()
		q.Set("type", payload.Type)
		r.URL.RawQuery = q.Encode()
	}

	h.serveProxy(w, r, payload.URL, payload.Referer, true)
}

// serveProxy is the shared body of HLSProxy and MaskedProxy: connection
// semaphore, metrics, byte counting, egress folding, and error mapping around
// one videoProxy call. preauth=true selects ProxyPreauthCounted — the caller
// already authorized sourceURL by opening a sealed stream token, so the
// allowlist/provenance gate is skipped.
func (h *StreamHandler) serveProxy(w http.ResponseWriter, r *http.Request, sourceURL, referer string, preauth bool) {
	// Try to acquire semaphore (limit concurrent connections)
	if !hlsProxySemaphore.TryAcquire(1) {
		h.log.Warnw("HLS proxy at capacity", "active_connections", hlsActiveConnections.Load())
		w.Header().Set("Retry-After", "30")
		httputil.Error(w, apperrors.ServiceUnavailable("server busy, try again later"))
		return
	}

	// Label self-hosted (`ae` provider) playback distinctly from external-CDN
	// traffic: a request whose upstream host is ANY of our own storage
	// backends (local MinIO or external S3) is "hls_minio", everything else
	// stays "hls". This is the self-hosted playback load signal used by the
	// Playback dashboard's AnimeEnigma row.
	proxyType := "hls"
	if h.ownStorages != nil && h.ownStorages.IsOwnHost(sourceURL) {
		proxyType = "hls_minio"
	}

	// Track active connections
	hlsActiveConnections.Add(1)
	metrics.ProxyActiveConnections.Inc()
	metrics.ProxyRequestsTotal.WithLabelValues(proxyType).Inc()
	defer func() {
		hlsProxySemaphore.Release(1)
		hlsActiveConnections.Add(-1)
		metrics.ProxyActiveConnections.Dec()
	}()

	h.log.Debugw("HLS proxy request",
		"url", sourceURL,
		"referer", referer,
		"active_connections", hlsActiveConnections.Load(),
	)

	// Wrap writer to count bytes transferred (same hls/hls_minio split as the
	// request counter above, so on-prem egress load is measurable on its own).
	cw := &metrics.CountingResponseWriter{
		ResponseWriter: w,
		Counter:        metrics.ProxyBytesTransferredTotal.WithLabelValues(proxyType),
	}

	// Proxy the request with the provided referer, capturing per-call byte
	// counts so a segment GET bearing a ?sess= token can be folded into one
	// aggregated egress row (AR-EGRESS-04 / AR-EGRESS-05). The ?sess= token was
	// injected into this segment URL when its parent manifest was rewritten;
	// the upstream host is derived from the proxied URL.
	proxyCall := h.videoProxy.ProxyWithRefererCounted
	if preauth {
		proxyCall = h.videoProxy.ProxyPreauthCounted
	}
	bytesIn, bytesOut, err := proxyCall(r.Context(), sourceURL, referer, cw, r)
	if err == nil {
		h.observeEgress(r, sourceURL, bytesIn, bytesOut)
	}
	if err != nil {
		// Check if this is an upstream CDN error (403, 5xx, Cloudflare block, etc.)
		var upstreamErr *videoutils.UpstreamError
		if errors.As(err, &upstreamErr) {
			metrics.ProxyUpstreamErrors.WithLabelValues(
				strconv.Itoa(upstreamErr.StatusCode),
				upstreamErr.Domain,
			).Inc()
			h.log.Warnw("upstream CDN error",
				"status", upstreamErr.StatusCode,
				"domain", upstreamErr.Domain,
				"html_response", upstreamErr.HTML,
				"url", sourceURL,
			)
			// Return a clean error so HLS.js stops retrying
			http.Error(w, "upstream stream unavailable", http.StatusBadGateway)
			return
		}

		// Phase 25 W-INT-03 / SCRAPER-HEAL-24: allowlist gap surfaces as 502
		// instead of the prior silent 200 / Content-Length:0. The FE error
		// boundary, Prometheus dashboards, and the BLK-INT-01 self-heal
		// canary all rely on this becoming an observable failure.
		var domainErr *videoutils.DomainNotAllowedError
		if errors.As(err, &domainErr) {
			metrics.ProxyUpstreamErrors.WithLabelValues("403", domainErr.Domain).Inc()
			h.log.Warnw("HLS proxy rejected non-allowlisted domain",
				"domain", domainErr.Domain,
				"url", sourceURL,
				"referer", referer,
			)
			http.Error(w, "domain not allowed for HLS proxy", http.StatusBadGateway)
			return
		}

		// Client-disconnect (navigation, seek, tab close) cancels r.Context(),
		// surfacing here as context.Canceled. That is not a proxy failure: the
		// peer is simply gone. Logging it at ERROR and returning 502 inflates the
		// 5xx rate and fires false High Error Rate alerts (AUTO-292). Silently
		// drop it — there is no client left to receive a response anyway.
		if errors.Is(err, context.Canceled) {
			return
		}

		h.log.Errorw("failed to proxy HLS stream",
			"error", err,
			"url", sourceURL,
			"referer", referer,
		)
		// Belt-and-braces: if we reach here, headers may or may not have
		// been written. Try a final 502 — http.Error is a no-op when
		// headers are already committed, so this is safe in both states.
		http.Error(w, "internal proxy error", http.StatusBadGateway)
	}
}

// observeEgress folds one proxied HLS request into the per-session egress
// aggregator. Only requests bearing a ?sess= token participate — those are the
// segment/child GETs whose parent manifest was rewritten through the proxy
// (rewriteHLSURL minted the token). The manifest fetch itself carries no
// ?sess= and is skipped here (its bytes are small and uncorrelated).
//
// Attribution (provider/operation/user_id) is captured on first touch via
// Mint, which backfills empty fields without resetting the byte tally. Browser
// segment GETs usually have no baggage, so these are typically empty — the
// manifest-fetch baggage is the intended source per the plan, but the in-
// manifest token is not visible to the handler, so first-touch capture from
// the segment request context is the available signal. Provider is derived
// from the upstream host.
func (h *StreamHandler) observeEgress(r *http.Request, sourceURL string, bytesIn, bytesOut uint64) {
	if h.hlsSessions == nil {
		return
	}
	sess := r.URL.Query().Get("sess")
	if sess == "" {
		return
	}
	host := ""
	if u, err := url.Parse(sourceURL); err == nil {
		host = u.Host
	}

	// Capture attribution on first touch (idempotent backfill).
	origin, operation := tracing.ReadBaggage(r.Context())
	_ = origin
	provider := tracing.ProviderFromContext(r.Context())
	userID := tracing.UserIDFromContext(r.Context())
	h.hlsSessions.Mint(sess, host, provider, operation, userID)

	h.hlsSessions.Observe(sess, host, bytesIn, bytesOut)
}

// GetProxyStatus returns the current HLS proxy load status
func (h *StreamHandler) GetProxyStatus(w http.ResponseWriter, r *http.Request) {
	active := hlsActiveConnections.Load()
	loadPercent := int(float64(active) / float64(maxHLSProxyConnections) * 100)

	httputil.OK(w, map[string]interface{}{
		"active_connections": active,
		"max_connections":    maxHLSProxyConnections,
		"load_percent":       loadPercent,
		"available":          active < maxHLSProxyConnections,
	})
}
