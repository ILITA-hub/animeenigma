package handler

import (
	"io"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/service"
)

type ProxyHandler struct {
	proxyService *service.ProxyService
	log          *logger.Logger
}

func NewProxyHandler(proxyService *service.ProxyService, log *logger.Logger) *ProxyHandler {
	return &ProxyHandler{
		proxyService: proxyService,
		log:          log,
	}
}

// ProxyToAuth proxies requests to auth service
func (h *ProxyHandler) ProxyToAuth(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "auth")
}

// ProxyToCatalog proxies requests to catalog service
func (h *ProxyHandler) ProxyToCatalog(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "catalog")
}

// ProxyToPlayer proxies requests to player service
func (h *ProxyHandler) ProxyToPlayer(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "player")
}

// ProxyToRooms proxies requests to rooms service
func (h *ProxyHandler) ProxyToRooms(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "rooms")
}

// ProxyToStreaming proxies requests to streaming service
func (h *ProxyHandler) ProxyToStreaming(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "streaming")
}

func (h *ProxyHandler) proxy(w http.ResponseWriter, r *http.Request, service string) {
	resp, err := h.proxyService.Forward(r, service)
	if err != nil {
		h.log.Errorw("proxy failed", "service", service, "error", err)
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, _ = io.Copy(w, resp.Body)
}

// GetOpenAPISpec aggregates OpenAPI specs from all services
func (h *ProxyHandler) GetOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	spec := map[string]interface{}{
		"openapi": "3.0.0",
		"info": map[string]interface{}{
			"title":       "AnimeEnigma API",
			"description": "API Gateway for AnimeEnigma Platform",
			"version":     "1.0.0",
		},
		"servers": []map[string]string{
			{"url": "/api/v1"},
		},
		"paths": map[string]interface{}{
			"/auth/*":      map[string]string{"description": "Authentication endpoints"},
			"/catalog/*":   map[string]string{"description": "Anime catalog endpoints"},
			"/player/*":    map[string]string{"description": "Player and watch history endpoints"},
			"/rooms/*":     map[string]string{"description": "Game rooms and leaderboard endpoints"},
			"/streaming/*": map[string]string{"description": "Video streaming endpoints"},
		},
	}

	httputil.OK(w, spec)
}
