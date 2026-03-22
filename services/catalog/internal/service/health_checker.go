package service

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
)

const (
	playerKodik    = "kodik"
	playerAnimeLib = "animelib"
	playerHiAnime  = "hianime"
	playerConsumet = "consumet"
)

// PlayerHealthChecker periodically tests each player/parser to verify availability
// and exposes the results as Prometheus metrics.
type PlayerHealthChecker struct {
	kodikClient    *kodik.Client
	animelibClient *animelib.Client
	aniwatchURL    string
	consumetURL    string
	httpClient     *http.Client
	interval       time.Duration
	log            *logger.Logger

	// track previous status for transition logging
	prevStatus map[string]bool
}

// NewPlayerHealthChecker creates a new health checker.
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	animelibClient *animelib.Client,
	aniwatchURL string,
	consumetURL string,
	interval time.Duration,
	log *logger.Logger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PlayerHealthChecker{
		kodikClient:    kodikClient,
		animelibClient: animelibClient,
		aniwatchURL:    aniwatchURL,
		consumetURL:    consumetURL,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		interval:       interval,
		log:            log,
		prevStatus:     make(map[string]bool),
	}
}

// Start runs the health checker loop until ctx is cancelled.
func (h *PlayerHealthChecker) Start(ctx context.Context) {
	h.log.Infow("player health checker started", "interval", h.interval.String())

	// Run immediately on start
	h.checkAll()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.log.Info("player health checker stopped")
			return
		case <-ticker.C:
			h.checkAll()
		}
	}
}

func (h *PlayerHealthChecker) checkAll() {
	h.checkPlayer(playerKodik, h.checkKodik)
	h.checkPlayer(playerAnimeLib, h.checkAnimeLib)
	h.checkPlayer(playerHiAnime, h.checkHiAnime)
	h.checkPlayer(playerConsumet, h.checkConsumet)
}

func (h *PlayerHealthChecker) checkPlayer(name string, check func() error) {
	start := time.Now()
	err := check()
	duration := time.Since(start).Seconds()

	metrics.PlayerHealthCheckDuration.WithLabelValues(name).Observe(duration)
	metrics.PlayerHealthLastCheck.WithLabelValues(name).SetToCurrentTime()

	up := err == nil
	if up {
		metrics.PlayerHealthUp.WithLabelValues(name).Set(1)
	} else {
		metrics.PlayerHealthUp.WithLabelValues(name).Set(0)
	}

	// Log status transitions
	prev, hasPrev := h.prevStatus[name]
	if !hasPrev || prev != up {
		if up {
			h.log.Infow("player is UP", "player", name, "duration_s", fmt.Sprintf("%.2f", duration))
		} else {
			h.log.Warnw("player is DOWN", "player", name, "error", err, "duration_s", fmt.Sprintf("%.2f", duration))
		}
	}
	h.prevStatus[name] = up
}

// checkKodik verifies Kodik API is reachable by searching for a well-known anime.
func (h *PlayerHealthChecker) checkKodik() error {
	if h.kodikClient == nil {
		return fmt.Errorf("kodik client not initialized")
	}
	results, err := h.kodikClient.SearchByShikimoriID("20") // Naruto
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("kodik returned 0 results for test query")
	}
	return nil
}

// checkAnimeLib verifies AnimeLib API is reachable.
func (h *PlayerHealthChecker) checkAnimeLib() error {
	if h.animelibClient == nil {
		return fmt.Errorf("animelib client not initialized")
	}
	results, err := h.animelibClient.Search("naruto")
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("animelib returned 0 results for test query")
	}
	return nil
}

// checkHiAnime verifies the Aniwatch API sidecar is reachable.
func (h *PlayerHealthChecker) checkHiAnime() error {
	if h.aniwatchURL == "" {
		return fmt.Errorf("aniwatch API URL not configured")
	}
	resp, err := h.httpClient.Get(h.aniwatchURL + "/api/v2/hianime/search?q=naruto&page=1")
	if err != nil {
		return fmt.Errorf("aniwatch API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("aniwatch API returned status %d", resp.StatusCode)
	}
	return nil
}

// checkConsumet verifies the Consumet API sidecar is reachable.
func (h *PlayerHealthChecker) checkConsumet() error {
	if h.consumetURL == "" {
		return fmt.Errorf("consumet API URL not configured")
	}
	resp, err := h.httpClient.Get(h.consumetURL + "/anime/animepahe/naruto")
	if err != nil {
		return fmt.Errorf("consumet API unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("consumet API returned status %d", resp.StatusCode)
	}
	return nil
}
