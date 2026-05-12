// Package animekai implements the AnimeKai scraper provider (domain.Provider).
// Phase 19 of the v3.0 milestone.
//
// Naming: backend package slug is "animekai" (stable across mirror rebrands;
// the canonical mirror anikai.to is reached via a 301 from animekai.to as of
// 2026-05-12). User-facing display label is "AnimeKai" — slug and label match,
// no rebrand applied (CONTEXT.md §Frontend).
//
// ESCAPE HATCH (2026-05-12): All Provider methods currently return
// domain.ErrProviderDown because Phase 19 R&D did not converge on an in-house
// MegaUp token generator. AnimeKai officially announced shutdown on 2026-05-10
// (two days before Phase 19 research began). The convergence-probability
// assessment in .planning/phases/19-animekai-gated/19-RESEARCH.md §Convergence
// Probability Assessment scored the escape-hatch path at ~3-4 days versus
// ~14-21 days for full implementation; the escape hatch was selected to
// unblock Phase 20 (cutover).
//
// SCRAPER-KAI-01..04 (full implementation) and SCRAPER-KAI-07 (end-to-end
// failover verification with the flag on) are carried to the v3.1 milestone.
// SCRAPER-KAI-05 (env-flag wiring, default off) and SCRAPER-KAI-06 (escape
// hatch taken; sidecar stub returns HTTP 501) ship in Phase 19.
//
// SCRAPER-KAI-01..07 are the requirement IDs traced to this package.
//
// v3.1 fill-in surface: only the Provider method bodies in client.go and the
// /animekai-token handler body in docker/megacloud-extractor/server.js need
// to change. Wiring (main.go), config (config.go), docker plumbing, and docs
// are already in place.
package animekai
