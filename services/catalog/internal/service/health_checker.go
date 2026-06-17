package service

import (
	"context"
	"fmt"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
)

const (
	// providerKodik is the provider label Kodik reports under in the shared
	// provider-health metric family (libs/metrics/provider.go). Kodik is the RU
	// iframe player; it is probed by catalog (not the scraper microservice) but
	// surfaces in the unified "Provider Health" Grafana view alongside the EN
	// scraper providers. See docs/superpowers/specs/2026-06-05-playback-health-v2.
	providerKodik = "kodik"
	// kodikStage is the single synthetic probe stage for Kodik (distinct from the
	// scraper stages search/episodes/servers/stream/stream_segment).
	kodikStage = "liveness"
)

// PlayerHealthChecker periodically tests Kodik to verify availability and exposes
// the result via the shared provider-health metrics so it appears in the unified
// Provider Health dashboard. (The AnimeLib probe was retired with the AniLib
// player; player_health_* metrics are no longer emitted.)
type PlayerHealthChecker struct {
	kodikClient *kodik.Client
	interval    time.Duration
	log         *logger.Logger

	// track previous status for transition logging
	prevStatus map[string]bool
}

// NewPlayerHealthChecker creates a new health checker.
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	interval time.Duration,
	log *logger.Logger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	// Register Kodik as a first-class provider row in the management table even
	// before the first probe tick completes.
	metrics.ProviderEnabled.WithLabelValues(providerKodik).Set(1)
	metrics.ProviderInfo.WithLabelValues(
		providerKodik,
		"enabled",
		"RU iframe player",
		"Kodik RU iframe — liveness via Naruto search probe (catalog-side, not part of the EN failover chain)",
	).Set(1)
	// Register the catalog-side players as first-class rows in the unified
	// provider-management metrics, so the Grafana table + count cover the FULL
	// player roster — not just Kodik + the EN scraper chain. These have no catalog
	// liveness probe (only Kodik does), so they emit no provider_health_up; they're
	// surfaced purely for management visibility. animelib + hanime are marked
	// disabled (no longer surfaced in the frontend player); raw + ae stay enabled.
	for _, p := range []struct{ name, status, reason, desc string }{
		{"animelib", "disabled", "RU direct-MP4 player", "AniLib RU direct-MP4 player (catalog parser). Disabled — no longer surfaced in the frontend player."},
		{"hanime", "disabled", "18+ HLS player", "Hanime 18+ HLS player (catalog parser). Disabled — no longer surfaced in the frontend player."},
		{"raw", "enabled", "JP original-audio player", "Raw JP player (AllAnime HLS / fast4speed.rsvp, catalog parser). Always-on; not part of the EN failover chain."},
		{"ae", "enabled", "Self-hosted library", "AnimeEnigma self-hosted library (BitTorrent → HLS → MinIO). 200 = served on-prem, 404 = no local copy yet."},
	} {
		enabled := 0.0
		if p.status == "enabled" {
			enabled = 1.0
		}
		metrics.ProviderEnabled.WithLabelValues(p.name).Set(enabled)
		metrics.ProviderInfo.WithLabelValues(p.name, p.status, p.reason, p.desc).Set(1)
	}
	return &PlayerHealthChecker{
		kodikClient: kodikClient,
		interval:    interval,
		log:         log,
		prevStatus:  make(map[string]bool),
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
	h.checkProvider(providerKodik, kodikStage, h.checkKodik)
}

// checkProvider runs a single probe and reports it via the shared provider-health
// metrics (provider_health_up{provider,stage} + provider_probe_last_tick_timestamp).
func (h *PlayerHealthChecker) checkProvider(provider, stage string, check func() error) {
	start := time.Now()
	err := check()
	duration := time.Since(start).Seconds()

	metrics.ProviderProbeLastTick.WithLabelValues(provider).SetToCurrentTime()

	up := err == nil
	if up {
		metrics.ProviderHealthUp.WithLabelValues(provider, stage).Set(1)
	} else {
		metrics.ProviderHealthUp.WithLabelValues(provider, stage).Set(0)
	}

	// Log status transitions
	prev, hasPrev := h.prevStatus[provider]
	if !hasPrev || prev != up {
		if up {
			h.log.Infow("provider is UP", "provider", provider, "stage", stage, "duration_s", fmt.Sprintf("%.2f", duration))
		} else {
			h.log.Warnw("provider is DOWN", "provider", provider, "stage", stage, "error", err, "duration_s", fmt.Sprintf("%.2f", duration))
		}
	}
	h.prevStatus[provider] = up
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
