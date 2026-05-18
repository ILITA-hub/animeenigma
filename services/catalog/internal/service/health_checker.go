package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
)

const (
	playerKodik    = "kodik"
	playerAnimeLib = "animelib"
)

// PlayerHealthChecker periodically tests each player/parser to verify availability
// and exposes the results as Prometheus metrics.
type PlayerHealthChecker struct {
	kodikClient    *kodik.Client
	animelibClient *animelib.Client
	interval       time.Duration
	log            *logger.Logger

	// track previous status for transition logging
	prevStatus map[string]bool
}

// NewPlayerHealthChecker creates a new health checker.
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	animelibClient *animelib.Client,
	interval time.Duration,
	log *logger.Logger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PlayerHealthChecker{
		kodikClient:    kodikClient,
		animelibClient: animelibClient,
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

// checkKodik verifies Kodik API is reachable and returns results with embed links.
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
	// Verify at least one result has an embed link
	for _, r := range results {
		if r.Link != "" {
			return nil
		}
	}
	return fmt.Errorf("kodik results have no embed links")
}

// checkAnimeLib verifies the full AnimeLib playback chain:
// search → get episodes → get episode streams → verify video sources exist.
func (h *PlayerHealthChecker) checkAnimeLib() error {
	if h.animelibClient == nil {
		return fmt.Errorf("animelib client not initialized")
	}

	// Step 1: Search
	results, err := h.animelibClient.Search("naruto")
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("search returned 0 results")
	}

	// Step 2: Get episodes
	episodes, err := h.animelibClient.GetEpisodes(results[0].ID)
	if err != nil {
		return fmt.Errorf("get episodes failed (anime %d): %w", results[0].ID, err)
	}
	if len(episodes) == 0 {
		return fmt.Errorf("anime %d has 0 episodes", results[0].ID)
	}

	// Step 3: Get streams for first episode
	detail, err := h.animelibClient.GetEpisodeStreams(episodes[0].ID)
	if err != nil {
		return fmt.Errorf("get streams failed (episode %d): %w", episodes[0].ID, err)
	}
	if len(detail.Players) == 0 {
		return fmt.Errorf("episode %d has 0 players", episodes[0].ID)
	}

	// Step 4: Verify at least one player has a video source
	for _, p := range detail.Players {
		if p.Video != nil && len(p.Video.Quality) > 0 {
			return nil // Has direct MP4
		}
		if p.Src != "" {
			return nil // Has Kodik iframe
		}
	}
	return fmt.Errorf("episode %d players have no video sources", episodes[0].ID)
}
