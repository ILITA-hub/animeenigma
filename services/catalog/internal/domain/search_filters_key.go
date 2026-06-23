package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CacheKey returns a stable cache key that reflects the FULL canonical filter
// set — not just Query+Page. The previous key (cache.KeySearchResults) keyed
// only on query+page, so two requests with the same query but different
// genre/year/kind/sort/provider/score filters collided in the 15m search cache
// and served each other's results.
//
// The key is deterministic and invariant to the order of the GenreIDs /
// Providers slices (the handler may emit them in any order), so logically-equal
// filters share a cache entry. The "search:" prefix mirrors cache.PrefixSearch
// (kept as a literal here to avoid a domain→libs/cache import edge).
func (f SearchFilters) CacheKey() string {
	genres := append([]string(nil), f.GenreIDs...)
	sort.Strings(genres)
	providers := append([]string(nil), f.Providers...)
	sort.Strings(providers)
	kinds := append([]string(nil), f.Kinds...)
	sort.Strings(kinds)

	var b strings.Builder
	fmt.Fprintf(&b, "q=%s;page=%d;size=%d", f.Query, f.Page, f.PageSize)
	fmt.Fprintf(&b, ";year=%s", intpStr(f.Year))
	fmt.Fprintf(&b, ";yearFrom=%s", intpStr(f.YearFrom))
	fmt.Fprintf(&b, ";yearTo=%s", intpStr(f.YearTo))
	fmt.Fprintf(&b, ";season=%s;status=%s;kinds=%s", f.Season, f.Status, strings.Join(kinds, ","))
	fmt.Fprintf(&b, ";scoreMin=%s", f64pStr(f.ScoreMin))
	fmt.Fprintf(&b, ";sort=%s;order=%s", f.Sort, f.Order)
	fmt.Fprintf(&b, ";genres=%s;providers=%s", strings.Join(genres, ","), strings.Join(providers, ","))

	sum := sha1.Sum([]byte(b.String()))
	return "search:" + hex.EncodeToString(sum[:])
}

func intpStr(p *int) string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d", *p)
}

func f64pStr(p *float64) string {
	if p == nil {
		return ""
	}
	// %g gives a stable, compact representation; trailing-zero noise is irrelevant
	// since the same *float64 value always formats identically.
	return fmt.Sprintf("%g", *p)
}
