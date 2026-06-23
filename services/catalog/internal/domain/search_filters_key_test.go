package domain

import "testing"

func intp(v int) *int         { return &v }
func f64p(v float64) *float64 { return &v }

// TestSearchFilters_CacheKey_DiscriminatesFilters asserts the cache key reflects
// the FULL canonical filter set, not just query+page. Two filter sets that
// differ only in a non-query/page facet (genre, year, kind, sort, ...) MUST
// produce distinct keys, otherwise filtered searches collide in the 15m search
// cache and return each other's stale results.
func TestSearchFilters_CacheKey_DiscriminatesFilters(t *testing.T) {
	base := SearchFilters{Query: "frieren", Page: 1, PageSize: 24, Sort: "score", Order: "desc"}

	// Each variant differs from base in exactly one facet.
	variants := map[string]SearchFilters{
		"base":      base,
		"genre":     mutate(base, func(f *SearchFilters) { f.GenreIDs = []string{"5"} }),
		"year":      mutate(base, func(f *SearchFilters) { f.Year = intp(2024) }),
		"yearFrom":  mutate(base, func(f *SearchFilters) { f.YearFrom = intp(2010) }),
		"yearTo":    mutate(base, func(f *SearchFilters) { f.YearTo = intp(2020) }),
		"season":    mutate(base, func(f *SearchFilters) { f.Season = "winter" }),
		"status":    mutate(base, func(f *SearchFilters) { f.Status = "ongoing" }),
		"kind":      mutate(base, func(f *SearchFilters) { f.Kinds = []string{"movie"} }),
		"providers": mutate(base, func(f *SearchFilters) { f.Providers = []string{"kodik"} }),
		"scoreMin":  mutate(base, func(f *SearchFilters) { f.ScoreMin = f64p(7.5) }),
		"sort":      mutate(base, func(f *SearchFilters) { f.Sort = "year" }),
		"order":     mutate(base, func(f *SearchFilters) { f.Order = "asc" }),
		"pageSize":  mutate(base, func(f *SearchFilters) { f.PageSize = 48 }),
		"page2":     mutate(base, func(f *SearchFilters) { f.Page = 2 }),
	}

	seen := map[string]string{}
	for name, f := range variants {
		k := f.CacheKey()
		if k == "" {
			t.Fatalf("%s: empty cache key", name)
		}
		if prev, ok := seen[k]; ok {
			t.Fatalf("cache key collision: %q and %q produced the same key %q", prev, name, k)
		}
		seen[k] = name
	}
}

// TestSearchFilters_CacheKey_StableAcrossSliceOrder asserts the key is invariant
// to the order of equal-but-reordered GenreIDs / Providers slices (the handler
// may produce them in any order). Same logical filter => same key => a cache hit.
func TestSearchFilters_CacheKey_StableAcrossSliceOrder(t *testing.T) {
	a := SearchFilters{Query: "q", Page: 1, GenreIDs: []string{"1", "2", "3"}, Providers: []string{"kodik", "animelib"}}
	b := SearchFilters{Query: "q", Page: 1, GenreIDs: []string{"3", "1", "2"}, Providers: []string{"animelib", "kodik"}}
	if a.CacheKey() != b.CacheKey() {
		t.Fatalf("reordered-but-equal filters produced different keys:\n a=%s\n b=%s", a.CacheKey(), b.CacheKey())
	}
}

// TestSearchFilters_CacheKey_Deterministic asserts the key is stable across
// calls (no map iteration / time nondeterminism leaking in).
func TestSearchFilters_CacheKey_Deterministic(t *testing.T) {
	f := SearchFilters{Query: "q", Page: 1, GenreIDs: []string{"2", "1"}, Year: intp(2024)}
	if f.CacheKey() != f.CacheKey() {
		t.Fatal("CacheKey is not deterministic")
	}
}

func mutate(base SearchFilters, fn func(*SearchFilters)) SearchFilters {
	f := base
	fn(&f)
	return f
}
