// Package gogoanime implements the Gogoanime/Anitaku scraper provider
// (domain.Provider). Backend slug "gogoanime"; user-facing display label
// "Anitaku".
//
// SCRAPER-9ANI-01..06 (literal requirement IDs retained per Phase 18
// CONTEXT.md S4 — IDs survive the 9anime → Anitaku/Gogoanime pivot).
//
// Wave-2 of Phase 18 — implementation arrives in Plan 18-02. This file
// exists in Plan 18-01 (Task 5) only so the *_test.go scaffolds compile
// against a real package. The provider impl (client.go, dto.go, malsync.go,
// cache.go) is added in Plan 18-02, at which point the RED-state Skips in
// the test files turn into real GREEN assertions.
//
// Builds on:
//
//   - Phase 16 — animepahe provider (analog template).
//   - Phase 15 — embeds.Registry (extractor seam).
//   - Phase 17 — health package canonical stage constants.
package gogoanime
