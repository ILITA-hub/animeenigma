package config

import (
	"sort"
	"sync/atomic"
)

// KnownProviders is the canonical set of scraper provider names recognized at
// compile time. AUTO-608: it no longer gates the remote loader (LoadProvidersRemote
// is fail-open — see its doc comment) — DB rows are the source of truth, and
// main.go keys roster-reflection metrics + the wiring-invariant checks off
// cfg.Providers.AllNames() instead. KnownProviders only (1) seeds the offline
// fallback used until the catalog answers and (2) drives the intrinsic
// EN-vs-adult candidate split for that fallback path.
//
// These are the registration names in cmd/scraper-api/main.go. Retired DB
// tombstones are intentionally excluded from the offline fallback.
var KnownProviders = []string{
	"gogoanime", "animepahe", "allanime-okru", "miruro", "nineanime",
	"18anime",
}

// Provider groups. The EN failover chain serves GroupEN providers; GroupAdult
// providers (18+) are served by a SEPARATE orchestrator on /anime18/* routes
// and are NEVER part of the EN chain (no 18+ leakage into OurEnglish).
const (
	GroupEN    = "en"
	GroupAdult = "adult"
)

// providerGroups assigns each known provider to its intrinsic group. Absent
// entries default to GroupEN.
var providerGroups = map[string]string{
	"18anime": GroupAdult,
}

// GroupOf returns the canonical group for a provider name (GroupEN by default).
func GroupOf(name string) string {
	if g, ok := providerGroups[name]; ok {
		return g
	}
	return GroupEN
}

// KnownProvidersInGroup returns the KnownProviders belonging to the given group,
// preserving KnownProviders order. main.go uses this to build the EN vs adult
// candidate lists (metrics seeding + boot-count split).
func KnownProvidersInGroup(group string) []string {
	out := []string{}
	for _, name := range KnownProviders {
		if GroupOf(name) == group {
			out = append(out, name)
		}
	}
	return out
}

// ProviderStatus mirrors catalog's domain.ProviderStatus tri-state (the DB is
// the source of truth). enabled = in auto-failover; degraded = registered but
// excluded from auto-failover (reachable only via explicit `prefer` / hacker-mode
// pin, sorted last in the player); disabled = not registered at all.
type ProviderStatus string

const (
	StatusEnabled  ProviderStatus = "enabled"
	StatusDegraded ProviderStatus = "degraded"
	StatusDisabled ProviderStatus = "disabled"
)

// ProviderMeta is one resolved provider entry.
type ProviderMeta struct {
	Name             string
	Status           ProviderStatus // enabled | degraded | disabled (replaces the former Enabled bool)
	Reason           string
	Description      string
	Group            string // "en" (default) or "adult" — intrinsic, from GroupOf(name)
	SupportsSub      bool
	SupportsDub      bool
	SupportsRaw      bool
	SubDelivery      string // "soft" | "hard" | "none" (default "hard")
	QualityCeiling   string
	PreferenceWeight int
	// Engine selects the scraping engine for this provider: "http" (legacy
	// in-process Go scraper, default) or "browser" (Camoufox stealth-scraper
	// sidecar). DB-driven — there is NO SCRAPER_*_ENGINE env.
	Engine string
	// BaseURL is the provider's mirror origin from the DB (replaces the former
	// SCRAPER_<NAME>_BASE_URL envs). Empty ⇒ the provider's built-in default.
	BaseURL string
	// Health carries the operator-facing health signal (up|recovering|down).
	// This is a display-only field passed through to the /health response for
	// the frontend pill — it does NOT gate the auto-failover chain (that stays
	// keyed on Status/IsEnabled).
	Health string
}

// EngineHTTP / EngineBrowser are the ProviderMeta.Engine values.
const (
	EngineHTTP    = "http"
	EngineBrowser = "browser"
)

// EngineOf returns the configured engine for a provider, defaulting to
// EngineHTTP when unset (absent provider or empty column).
func (p ProvidersConfig) EngineOf(name string) string {
	if m, ok := p.load()[name]; ok && m.Engine != "" {
		return m.Engine
	}
	return EngineHTTP
}

// BrowserEngineNames returns the KnownProviders whose resolved engine is
// EngineBrowser, in KnownProviders order. main.go uses it to grant every
// Camoufox/stealth-sidecar provider the longer shared browser failover budget
// (their cold Cloudflare solve overruns the short HTTP chain budget) without
// hardcoding a per-name list — a new browser provider added to the DB roster is
// picked up automatically, so none can be silently forgotten.
func (p ProvidersConfig) BrowserEngineNames() []string {
	out := []string{}
	for _, name := range KnownProviders {
		if p.EngineOf(name) == EngineBrowser {
			out = append(out, name)
		}
	}
	return out
}

// BaseURLOf returns the DB-configured mirror base URL for a provider (empty when
// unset — callers fall back to the provider's built-in default).
func (p ProvidersConfig) BaseURLOf(name string) string {
	if m, ok := p.load()[name]; ok {
		return m.BaseURL
	}
	return ""
}

// ProvidersConfig is the resolved provider management config. Source is one of
// "default", "remote", or "test".
//
// The metas field is a pointer to an atomic.Pointer so ProvidersConfig can be
// copied by value (e.g. assigned to a struct field) while all copies share the
// same atomic slot — a refresher calling Replace on any copy atomically updates
// what every reader (IsEnabled, Meta, Rows, …) sees. The pointer-to-atomic
// approach avoids the go vet copylocks diagnostic that would fire if the
// atomic.Pointer were embedded directly as a value.
type ProvidersConfig struct {
	metas  *atomic.Pointer[map[string]ProviderMeta]
	Source string
}

// newProvidersConfig wraps a metas map in an atomic pointer.
func newProvidersConfig(metas map[string]ProviderMeta, source string) ProvidersConfig {
	ap := &atomic.Pointer[map[string]ProviderMeta]{}
	ap.Store(&metas)
	return ProvidersConfig{metas: ap, Source: source}
}

// load returns the current metas map (nil-safe).
func (p ProvidersConfig) load() map[string]ProviderMeta {
	if p.metas == nil {
		return nil
	}
	if m := p.metas.Load(); m != nil {
		return *m
	}
	return nil
}

// Replace atomically swaps the provider metadata (used by the refresher).
func (p ProvidersConfig) Replace(entries []ProviderMeta) {
	if p.metas == nil {
		return
	}
	m := make(map[string]ProviderMeta, len(entries))
	for _, e := range entries {
		m[e.Name] = e
	}
	p.metas.Store(&m)
}

// Status returns a provider's tri-state. Absent names default to StatusEnabled —
// forgetting to list a provider never silently disables it.
func (p ProvidersConfig) Status(name string) ProviderStatus {
	if m, ok := p.load()[name]; ok && m.Status != "" {
		return m.Status
	}
	return StatusEnabled
}

// Health returns the operator-facing health signal (up|recovering|down) for a
// provider. Empty string when the provider is absent or the field is unset.
// This is a display-only passthrough — the failover gate stays on Status.
func (p ProvidersConfig) Health(name string) string { return p.Meta(name).Health }

// IsEnabled reports whether a provider is in the normal auto-failover chain.
// Absent names default to enabled.
func (p ProvidersConfig) IsEnabled(name string) bool {
	return p.Status(name) == StatusEnabled
}

// IsRegistered reports whether a provider is registered at all (enabled OR
// degraded). Only disabled providers are not registered.
func (p ProvidersConfig) IsRegistered(name string) bool {
	return p.Status(name) != StatusDisabled
}

// IsSoftDegraded reports the soft-degraded state: registered + manually
// selectable but excluded from the auto-failover chain (AUTO-484).
func (p ProvidersConfig) IsSoftDegraded(name string) bool {
	return p.Status(name) == StatusDegraded
}

// Meta returns the metadata for a provider (zero value if absent).
func (p ProvidersConfig) Meta(name string) ProviderMeta { return p.load()[name] }

// AllNames returns every loaded provider name, sorted. Under the remote
// loader all entries are scraper_operated rows; under the offline fallback
// it equals KnownProviders. AUTO-608: main.go keys roster-reflection metrics
// and the unwired check off this (NOT off compile-time KnownProviders), so a
// new DB row surfaces on the dashboard without a code change.
func (p ProvidersConfig) AllNames() []string {
	m := p.load()
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// NewProvidersConfigForTest constructs a ProvidersConfig from a slice of
// ProviderMeta entries. Intended only for unit tests that need to drive the
// handler without a catalog response.
func NewProvidersConfigForTest(entries []ProviderMeta) ProvidersConfig {
	metas := make(map[string]ProviderMeta, len(entries))
	for _, m := range entries {
		metas[m.Name] = m
	}
	return newProvidersConfig(metas, "test")
}

// ProviderRow is a flattened row for metric emission / display.
type ProviderRow struct {
	Name        string
	Enabled     bool           // true only for StatusEnabled (in the auto-failover chain)
	Status      ProviderStatus // enabled | degraded | disabled
	Reason      string
	Description string
}

// Rows returns one row per candidate provider, in the given order.
func (p ProvidersConfig) Rows(candidates []string) []ProviderRow {
	metas := p.load()
	rows := make([]ProviderRow, 0, len(candidates))
	for _, name := range candidates {
		m, ok := metas[name]
		status := StatusEnabled
		if ok && m.Status != "" {
			status = m.Status
		}
		rows = append(rows, ProviderRow{
			Name:        name,
			Enabled:     status == StatusEnabled,
			Status:      status,
			Reason:      m.Reason,
			Description: m.Description,
		})
	}
	return rows
}

// allProvidersEnabled builds the offline-fallback ProvidersConfig: every known
// provider enabled, no metadata. Used at boot until/if the catalog DB (the
// single source of truth, AUTO-484) answers and re-gates registration.
func allProvidersEnabled(source string) ProvidersConfig {
	metas := make(map[string]ProviderMeta, len(KnownProviders))
	for _, name := range KnownProviders {
		metas[name] = ProviderMeta{Name: name, Status: StatusEnabled, Group: GroupOf(name)}
	}
	return newProvidersConfig(metas, source)
}

// DisabledNames returns the sorted names of disabled (not-registered) providers
// for logging. Soft-degraded providers are registered, so they are NOT listed.
func (p ProvidersConfig) DisabledNames() []string {
	out := []string{}
	for name, m := range p.load() {
		if m.Status == StatusDisabled {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// DegradedNames returns the sorted names of soft-degraded providers (for logging).
func (p ProvidersConfig) DegradedNames() []string {
	out := []string{}
	for name, m := range p.load() {
		if m.Status == StatusDegraded {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
