package handler

import (
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gateway/internal/config"
)

type StatusHandler struct {
	services []serviceCheck
	client   *http.Client
	log      *logger.Logger
}

type serviceCheck struct {
	Name     string
	URL      string // HTTP health URL, empty for TCP checks
	TCPAddr  string // TCP address for non-HTTP services
	Category string // "app" or "infra"
}

// ServiceStatus represents the health of a single service.
type ServiceStatus struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	ResponseTimeMs int64  `json:"response_time_ms"`
	Category       string `json:"category"`
	Error          string `json:"error,omitempty"`
}

// StatusResponse is the aggregated health response.
type StatusResponse struct {
	Services  []ServiceStatus `json:"services"`
	Overall   string          `json:"overall"`
	CheckedAt time.Time       `json:"checked_at"`
}

func NewStatusHandler(urls config.ServiceURLs, log *logger.Logger) *StatusHandler {
	services := []serviceCheck{
		// Application services (HTTP /health)
		{Name: "gateway", URL: "", Category: "app"},
		{Name: "auth", URL: urls.AuthService + "/health", Category: "app"},
		{Name: "catalog", URL: urls.CatalogService + "/health", Category: "app"},
		{Name: "streaming", URL: urls.StreamingService + "/health", Category: "app"},
		{Name: "player", URL: urls.PlayerService + "/health", Category: "app"},
		{Name: "rooms", URL: urls.RoomsService + "/health", Category: "app"},
		{Name: "scheduler", URL: urls.SchedulerService + "/health", Category: "app"},
		{Name: "themes", URL: urls.ThemesService + "/health", Category: "app"},
		// Infrastructure (mixed)
		{Name: "postgres", TCPAddr: urls.PostgresAddr, Category: "infra"},
		{Name: "redis", TCPAddr: urls.RedisAddr, Category: "infra"},
		{Name: "nats", TCPAddr: urls.NatsAddr, Category: "infra"},
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
			MaxIdleConns:    20,
			IdleConnTimeout: 30 * time.Second,
		},
	}

	return &StatusHandler{
		services: services,
		client:   client,
		log:      log,
	}
}

func (h *StatusHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	results := make([]ServiceStatus, len(h.services))
	var wg sync.WaitGroup

	for i, svc := range h.services {
		wg.Add(1)
		go func(idx int, s serviceCheck) {
			defer wg.Done()
			results[idx] = h.checkService(s)
		}(i, svc)
	}

	wg.Wait()

	upCount := 0
	for _, s := range results {
		if s.Status == "up" {
			upCount++
		}
	}

	overall := "operational"
	if upCount == 0 {
		overall = "down"
	} else if upCount < len(results) {
		overall = "degraded"
	}

	httputil.OK(w, StatusResponse{
		Services:  results,
		Overall:   overall,
		CheckedAt: time.Now().UTC(),
	})
}

func (h *StatusHandler) checkService(svc serviceCheck) ServiceStatus {
	result := ServiceStatus{
		Name:     svc.Name,
		Category: svc.Category,
	}

	// Gateway is always up (it's serving this request)
	if svc.Name == "gateway" {
		result.Status = "up"
		result.ResponseTimeMs = 0
		return result
	}

	start := time.Now()

	if svc.TCPAddr != "" {
		// TCP health check (Redis, Postgres)
		conn, err := net.DialTimeout("tcp", svc.TCPAddr, 5*time.Second)
		result.ResponseTimeMs = time.Since(start).Milliseconds()
		if err != nil {
			result.Status = "down"
			result.Error = fmt.Sprintf("tcp dial failed: %v", err)
			return result
		}
		conn.Close()
		result.Status = "up"
		return result
	}

	// HTTP health check
	resp, err := h.client.Get(svc.URL)
	result.ResponseTimeMs = time.Since(start).Milliseconds()
	if err != nil {
		result.Status = "down"
		result.Error = fmt.Sprintf("request failed: %v", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		result.Status = "up"
	} else {
		result.Status = "down"
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}

	return result
}
