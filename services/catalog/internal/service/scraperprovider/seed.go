// Package scraperprovider seeds the scraper_providers table from the bundled
// docker/scraper-providers.yaml. Insert-if-absent only: a row that already
// exists is never overwritten, so operator edits in the DB survive re-seeding.
// (Catalog cannot import services/scraper/internal/* — Go internal rule — so a
// small local YAML shape is defined here rather than reused.)
package scraperprovider

import (
	"fmt"
	"os"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type seedEntry struct {
	Name             string `yaml:"name"`
	Enabled          *bool  `yaml:"enabled"`
	Group            string `yaml:"group"`
	Reason           string `yaml:"reason"`
	Description      string `yaml:"description"`
	SupportsSub      *bool  `yaml:"supports_sub"`
	SupportsDub      *bool  `yaml:"supports_dub"`
	SupportsRaw      *bool  `yaml:"supports_raw"`
	SubDelivery      string `yaml:"sub_delivery"`
	QualityCeiling   string `yaml:"quality_ceiling"`
	PreferenceWeight *int   `yaml:"preference_weight"`
}

type seedFile struct {
	Providers []seedEntry `yaml:"providers"`
}

func deref(p *bool) bool { return p != nil && *p }

// intrinsicGroups mirrors services/scraper/internal/config/providers.go's
// providerGroups: group is INTRINSIC to a provider (a hentai source is always
// "adult"), so a missing/typo'd YAML group can never move 18anime into the EN
// failover chain. Absent entries default to "en".
var intrinsicGroups = map[string]string{
	"18anime": "adult",
}

func intrinsicGroup(name string) string {
	if g, ok := intrinsicGroups[name]; ok {
		return g
	}
	return "en"
}

// SeedFromYAML reads path and inserts any provider rows not already present.
// Returns nil (no-op) if path is empty so a missing seed file never blocks boot.
func SeedFromYAML(db *gorm.DB, path string) error {
	if path == "" {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read scraper providers seed %q: %w", path, err)
	}
	var sf seedFile
	if err := yaml.Unmarshal(raw, &sf); err != nil {
		return fmt.Errorf("parse scraper providers seed: %w", err)
	}
	for _, e := range sf.Providers {
		if e.Name == "" {
			continue
		}
		var count int64
		if err := db.Model(&domain.ScraperProvider{}).Where("name = ?", e.Name).Count(&count).Error; err != nil {
			return fmt.Errorf("count %q: %w", e.Name, err)
		}
		if count > 0 {
			continue // insert-if-absent: never overwrite an existing row
		}
		// Group is intrinsic — always derive from the provider name, never trust
		// the YAML value (defense-in-depth for the 18+/EN separation).
		group := intrinsicGroup(e.Name)
		subDelivery := e.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		enabled := true
		if e.Enabled != nil {
			enabled = *e.Enabled
		}
		weight := 0
		if e.PreferenceWeight != nil {
			weight = *e.PreferenceWeight
		}
		row := domain.ScraperProvider{
			Name:             e.Name,
			Enabled:          enabled,
			Group:            group,
			Reason:           e.Reason,
			Description:      e.Description,
			SupportsSub:      deref(e.SupportsSub),
			SupportsDub:      deref(e.SupportsDub),
			SupportsRaw:      deref(e.SupportsRaw),
			SubDelivery:      subDelivery,
			QualityCeiling:   e.QualityCeiling,
			PreferenceWeight: weight,
		}
		if err := db.Create(&row).Error; err != nil {
			return fmt.Errorf("create %q: %w", e.Name, err)
		}
	}
	return nil
}
