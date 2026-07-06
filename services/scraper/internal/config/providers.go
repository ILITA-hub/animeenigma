package config

import (
	"fmt"
	"os"
	"sort"
	"sync/atomic"

	"gopkg.in/yaml.v3"
)

// KnownProviders is the canonical set of scraper provider names that may appear
// in scraper-providers.yaml / the catalog stream_providers roster. Anything else
// is a typo and fails validation (LoadProviders + the remote loader).
//
// These are the registration names in cmd/scraper-api/main.go, PLUS "animefever":
// its provider code was removed from the binary 2026-07-05 (dead upstream —
// tombstone), but the catalog keeps a disabled `scraper_operated` animefever row
// as the historical record, and the remote loader hard-fails on any
// scraper_operated name it doesn't recognize — so the name must stay listed here
// even though nothing registers it.
var KnownProviders = []string{
	"gogoanime", "animepahe", "allanime", "okru", "animefever", "miruro", "nineanime", "animekai",
	"18anime",
}

// Provider groups. The EN failover chain serves GroupEN providers; GroupAdult
// providers (18+) are served by a SEPARATE orchestrator on /anime18/* routes
// and are NEVER part of the EN chain (no 18+ leakage into OurEnglish).
const (
	GroupEN    = "en"
	GroupAdult = "adult"
)

// providerGroups assigns each known provider to its group. Group is INTRINSIC
// to the provider (a hentai source is always adult) — it is NOT operator-
// editable via YAML, so a typo can't move 18anime into the EN chain. Absent
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

// BaseURLOf returns the DB-configured mirror base URL for a provider (empty when
// unset — callers fall back to the provider's built-in default).
func (p ProvidersConfig) BaseURLOf(name string) string {
	if m, ok := p.load()[name]; ok {
		return m.BaseURL
	}
	return ""
}

// providerEntry is the raw YAML shape. Enabled is a pointer so an omitted
// `enabled:` is distinguishable from an explicit `false` (we require it).
type providerEntry struct {
	Name             string  `yaml:"name"`
	Enabled          *bool   `yaml:"enabled"`
	Reason           string  `yaml:"reason"`
	Description      string  `yaml:"description"`
	Group            *string `yaml:"group"` // optional; if present MUST equal GroupOf(name)
	SupportsSub      *bool   `yaml:"supports_sub"`
	SupportsDub      *bool   `yaml:"supports_dub"`
	SupportsRaw      *bool   `yaml:"supports_raw"`
	SubDelivery      string  `yaml:"sub_delivery"`
	QualityCeiling   string  `yaml:"quality_ceiling"`
	PreferenceWeight *int    `yaml:"preference_weight"`
}

type providersFile struct {
	Providers []providerEntry `yaml:"providers"`
}

// ProvidersConfig is the resolved provider management config. Source is one of
// "file", "env-fallback" (file path set but missing), or "env".
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

// NewProvidersConfigForTest constructs a ProvidersConfig from a slice of
// ProviderMeta entries. Intended only for unit tests that need to drive the
// handler without a real YAML file.
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

// LoadProviders reads + validates the YAML provider config at path.
func LoadProviders(path string) (ProvidersConfig, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ProvidersConfig{}, fmt.Errorf("read providers file: %w", err)
	}
	var pf providersFile
	if err := yaml.Unmarshal(raw, &pf); err != nil {
		return ProvidersConfig{}, fmt.Errorf("parse providers yaml: %w", err)
	}
	known := make(map[string]bool, len(KnownProviders))
	for _, n := range KnownProviders {
		known[n] = true
	}
	derefBool := func(p *bool) bool { return p != nil && *p }
	metas := make(map[string]ProviderMeta, len(pf.Providers))
	for _, e := range pf.Providers {
		if e.Name == "" {
			return ProvidersConfig{}, fmt.Errorf("providers file: entry with empty name")
		}
		if !known[e.Name] {
			return ProvidersConfig{}, fmt.Errorf("providers file: unknown provider %q (known: %v)", e.Name, KnownProviders)
		}
		if e.Enabled == nil {
			return ProvidersConfig{}, fmt.Errorf("providers file: provider %q missing required 'enabled' field", e.Name)
		}
		if _, dup := metas[e.Name]; dup {
			return ProvidersConfig{}, fmt.Errorf("providers file: duplicate provider %q", e.Name)
		}
		// Group is intrinsic; an explicit YAML group must match the canonical
		// assignment (defense against typo-moving 18anime into the EN chain).
		if e.Group != nil && *e.Group != GroupOf(e.Name) {
			return ProvidersConfig{}, fmt.Errorf("providers file: provider %q group %q != intrinsic %q", e.Name, *e.Group, GroupOf(e.Name))
		}
		subDelivery := e.SubDelivery
		if subDelivery == "" {
			subDelivery = "hard"
		}
		weight := 0
		if e.PreferenceWeight != nil {
			weight = *e.PreferenceWeight
		}
		// The YAML shape (retired 2026-06-17 — dormant fallback only) has no
		// "degraded" concept: bool enabled maps to enabled|disabled.
		status := StatusEnabled
		if !*e.Enabled {
			status = StatusDisabled
		}
		metas[e.Name] = ProviderMeta{
			Name:             e.Name,
			Status:           status,
			Reason:           e.Reason,
			Description:      e.Description,
			Group:            GroupOf(e.Name),
			SupportsSub:      derefBool(e.SupportsSub),
			SupportsDub:      derefBool(e.SupportsDub),
			SupportsRaw:      derefBool(e.SupportsRaw),
			SubDelivery:      subDelivery,
			QualityCeiling:   e.QualityCeiling,
			PreferenceWeight: weight,
		}
	}
	return newProvidersConfig(metas, "file"), nil
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
