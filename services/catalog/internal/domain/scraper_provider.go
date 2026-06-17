package domain

import "time"

// ProviderStatus is the tri-state lifecycle of a scraper EN-provider.
//
//   - StatusEnabled  — normal: in the auto-failover chain, auto-selectable.
//   - StatusDegraded — registered + manually selectable (hacker-mode pin / explicit
//     `prefer`), but EXCLUDED from the auto-failover chain (never auto-fallen-back
//     to) and sorted LAST in the player source picker, behind a "degraded" pill.
//     Use when a provider technically resolves but is unwatchable for our users
//     (e.g. AnimeFever's region-walled ad-substitution — AUTO-484).
//   - StatusDisabled — not registered at all (zero per-request cost, invisible).
type ProviderStatus string

const (
	StatusEnabled  ProviderStatus = "enabled"
	StatusDegraded ProviderStatus = "degraded"
	StatusDisabled ProviderStatus = "disabled"
)

// ScraperProvider is the DB-backed source of truth for scraper EN-provider
// management + capability traits. The DB is the SINGLE source of truth
// (docker/scraper-providers.yaml was retired 2026-06-17, AUTO-484); a fresh DB
// is bootstrapped by the Go-embedded seed in service/scraperprovider, and the
// scraper service fetches these rows via GET /internal/scraper/providers at boot
// + on a refresh interval. Maintained in the DB (edited via SQL/migration; the
// `reason`/`description` columns record WHY a provider is in its state).
type ScraperProvider struct {
	// Name is the canonical provider id (gogoanime, animepahe, …). Primary key.
	Name string `gorm:"primaryKey;size:32" json:"name"`
	// Status is the tri-state lifecycle (enabled|degraded|disabled). Replaces the
	// former Enabled bool (migrated 2026-06-17). Controls failover participation:
	// only StatusEnabled providers join the auto-failover chain.
	Status ProviderStatus `gorm:"size:16;default:'enabled'" json:"status"`
	// Group is intrinsic: "en" (default) or "adult". `group` is a reserved word
	// in some SQL dialects — keep the column name explicit via the tag.
	Group string `gorm:"column:group;size:16;default:'en'" json:"group"`
	// Reason is a short dashboard label; Description is the full why (records
	// WHY this provider is enabled/degraded/disabled).
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

// IsEnabled reports whether the provider is in the normal auto-failover chain.
func (p ScraperProvider) IsEnabled() bool { return p.Status == StatusEnabled }

// IsDegraded reports the soft-degraded state: registered + manually selectable
// but excluded from auto-failover and sorted last in the picker.
func (p ScraperProvider) IsDegraded() bool { return p.Status == StatusDegraded }

// IsRegistered reports whether the provider is registered at all (enabled OR
// degraded). Disabled providers are not registered.
func (p ScraperProvider) IsRegistered() bool { return p.Status != StatusDisabled }
