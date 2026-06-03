package config

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// KnownProviders is the canonical set of scraper provider names that may appear
// in scraper-providers.yaml. Anything else is a typo and fails validation.
// Must match the registration names in cmd/scraper-api/main.go.
var KnownProviders = []string{
	"gogoanime", "animepahe", "allanime", "animefever", "miruro", "nineanime", "animekai",
}

// ProviderMeta is one resolved provider entry.
type ProviderMeta struct {
	Name        string
	Enabled     bool
	Reason      string
	Description string
}

// providerEntry is the raw YAML shape. Enabled is a pointer so an omitted
// `enabled:` is distinguishable from an explicit `false` (we require it).
type providerEntry struct {
	Name        string `yaml:"name"`
	Enabled     *bool  `yaml:"enabled"`
	Reason      string `yaml:"reason"`
	Description string `yaml:"description"`
}

type providersFile struct {
	Providers []providerEntry `yaml:"providers"`
}

// ProvidersConfig is the resolved provider management config. Source is one of
// "file", "env-fallback" (file path set but missing), or "env".
type ProvidersConfig struct {
	metas  map[string]ProviderMeta
	Source string
}

// IsEnabled reports whether a provider is enabled. Absent names default to
// enabled — forgetting to list a provider never silently disables it.
func (p ProvidersConfig) IsEnabled(name string) bool {
	if m, ok := p.metas[name]; ok {
		return m.Enabled
	}
	return true
}

// Meta returns the metadata for a provider (zero value if absent).
func (p ProvidersConfig) Meta(name string) ProviderMeta { return p.metas[name] }

// ProviderRow is a flattened row for metric emission / display.
type ProviderRow struct {
	Name        string
	Enabled     bool
	Reason      string
	Description string
}

// Rows returns one row per candidate provider, in the given order.
func (p ProvidersConfig) Rows(candidates []string) []ProviderRow {
	rows := make([]ProviderRow, 0, len(candidates))
	for _, name := range candidates {
		m := p.metas[name]
		rows = append(rows, ProviderRow{
			Name:        name,
			Enabled:     p.IsEnabled(name),
			Reason:      m.Reason,
			Description: m.Description,
		})
	}
	return rows
}

// toDegradedConfig projects disabled providers into the existing
// DegradedProvidersConfig shape so main.go's IsDegraded checks work unchanged.
func (p ProvidersConfig) toDegradedConfig() DegradedProvidersConfig {
	m := make(map[string]bool)
	for name, meta := range p.metas {
		if !meta.Enabled {
			m[name] = true
		}
	}
	return DegradedProvidersConfig{Names: m}
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
		metas[e.Name] = ProviderMeta{
			Name:        e.Name,
			Enabled:     *e.Enabled,
			Reason:      e.Reason,
			Description: e.Description,
		}
	}
	return ProvidersConfig{metas: metas, Source: "file"}, nil
}

// providersFromDegraded builds a ProvidersConfig from the legacy degraded set
// (env fallback): every known provider is enabled unless degraded; no metadata.
func providersFromDegraded(d DegradedProvidersConfig, source string) ProvidersConfig {
	metas := make(map[string]ProviderMeta, len(KnownProviders))
	for _, name := range KnownProviders {
		metas[name] = ProviderMeta{Name: name, Enabled: !d.IsDegraded(name)}
	}
	return ProvidersConfig{metas: metas, Source: source}
}

// DisabledNames returns the sorted names of disabled providers (for logging).
func (p ProvidersConfig) DisabledNames() []string {
	out := []string{}
	for name, m := range p.metas {
		if !m.Enabled {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}
