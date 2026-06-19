// Package nineanime — Phase 28 SCRAPER-HEAL-39. EN failover slot 6 (LAST).
//
// 9anime.me.uk is a brand-jacking WordPress 6.9.4 + dramastream theme
// instance — NOT the original 9anime/aniwave ecosystem (which is dead).
// It is the SIXTH and LAST provider in the EN failover chain:
//
//	gogoanime (degraded) → animepahe (degraded) → allanime (★ working) →
//	  animefever → miruro → nineanime (THIS PROVIDER) → animekai (gated stub)
//
// Per CONTEXT.md D2, 9anime.me.uk is EXPLICITLY ACCEPTED as a low-quality,
// last-resort source. Operator policy "as many providers as possible"
// overrides the natural "not-worth" verdict. The upstream has:
//
//   - Brand-jacking WP install (not the real 9anime)
//   - No `anime`/`series`/`episode` custom post types in WP REST API —
//     only stock `post,page,attachment` (but `subtype:"series"` IS exposed
//     via /wp-json/wp/v2/search)
//   - Default ?s= search is broken (returns 19 irrelevant episode-7 stubs)
//   - Some series absent from upstream catalog (e.g. Frieren Season 1
//     missing, only Season 2 present)
//   - Observed: episode-7 page found embedding episode-6 MP4 — data quality
//     is mid (documented in D2 as accepted trade-off)
//   - ~6-month half-life expected
//
// **Operator kill-switch:** set this provider's status to `disabled` (or
// `degraded`) in the catalog `scraper_providers` DB table — the single source
// of truth (AUTO-484) — to exclude it from main.go orchestrator registration
// (zero per-request cost). When the brand-jack rebrands or DMCAs, this is the
// response — no replanning needed.
//
// Lift Decision Log (CONTEXT.md D1 — copy-with-adaptation, per Phase 26):
//
//  1. doc.go        — Package doc + upstream contract notes (this file).
//  2. dto.go        — JSON DTO for the WP REST search response.
//  3. cache.go      — Mirror of allanime/cache.go: 4 key families + negative
//                      cache on WP search misses (CONTEXT.md D2 — operator
//                      kill assumed) + per-key TTLs from libs/cache.
//  4. client.go     — Adapt allanime/client.go's shape to 9anime's WP REST
//                      + WP-post HTML-scrape + 1anime.site MP4 extraction
//                      data path. NEVER catalog's HTTP client, NEVER bare
//                      *http.Client.
//  5. HTTP client   — Use *domain.BaseHTTPClient (per SCRAPER-FOUND-06 NF).
//
// Upstream data path (per 28-RESEARCH.md 9anime section + live recon
// 2026-05-20):
//
//	GET /wp-json/wp/v2/search?search=<term>&per_page=20  → JSON. Filter
//	                                                       subtype="series".
//	                                                       JaroWinkler ≥0.85
//	                                                       on title; year +
//	                                                       season-tag bonuses
//	                                                       for tie-breaks
//	                                                       (CONTEXT.md
//	                                                       Discretion #2).
//	GET /series/<slug>/                                 → HTML. Parse
//	                                                       <a class="ep-item"
//	                                                       data-number="N">
//	                                                       elements. Episode
//	                                                       ID = the FULL
//	                                                       canonical episode
//	                                                       URL (per Pitfall 5
//	                                                       — slugs are
//	                                                       irregular, some
//	                                                       have "hd-" prefix
//	                                                       and some do not).
//	GET <episode-url>                                   → HTML. Regex
//	                                                       extract <iframe
//	                                                       src="https://my.
//	                                                       1anime.site/..."
//	                                                       />.
//	GET <iframe-src> (Referer: 9anime.me.uk/)           → HTML. Regex
//	                                                       extract <source
//	                                                       src="videos/...
//	                                                       mp4"> and join
//	                                                       against iframe
//	                                                       host to build
//	                                                       absolute URL.
//	                                                       Returns
//	                                                       Stream{Sources:
//	                                                       [{Type:"mp4"}],
//	                                                       Headers:{Referer:
//	                                                       "https://my.1anime
//	                                                       .site/"}}.
//
// Pitfalls (per 28-RESEARCH.md):
//
//   - Pitfall 4: do NOT use the default WP `?s=` search (returns garbage).
//     Use the WP REST endpoint `/wp-json/wp/v2/search` instead and filter
//     `subtype:"series"` client-side.
//   - Pitfall 5: episode slugs are irregular. Some have an "hd-" prefix and
//     some do not. The recipe is to STORE the canonical episode `href` from
//     the series page anchor list — never reconstruct the URL by string
//     concatenation.
//   - Pitfall 6: my.1anime.site returns MP4, NOT HLS. Return
//     `Stream.Sources[0].Type = "mp4"`. Frontend supports MP4 via the
//     AnimePahe-via-Kwik precedent shipped Phase 16 (per CONTEXT.md `<risks>`).
//
// E2E target (per CONTEXT.md D6): Frieren Season 1 is ABSENT from the
// upstream catalog. Use Marriagetoxin episode 1 OR 7 as the canary instead.
// Frieren Season 2 IS present and is the testdata baseline used by the
// table-driven tests in client_test.go.
//
// Anti-Patterns (must NOT do):
//
//   - Hand-rolled http.Client (use BaseHTTPClient.Get/Do)
//   - iframe_url field on Stream (the iframe URL is INPUT to the MP4 walk,
//     never part of the returned Stream — see services/scraper/internal/
//     domain/provider.go top-of-file comment)
//   - Caching stream URLs longer than 5 minutes (signed-edge URLs expire;
//     1anime.site MP4 is on Cloudflare-fronted Engintron + has no signed
//     params we observed, so 5 min is the conservative cap)
//   - Copying allanime.Deps{Embeds} into nineanime.Deps — nineanime extracts
//     MP4 inline, does NOT dispatch via the embed registry. Dead field
//     dropped (REVIEW WR-04, dead-field footgun).
//
// References:
//   - .planning/phases/28-provider-expansion-r2/28-CONTEXT.md (D1-D7)
//   - .planning/phases/28-provider-expansion-r2/28-RESEARCH.md (9anime
//     section + Code Examples + Pitfalls 4-6)
//   - services/scraper/internal/providers/allanime/ — base template.
//   - services/scraper/internal/providers/animefever/ — Wave 1 sibling.
package nineanime
