package domain

import "time"

// ProviderPolicy is the failover-participation dimension. As of 2026-07-13 it
// is HEALTH-DRIVEN, not admin-set: the probe state machine reconciles auto↔manual
// from health on every verdict (ReconcilePolicyFromHealth — health==down ⇒ manual,
// otherwise auto), reversing the 2026-07-08 "policy admin-only" decision. The admin
// controls only the disabled hard-lock (the Auto/Disabled probe-status toggle);
// disabled is never auto-changed by the machine. disabled = not registered; manual
// parks a provider out of auto-failover while keeping it hacker-selectable.
type ProviderPolicy string

const (
	PolicyAuto     ProviderPolicy = "auto"
	PolicyManual   ProviderPolicy = "manual"
	PolicyDisabled ProviderPolicy = "disabled"
)

// ProviderHealth is the probe-observed dimension.
type ProviderHealth string

const (
	HealthUp         ProviderHealth = "up"
	HealthDegraded   ProviderHealth = "degraded" // one failed probe, pending confirmation — transient, still failover-trusted
	HealthRecovering ProviderHealth = "recovering"
	HealthDown       ProviderHealth = "down"
)

// CadenceConfig holds the tunable probe cadences + sample sizes (from env).
type CadenceConfig struct {
	Up               time.Duration
	Recovering       time.Duration
	Manual           time.Duration
	RecoveringSample int
	FullSample       int
}

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
	Status ProviderStatus `gorm:"size:16;default:'disabled'" json:"status"`
	// Health is machine-managed by the probe state machine (spec 2026-06-23,
	// hysteresis 2026-07-08). Policy is ALSO machine-managed as of 2026-07-13
	// (health-driven auto↔manual via ReconcilePolicyFromHealth); the admin sets
	// only the disabled hard-lock. Status above is DERIVED for the wire via
	// WireStatus().
	Policy       ProviderPolicy `gorm:"size:16;default:'disabled'" json:"policy"`
	Health       ProviderHealth `gorm:"size:16;default:'down'" json:"health"`
	HealthSince  time.Time      `json:"health_since"`
	PolicySince  time.Time      `json:"policy_since"`
	LastProbedAt time.Time      `json:"last_probed_at"`
	// Group is intrinsic: "en" (default) or "adult". `group` is a reserved word
	// in some SQL dialects — keep the column name explicit via the tag.
	Group string `gorm:"column:group;size:16;default:'en'" json:"group"`
	// Reason is a short dashboard label; Description is the full why (records
	// WHY this provider is enabled/degraded/disabled).
	Reason      string `json:"reason"`
	Description string `json:"description"`
	// AIProbeNotes is a free-form analysis field curated by the AI probe operator
	// and surfaced as the "AI Probe Notes" column on the playback-health dashboard.
	// No service logic reads it; routine catalog writes (health/policy/reason) never
	// touch it, so it persists across probe health cycles.
	AIProbeNotes string `gorm:"column:ai_probe_notes" json:"ai_probe_notes"`
	// LastTickMetrics is the JSON summary of the most recent probe tick (warmup/
	// resolve/validate timings, throughput, CDN, quality), written by the
	// probe-result handler and rendered on the Grafana "Last Tick Metrics" panel.
	// Stored as text (the JSON blob); the panel casts ::jsonb. Empty until first
	// probed under the warmup pipeline. Routine health/policy writes never touch
	// it, so it persists across probe health cycles (like ai_probe_notes).
	LastTickMetrics string `gorm:"column:last_tick_metrics;type:text" json:"last_tick_metrics"`
	// Capability traits (curated; refined per-title by live discovery in P2).
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `gorm:"size:8;default:'hard'" json:"sub_delivery"` // soft|hard|none
	QualityCeiling   string `gorm:"size:8" json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
	// EngineKind selects the executable constructor from the scraper's sole
	// constructor registry. It is DB-owned and validated against the
	// stream_provider_engine_kinds table before a scraper-operated row can be
	// enabled. Empty is allowed only for disabled historical tombstones.
	EngineKind string `gorm:"size:32" json:"engine_kind"`
	// FailoverPriority is the DB-owned scraper runtime order within a group.
	// Higher values are attempted first; Name is the deterministic tie-breaker.
	FailoverPriority int `json:"failover_priority"`
	// DisplayName is the operator-editable pretty label for player/dashboard
	// surfaces (capability DisplayName, Grafana). Empty ⇒ callers fall back to
	// a title-cased Name. Seeded; backfilled once by BackfillProviderIdentityV1.
	DisplayName string `gorm:"size:64" json:"display_name"`
	// PlayerKey maps this row into the legacy watch_history.player namespace
	// ('english', 'kodik', 'ae', 'hanime', …) consumed by watch preferences,
	// notifications hot-combos and episode validation. Multiple rows may share
	// one key (the whole EN chain is 'english'; both kodik rows are 'kodik').
	// Empty ⇒ the provider has no legacy-player identity.
	PlayerKey string `gorm:"size:32" json:"player_key"`
	// AnimeLevel marks providers whose new-episode lookup works without a
	// translation_id (any-team/anime-level: english, ae, kodik, animelib,
	// animejoy legs). Drives the notifications hot-combos eligibility subselect.
	AnimeLevel bool `json:"anime_level"`
	// Engine selects HOW this provider is scraped (DB-driven; there is NO
	// SCRAPER_*_ENGINE env):
	//   - "http"    — legacy in-process Go net/http scraper (default).
	//   - "browser" — resolved via the Camoufox stealth-scraper sidecar
	//     (services/stealth-scraper), for providers whose CDN/player is
	//     JS/fingerprint/clearance-walled and a curl-class client cannot reach
	//     (e.g. gogoanime → megaplay → cdn.mewstream.buzz Cloudflare).
	Engine string `gorm:"size:16;default:'http'" json:"engine"`
	// BaseURL is the provider's mirror origin (e.g. https://gogoanimes.fi),
	// replacing the former SCRAPER_<NAME>_BASE_URL envs. Empty ⇒ the provider's
	// built-in default still applies.
	BaseURL string `gorm:"size:256" json:"base_url"`
	// ScraperOperated marks providers operated by the scraper microservice (the
	// EN failover chain + the 18+ orchestrator). Intrinsic (derived from name,
	// NOT operator-editable). The scraper consumes only scraper_operated=true
	// rows, so first-party/legacy players (ae, kodik, animelib, hanime, raw) in
	// this table never enter EN failover. Added 2026-06-17 (roster unification).
	ScraperOperated bool      `json:"scraper_operated"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName pins the physical table. Renamed scraper_providers → stream_providers
// 2026-06-17: the table is now the roster for EVERY stream source (ae + EN chain
// + adult + legacy players), not just scraper EN-providers. The Go type keeps its
// ScraperProvider name (table rename only) to limit blast radius.
func (ScraperProvider) TableName() string { return "stream_providers" }

// ProviderEngineKind is the DB-contained allowlist of executable provider
// implementation kinds. Activation validates only against this table; it does
// not call the scraper service or depend on a transient runtime registry.
type ProviderEngineKind struct {
	Kind string `gorm:"primaryKey;size:32" json:"kind"`
	// ProviderName binds native constructors whose implementation reports a
	// fixed provider id. Empty is reserved for genuinely generic/reusable kinds.
	ProviderName string `gorm:"size:32" json:"provider_name,omitempty"`
	Description  string `json:"description"`
}

func (ProviderEngineKind) TableName() string { return "stream_provider_engine_kinds" }

// IsEnabled reports whether the provider is in the normal auto-failover chain.
func (p ScraperProvider) IsEnabled() bool { return p.Status == StatusEnabled }

// IsDegraded reports the soft-degraded state: registered + manually selectable
// but excluded from auto-failover and sorted last in the picker.
func (p ScraperProvider) IsDegraded() bool { return p.Status == StatusDegraded }

// IsRegistered reports whether the provider is registered at all (enabled OR
// degraded). Disabled providers are not registered.
func (p ScraperProvider) IsRegistered() bool { return p.Status != StatusDisabled }

// Eligible reports auto-failover eligibility — exactly the WireStatus enabled
// tri-state (policy auto AND health up|degraded), kept as one source of truth.
func (p ScraperProvider) Eligible() bool { return p.WireStatus() == StatusEnabled }

// State labels for the playback-health dashboard's roster "State" column and
// the /admin/policy pill — both driven by DerivedState. The Postgres CASE in
// playback-health.json mirrors DerivedState one-for-one.
const (
	StateUP         = "UP"         // health up
	StateRecovering = "Recovering" // health recovering: climbing back after a confirmed outage
	StateDegrading  = "Degrading"  // health degraded: one failed probe, pending confirmation
	StateDown       = "Down"       // health down: confirmed failing (two consecutive fails)
	StateDisabled   = "Disabled"   // admin lock: policy disabled ONLY (a parked manual provider shows its health, not this)
)

// DerivedState is the HEALTH-lifecycle label shown on the playback-health
// roster "State" column (the Postgres CASE mirrors this one-for-one) and the
// /admin/policy pill. Only an explicit admin disable (policy=disabled) reads as
// "Disabled"; a parked manual provider shows its LIVE health on the same
// 4-state scale as auto — the roster's separate "In auto-failover chain" column
// carries the auto/manual distinction. Deliberately DECOUPLED from StateCode
// (the failover-participation gauge behind the fleet alerts).
func (p ScraperProvider) DerivedState() string {
	if p.Policy == PolicyDisabled {
		return StateDisabled
	}
	switch p.Health {
	case HealthUp:
		return StateUP
	case HealthRecovering:
		return StateRecovering
	case HealthDegraded:
		return StateDegrading
	default: // down
		return StateDown
	}
}

// DerivedStateCode is the numeric encoding of DerivedState, feeding the
// provider_health_state gauge behind the "Provider State History" timeline.
// Higher = healthier; kept in lock-step with DerivedState.
func (p ScraperProvider) DerivedStateCode() float64 {
	switch p.DerivedState() {
	case StateUP:
		return 4
	case StateRecovering:
		return 3
	case StateDegrading:
		return 2
	case StateDown:
		return 1
	default: // Disabled
		return 0
	}
}

// StateCode is the FAILOVER-PARTICIPATION lifecycle behind the provider_state
// gauge that the fleet alert rules aggregate `by (group)`. Manual and disabled
// providers are NOT in the auto-failover chain, so both collapse to 0 — this is
// DELIBERATELY different from DerivedState (the health display label) so a
// parked-but-healthy provider never masks the "no auto-playable source" alert
// math. Encoding: 4=UP, 3=Recovering, 2=Degraded(one failed probe), 1=Down,
// 0=not in auto-failover (manual or disabled).
func (p ScraperProvider) StateCode() float64 {
	if p.Policy != PolicyAuto {
		return 0
	}
	switch p.Health {
	case HealthUp:
		return 4
	case HealthRecovering:
		return 3
	case HealthDegraded:
		return 2
	default: // down
		return 1
	}
}

// WireStatus derives the legacy tri-state the scraper failover gate consumes.
// auto+degraded stays enabled — a single failed probe is a warning, not a
// confirmed outage, and runtime failover already covers a genuine miss. Note
// the deliberate axis split with DerivedState: a manual provider shows its live
// health on the dashboard but stays degraded here (out of auto-failover, still
// hacker-mode selectable), so selectability is unchanged.
func (p ScraperProvider) WireStatus() ProviderStatus {
	switch p.Policy {
	case PolicyDisabled:
		return StatusDisabled
	case PolicyAuto:
		if p.Health == HealthUp || p.Health == HealthDegraded {
			return StatusEnabled
		}
		return StatusDegraded
	default: // manual
		return StatusDegraded
	}
}

// ProbeCadence returns how often this provider should be probed; 0 = never.
func (p ScraperProvider) ProbeCadence(c CadenceConfig) time.Duration {
	if p.Policy == PolicyDisabled {
		return 0
	}
	switch p.Health {
	case HealthUp:
		return c.Up
	case HealthDegraded:
		return c.Up // re-probe next cycle to confirm/clear (Phase-1 interim; Phase 2 replaces)
	case HealthRecovering:
		return c.Recovering
	default: // down
		if p.Policy == PolicyManual {
			return c.Manual
		}
		return c.Up // auto+down (Failing): probe fast to confirm/recover
	}
}

// ProbeSample returns the title sample size + fail-fast flag for a run.
//
// Recovery is ANCHOR-GATED, symmetric with how up-providers are judged: no
// branch fail-fasts on a non-anchor miss. The old fail_fast=true recovery gate
// (2026-07-13) demanded 100% of a random sample play, which per-title providers
// (ok.ru-only allanime-okru, animepahe, gogoanime — each legitimately lacks some
// titles) could never clear, so a provider serving the anchor + majority stayed
// pinned "down" while Rollup scored it "Up". fail_fast=false makes `pass` fall to
// topPlayed (the anchor), so a provider that serves the anchor climbs back.
func (p ScraperProvider) ProbeSample(c CadenceConfig) (int, bool) {
	if p.Policy == PolicyDisabled {
		return 0, false
	}
	switch {
	case p.Health == HealthUp, p.Health == HealthDegraded:
		return c.FullSample, false // full picture, no abort (degraded: honest confirm/clear)
	case p.Health == HealthRecovering:
		return c.RecoveringSample, false // anchor-gated climb-back, not all-sample
	default: // down — always manual now (health-driven policy); cheapest "is it back?"
		return 1, false // probe the anchor only; fail_fast is moot at size 1
	}
}
