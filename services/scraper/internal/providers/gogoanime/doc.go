// Package gogoanime implements the Gogoanime/Anitaku scraper provider
// (domain.Provider). Phase 18 of the v3.0 milestone.
//
// Naming: backend package slug is "gogoanime" (stable across mirror
// rebrands — Gogoanime has rotated 5+ times in 18 months per
// REQUIREMENTS.md). User-facing display label is "Anitaku" (matches the
// brand on the current mirror anitaku.to). Frontend resolves the display
// label inside capitalizeProvider('gogoanime') -> 'Anitaku'.
//
// Pivot rationale (2026-05-12): the 9anime mirror chain (9anime.to ->
// aniwave.to -> kaido.to) is unreachable; anitaku.to survives as the
// only EN provider with a clean per-episode server list. See
// .planning/phases/18-9anime/18-RESEARCH.md section "Mirror Viability".
//
// SCRAPER-9ANI-01..06 are the requirement IDs implemented by this package
// (literal names retained per CONTEXT.md S4 — IDs survive the 9anime →
// Anitaku/Gogoanime pivot).
//
// Wave-2 of Phase 18 — Plan 18-02 introduces the live implementation that
// turns the Plan 18-01 RED-state test scaffolds GREEN. The provider builds
// on:
//
//   - Phase 16 — animepahe provider (analog template).
//   - Phase 15 — embeds.Registry (extractor seam).
//   - Phase 17 — health package canonical stage constants.
//   - Plan 18-01 — services/scraper/internal/fuzzy (shared JaroWinkler +
//     NormalizeTitle) and the 8 anitaku.to / embed-wrapper / malsync goldens
//     under services/scraper/testdata/gogoanime/.
//
// Note: anitaku.to does NOT sit behind DDoS-Guard (see RESEARCH.md
// "Mirror Viability"). The ddosguard.go file present in animepahe is
// INTENTIONALLY OMITTED here — referencing CONTEXT.md D2's optional file
// list for traceability.
package gogoanime
