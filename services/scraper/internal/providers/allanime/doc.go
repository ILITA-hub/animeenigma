// Package allanime is the third live EN-sub domain.Provider for the
// scraper microservice. It targets AllAnime's GraphQL persisted-query API
// at api.allanime.day, lifted from services/catalog/internal/parser/allanime/
// per CONTEXT.md D1 (copy-with-adaptation, NOT a move).
//
// Lift rationale (CONTEXT.md D1):
//
//   - Catalog-side parser serves workstream raw-jp (original Japanese audio,
//     RU subs). Scraper-side provider serves EN-sub catalog for the failover
//     pool. The consumption contracts diverge — different category, different
//     server preferences — so a shared lib creates a new package boundary for
//     three GraphQL strings.
//   - Catalog-side parser uses *http.Client. Scraper-side provider MUST use
//     *domain.BaseHTTPClient (per SCRAPER-FOUND-06 cross-cutting NF) — that
//     alone would require a parallel HTTP path.
//   - The catalog-side parser's API will keep evolving for raw-jp use cases
//     (subtitle extraction, etc.) independently of the scraper's needs.
//
// Upstream contract:
//
//   - GraphQL Apollo APQ at https://api.allanime.day/api
//   - translationType=sub (post commit 102c590 — switched from "raw")
//   - Required Referer: https://allmanga.to
//   - Persisted-query SHAs captured 2026-05-19; auto-registered via APQ on
//     cache miss, so we never need to chase SHA rotations.
//   - sourceUrls may be returned directly (legacy) or AES-CTR encrypted in
//     a `tobeparsed` blob (current — see decrypt.go).
//
// Failure modes:
//
//   - upstream 5xx / timeout / DNS  → wrap with domain.WrapProviderDown
//   - persisted-query SHA stale     → wrap with domain.WrapExtractFailed
//   - empty search results          → return domain.ErrNotFound
//   - empty sourceUrls              → return domain.WrapExtractFailed
//
// SCRAPER-HEAL-25 — lifted into the scraper failover pool 2026-05-19.
package allanime
