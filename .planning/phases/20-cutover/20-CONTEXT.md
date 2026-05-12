# Phase 20: Cutover - Context

**Gathered:** 2026-05-12
**Status:** Planning complete; execution gated on 7-day clean traffic guardrail
**Mode:** Auto-generated (autonomous mode)

<domain>
## Phase Boundary

Delete dead HiAnime + Consumet code paths in a single PR. After cutover:
- One English player surface (`EnglishPlayer.vue` only)
- One backend route family (scraper service only — `/api/scraper/*`)
- One set of locale strings (no `hianime` / `consumet` keys)
- Catalog Docker image is smaller (no dead parsers)
- `docker compose ps` shows neither dead container (no aniwatch / consumet-api)

</domain>

<decisions>
## Implementation Decisions

### HARD Guardrail (LOCKED — ROADMAP success criterion 1)
The deletion PR MUST NOT ship until ALL of the following are true:
- **EnglishPlayer has served ≥ 7 days of clean production traffic** (earliest ship: 2026-05-19, since EnglishPlayer first shipped 2026-05-12 via commit `9e9d9a2`)
- Per-provider error rate ≤ 5% over the 7-day window
- Zero Telegram alerts during the window
- Zero user-reported player breakage in `docs/issues/` for the window

**Pre-flight check task** is Wave 0 of this phase — it queries Prometheus for the 7-day window and EXITS NON-ZERO if any guardrail metric fails. The entire deletion plan is downstream of this gate.

### Scope of Deletion (LOCKED — ROADMAP success criteria 2-5)

**Backend (services/catalog/internal/parser/):**
- Delete: `hianime/` (entire directory), `consumet/` (entire directory)
- Delete: any handlers that reference them in `services/catalog/internal/handler/`
- Delete: any service-layer code that calls them in `services/catalog/internal/service/`
- Delete: env-var bindings (`ANIWATCH_API_URL`, `CONSUMET_API_URL`) from `services/catalog/internal/config/config.go`

**Docker:**
- Delete: `aniwatch` container service block from `docker/docker-compose.yml` + `docker/docker-compose.prod.yml`
- Delete: `consumet-api` container service block from both compose files
- Delete: `docker/megacloud-extractor/patch-aniwatch.sh` (no longer needed — sidecar entrypoint becomes plain `node server.js`)
- Update: `docker/megacloud-extractor/Dockerfile` ENTRYPOINT/CMD to `node server.js` (drop the patch step)
- Delete: `ANIWATCH_API_URL` + `CONSUMET_API_URL` from `docker/.env.example`

**Frontend:**
- Delete: `frontend/web/src/components/player/HiAnimePlayer.vue`
- Delete: `frontend/web/src/components/player/ConsumetPlayer.vue`
- Delete: `hianimeApi` + `consumetApi` from `frontend/web/src/api/client.ts` (or wherever they live)
- Delete: any `?legacy=1` flag handling in router / views
- Delete: HiAnime + Consumet locale strings from `en.json`, `ru.json`, `ja.json` (keep only the unified "English" label that EnglishPlayer uses)
- Verify: `grep -r "HiAnimePlayer\|ConsumetPlayer\|hianimeApi\|consumetApi\|legacy=1" frontend/web/src/` returns empty

**Redis:**
- One-shot cleanup script: `scripts/cutover-purge-redis.sh` that DELs all keys matching:
  - `search:hianime:*`, `search:consumet:*`
  - `stream:hianime:*`, `stream:consumet:*`
  - `episodes:hianime:*`, `episodes:consumet:*`
- Script committed alongside the PR; runs once during deploy

### Reversibility (Claude's Discretion)
- Cutover is a one-way operation by design — re-adding HiAnime/Consumet would require re-evaluation against the alive-mirror landscape
- Git history preserves the deleted code; if needed, recovery is `git revert <cutover-commit>` (must come with re-deploy)
- No data is destroyed — only Redis cache keys (which are TTL'd anyway) and Docker image layers

### Sequencing (Claude's Discretion)
- Backend deletion FIRST (catalog service + docker-compose), THEN frontend deletion, THEN Redis cleanup, THEN changelog + final verification
- All in a single PR (per ROADMAP "single PR" requirement) but executed in atomic commits per area

### Telegram Notification (LOCKED — project convention)
- Post-deploy: send Telegram notification announcing the cutover
- Russian tone matching changelog precedent
- Include the date and the 7-day guardrail satisfaction confirmation

### Claude's Discretion
- Exact Prometheus PromQL queries for the 7-day pre-flight check (will use existing scraper-health dashboard patterns)
- Whether to delete the `docker/megacloud-extractor/server.js` `/animekai-token` route (NO — Phase 19's escape hatch keeps it; cutover only deletes HiAnime/Consumet)
- Whether to keep `megacloud-extractor` container itself (YES — Phase 19 still uses it for the /animekai-token stub; only the `patch-aniwatch.sh` Dockerfile step is removed)

</decisions>

<code_context>
## Existing Code Insights

### Files to delete (per ROADMAP grep verification targets)
- `services/catalog/internal/parser/hianime/` (entire directory)
- `services/catalog/internal/parser/consumet/` (entire directory)
- `services/catalog/internal/service/health_checker.go` (if it references hianime/consumet — verify; may need surgical edit instead of full delete)
- `services/catalog/internal/handler/{whichever-routes-hianime-consumet}.go`
- `frontend/web/src/components/player/HiAnimePlayer.vue`
- `frontend/web/src/components/player/ConsumetPlayer.vue`
- `docker/megacloud-extractor/patch-aniwatch.sh`

### Files to surgically edit
- `services/catalog/internal/config/config.go` — drop ANIWATCH_API_URL + CONSUMET_API_URL
- `services/catalog/internal/transport/router.go` — drop hianime/consumet routes
- `services/catalog/cmd/catalog-api/main.go` — drop hianime/consumet parser construction
- `docker/docker-compose.yml` + `docker/docker-compose.prod.yml` — drop aniwatch + consumet-api service blocks
- `docker/megacloud-extractor/Dockerfile` — change ENTRYPOINT to `node server.js`
- `docker/.env.example` — drop the two env var lines
- `frontend/web/src/api/client.ts` (or routerguards) — drop hianimeApi + consumetApi exports + ?legacy=1 handling
- `frontend/web/src/locales/{en,ru,ja}.json` — drop HiAnime + Consumet strings

### Established Patterns
- Single-PR deletion: use atomic commits per area (backend, docker, frontend, redis, docs)
- Pre-flight guardrail check: Bash script that exits non-zero if not met (follows Phase 17 health-check patterns)
- Redis purge: SCAN + DEL (NOT KEYS) to avoid blocking the production Redis instance

### Integration Points
- After deletion, no code path should reference hianime/consumet — verified by `grep -rE "hianime|consumet" services/ frontend/web/src/ docker/` returning empty (except in commit messages / CHANGELOG / docs/issues archive)
- After-update skill: `/animeenigma-after-update` runs lint + build + tests + redeploy + changelog + commit + push

</code_context>

<specifics>
## Specific Ideas

- Pre-flight check should query `parser_zero_match_total` and `provider_health_up` from Prometheus over the 7-day window
- Single-PR deletion: tag the merge commit with `phase-20-cutover` for easy rollback
- Changelog entry: Russian tone, emphasize the simplification ("один плеер, один источник, одна локаль")
- Post-cutover: catalog Docker image should be ≥ 10MB smaller (rough estimate; actual savings depend on parser pkg sizes)

</specifics>

<deferred>
## Deferred Ideas

- AnimeLib (RU player) cutover or refactor — out of v3.0 scope
- Kodik cutover — out of v3.0 scope (still primary RU player)
- Migrating the docker/megacloud-extractor sidecar to a Go service — v4.0+
- Re-evaluating Phase 19's AnimeKai stub if the upstream resurrects — v3.1+

</deferred>
