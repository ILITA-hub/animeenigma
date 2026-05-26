// Package animefever — Phase 28 SCRAPER-HEAL-36. EN failover slot 4.
//
// AnimeFever (https://animefever.cc) is an English-subtitled streaming site
// backed by a PHP application with a Cloudflare-passive frontend (no JS
// challenge, no anti-bot). It is the FOURTH live EN provider in the failover
// chain:
//
//	gogoanime (degraded) → animepahe (degraded) → allanime (★ working) →
//	  animefever (NEW Phase 28) → miruro → nineanime → animekai (gated stub)
//
// Lift Decision Log (CONTEXT.md D1 — copy-with-adaptation, per Phase 26):
//
//  1. doc.go        — Package doc + upstream contract notes (this file).
//  2. dto.go        — Internal DTOs for the AJAX iframe-load response.
//                      Adapted from observed shape; no inheritance.
//  3. cache.go      — Mirror of allanime/cache.go: 4 key families with the
//                      cross-cutting TTL invariants from libs/cache.
//  4. client.go     — Adapt allanime/client.go's shape to AnimeFever's
//                      HTML-scrape + AJAX-POST data path. NEVER catalog's
//                      HTTP client, NEVER bare *http.Client.
//  5. HTTP client   — Use *domain.BaseHTTPClient (per SCRAPER-FOUND-06 NF).
//                      Cookie jar handled by BaseHTTPClient implicitly.
//
// Upstream data path (per 28-RESEARCH.md AnimeFever section + live recon
// 2026-05-20):
//
//	/search/<term>                                  → HTML, parse div.card-block
//	                                                  for /info/<slug> anchors.
//	/info/<slug>                                    → HTML, parse episode anchor
//	                                                  list to /watch/<slug>?ep=<id>.
//	/watch/<slug>?ep=<id>                           → HTML, extract `var ctk =
//	                                                  '<token>'` for CSRF.
//	POST /ajax/anime/load_episodes_v2?s=<server>    → JSON, contains
//	  body: episode_id=<eid>&ctk=<token>             {status:bool,value:"<html
//	                                                  iframe...>",embed:bool}.
//	                                                  Iframe target is
//	                                                  am.vidstream.vip.
//	GET <iframe-src>                                → JWPlayer-style HTML with
//	                                                  inline `sources: [{"file":
//	                                                  "...m3u8"}]` literal.
//	                                                  Extracted by the new
//	                                                  embeds/vidstream_vip.go
//	                                                  (Plan 28-03).
//
// Pitfalls (per 28-RESEARCH.md):
//
//   - Pitfall 1: search path is /search/<term>, NOT /search?keyword=<term>.
//   - Pitfall 2: AJAX requires `ctk` token scraped from watch page HTML AND
//     the PHPSESSID cookie set on the same browser session (the
//     BaseHTTPClient cookie jar handles propagation).
//   - Pitfall 3: AnimeFever's watch page offers two servers (tserver, hserver),
//     but per AUTO-275 we advertise ONLY tserver — hserver embeds never expose
//     a parseable `sources:` literal (dead-end probes). See supportedServers in
//     client.go.
//
// Anti-Patterns (must NOT do):
//
//   - Hand-rolled http.Client (use BaseHTTPClient.Get/Do)
//   - iframe_url field on Stream (the iframe URL is INPUT to extractor, never
//     part of the returned Stream — see services/scraper/internal/domain/
//     provider.go top-of-file comment)
//   - Caching stream URLs longer than 5 minutes (signed URLs expire fast)
//
// References:
//   - .planning/phases/28-provider-expansion-r2/28-CONTEXT.md (D1-D7)
//   - .planning/phases/28-provider-expansion-r2/28-RESEARCH.md (AnimeFever
//     section + Code Examples + Anti-Patterns to Avoid)
//   - services/scraper/internal/providers/allanime/ — template provider.
//
// Operator kill-switch: SCRAPER_DEGRADED_PROVIDERS=animefever excludes this
// provider from main.go orchestrator registration (zero per-request cost).
package animefever
