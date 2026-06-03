# Scraper Provider Management (config file + Grafana) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the invisible `SCRAPER_DEGRADED_PROVIDERS` env kill-switch with a git-versioned `docker/scraper-providers.yaml` (enabled + reason + description per provider), reflect it into Prometheus metrics, and render an admin Grafana dashboard (management table + disconnect-history timeline).

**Architecture:** The scraper loads the YAML at boot and projects it into the existing `DegradedProvidersConfig` (so registration logic is untouched) plus a new metadata struct. Two new gauges (`provider_enabled`, `provider_info`) are emitted for ALL known providers — so disabled ones stay visible. A new auto-provisioned Grafana dashboard reads those plus the existing `provider_health_up`. If the file is absent, it falls back to the old env var (zero-break migration).

**Tech Stack:** Go (scraper service), `gopkg.in/yaml.v3` (already in module graph), Prometheus `promauto` gauges (`libs/metrics`), Grafana dashboard JSON (auto-provisioned from `infra/grafana/dashboards/`).

**Spec:** `docs/superpowers/specs/2026-06-03-scraper-provider-management-grafana-design.md`

---

## File Structure

- **Create** `services/scraper/internal/config/providers.go` — YAML loader + `ProvidersConfig` type + `KnownProviders` + validation + projections (`toDegradedConfig`, `Rows`).
- **Create** `services/scraper/internal/config/providers_test.go` — loader + projection tests.
- **Modify** `services/scraper/internal/config/config.go` — add `Providers ProvidersConfig` field; resolve file-vs-env in `Load()`; add `SCRAPER_PROVIDERS_FILE`.
- **Modify** `services/scraper/internal/config/config_test.go` — add a `Load()` resolution test.
- **Modify** `libs/metrics/provider.go` — add `ProviderEnabled` + `ProviderInfo` gauges.
- **Modify** `services/scraper/cmd/scraper-api/main.go` — emit the two gauges for `candidateProviders` after registration.
- **Create** `docker/scraper-providers.yaml` — seed (current reality).
- **Modify** `docker/docker-compose.yml` — mount the file into `scraper`; add `SCRAPER_PROVIDERS_FILE`; mark `SCRAPER_DEGRADED_PROVIDERS` deprecated/fallback.
- **Create** `infra/grafana/dashboards/scraper-providers.json` — dashboard (table + state-timeline).

---

## Task 1: Provider config loader (`internal/config/providers.go`)

**Files:**
- Create: `services/scraper/internal/config/providers.go`
- Test: `services/scraper/internal/config/providers_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/scraper/internal/config/providers_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTempYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "providers.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp yaml: %v", err)
	}
	return p
}

func TestLoadProviders_ValidFile(t *testing.T) {
	path := writeTempYAML(t, `
providers:
  - { name: allanime, enabled: true }
  - name: animepahe
    enabled: false
    reason: "Cloudflare challenge"
    description: "moved to Cloudflare; sidecar can't solve it"
`)
	pc, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders err = %v; want nil", err)
	}
	if pc.Source != "file" {
		t.Errorf("Source = %q; want file", pc.Source)
	}
	if !pc.IsEnabled("allanime") {
		t.Errorf("allanime should be enabled")
	}
	if pc.IsEnabled("animepahe") {
		t.Errorf("animepahe should be disabled")
	}
	// Unknown/absent provider defaults to enabled.
	if !pc.IsEnabled("miruro") {
		t.Errorf("absent provider miruro should default enabled")
	}
	if m := pc.Meta("animepahe"); m.Reason != "Cloudflare challenge" {
		t.Errorf("animepahe reason = %q; want Cloudflare challenge", m.Reason)
	}
	if got := pc.toDegradedConfig(); !got.IsDegraded("animepahe") || got.IsDegraded("allanime") {
		t.Errorf("toDegradedConfig wrong: animepahe must be degraded, allanime must not")
	}
}

func TestLoadProviders_UnknownName(t *testing.T) {
	path := writeTempYAML(t, "providers:\n  - { name: nope, enabled: false }\n")
	if _, err := LoadProviders(path); err == nil {
		t.Fatal("LoadProviders err = nil; want error on unknown provider")
	}
}

func TestLoadProviders_EnabledRequired(t *testing.T) {
	path := writeTempYAML(t, "providers:\n  - { name: allanime }\n")
	if _, err := LoadProviders(path); err == nil {
		t.Fatal("LoadProviders err = nil; want error when 'enabled' omitted")
	}
}

func TestLoadProviders_MissingFile(t *testing.T) {
	if _, err := LoadProviders("/no/such/file.yaml"); err == nil {
		t.Fatal("LoadProviders err = nil; want error on missing file")
	}
}

func TestProvidersFromDegraded_EnvFallback(t *testing.T) {
	d := parseDegradedProviders("animepahe,gogoanime")
	pc := providersFromDegraded(d, "env")
	if pc.IsEnabled("animepahe") || pc.IsEnabled("gogoanime") {
		t.Errorf("degraded providers must be disabled")
	}
	if !pc.IsEnabled("allanime") {
		t.Errorf("non-degraded provider must be enabled")
	}
	if pc.Source != "env" {
		t.Errorf("Source = %q; want env", pc.Source)
	}
}

func TestRows_OrderAndFields(t *testing.T) {
	path := writeTempYAML(t, `
providers:
  - name: animepahe
    enabled: false
    reason: "CF"
    description: "d"
`)
	pc, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	rows := pc.Rows([]string{"allanime", "animepahe"})
	if len(rows) != 2 || rows[0].Name != "allanime" || rows[1].Name != "animepahe" {
		t.Fatalf("Rows order wrong: %+v", rows)
	}
	if !rows[0].Enabled {
		t.Errorf("allanime row should be enabled")
	}
	if rows[1].Enabled || rows[1].Reason != "CF" || rows[1].Description != "d" {
		t.Errorf("animepahe row wrong: %+v", rows[1])
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/scraper && go test ./internal/config/ -run 'TestLoadProviders|TestProvidersFromDegraded|TestRows' -count=1`
Expected: FAIL — `undefined: LoadProviders` (and the other symbols).

- [ ] **Step 3: Write the implementation**

Create `services/scraper/internal/config/providers.go`:

```go
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
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/scraper && go test ./internal/config/ -run 'TestLoadProviders|TestProvidersFromDegraded|TestRows' -count=1`
Expected: PASS (6 tests).

- [ ] **Step 5: Tidy module (promote yaml.v3 to direct) and commit**

Run: `cd services/scraper && go mod tidy && go build ./...`
Expected: build OK; `gopkg.in/yaml.v3` moves from indirect to a direct require in `go.mod`.

```bash
git add services/scraper/internal/config/providers.go services/scraper/internal/config/providers_test.go services/scraper/go.mod services/scraper/go.sum
git commit -m "feat(scraper): provider config loader (scraper-providers.yaml) — ISS-023"
```

---

## Task 2: Resolve file-vs-env in `config.Load()`

**Files:**
- Modify: `services/scraper/internal/config/config.go` (struct field + `Load()` resolution + `SCRAPER_PROVIDERS_FILE`)
- Test: `services/scraper/internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

Append to `services/scraper/internal/config/config_test.go`:

```go
func TestLoad_ProvidersFile_WinsOverEnv(t *testing.T) {
	path := writeTempYAML(t, `
providers:
  - { name: animepahe, enabled: false, reason: "CF", description: "d" }
  - { name: allanime, enabled: true }
`)
	t.Setenv("SCRAPER_PROVIDERS_FILE", path)
	t.Setenv("SCRAPER_DEGRADED_PROVIDERS", "miruro") // must be ignored when file present

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load err = %v; want nil", err)
	}
	if cfg.Providers.Source != "file" {
		t.Errorf("Providers.Source = %q; want file", cfg.Providers.Source)
	}
	if !cfg.DegradedProviders.IsDegraded("animepahe") {
		t.Errorf("animepahe must be degraded (from file)")
	}
	if cfg.DegradedProviders.IsDegraded("miruro") {
		t.Errorf("miruro must NOT be degraded — env ignored when file present")
	}
}

func TestLoad_NoProvidersFile_FallsBackToEnv(t *testing.T) {
	t.Setenv("SCRAPER_PROVIDERS_FILE", "")
	t.Setenv("SCRAPER_DEGRADED_PROVIDERS", "gogoanime,animepahe")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load err = %v; want nil", err)
	}
	if cfg.Providers.Source != "env" {
		t.Errorf("Providers.Source = %q; want env", cfg.Providers.Source)
	}
	if !cfg.DegradedProviders.IsDegraded("gogoanime") || !cfg.DegradedProviders.IsDegraded("animepahe") {
		t.Errorf("env degraded set not applied")
	}
}

func TestLoad_ProvidersFileMissing_FallsBackWithSource(t *testing.T) {
	t.Setenv("SCRAPER_PROVIDERS_FILE", "/no/such/providers.yaml")
	t.Setenv("SCRAPER_DEGRADED_PROVIDERS", "nineanime")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load err = %v; want nil (missing file must fall back, not fail)", err)
	}
	if cfg.Providers.Source != "env-fallback" {
		t.Errorf("Providers.Source = %q; want env-fallback", cfg.Providers.Source)
	}
	if !cfg.DegradedProviders.IsDegraded("nineanime") {
		t.Errorf("env fallback degraded set not applied")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/scraper && go test ./internal/config/ -run TestLoad_Providers -count=1`
Expected: FAIL — `cfg.Providers undefined` (field doesn't exist yet).

- [ ] **Step 3: Add the `Providers` field to the `Config` struct**

In `services/scraper/internal/config/config.go`, add to the `Config` struct (right after `ProviderTimeout time.Duration`):

```go
	// Providers is the resolved provider-management config (scraper-providers.yaml,
	// or env fallback). Source of truth for enable/disable + reason/description.
	Providers ProvidersConfig
```

- [ ] **Step 4: Replace the inline `DegradedProviders` literal with file-vs-env resolution**

In `Load()`, **remove** this line from the `cfg := &Config{...}` literal:

```go
		DegradedProviders: parseDegradedProviders(getEnv("SCRAPER_DEGRADED_PROVIDERS", "")),
```

Then, immediately **after** the `cfg := &Config{...}` literal closes (before the existing URL-validation blocks), insert:

```go
	// Provider management config: scraper-providers.yaml is the source of truth.
	// Falls back to the legacy SCRAPER_DEGRADED_PROVIDERS env when the file path
	// is unset or the file is missing (zero-break migration). ISS-023.
	if providersPath := getEnv("SCRAPER_PROVIDERS_FILE", ""); providersPath != "" {
		if _, statErr := os.Stat(providersPath); statErr == nil {
			pc, err := LoadProviders(providersPath)
			if err != nil {
				return nil, fmt.Errorf("scraper providers file: %w", err)
			}
			cfg.Providers = pc
			cfg.DegradedProviders = pc.toDegradedConfig()
		} else {
			cfg.DegradedProviders = parseDegradedProviders(getEnv("SCRAPER_DEGRADED_PROVIDERS", ""))
			cfg.Providers = providersFromDegraded(cfg.DegradedProviders, "env-fallback")
		}
	} else {
		cfg.DegradedProviders = parseDegradedProviders(getEnv("SCRAPER_DEGRADED_PROVIDERS", ""))
		cfg.Providers = providersFromDegraded(cfg.DegradedProviders, "env")
	}
```

(`os` and `fmt` are already imported in `config.go`.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `cd services/scraper && go test ./internal/config/ -count=1`
Expected: PASS (all config tests, including the three new ones).

- [ ] **Step 6: Commit**

```bash
git add services/scraper/internal/config/config.go services/scraper/internal/config/config_test.go
git commit -m "feat(scraper): resolve provider config from file with env fallback"
```

---

## Task 3: Add `provider_enabled` + `provider_info` metrics

**Files:**
- Modify: `libs/metrics/provider.go`
- Test: `libs/metrics/provider_test.go` (create if absent)

- [ ] **Step 1: Write the failing test**

Create (or append to) `libs/metrics/provider_test.go`:

```go
package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestProviderEnabledAndInfo_Exist(t *testing.T) {
	ProviderEnabled.WithLabelValues("animepahe").Set(0)
	ProviderEnabled.WithLabelValues("allanime").Set(1)
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("animepahe")); got != 0 {
		t.Errorf("provider_enabled{animepahe} = %v; want 0", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("allanime")); got != 1 {
		t.Errorf("provider_enabled{allanime} = %v; want 1", got)
	}

	ProviderInfo.WithLabelValues("animepahe", "Cloudflare challenge", "moved to CF").Set(1)
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("animepahe", "Cloudflare challenge", "moved to CF")); got != 1 {
		t.Errorf("provider_info{...} = %v; want 1", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd libs/metrics && go test ./... -run TestProviderEnabledAndInfo_Exist -count=1`
Expected: FAIL — `undefined: ProviderEnabled` / `ProviderInfo`.

- [ ] **Step 3: Add the gauges**

In `libs/metrics/provider.go`, inside the existing `var ( ... )` block (after `ProviderProbeLastTick`), add:

```go
	// ProviderEnabled is the config-driven management gauge: 1 = enabled
	// (registered in the failover chain), 0 = disabled. Emitted for ALL known
	// providers so disabled ones remain visible in Grafana. Source:
	// scraper-providers.yaml. ISS-023.
	ProviderEnabled = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_enabled",
			Help: "Whether a scraper provider is enabled in the failover chain (1=enabled, 0=disabled), per scraper-providers.yaml",
		},
		[]string{"provider"},
	)

	// ProviderInfo is an info-style gauge (always 1) carrying per-provider
	// management metadata (reason, description) for the Grafana table. Values
	// change only on a config edit + scraper restart, so cardinality is bounded
	// (~7 providers). ISS-023.
	ProviderInfo = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "provider_info",
			Help: "Info gauge (always 1) exposing per-provider management metadata (reason, description) for Grafana",
		},
		[]string{"provider", "reason", "description"},
	)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd libs/metrics && go test ./... -run TestProviderEnabledAndInfo_Exist -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add libs/metrics/provider.go libs/metrics/provider_test.go
git commit -m "feat(metrics): provider_enabled + provider_info gauges for provider management"
```

---

## Task 4: Emit the metrics in `main.go`

**Files:**
- Modify: `services/scraper/cmd/scraper-api/main.go`

- [ ] **Step 1: Add the emission loop**

In `services/scraper/cmd/scraper-api/main.go`, locate the wiring-invariant block that ends after the `candidateProviders` slice is finalized (the `candidateProviders := []string{...}` + the `if cfg.AnimeKai.Enabled { candidateProviders = append(...) }` lines near line 508). **After** that `if cfg.AnimeKai.Enabled { ... }` block (and after the existing `expectedProviders`/invariant `Fatalw` check), insert:

```go
	// ISS-023: reflect the provider-management config into Prometheus so the
	// Grafana dashboard shows EVERY provider (enabled and disabled) with its
	// reason/description. Disabled providers are not Register()-ed, so without
	// this they would vanish from all metrics.
	for _, row := range cfg.Providers.Rows(candidateProviders) {
		enabled := 0.0
		if row.Enabled {
			enabled = 1.0
		}
		metrics.ProviderEnabled.WithLabelValues(row.Name).Set(enabled)
		metrics.ProviderInfo.WithLabelValues(row.Name, row.Reason, row.Description).Set(1)
	}
	log.Infow("provider management config loaded",
		"source", cfg.Providers.Source,
		"disabled", cfg.Providers.DisabledNames(),
	)
```

(`metrics` and `log` are already in scope in `main.go`.)

- [ ] **Step 2: Build to verify it compiles**

Run: `cd services/scraper && go build ./...`
Expected: build OK.

- [ ] **Step 3: Run the full scraper + metrics suites**

Run: `cd services/scraper && go test ./... -count=1` and `cd libs/metrics && go test ./... -count=1`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add services/scraper/cmd/scraper-api/main.go
git commit -m "feat(scraper): emit provider_enabled/provider_info for all providers at boot"
```

---

## Task 5: Seed config file `docker/scraper-providers.yaml`

**Files:**
- Create: `docker/scraper-providers.yaml`

- [ ] **Step 1: Create the file**

Create `docker/scraper-providers.yaml`:

```yaml
# Scraper EN-provider management — SOURCE OF TRUTH (replaces SCRAPER_DEGRADED_PROVIDERS).
#
# Edit this file, then: docker compose -f docker/docker-compose.yml restart scraper
# (no rebuild). `enabled: false` removes a provider from the failover chain with
# zero per-request cost. `reason` (short) + `description` (full why) render in the
# Grafana "Scraper / Provider Management" dashboard. Git history = who/when audit;
# the dashboard state-timeline = when each provider was disconnected/reconnected.
#
# Every entry REQUIRES name + enabled. Known names: gogoanime, animepahe, allanime,
# animefever, miruro, nineanime, animekai. An unknown name fails the scraper at boot.
providers:
  - { name: allanime,  enabled: true }
  - { name: animefever, enabled: true }
  - { name: miruro,    enabled: true }
  - { name: nineanime, enabled: true }

  - name: animepahe
    enabled: false
    reason: "Cloudflare challenge"
    description: >
      animepahe.pw migrated DDoS-Guard -> Cloudflare managed challenge; the
      stealth-Chromium sidecar can't solve it (0% solve rate). See ISS-023.
      Disabled 2026-06-03.

  - name: gogoanime
    enabled: false
    reason: "Platform rebrand"
    description: >
      anitaku.to migrated to a different platform the parser can't handle.
      Disabled 2026-05-13.
```

- [ ] **Step 2: Validate it parses against the loader**

Run:
```bash
cd services/scraper && cat > /tmp/providers_smoke_test.go <<'EOF'
package config
import "testing"
func TestSeedFileParses(t *testing.T) {
	pc, err := LoadProviders("../../../../docker/scraper-providers.yaml")
	if err != nil { t.Fatalf("seed file: %v", err) }
	if pc.IsEnabled("animepahe") || pc.IsEnabled("gogoanime") { t.Fatal("animepahe/gogoanime should be disabled") }
	if !pc.IsEnabled("allanime") || !pc.IsEnabled("miruro") { t.Fatal("allanime/miruro should be enabled") }
}
EOF
cp /tmp/providers_smoke_test.go internal/config/zz_seed_smoke_test.go
go test ./internal/config/ -run TestSeedFileParses -count=1
rm internal/config/zz_seed_smoke_test.go
```
Expected: PASS, then the temp test file is removed.

- [ ] **Step 3: Commit**

```bash
git add docker/scraper-providers.yaml
git commit -m "feat(scraper): seed scraper-providers.yaml (animepahe/gogoanime disabled w/ reasons)"
```

---

## Task 6: Mount the file + deprecate the env var (`docker-compose.yml`)

**Files:**
- Modify: `docker/docker-compose.yml` (the `scraper:` service block)

- [ ] **Step 1: Add the volume mount + env, update the comment**

In `docker/docker-compose.yml`, in the `scraper:` service:

1. Replace the `SCRAPER_DEGRADED_PROVIDERS` comment + line. Find:

```yaml
      # Global kill-switch: comma-separated list of provider names that should
      # NOT be registered with the orchestrator. Failover skips them with zero
      # per-request cost. Default disables gogoanime (anitaku.to migrated to
      # anineko.to — different platform, parser doesn't apply). Phase 27
      # (2026-05-19) revived animepahe via the stealth-Chromium sidecar at
      # services/animepahe-resolver/; animepahe is no longer in the default
      # degraded set. The env-override escape hatch is preserved — set
      # `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` in `docker/.env` to
      # re-disable on outage (e.g. animepahe.pw goes dark or stealth pins
      # rot). Flip off by removing names + `docker compose restart scraper`
      # (no rebuild needed).
      SCRAPER_DEGRADED_PROVIDERS: ${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}
```

Replace with:

```yaml
      # Provider management is now config-file driven via SCRAPER_PROVIDERS_FILE
      # below (./scraper-providers.yaml). That file is the SOURCE OF TRUTH for
      # enable/disable + reason/description, rendered in the Grafana
      # "Scraper / Provider Management" dashboard. ISS-023.
      #
      # SCRAPER_DEGRADED_PROVIDERS is DEPRECATED — used ONLY as a fallback when
      # the providers file is absent (zero-break migration). Do not rely on it.
      SCRAPER_DEGRADED_PROVIDERS: ${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}
      # Source of truth for provider management (mounted read-only below).
      SCRAPER_PROVIDERS_FILE: /config/providers.yaml
```

2. Add a `volumes:` entry to the `scraper:` service (the scraper currently has no `volumes:` key — add one, placed right after the `environment:` block and before `ports:`):

```yaml
    volumes:
      - ./scraper-providers.yaml:/config/providers.yaml:ro
```

- [ ] **Step 2: Validate compose still parses**

Run: `docker compose -f docker/docker-compose.yml config >/dev/null && echo "compose OK"`
Expected: `compose OK` (no YAML/scheme errors).

- [ ] **Step 3: Commit**

```bash
git add docker/docker-compose.yml
git commit -m "chore(scraper): mount scraper-providers.yaml; deprecate SCRAPER_DEGRADED_PROVIDERS"
```

---

## Task 7: Grafana dashboard `scraper-providers.json`

**Files:**
- Create: `infra/grafana/dashboards/scraper-providers.json`

- [ ] **Step 1: Create the dashboard JSON**

Create `infra/grafana/dashboards/scraper-providers.json`:

```json
{
  "uid": "scraper-provider-management",
  "title": "Scraper / Provider Management",
  "schemaVersion": 38,
  "version": 1,
  "editable": true,
  "tags": ["scraper", "providers"],
  "time": { "from": "now-30d", "to": "now" },
  "refresh": "1m",
  "templating": { "list": [] },
  "panels": [
    {
      "id": 1,
      "type": "table",
      "title": "Provider Management",
      "gridPos": { "h": 9, "w": 24, "x": 0, "y": 0 },
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "provider_enabled", "format": "table", "instant": true },
        { "refId": "B", "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "max by (provider) (provider_health_up)", "format": "table", "instant": true },
        { "refId": "C", "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "provider_info", "format": "table", "instant": true }
      ],
      "transformations": [
        { "id": "joinByField", "options": { "byField": "provider", "mode": "outer" } },
        {
          "id": "organize",
          "options": {
            "excludeByName": { "Time": true, "Time 1": true, "Time 2": true, "Time 3": true, "__name__": true, "__name__ 1": true, "__name__ 2": true, "__name__ 3": true, "job": true, "job 1": true, "job 2": true, "job 3": true, "instance": true, "instance 1": true, "instance 2": true, "instance 3": true },
            "renameByName": { "provider": "Provider", "Value #A": "Enabled", "Value #B": "Live Up", "reason": "Reason", "description": "Description" },
            "indexByName": { "Provider": 0, "Enabled": 1, "Live Up": 2, "Reason": 3, "Description": 4 }
          }
        }
      ],
      "fieldConfig": {
        "defaults": { "custom": { "align": "left" } },
        "overrides": [
          { "matcher": { "id": "byName", "options": "Enabled" }, "properties": [ { "id": "mappings", "value": [ { "type": "value", "options": { "1": { "text": "🟢 Enabled", "color": "green" }, "0": { "text": "⚪ Disabled", "color": "red" } } } ] }, { "id": "custom.cellOptions", "value": { "type": "color-text" } } ] },
          { "matcher": { "id": "byName", "options": "Live Up" }, "properties": [ { "id": "mappings", "value": [ { "type": "value", "options": { "1": { "text": "🟢 Up", "color": "green" }, "0": { "text": "🔴 Down", "color": "red" } } } ] }, { "id": "custom.cellOptions", "value": { "type": "color-text" } } ] },
          { "matcher": { "id": "byName", "options": "Description" }, "properties": [ { "id": "custom.width", "value": 520 } ] }
        ]
      }
    },
    {
      "id": 2,
      "type": "state-timeline",
      "title": "Connect / Disconnect History",
      "gridPos": { "h": 9, "w": 24, "x": 0, "y": 9 },
      "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" },
      "targets": [
        { "refId": "A", "datasource": { "type": "prometheus", "uid": "${DS_PROMETHEUS}" }, "expr": "provider_enabled", "legendFormat": "{{provider}}", "format": "time_series" }
      ],
      "options": { "mergeValues": true, "showValue": "never", "alignValue": "left", "rowHeight": 0.9, "legend": { "displayMode": "list", "placement": "bottom" } },
      "fieldConfig": {
        "defaults": {
          "mappings": [ { "type": "value", "options": { "1": { "text": "Enabled", "color": "green" }, "0": { "text": "Disabled", "color": "red" } } ],
          "color": { "mode": "thresholds" },
          "thresholds": { "mode": "absolute", "steps": [ { "value": null, "color": "red" }, { "value": 1, "color": "green" } ] }
        },
        "overrides": []
      }
    }
  ]
}
```

- [ ] **Step 2: Validate JSON**

Run: `python3 -m json.tool infra/grafana/dashboards/scraper-providers.json >/dev/null && echo "json OK"`
Expected: `json OK`.

- [ ] **Step 3: Commit**

```bash
git add infra/grafana/dashboards/scraper-providers.json
git commit -m "feat(grafana): scraper provider management dashboard (table + history timeline)"
```

---

## Task 8: Deploy + verify end-to-end

**Files:** none (deploy + verification).

- [ ] **Step 1: Remove the host env override (file now supersedes it)**

The host `docker/.env` has `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` from the ISS-022 mitigation. The file now carries this. Remove that line from `docker/.env` (host-only, not committed). Verify:

Run: `grep -n SCRAPER_DEGRADED_PROVIDERS docker/.env || echo "removed"`
Expected: `removed` (or delete the line if present).

- [ ] **Step 2: Redeploy the scraper**

Run: `make redeploy-scraper`
Expected: build + restart OK, scraper healthy. (`redeploy.sh` uses `--no-deps`, so the unhealthy animepahe-resolver does not block it.)

- [ ] **Step 3: Verify the metrics are emitted for ALL providers (incl. disabled)**

Run: `curl -s http://localhost:8088/metrics | grep -E '^provider_(enabled|info)'`
Expected: `provider_enabled{provider="animepahe"} 0`, `provider_enabled{provider="allanime"} 1` (etc.), and `provider_info{provider="animepahe",reason="Cloudflare challenge",description="..."} 1`.

- [ ] **Step 4: Verify the source + disabled set logged**

Run: `docker compose -f docker/docker-compose.yml logs scraper 2>&1 | grep "provider management config loaded"`
Expected: a line with `"source": "file"` and `"disabled": ["animepahe","gogoanime"]`.

- [ ] **Step 5: Verify the EN player still resolves (regression check)**

Run:
```bash
curl -s -m 20 "http://localhost:8000/api/anime/8dd0c714-2760-44c8-83d1-dbe090d6cd9f/scraper/episodes" -o /dev/null -w "episodes HTTP %{http_code} in %{time_total}s\n"
```
Expected: `HTTP 200` in well under 1s (allanime first; animepahe/gogoanime skipped via the file).

- [ ] **Step 6: Verify the Grafana dashboard (in-browser, per DS-NF-06 standing rule)**

Open `https://animeenigma.ru/admin/grafana` → dashboard **"Scraper / Provider Management"** (uid `scraper-provider-management`). Confirm:
- The **table** lists all providers with Enabled (🟢/⚪), Live Up (🟢/🔴 or empty for disabled), Reason, Description columns populated.
- The **state-timeline** renders (will be mostly green/enabled with animepahe+gogoanime red; history accrues going forward).

If a table column is mis-joined (Grafana table joins can need a tweak), adjust the `organize`/`joinByField` transform in the Grafana UI, then export JSON (Share → Export → Save to file) back over `infra/grafana/dashboards/scraper-providers.json`.

- [ ] **Step 7: Final commit (if the dashboard JSON was tweaked in Step 6)**

```bash
git add infra/grafana/dashboards/scraper-providers.json
git commit -m "fix(grafana): tweak provider management table transforms after in-browser check"
```

---

## Self-Review Notes

- **Spec coverage:** config file (T5) · loader + validation (T1) · env fallback (T2) · metrics (T3) · emission for all providers incl. disabled (T4) · compose mount + deprecate env (T6) · Grafana table + timeline (T7) · migration/removal of env override (T8). All §4 components mapped.
- **No frontend/gateway tasks** — intentional (Grafana-only display, per spec §2).
- **Back-compat:** the `DegradedProviders.IsDegraded` checks in `main.go` registration are unchanged — the file just feeds the set (T2), so the failover/registration path and the ISS-022 per-provider budget are untouched.
- **Type consistency:** `ProvidersConfig`, `ProviderMeta`, `ProviderRow`, `LoadProviders`, `providersFromDegraded`, `toDegradedConfig`, `Rows`, `IsEnabled`, `Meta`, `DisabledNames`, `Source` used consistently across T1/T2/T4. Metrics `ProviderEnabled`/`ProviderInfo` consistent across T3/T4. Provider name set (`KnownProviders`) matches `candidateProviders` in `main.go`.
```
