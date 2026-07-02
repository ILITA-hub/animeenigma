// Package miruro implements the Miruro upstream provider for the scraper
// service (SCRAPER-HEAL-37, Phase 28 Wave 2). Failover slot 5 — reached when
// allanime AND animefever are both unhealthy.
//
// Architecture (CONTEXT.md D5 + SPIKE-MIRURO.md):
//
//	┌─────────────────┐
//	│ orchestrator    │  picks provider; pings HealthCheck
//	└──────┬──────────┘
//	       │
//	       ▼
//	┌─────────────────────────────────────────────────────────────┐
//	│ miruro.Provider (this package)                              │
//	│                                                              │
//	│  FindID    → ARM (libs/idmapping) MAL→AniList → "<aniListID>"│
//	│  ListEpisodes(aniListID)     ──┐                             │
//	│  ListServers(eid)            ──┼─→ BuildSecurePipeURL +      │
//	│  GetStream(eid, srv, cat)    ──┘  DecodeObfuscatedResponse   │
//	│                                                              │
//	│            (from obfuscation.go — Plan 28-00)               │
//	└──────┬──────────────────────────────────────────────────────┘
//	       │
//	       ▼
//	GET https://www.miruro.tv/api/secure/pipe?e=<base64url(json desc)>
//	     headers: User-Agent: Chrome, Referer: https://www.miruro.tv/
//	response: x-obfuscated: 1|2 + base64url(gzip([xor(]json[)]))
//
// Upstream observations recorded in SPIKE-MIRURO.md (2026-05-20):
//
//   - The React SPA reads `VITE_PROXY_A=https://pro.ultracloud.cc/` and
//     `VITE_PROXY_B=https://pru.ultracloud.cc/` from env2.js, but the
//     SPA NEVER calls those hosts on the GET path. All anime API calls
//     are routed through www.miruro.tv/api/secure/pipe directly. We
//     preserve ProxyURL / ProxyURLAlt in Deps for API parity with the
//     plan's failover contract (Task 3 D7 allowlist), but the live code
//     paths talk to www.miruro.tv.
//
//   - Three GET endpoints encode/decode through the secure-pipe envelope:
//   - "info/<aniListID>"      — Frieren = 1.3 MiB JSON, includes
//     embedded episode listings keyed by provider
//     (`dune`, `kiwi`, `hop`, `bee`, `ANIMEKAI`).
//   - "episodes" + query={"anilistId":<id>}
//     — per-provider episode arrays under each
//     audio category (`sub`/`dub`).
//   - "sources" + query={"episodeId":<eid>, "provider":<p>}
//     — direct HLS m3u8 URLs (no separate
//     embed extractor needed for the happy
//     path; Kwik fallback is in the same
//     JSON for redundancy).
//
// FindID convention:
//
//	Provider ID = AniList ID (as string). Catalog passes Shikimori ID
//	(== MAL ID); we use libs/idmapping ARM to translate. There is no
//	fuzzy title search path — Miruro is AniList-keyed end to end.
//
// Episode ID convention:
//
//	Upstream returns opaque base64-looking strings keyed per inner
//	provider (e.g. "YW5pbWVwYWhlOjUzMTk6NjAwNTk6MQ"). We store the raw
//	upstream ID as Episode.ID — no parsing/round-tripping required.
//	ListServers and GetStream pass it back to upstream verbatim. For
//	cache scoping, we prefix with the AniList ID to keep keys per-anime.
//
// Threat surface (28-04-PLAN.md threat_model):
//
//   - T-28-04-01 Cloudflare challenge: REAPPEARED 2026-07-02 (Turnstile on the
//     SPA + a hard WAF block on /api/secure/pipe for un-cleared clients).
//     RESOLVED by routing the secure-pipe GET through the Camoufox
//     stealth-scraper sidecar when the DB roster sets engine="browser" (see
//     transport.go): the warm /fetch session solves the homepage Turnstile and
//     the in-page fetch to /api/secure/pipe rides cf_clearance. Go still builds
//     the `e=` descriptor + decodes the x-obfuscated envelope (Approach 2). The
//     stdlib-only engine="http" path (below) remains as a degraded fallback but
//     cannot pass the block on its own (per D3 gate 2, no utls workaround).
//   - T-28-04-04 SSRF: TransformProxyURL takes endpoint strings only
//     constructed internally from validated AniList IDs / opaque IDs.
//     The base host comes from Deps.BaseURL (validated at config.Load).
//   - T-28-04-05 DoS: 4 MiB read cap from MaxDecodedResponseBytes in
//     obfuscation.go + 4 MiB on the raw response read.
//
// Transport: the stdlib-only path (engine="http") has no third-party HTTP
// fingerprint libraries and no in-process headless browser (RESEARCH.md
// "Don't Hand-Roll" boundary). When the DB roster sets engine="browser", the
// secure-pipe GET is delegated to the out-of-process Camoufox stealth-scraper
// sidecar (transport.go) — the provider itself stays stdlib-only.
package miruro
