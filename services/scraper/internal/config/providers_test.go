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

func TestLoadProviders_DuplicateName(t *testing.T) {
	path := writeTempYAML(t, "providers:\n  - { name: allanime, enabled: true }\n  - { name: allanime, enabled: false }\n")
	if _, err := LoadProviders(path); err == nil {
		t.Fatal("LoadProviders err = nil; want error on duplicate provider")
	}
}

func TestLoadProviders_MalformedYAML(t *testing.T) {
	path := writeTempYAML(t, "providers: [unclosed flow sequence\n")
	if _, err := LoadProviders(path); err == nil {
		t.Fatal("LoadProviders err = nil; want error on malformed yaml")
	}
}

// TestLoad_InvalidFile_FailsFast: a present-but-invalid providers file must
// fail the scraper at boot, NOT silently fall back to the env (spec: missing
// file = fallback; invalid file = fail-fast).
func TestLoad_InvalidFile_FailsFast(t *testing.T) {
	path := writeTempYAML(t, "providers:\n  - { name: bogus_provider, enabled: false }\n")
	t.Setenv("SCRAPER_PROVIDERS_FILE", path)
	if _, err := Load(); err == nil {
		t.Fatal("Load err = nil; want fail-fast on an invalid providers file")
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
