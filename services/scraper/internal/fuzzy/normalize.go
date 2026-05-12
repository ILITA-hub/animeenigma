// Package fuzzy hosts shared title-similarity helpers used by scraper
// providers when malsync.moe lacks coverage (e.g. Gogoanime/Anitaku as of
// 2026-05-12 per Phase 18 research).
//
// Originally lived inside services/scraper/internal/providers/animepahe/cache.go
// (Phase 16). Moved here as Phase 18 introduces a second consumer.
package fuzzy

import "strings"

// NormalizeTitle is a verbatim relocation of animepahe.cache.go::normalizeTitle.
// Lower-cases, collapses whitespace, and folds the "Season N" / "Nth Season" /
// "Part N" variants into a single canonical form so the Jaro-Winkler scorer in
// the fuzzy fallback can compare season-suffixed titles meaningfully.
//
// Per RESEARCH.md (Phase 16) Pitfall 5: real titles arrive as "Naruto" /
// "Naruto: Shippuuden" / "Vinland Saga Season 2" / "Vinland Saga: 2nd Season"
// — without normalization, the scorer drops below 0.85 for what a human would
// call a perfect match.
func NormalizeTitle(s string) string {
	s = strings.ToLower(s)
	// Common season suffix variants — fold to "season N".
	for _, pat := range []struct{ in, out string }{
		{" 2nd season", " season 2"},
		{" 3rd season", " season 3"},
		{" 4th season", " season 4"},
		{" 5th season", " season 5"},
		{" part 2", " season 2"},
		{" part 3", " season 3"},
		{" part 4", " season 4"},
		{" part 5", " season 5"},
	} {
		s = strings.ReplaceAll(s, pat.in, pat.out)
	}
	// Collapse separator punctuation into single spaces.
	s = strings.NewReplacer(":", " ", "·", " ", "—", " ", "–", " ", "-", " ").Replace(s)
	// Collapse runs of whitespace.
	s = strings.Join(strings.Fields(s), " ")
	return s
}
