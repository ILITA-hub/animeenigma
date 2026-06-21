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
	// providerKodik is the provider label the Kodik iframe liveness check reports
	// under in the shared provider-health metric family (libs/metrics/provider.go).
	// Matches the roster row name "kodik-iframe" (the un-probeable embed) so the
	// liveness gauge lines up with the roster after the kodik split. The scraped
	// "kodik-noads" row gets its verdict from the analytics playback probe instead.
	providerKodik = "kodik-iframe"
	// kodikStage is the single synthetic probe stage for Kodik (distinct from the
	// scraper stages search/episodes/servers/stream/stream_segment).
	kodikStage = "liveness"

	// providerAe is the provider label the self-hosted library reports under.
	providerAe = "ae"
	// aeStage is ae's single synthetic liveness stage (library-service reachability).
	aeStage = "liveness"
)

// aePinger is the minimal library-client surface the ae liveness probe needs.
// library.Client satisfies it via Ping(ctx) (HTTP GET, non-2xx → error).
type aePinger interface {
	Ping(ctx context.Context) error
}

// PlayerHealthChecker periodically tests Kodik and the self-hosted ae library to
// verify availability and exposes the result via the shared provider-health metrics
// so it appears in the unified Provider Health dashboard. (The AnimeLib probe was
// retired with the AniLib player; player_health_* metrics are no longer emitted.)
type PlayerHealthChecker struct {
	kodikClient *kodik.Client
	aeProbe     aePinger
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
	aeProbe aePinger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	// provider_info / provider_enabled for kodik + the other catalog-side players is
	// emitted from the DB roster by scraperprovider.EmitCatalogSideRoster at boot
	// (single-emitter partition). This checker runs the LIVE Kodik liveness probe
	// (provider_health_up{kodik,liveness}) and the ae library liveness probe
	// (provider_health_up{ae,liveness} + probe_last_tick for both).
	return &PlayerHealthChecker{
		kodikClient: kodikClient,
		aeProbe:     aeProbe,
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
	h.checkProvider(providerAe, aeStage, h.checkAe)
}

// checkAe probes the self-hosted library service for liveness (ae availability IS
// library availability). A 5s-bounded Ping; non-2xx/unreachable → DOWN. Per-title
// "not encoded yet" (404 on a real resolve) is NOT measured here.
func (h *PlayerHealthChecker) checkAe() error {
	if h.aeProbe == nil {
		return fmt.Errorf("ae library client not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return h.aeProbe.Ping(ctx)
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
