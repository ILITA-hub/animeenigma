# Phase 19: AnimeKai (gated) - Context

**Gathered:** 2026-05-12
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous mode; user invoked `/gsd-autonomous --from 18`; user said "work without stopping for clarifying questions")

<domain>
## Phase Boundary

Add AnimeKai as a third English-language scraper provider behind a feature flag that is **default-off in production**. The phase is R&D with a built-in escape hatch — if the in-house token generator does not converge, ship flag-off and carry the four AnimeKai impl requirements (`SCRAPER-KAI-01..04`) to a v3.1 milestone. Phase 20 (cutover) MUST NOT be blocked by this phase's R&D outcome.

The full failover chain after Phase 19 ships: `AnimePahe → gogoanime → AnimeKai (gated)`. With the flag off, only the first two providers serve traffic.

</domain>

<decisions>
## Implementation Decisions

### Feature Flag Strategy (LOCKED — from ROADMAP success criterion 1)
- Env-var name: `SCRAPER_ANIMEKAI_ENABLED` (boolean string, "true"/"false")
- Default: `false` in production (`docker/.env.example` documents the toggle)
- Read at: scraper-api orchestrator startup (registration order matters: animepahe → gogoanime → animekai if enabled)
- Toggle without rebuild: `docker compose restart scraper`
- Frontend never sees a 3rd source dropdown option while flag is off (orchestrator's `RegisteredProviders` list is the source of truth)

### Token Generation Topology (LOCKED — from ROADMAP success criterion 2)
- All MegaUp embed token + AES key derivation lives inside `docker/megacloud-extractor/` (node service)
- New endpoint: `POST /animekai-token` on the megacloud-extractor service
- ZERO external dependencies on `enc-dec.app` — `grep -r "enc-dec.app" services/ docker/megacloud-extractor/` returns nothing
- The Go scraper service calls the in-cluster megacloud-extractor over HTTP (same pattern as Phase 16 HiAnime → megacloud-extractor)

### Provider Package Layout
- New package: `services/scraper/internal/providers/animekai/`
- Files follow the Phase 18 gogoanime pattern: `doc.go`, `client.go`, `dto.go`, `cache.go`, `malsync.go` (forward-compat probe with 24h negative cache; malsync may have no AnimeKai entries — degrade gracefully)
- DDoS-Guard: investigate during research; if `animekai.to` is clean, omit `ddosguard.go` (same precedent as gogoanime); if it requires the guard, port `animepahe/ddosguard.go` shape
- Exported shape: `animekai.Deps` + `animekai.New(d Deps) (*Provider, error)` matching the established convention

### Convergence Definition (Claude's discretion — research-driven)
- "Convergence" = the in-house token generator produces working MegaUp tokens that resolve to a playable HLS stream against live `animekai.to` for at least 3 sample anime across 2 different episode types (sub + dub) over a 24h period
- If the extractor returns errors persistently (>50% failure rate across 10 sample requests), declare non-convergence
- The decision point is documented in `19-RESEARCH.md` during the research step; carries forward to `19-04-SUMMARY.md` if the escape hatch is taken
- Non-convergence outcome: flag remains default-off, requirements SCRAPER-KAI-01..04 are explicitly marked "carry to v3.1" in REQUIREMENTS.md, SCRAPER-KAI-05..07 (flag wiring, observability, docs) still SHIP

### Escape Hatch (LOCKED — from ROADMAP success criterion 5)
- The phase ships either way (flag default-off with extractor wired, OR flag default-off with extractor carried over)
- Phase 20 (cutover) MUST NOT block on AnimeKai's R&D outcome — cutover removes HiAnime + Consumet dead code regardless

### HLS Proxy Allowlist
- Append AnimeKai CDN hostnames to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` (specific hostnames discovered during research)
- Phase 18's append-only invariant preserved — kwik.cx, anitaku.to, vibeplayer.site, etc. all retained
- Regression test: existing Phase 16 + Phase 18 hosts still match

### Observability (LOCKED — Phase 17 patterns)
- `provider_health_up{provider="animekai",stage=...}` gauges via the existing health probe pattern
- `parser_fallback_total{from="gogoanime",to="animekai"}` counter (Phase 17 already emits this; new label tuple appears once flag flips on)
- `parser_requests_total{provider="animekai"}` MUST stay flat-zero while flag is off (success criterion 4)

### Frontend
- No new frontend code while flag is off — the source dropdown's "AnimeKai" option ONLY appears when `RegisteredProviders` includes it
- `capitalizeProvider('animekai')` returns `'AnimeKai'` (the display label matches the backend slug for this one — no rebrand)
- Locale keys: reuse existing `player.sourceMultiTooltip` etc. (Phase 16 declared them for N-provider scaling)

### Claude's Discretion
- Exact `/animekai-token` request/response schema (will be informed by reading existing megacloud-extractor endpoints)
- Token cache TTL inside megacloud-extractor (likely 1-5 min based on AnimeKai's token expiry)
- Retry/backoff policy when token generation fails (will mirror existing Phase 16 HiAnime patterns)
- Number of embed extractors AnimeKai dispatches to — discovered during research (megacloud variants + any new types)

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `docker/megacloud-extractor/server.js` — existing node service that already extracts tokens for HiAnime/Zoro (Phase 16); add `/animekai-token` route alongside
- `services/scraper/internal/providers/{animepahe,gogoanime}/` — paired analog for the new `animekai/` package
- `services/scraper/internal/embeds/{kwik,megacloud,packed_common}.go` — embed extractor patterns; AnimeKai's megacloud variant should reuse the runGoja helper lifted in Phase 18
- `services/scraper/internal/service/orchestrator.go` — provider registry auto-iterates; adding animekai needs no orchestrator changes
- `libs/videoutils/proxy.go::HLSProxyAllowedDomains` — append-only allowlist

### Established Patterns
- Provider package shape: `doc.go` + `client.go` (Deps + New + Provider impl) + `dto.go` + `cache.go` + `malsync.go`
- Embed extractor: implements `domain.EmbedExtractor` (Name + Matches + Extract); Matches() defends against SSRF impostors
- HTTP client: `domain.NewBaseHTTPClient(logger, opts...)` with `WithTransport` (Phase 18) for offline integration tests
- Feature flag: env-var read in main.go, conditional registration in orchestrator setup

### Integration Points
- `services/scraper/cmd/scraper-api/main.go` — register animekai provider conditional on `SCRAPER_ANIMEKAI_ENABLED`
- `services/scraper/internal/config/config.go` — add `AnimeKaiConfig` struct + env binding
- `docker/megacloud-extractor/server.js` — add `/animekai-token` route
- `docker/docker-compose.yml` + `docker/.env.example` — env var documentation
- `services/scraper/internal/service/orchestrator.go` — failover order unchanged (registration order = priority)

</code_context>

<specifics>
## Specific Ideas

- Reference Phase 16 HiAnime + megacloud-extractor wiring as the closest analog for the in-house token generation pattern
- Reference Phase 18 gogoanime package shape for the new animekai package
- Use the `docker/megacloud-extractor/patch-aniwatch.sh` script as a reference for in-cluster token extractor maintenance

</specifics>

<deferred>
## Deferred Ideas

- AnimeKai as a default-on provider (post-7-days clean traffic decision; out of v3.0 scope)
- Multi-mirror failover within AnimeKai if `animekai.to` rotates (v3.1+)
- Server-side analytics on which provider users prefer (v2.1 backlog item)
- Tower defense convergence: if R&D produces multiple competing token-generation strategies, ship the one with cleanest test surface; alternates carried to v3.1

</deferred>
