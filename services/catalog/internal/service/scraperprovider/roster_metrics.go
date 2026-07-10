package scraperprovider

import (
	"fmt"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// EmitCatalogSideRoster reflects the catalog-OWNED provider rows
// (scraper_operated = false: ae, kodik, animelib, hanime, raw) into the
// provider_info / provider_enabled management metrics via the shared
// metrics.EmitProviderRoster helper. The scraper emits the rows IT owns
// (scraper_operated = true); the two sets partition the roster with no overlap,
// so there are no duplicate series across Prometheus targets. Call once at catalog
// boot, AFTER the Phase 1 rename/seed/backfill so all rows + the scraper_operated
// flag are present. Idempotent (pure Set()).
func EmitCatalogSideRoster(db *gorm.DB) error {
	var rows []domain.ScraperProvider
	if err := db.Where("scraper_operated = ?", false).Order("name asc").Find(&rows).Error; err != nil {
		return fmt.Errorf("load catalog-side roster: %w", err)
	}
	entries := make([]metrics.RosterEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, metrics.RosterEntry{
			Name:        r.Name,
			Status:      string(r.Status),
			Reason:      r.Reason,
			Description: r.Description,
		})
	}
	metrics.EmitProviderRoster(entries)
	return nil
}

// EmitProviderStates seeds the provider_state gauge for the FULL roster at boot
// (every row, both scraper- and catalog-operated) from each row's derived
// (policy, health) state. Catalog is the sole emitter of provider_state, so this
// one call covers all providers with no cross-target duplication. Live
// transitions are layered on top by the probe-result handler; this boot seed
// ensures never-probed rows (ae, kodik, legacy players, disabled providers) still
// render a continuous band on the state-history timeline. Idempotent (pure Set).
func EmitProviderStates(db *gorm.DB) error {
	var rows []domain.ScraperProvider
	if err := db.Order("name asc").Find(&rows).Error; err != nil {
		return fmt.Errorf("load roster for state metrics: %w", err)
	}
	for _, r := range rows {
		metrics.ProviderState.WithLabelValues(r.Name, r.Group).Set(r.StateCode())
		metrics.ProviderHealthState.WithLabelValues(r.Name, r.Group).Set(r.DerivedStateCode())
	}
	return nil
}
