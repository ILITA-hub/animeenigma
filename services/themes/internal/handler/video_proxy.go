package handler

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/go-chi/chi/v5"
)

var validBasename = regexp.MustCompile(`^[a-zA-Z0-9_\-]+\.(webm|ogg)$`)

type VideoProxyHandler struct {
	httpClient *http.Client
	log        *logger.Logger
}

func NewVideoProxyHandler(log *logger.Logger) *VideoProxyHandler {
	return &VideoProxyHandler{
		httpClient: &http.Client{
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
				TLSHandshakeTimeout:  10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		log: log,
	}
}

// ProxyVideo handles GET /api/themes/video/{basename}
func (h *VideoProxyHandler) ProxyVideo(w http.ResponseWriter, r *http.Request) {
	h.proxyMedia(w, r, "v.animethemes.moe")
}

// ProxyAudio handles GET /api/themes/audio/{basename}
func (h *VideoProxyHandler) ProxyAudio(w http.ResponseWriter, r *http.Request) {
	h.proxyMedia(w, r, "a.animethemes.moe")
}

func (h *VideoProxyHandler) proxyMedia(w http.ResponseWriter, r *http.Request, host string) {
	basename := chi.URLParam(r, "basename")
	if basename == "" || !validBasename.MatchString(basename) {
		httputil.BadRequest(w, "invalid basename")
		return
	}

	targetURL := fmt.Sprintf("https://%s/%s", host, basename)

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		httputil.Error(w, err)
		return
	}

	// Forward Range header for seeking support
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		h.log.Errorw("failed to proxy media", "url", targetURL, "error", err)
		httputil.BadRequest(w, "failed to fetch media")
		return
	}
	defer resp.Body.Close()

	// Copy relevant response headers
	for _, header := range []string{
		"Content-Type", "Content-Length", "Accept-Ranges", "Content-Range",
	} {
		if val := resp.Header.Get(header); val != "" {
			w.Header().Set(header, val)
		}
	}

	// Set content type if not provided
	if w.Header().Get("Content-Type") == "" {
		if strings.HasSuffix(basename, ".webm") {
			w.Header().Set("Content-Type", "video/webm")
		} else if strings.HasSuffix(basename, ".ogg") {
			w.Header().Set("Content-Type", "audio/ogg")
		}
	}

	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.log.Debugw("proxy stream copy interrupted", "url", targetURL, "error", err)
	}
}
