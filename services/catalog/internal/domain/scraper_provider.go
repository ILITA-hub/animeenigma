package domain

import "time"

// ScraperProvider is the DB-backed source of truth for scraper EN-provider
// management + capability traits (migrated from docker/scraper-providers.yaml,
// spec 2026-06-15-scraper-capability-api). The scraper service fetches these
// rows via GET /internal/scraper/providers at boot + on a refresh interval;
// the YAML remains the seed + offline fallback. Maintained in the DB (no admin
// UI this phase — edited via SQL/migration).
type ScraperProvider struct {
	// Name is the canonical provider id (gogoanime, animepahe, …). Primary key.
	Name string `gorm:"primaryKey;size:32" json:"name"`
	// Enabled controls failover participation.
	Enabled bool `json:"enabled"`
	// Group is intrinsic: "en" (default) or "adult". `group` is a reserved word
	// in some SQL dialects — keep the column name explicit via the tag.
	Group string `gorm:"column:group;size:16;default:'en'" json:"group"`
	// Reason is a short dashboard label; Description is the full why.
	Reason      string `json:"reason"`
	Description string `json:"description"`
	// Capability traits (curated; refined per-title by live discovery in P2).
	SupportsSub      bool      `json:"supports_sub"`
	SupportsDub      bool      `json:"supports_dub"`
	SupportsRaw      bool      `json:"supports_raw"`
	SubDelivery      string    `gorm:"size:8;default:'hard'" json:"sub_delivery"` // soft|hard|none
	QualityCeiling   string    `gorm:"size:8" json:"quality_ceiling"`
	PreferenceWeight int       `json:"preference_weight"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// TableName pins the table name for the internal endpoint contract.
func (ScraperProvider) TableName() string { return "scraper_providers" }
