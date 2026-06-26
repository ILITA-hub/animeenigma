package handler

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// ExternalAPIHandler proxies /worker/* requests to the upscaler service.
// It is the gateway-side edge for ext.animeenigma.org — the only internet-
// facing surface GPU workers talk to.
//
// Three sub-paths have different streaming requirements:
//
//	/worker/enroll  — small JSON; normal proxy (no special buffering).
//	/worker/ws      — WebSocket upgrade; handled by a dedicated WS reverse
//	                  proxy (ExternalWSProxy, same pattern as ws_proxy.go).
//	/worker/segments/* — large binary segment bytes; MUST stream without
//	                  full-body buffering (FlushInterval=-1 director).
//
// Cookie stripping: copyForwardHeaders in service/proxy.go already strips
// Cookie for the Forward path, but this handler uses httputil.ReverseProxy
// directly (needed for WS + streaming), so it strips Cookie in the Director.
type ExternalAPIHandler struct {
	// streamProxy is used for ALL /worker/* non-WS routes. FlushInterval=-1
	// flushes immediately after every write — mandatory for segment bytes so
	// they don't accumulate in the ResponseWriter buffer and cause OOM on the
	// gateway when a large (hundreds-of-MB) segment arrives.
	streamProxy *httputil.ReverseProxy
	log         *logger.Logger
}

// NewExternalAPIHandler builds an ExternalAPIHandler that proxies to
// targetBaseURL (e.g. "http://upscaler:8096"). Returns an error if the URL is
// unparseable — caller wires this at router-build time so misconfiguration
// fails fast at startup.
func NewExternalAPIHandler(targetBaseURL string, log *logger.Logger) (*ExternalAPIHandler, error) {
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		return nil, fmt.Errorf("external api handler: parse target %q: %w", targetBaseURL, err)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("external api handler: target %q has no host", targetBaseURL)
	}

	rp := httputil.NewSingleHostReverseProxy(target)

	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		// Set Host to target so backends don't see the edge domain.
		req.Host = target.Host
		// Strip Cookie — workers authenticate via X-API-Key + session tokens,
		// not browser cookies. Forwarding cookies would leak any SPA session
		// cookies that somehow reach this path.
		req.Header.Del("Cookie")
		// Strip Set-Cookie from the inbound request (defence-in-depth; Set-Cookie
		// is a response header but some clients forward it).
		req.Header.Del("Set-Cookie")
		// Strip X-Gateway-Internal on this INTERNET-FACING edge (defence-in-depth;
		// mirrors the admin proxy's strip-then-set). The /worker/* surface is the
		// only public-facing path to the upscaler; an external client must never be
		// able to spoof the gateway-internal admin marker so a converging route
		// (admin gate at upscaler:8096) can never be tricked into trusting it.
		req.Header.Del("X-Gateway-Internal")
	}

	// FlushInterval=-1 flushes after every write. This is MANDATORY for
	// /worker/segments/* — segment files can be hundreds of MB; without
	// immediate flush the ResponseWriter buffers the whole body in memory
	// (gateway OOM, CD-12 concern). The same setting is safe for enroll/ws.
	rp.FlushInterval = -1

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Errorw("worker proxy error",
			"path", r.URL.Path,
			"target", target.String(),
			"error", err,
		)
		// Generic body — no internal topology detail (no host, no bucket, no
		// infohash) visible to the GPU operator.
		w.Header().Set("Content-Type", "application/json")
		http.Error(w, `{"error":"bad_gateway"}`, http.StatusBadGateway)
	}

	// ModifyResponse strips sensitive response headers before they reach
	// the GPU worker. In particular, any Set-Cookie from the upscaler
	// must not reach an untrusted external client.
	rp.ModifyResponse = func(resp *http.Response) error {
		resp.Header.Del("Set-Cookie")
		return nil
	}

	return &ExternalAPIHandler{
		streamProxy: rp,
		log:         log,
	}, nil
}

// ProxyWorker handles all /worker/* non-WS paths. The same proxy is used for
// both /worker/enroll (small JSON) and /worker/segments/* (large binary) —
// FlushInterval=-1 ensures segments stream through without buffering.
func (h *ExternalAPIHandler) ProxyWorker(w http.ResponseWriter, r *http.Request) {
	h.streamProxy.ServeHTTP(w, r)
}

// NewWorkerWSProxy returns an http.HandlerFunc that reverse-proxies WebSocket
// Upgrade requests for /worker/ws to the upscaler service. Mirrors the
// pattern in transport/ws_proxy.go exactly: preserves Upgrade/Connection
// hop-by-hop headers (which ProxyService.Forward strips), strips Cookie,
// uses FlushInterval=-1 for immediate WS frame delivery.
func NewWorkerWSProxy(targetBaseURL string, log *logger.Logger) (http.HandlerFunc, error) {
	target, err := url.Parse(targetBaseURL)
	if err != nil {
		return nil, fmt.Errorf("worker ws proxy: parse target %q: %w", targetBaseURL, err)
	}
	if target.Host == "" {
		return nil, fmt.Errorf("worker ws proxy: target %q has no host", targetBaseURL)
	}

	rp := httputil.NewSingleHostReverseProxy(target)

	origDirector := rp.Director
	rp.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = target.Host
		req.Header.Del("Cookie")
		req.Header.Del("Set-Cookie")
		// Strip X-Gateway-Internal on the internet-facing WS edge too (defence-in-
		// depth) — an external worker must never spoof the gateway-internal marker.
		req.Header.Del("X-Gateway-Internal")
	}

	// FlushInterval=-1: mandatory for streaming WS frames (same as watch-together
	// ws_proxy.go — without this, frames buffer and the client may never see them).
	rp.FlushInterval = -1

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		// Distinguish WS upgrade failures from normal proxy errors for ops visibility.
		isUpgrade := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
		log.Errorw("worker ws proxy error",
			"path", r.URL.Path,
			"target", target.String(),
			"upgrade", isUpgrade,
			"error", err,
		)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}

	return rp.ServeHTTP, nil
}
