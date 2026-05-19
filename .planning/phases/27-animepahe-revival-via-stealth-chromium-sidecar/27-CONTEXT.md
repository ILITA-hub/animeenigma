# Phase 27: AnimePahe Revival via Stealth-Chromium Sidecar — Context

**Gathered:** 2026-05-19
**Status:** Ready for planning (`/gsd-plan-phase 27`)
**Milestone:** v3.1 Scraper Self-Healing
**Spec:** new requirements SCRAPER-HEAL-29..SCRAPER-HEAL-33 (to be added to `.planning/milestones/v3.1-REQUIREMENTS.md` during planning)
**Source:** Live diagnostic conversation + stealth-puppeteer probe on 2026-05-19. CONTEXT.md derived from operator-supplied discuss substitute (no `/gsd-discuss-phase` invocation needed — operator authorized "work without stopping").
**Unblocks:** Phase 24 EN-tab hard gate (`docs/issues/scraper-provider-verification-2026-05-19.md`) for the `animepahe` column.

<domain>
## Phase Boundary

A maintenance call to `GET /api/anime/{uuid}/scraper/episodes?prefer=animepahe` (gateway → catalog → scraper microservice → animepahe parser) returns ≥ 28 episodes of Frieren: Beyond Journey's End (MAL 52991) with playable embed metadata. Each episode resolves to a fetchable stream URL through the in-browser HLS proxy (Referer-gated playback is acceptable since `libs/videoutils/proxy.go` handles Referer). The `animepahe` row in the Phase 24 SCRAPER-HEAL-20 verdict log flips from FAIL to PASS.

**Concretely, this phase delivers:**

1. **New sidecar service** `services/animepahe-resolver/` — Node 20 + `puppeteer-extra` + `puppeteer-extra-plugin-stealth`. Maintains a single warmed Chromium context. Exposes a thin internal HTTP API on `:3000`:
   - `GET /healthz` — 200 with `{browser: "up"|"down", lastChallengeSolveAt, pageCount}`.
   - `GET /search?q=<title>` — returns animepahe's `m=search` JSON shape (including the per-anime `session` UUID).
   - `GET /release?session=<animeSession>&page=N` — returns animepahe's `m=release` JSON shape with per-episode `session` tokens.
   - `GET /play?episodeSession=<sess>&animeSession=<sess>` — returns the per-episode play page HTML or extracted server links the parser can hand to the Kwik embed extractor.
   The service auto-respawns Chromium if the page context dies. On any upstream 403, it solves the DDoS-Guard challenge fresh and retries once.

2. **Parser rewrite** at `services/scraper/internal/providers/animepahe/client.go` and its helpers:
   - HTTP-calls the sidecar via `SCRAPER_ANIMEPAHE_RESOLVER_URL` (default `http://animepahe-resolver:3000`) instead of fetching `https://animepahe.{ru,pw,io}` directly.
   - Migrates from the stale `m=release&id=<numeric-MAL-id>` contract (verified 404 on 2026-05-19) to the new `m=release&id=<session-UUID>` contract returned by `m=search`.
   - Chain: search → animeSession → release → episodeSession → play page → embed → Kwik. The Kwik embed extractor (already in `services/scraper/internal/embed/kwik/`) stays unchanged; only the route from MAL ID to a Kwik URL is rewired.
   - `MalSync` lookup is retained as a path optimization (skip search when a MAL → animeSession mapping is cached) but the new flow MUST work even when MalSync misses.

3. **Compose wiring** at `docker/docker-compose.yml`:
   - Adds the `animepahe-resolver` service with healthcheck against `:3000/healthz` (interval `30s`, retries `3`, start_period `20s`).
   - Scraper service grows `SCRAPER_ANIMEPAHE_RESOLVER_URL=http://animepahe-resolver:3000` env + `depends_on: animepahe-resolver: { condition: service_healthy }`.
   - Resource limits on resolver: `mem_limit: 500m`, `cpu: "0.5"`. Chromium MUST stay under the 500 MB resident budget — out-of-budget is a release blocker, not a follow-up.

4. **End-to-end gate-clear** — the Phase 24 SCRAPER-HEAL-20 verification curl pipeline is re-run against Frieren through the gateway; the `animepahe` row in `docs/issues/scraper-provider-verification-2026-05-19.md` flips from FAIL to PASS; a post-ship section is appended documenting the new operating posture.

5. **Compose default cleanup** — once the gate clears, `SCRAPER_DEGRADED_PROVIDERS=${SCRAPER_DEGRADED_PROVIDERS:-gogoanime,animepahe}` becomes `SCRAPER_DEGRADED_PROVIDERS=${SCRAPER_DEGRADED_PROVIDERS:-gogoanime}`. The env-override escape hatch is retained so the operator can re-disable on outage with `SCRAPER_DEGRADED_PROVIDERS=gogoanime,animepahe` in `docker/.env`.

**Out of scope:**

- gogoanime's anitaku → anineko migration (separate phase; gogoanime stays in `SCRAPER_DEGRADED_PROVIDERS`).
- Resurrecting `animepahe.ru` directly (the sidecar uses the `.pw` mirror because that is what cleared during the 2026-05-19 probe; this is a constraint of the upstream, not a parser limitation).
- AllAnime lift (already scoped as Phase 26 Wave 1 plan 26-01-PLAN.md — runs in parallel; Phase 27 does not block Phase 26).
- VibePlayer revival via WARP egress (demoted from numbered reservation to unnumbered "future idea" during this phase's setup; phase number reassigned when committed to a future milestone).
- FingerprintJS bypass on `animepahe.io` (the sidecar uses `.pw` exclusively because `.pw` only needs DDoS-Guard solving, which the stealth plugin handles; `.io`'s FingerprintJS adds a separate fingerprint-spoofing burden we do not need).
- Auto-refreshing the stealth plugin pin on DDoS-Guard rotation (Phase 27 ships a *static* pinned version with documented refresh procedure for the maintenance-bot's later auto-refresh pipeline; the auto-refresh itself is a separate future phase).

**Requirements covered (new — to be assigned during planning):**
- SCRAPER-HEAL-29 — Stealth-Chromium sidecar service scaffold + HTTP API + healthcheck.
- SCRAPER-HEAL-30 — Parser rewrite to call sidecar + migrate to UUID `session`-token API contract.
- SCRAPER-HEAL-31 — Docker compose wiring with healthcheck dependency + resource limits.
- SCRAPER-HEAL-32 — End-to-end gate-clear + Phase 24 verdict-log update.
- SCRAPER-HEAL-33 — `SCRAPER_DEGRADED_PROVIDERS` default cleanup + post-ship redeploy.

</domain>

<decisions>
## Implementation Decisions

### D1 — Stealth-Chromium sidecar (Option A), NOT one-shot cookie warmup, NOT skip-to-AllAnime

The operator explicitly chose Option A from a three-way comparison surfaced 2026-05-19. Rationale:

- **B (one-shot cookie warmup)** was a false economy. DDoS-Guard rotates challenges every few hours to days; every refresh requires re-running Chromium anyway. We end up with the same operational complexity but a worse abstraction (the cookie state lives in two places — a transient warmup script and a long-running Go HTTP client). Stealth-plugin updates are also harder to roll out when the runtime is split.
- **C (skip to AllAnime lift / Phase 26-01)** is happening anyway as a parallel path. AllAnime is the natural redundancy partner — once Phase 27 + Phase 26-01 both ship, the orchestrator's failover chain has two live EN providers. Choosing C *instead of* A would leave the EN surface single-provider, defeating the redundancy goal.
- **A** also forms a reusable pattern: if `.io`'s FingerprintJS-gated surface ever becomes useful, the same sidecar topology handles it (with a different plugin pin). If a future provider lands behind Cloudflare Turnstile or hCaptcha, same pattern. The sidecar is infrastructure, not animepahe-specific code.

**Why:** Operator pick, 2026-05-19 conversation. Locked.

### D2 — Sidecar uses `animepahe.pw` exclusively (not `.ru`, not `.si`, not `.io`)

`.ru` and `.si` are TCP-blackholed from this server's egress IP AND through Cloudflare WARP egress (verified 2026-05-19). They are unreachable at the network layer regardless of what runtime we use. `.io` adds FingerprintJS on top of any IP, which means more plugin surface and more fragility. `.pw` only needs DDoS-Guard solving, which the stealth plugin handles in ~1.3 seconds.

**Why:** Two of four domains are unreachable; the third adds avoidable complexity. `.pw` is the one path that worked in the 2026-05-19 probe.

**How to apply:** The sidecar's base URL is `https://animepahe.pw`. If `.pw` rotates or dies later, the resolver service is the single point to update. Parser code stays domain-agnostic.

### D3 — API contract migration is required even if upstream weren't gated

Live probe today proved `m=release&id=5319` (numeric MAL-style id, what the parser currently sends) returns HTTP 404 `{"message": ""}`. The endpoint now requires `m=release&id=<UUID session>`, where the session is per-anime and comes from `m=search`'s response. Per-episode playback also uses a per-episode `session` token (the `session` field in each `data[]` entry of the release response), which feeds the server-list / Kwik URL stage.

**Why:** This is a *separate* breakage from the DDoS-Guard issue. Even if the operator had picked Option B or Option C and somehow defeated DDoS-Guard, the parser as written would still 404 on every release. Fixing one without the other is wasted work.

**How to apply:** Parser surgery rewrites the search → release → play chain. Helpers in `services/scraper/internal/providers/animepahe/` (`dto.go`, `cache.go`, `malsync.go`, `client.go`) all change shape. `ddosguard.go` becomes load-bearing-zero (the sidecar handles DDoS-Guard; the Go side never sees a 403) and is deleted.

### D4 — Test goldens captured fresh from upstream during planning, not from `/tmp/pup/probe2.js`

The probe artifacts at `/tmp/pup/probe.js` and `/tmp/pup/probe2.js` proved feasibility but should not be treated as canonical test inputs. They are throwaway research scaffolds. Plan 27-02 will re-capture goldens fresh against Frieren (MAL 52991) using the actual sidecar's HTTP responses, so the parser's unit tests exercise the same DTO shapes the sidecar produces — not whatever the upstream returned to a one-off Node script today.

**Why:** Goldens captured from production code paths are more faithful than goldens captured from research scaffolds. Also, the `/tmp/pup/` artifacts include browser-context artifacts (e.g. cookie names) that won't appear in the sidecar's HTTP response shape.

**How to apply:** Plan 27-02 explicitly captures three golden files: `testdata/animepahe/frieren-search.json`, `testdata/animepahe/frieren-release.json`, `testdata/animepahe/frieren-play.html`. Unit tests run offline; an integration test (manual gate, not CI) exercises live upstream end-to-end.

### D5 — Memory budget is a hard ship gate, not a follow-up

Sidecar Chromium MUST stay under 500 MB resident. The compose `mem_limit: 500m` is the contract; if the sidecar OOMs under steady-state traffic (≤ 10 req/s expected; the EN tab isn't a hot path), the phase does NOT ship. Compose memory-limit is a guard, not a tuning knob to relax.

**Why:** Self-hosted budget — this server runs all microservices on a single VPS. Chromium is the biggest single-process memory user we'd be adding; an unbounded process here would push the host into swap.

**How to apply:** Plan 27-01 includes a steady-state memory probe (`docker stats animepahe-resolver` snapshot under 100 sequential requests). If RSS exceeds 500 MB, the sidecar gains a "page recycle every N requests" mechanism (close + relaunch the Chromium tab — not the whole process) before the phase ships.

### D6 — Stealth plugin pin is version-locked in `package.json` + documented refresh procedure

`puppeteer-extra-plugin-stealth` and `puppeteer-extra` are exact-version-pinned (no `^` or `~`). The pinned versions are recorded in `services/animepahe-resolver/package.json` and ALSO in a `services/animepahe-resolver/STEALTH-PINS.md` doc that the maintenance-bot reads when DDoS-Guard rotates.

**Why:** Stealth plugin's defeat mechanism is reverse-engineered from current DDoS-Guard. When DDoS-Guard ships a new challenge variant, the stealth plugin needs updating; this is the maintenance-bot's job per the existing self-heal pipeline. Pinning is what makes "the plugin works today" reproducible.

**How to apply:** Plan 27-01 writes `STEALTH-PINS.md` with current versions + last-tested-against date + a one-line refresh procedure (`npm install puppeteer-extra@latest puppeteer-extra-plugin-stealth@latest && npm test && commit`). The maintenance-bot's escalation prompt (`.claude/maintenance-prompt.md` Pattern 7) gains an animepahe-specific branch referencing this doc.

### D7 — Removing `animepahe` from `SCRAPER_DEGRADED_PROVIDERS` is the LAST step, gated on end-to-end probe pass

Even if the sidecar builds, the parser compiles, and unit tests are green, `animepahe` stays in `SCRAPER_DEGRADED_PROVIDERS` until the live curl pipeline against Frieren (the same pipeline `docs/issues/scraper-provider-verification-2026-05-19.md` codifies) returns the full episodes-then-servers-then-stream chain end-to-end. The orchestrator MUST observe `animepahe` returning real data before the operator flips the switch.

**Why:** v3.1 has been burned by "we shipped the surface; it was broken" twice already (v3.0 Phase 20 cutover; the 2026-05-18 EnglishPlayer cleanup). Sequencing this last step against live evidence is the brake.

**How to apply:** Plan 27-05 contains TWO assertion gates — (a) curl pipeline green, (b) `make logs-scraper | grep "animepahe"` shows no continuous 403/timeout pattern in the first 10 minutes after redeploy. Only after both gates does the compose-default edit commit.

### D8 — Renumbered "Reserved future phases" stubs are now unnumbered

As part of this phase's ROADMAP setup, the two reserved-future-phase stubs (originally Phase 27 VibePlayer Recovery via WARP egress, Phase 28 MinIO Hot Archival) were renumbered to unnumbered "future ideas" in ROADMAP.md. They will be assigned phase numbers when each is committed to a milestone. The Phase 27 slot is now this phase.

**Why:** The reserved numbering was provisional; demoting to unnumbered ideas is honest and prevents future renumber churn.

**How to apply:** Other prose elsewhere in `.planning/` that references "Phase 27: VibePlayer" or "Phase 28: MinIO" is stale — flag in any cross-link Plan 27-XX touches. The Plan-checker should NOT fail on these references; they are historical context that ages out as future phases get committed.

### Claude's Discretion

Items not pre-decided by the operator — the planner / executor can choose based on context:

- Specific HTTP framework for the sidecar (Express vs. Fastify vs. native `http`). Recommend Fastify for built-in JSON schema validation but Express is fine.
- Exact log format from the sidecar (JSON vs. text). Recommend JSON for ingestion into the existing Loki stack.
- Whether the sidecar exposes Prometheus `/metrics` for sidecar-level observability. Recommended yes (challenge-solve count, page-recycle count, upstream-403 count) but not strictly required for ship.
- Concurrency model inside the sidecar — single page reused for all requests vs. page pool. Recommend a small pool (size 2–3) to overlap requests without breaking the 500 MB budget.
- Whether the sidecar runs Chromium with `--single-process` or default multi-process. Test both during 27-01 and pick whichever fits the 500 MB ceiling at steady state.
- Whether `MalSync` cache is invalidated when the parser starts seeing 404s from `m=release` (probably yes — a stale animeSession is a likely cause).

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Phase 24 hard-gate verdict log
- `docs/issues/scraper-provider-verification-2026-05-19.md` — codifies the SCRAPER-HEAL-20 hard-gate curl pipeline against Frieren (MAL 52991). Defines what "animepahe column flips green" actually means. Plan 27-04 re-runs this exact pipeline and appends a post-ship section.

### v3.1 milestone shape
- `.planning/milestones/v3.1-REQUIREMENTS.md` — current requirement IDs SCRAPER-HEAL-01..28. New requirements 29..33 get added here during 27-01 / 27-02 planning.
- `.planning/milestones/v3.1-ROADMAP.md` — v3.1 phase ordering. Phase 27 inserts after Phase 26 in shipping order, but it can ship in parallel with Phase 26 Wave 1 (no file overlap).
- `.planning/ROADMAP.md` — top-level Phase 27 entry already created (`### Phase 27: AnimePahe Revival via Stealth-Chromium Sidecar`).

### Parser surface to rewrite
- `services/scraper/internal/providers/animepahe/client.go` — main rewrite target (currently 700+ LOC).
- `services/scraper/internal/providers/animepahe/dto.go` — DTO shape changes per new API contract.
- `services/scraper/internal/providers/animepahe/cache.go` — likely keep; cache keys change (now animeSession-keyed, not MAL-keyed).
- `services/scraper/internal/providers/animepahe/malsync.go` — keep as path optimization; cache invalidation on `m=release` 404 added.
- `services/scraper/internal/providers/animepahe/ddosguard.go` — DELETED. Sidecar handles DDoS-Guard.
- `services/scraper/cmd/scraper-api/main.go` — provider registration; respects `SCRAPER_DEGRADED_PROVIDERS`. No surgery; the env-var read path is correct.
- `services/scraper/internal/config/config.go` — env-var binding. Add `AnimepaheResolverURL` field bound to `SCRAPER_ANIMEPAHE_RESOLVER_URL`.

### Existing embed extractor (untouched)
- `services/scraper/internal/embed/kwik/` — the Kwik embed extractor used after the play page is resolved. Phase 27 leaves this entirely alone; only the upstream of Kwik changes.

### Compose surface
- `docker/docker-compose.yml` — sidecar service definition + scraper env wiring + `SCRAPER_DEGRADED_PROVIDERS` default. Current value: `gogoanime,animepahe`; post-Phase-27 value: `gogoanime`.
- `docker/.env` — has no override today; relying on compose defaults. Plan 27-05 does NOT need to edit `docker/.env`.

### Maintenance bot pattern reference
- `.claude/maintenance-prompt.md` — Pattern 7 (provider-down detection / FingerprintJS escalation). Gains an animepahe-resolver branch in Plan 27-01 referencing `STEALTH-PINS.md`.

### Issue tracker
- `docs/issues/README.md` — add ISS-NNN for Phase 27 ship under "Resolved Issues" section per `MEMORY.md → Issues & Incidents Documentation` convention.

### Project guidelines
- `CLAUDE.md` — overall project conventions; in particular "Adding New libs/ Module" steps (NOT applicable here — sidecar is a service, not a lib, but the after-update flow IS applicable).
- `.claude/commands/animeenigma-after-update.md` — the after-update skill 27-05 invokes for changelog + commit + push.

### Memory-derived references
- `/root/.claude/projects/-data-animeenigma/memory/MEMORY.md` — project memory. Particularly: API key auth pattern (relevant for ReportButton on the EN tab once it works), AnimeLib precedent for HLS/MP4 fallback patterns (informs how the parser handles "sidecar returns a play URL we can't extract").

</canonical_refs>

<specifics>
## Specific Ideas

### Live probe payload shapes (capture these in goldens)

From the 2026-05-19 stealth-puppeteer probe at `/tmp/pup/probe2.js` against `animepahe.pw`:

**`m=search&q=Frieren` response shape:**
```json
{
  "total": 3,
  "per_page": 8,
  "current_page": 1,
  "data": [
    {
      "id": 5319,
      "title": "Frieren: Beyond Journey's End",
      "type": "TV",
      "episodes": 28,
      "status": "Finished Airing",
      "season": "Fall",
      "year": 2023,
      "score": 9.27,
      "poster": "https://i.animepahe.pw/posters/<hash>.jpg",
      "session": "65a00d22-e684-4a33-5fa2-707b8e64a84d"
    },
    ...
  ]
}
```

**`m=release&id=<animeSession>` response shape:**
```json
{
  "total": 28,
  "per_page": 30,
  "current_page": 1,
  "data": [
    {
      "id": 60059,
      "anime_id": 5319,
      "episode": 1,
      "episode2": 0,
      "edition": "",
      "title": "",
      "snapshot": "https://i.animepahe.pw/snapshots/<hash>.jpg",
      "disc": "BD",
      "audio": "eng",
      "duration": "00:26:01",
      "session": "7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7",
      "filler": 0,
      "created_at": "2023-09-29 15:03:47"
    },
    ...
  ]
}
```

**`/anime/<animeSession>` HTML page** loads with HTTP 200 and the standard animepahe template (`<title>Frieren: Beyond Journey's End Ep. 1-28 [Completed] :: animepahe</title>`).

### Sidecar HTTP contract (operator-suggested shape)

```
GET /healthz
  → 200 { browser: "up", lastChallengeSolveAt: "2026-05-19T...", pageCount: 1 }
  → 503 { browser: "down", lastError: "..." }

GET /search?q=<title>
  → 200 (verbatim m=search JSON from upstream)
  → 502 if challenge un-solvable after 2 retries

GET /release?session=<animeSession>&page=<N>
  → 200 (verbatim m=release JSON from upstream)
  → 404 if animeSession unknown (stale cache → parser should invalidate MalSync entry)
  → 502 if challenge un-solvable

GET /play?animeSession=<sess>&episodeSession=<sess>
  → 200 with either raw HTML of the play page OR pre-extracted server links
  → 404 if episodeSession unknown
  → 502 if challenge un-solvable
```

### Challenge-solve flow inside the sidecar

1. On first request: load `https://animepahe.pw/` with `waitUntil: 'networkidle2'`. The stealth plugin's overrides handle FP detection; DDoS-Guard sets `__ddg5_` and friends via its challenge JS.
2. Cache the page object and cookies. Subsequent requests use the same page, calling `page.evaluate(async () => fetch(url, ...))` so the request goes through the same browser context (preserving DDoS-Guard cookies).
3. On `403` from upstream (DDoS-Guard cookie rotation): reload `https://animepahe.pw/`, retry the in-page fetch ONCE.
4. On second `403`: respond 502 to the parser; bump a `stealth_challenge_failures_total` metric.

### Parser flow rewrite (high level)

```go
// Before:
// ListEpisodes(malID) → fetchURL("animepahe.ru/api?m=search&q=<title>") → fetchURL("...m=release&id=<numeric MAL id>")

// After:
// ListEpisodes(malID) →
//   1. (Optional) MalSync.Lookup(malID, "animepahe") → animeSession?
//   2. If miss: GET ${resolver}/search?q=<title> → pick best match → animeSession
//   3. GET ${resolver}/release?session=<animeSession>&page=1 (loop pages if total > per_page)
//   4. Return []Episode with each carrying episodeSession
//
// GetServers(episode) →
//   1. GET ${resolver}/play?animeSession=...&episodeSession=...
//   2. Parse server links (Kwik / kwik.cx) from response
//   3. Return []Server (Kwik primary)
//
// GetStream(server) →
//   1. Hand Kwik URL to existing embed/kwik/ extractor
//   2. Return Stream with .m3u8 URL + Referer header (required by Kwik)
```

### Expected upstream rate limits

`animepahe.pw` did not return any 429s during the 2026-05-19 probe across ~10 requests in ~5 seconds. We expect the EN-tab traffic from real users to be roughly 1 req/s peak. The sidecar's single-page reuse keeps each in-page `fetch` cheap (~50ms) since the browser context is already warm.

### Failure-mode catalog (planner should cover each)

| Failure | Behavior |
|---|---|
| Sidecar Chromium crashes | Sidecar process auto-restarts within 5s (Node's child_process exit listener). Parser sees 502 during the window; orchestrator falls through to next provider in candidate list. |
| Sidecar healthcheck fails for > 90s | Compose-level marks it unhealthy; scraper service does not depend on it for boot (degraded mode); only `animepahe` requests fail-fast with the orchestrator falling through. |
| Stealth plugin defeated by new DDoS-Guard challenge | All requests return 502; `stealth_challenge_failures_total` spikes; Grafana alerts; maintenance bot picks up the pattern via `.claude/maintenance-prompt.md` Pattern 7 + the new animepahe-resolver branch from D6. |
| `animepahe.pw` itself goes dark | All requests return 502 with `lastError: "upstream timeout"`. Operator re-adds `animepahe` to `SCRAPER_DEGRADED_PROVIDERS` in `docker/.env` (env override). |
| MalSync cache poisoning (stale animeSession) | `m=release` returns 404 → parser invalidates the MalSync entry and falls back to `m=search`. |
| Page memory leak | Sidecar recycles the page (close + relaunch tab) every N=100 requests; counter exposed at `/metrics`. |

</specifics>

<deferred>
## Deferred Ideas

These items are explicitly OUT-OF-SCOPE for Phase 27 — capture for future phases / future-ideas list. Do NOT bleed scope into them during planning.

- **Auto-refreshing the stealth plugin pin on DDoS-Guard rotation.** Phase 27 ships static pins + a documented refresh procedure. The maintenance-bot's existing auto-refresh pipeline can later extend Pattern 7 to dispatch `npm update` commits when DDoS-Guard rotates. Separate phase when there is appetite.
- **Multi-domain resolver (handle `.io` + `.pw` failover, FingerprintJS bypass).** The sidecar is single-domain (`.pw`) for now. A future phase can add `.io` handling with a FingerprintJS-spoofing plugin when `.pw` finally goes dark. Until then YAGNI.
- **Resolver reuse for other gated providers (Cloudflare Turnstile, hCaptcha, etc.).** The architecture supports it but no current provider needs it. Future phase scoped per-provider as their challenges emerge.
- **gogoanime anitaku → anineko migration.** Separate parser; separate fragility profile (`0 search results` from upstream is NOT a DDoS-Guard problem). Belongs in a parallel Phase 28 (TBD numbering).
- **AllAnime lift (already Phase 26-01).** Parallel work; Phase 27 does not block.
- **AnimeKai escape-hatch fill-in.** Already on the v3.1 roadmap as Phase 26-06.
- **VibePlayer Recovery via WARP egress.** Demoted to unnumbered future idea during this phase's setup. Note: the 2026-05-19 probe proved WARP egress does NOT defeat animepahe's IP block (the Cloudflare range is blocked), so the same WARP approach for VibePlayer would need separate evidence-gathering before it ships.
- **MinIO Hot Archival.** Demoted to unnumbered future idea during this phase's setup. v3.2 scope.
- **Health-aware EN tab hiding** (already deferred from Phase 24). Phase 27 does not re-open this; the EN tab is shown unconditionally per Phase 24's D5.
- **Browse filter `has_english` activation** (already deferred to Phase 26-02). Phase 27 does not touch.

</deferred>

---

*Phase: 27-animepahe-revival-via-stealth-chromium-sidecar*
*Context gathered: 2026-05-19 (operator-supplied discuss substitute; `/gsd-discuss-phase` skipped under "work without stopping" authorization)*
