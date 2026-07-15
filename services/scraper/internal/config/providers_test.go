package config

import "testing"

func TestAllProvidersEnabled_OfflineFallback(t *testing.T) {
	pc := allProvidersEnabled("default")
	for _, name := range KnownProviders {
		if !pc.IsEnabled(name) {
			t.Errorf("offline fallback: provider %q must be enabled", name)
		}
	}
	if pc.Source != "default" {
		t.Errorf("Source = %q; want default", pc.Source)
	}
}

func TestRows_OrderAndFields(t *testing.T) {
	pc := NewProvidersConfigForTest([]ProviderMeta{
		{Name: "allanime", Status: StatusEnabled},
		{Name: "animepahe", Status: StatusDisabled, Reason: "CF", Description: "d"},
	})
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

func TestGroupOf(t *testing.T) {
	if GroupOf("18anime") != GroupAdult {
		t.Fatalf("18anime group = %q, want adult", GroupOf("18anime"))
	}
	if GroupOf("allanime") != GroupEN {
		t.Fatalf("allanime group = %q, want en", GroupOf("allanime"))
	}
}

func TestKnownProvidersInGroup(t *testing.T) {
	adult := KnownProvidersInGroup(GroupAdult)
	if len(adult) != 1 || adult[0] != "18anime" {
		t.Fatalf("adult group = %v, want [18anime]", adult)
	}
	en := KnownProvidersInGroup(GroupEN)
	for _, n := range en {
		if n == "18anime" {
			t.Fatal("18anime must NOT be in the EN group")
		}
	}
}

func TestBrowserEngineNames(t *testing.T) {
	// Mixed roster: two browser-engine providers (out of KnownProviders order in
	// the map), the rest HTTP or engine-unset. Expect only the browser ones,
	// returned in canonical KnownProviders order.
	metas := map[string]ProviderMeta{
		"miruro":    {Name: "miruro", Status: StatusDegraded, Engine: EngineBrowser},
		"animepahe": {Name: "animepahe", Status: StatusDegraded, Engine: EngineBrowser},
		"okru":      {Name: "okru", Status: StatusEnabled, Engine: EngineHTTP},
		"allanime":  {Name: "allanime", Status: StatusEnabled}, // engine unset ⇒ http
	}
	got := newProvidersConfig(metas, "test").BrowserEngineNames()
	want := []string{"animepahe", "miruro"} // KnownProviders order: animepahe before miruro
	if len(got) != len(want) {
		t.Fatalf("BrowserEngineNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BrowserEngineNames()[%d] = %q, want %q (full: %v)", i, got[i], want[i], got)
		}
	}

	// The offline all-enabled fallback carries no engine info (DB-driven), so no
	// provider is browser — guards the catalog-down boot path from wrongly
	// granting the long budget to HTTP-path providers.
	if names := allProvidersEnabled("default").BrowserEngineNames(); len(names) != 0 {
		t.Fatalf("offline fallback BrowserEngineNames() = %v, want empty", names)
	}
}
