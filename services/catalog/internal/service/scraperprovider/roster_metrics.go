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
