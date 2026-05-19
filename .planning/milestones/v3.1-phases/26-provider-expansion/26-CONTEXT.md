# Phase 26: Provider Expansion — Context

**Gathered:** 2026-05-19
**Status:** Ready for planning (`/gsd-plan-phase --phase 26`)
**Milestone:** v3.1 Scraper Self-Healing (REOPENED 2026-05-19)
**Spec:** `.planning/milestones/v3.1-REQUIREMENTS.md` (SCRAPER-HEAL-25..28)
**v3.0 carryover absorbed:** SCRAPER-KAI-01..04 + SCRAPER-KAI-07 (the AnimeKai escape-hatch fill-in)

<domain>
## Phase Boundary

Grow the scraper's failover pool from two live providers (gogoanime, animepahe) to three or four. The EnglishPlayer's in-player source dropdown — restored as infrastructure in Phase 24 — lights up with 2-4 selectable options. The orchestrator's failover chain gets meaningfully deeper so a single provider outage no longer halves the available coverage.

**Concretely, this phase delivers:**

1. **AllAnime lift (SCRAPER-HEAL-25).** The existing `services/catalog/internal/parser/allanime/` GraphQL persisted-query client (currently used by workstream raw-jp for original Japanese audio) is lifted into `services/scraper/internal/providers/allanime/` as a third `domain.Provider`. The catalog-side raw JP path keeps its own consumption flow unchanged — the lift is a copy-with-adaptation, not a move. The new provider implements `FindID` (Shikimori → AllAnime ID), `ListEpisodes`, `ListServers`, `GetStream`, `HealthCheck` against AllAnime's EN sub catalog. Registered after gogoanime in the orchestrator's failover chain.
2. **Fresh 2026 EN-source survival sweep (SCRAPER-HEAL-26).** Research artifact in `.planning/research/2026-05-19-en-source-survival.md` lists every candidate evaluated as of 2026-05-19 — at minimum: Miruro, AnimeOwl, AnimeFever, AniWatchTV (post-March 2026 USTR shutdown — confirm dead), Crunchyroll-Free (legal status), HiAnime (confirm dead). Each candidate gets a verdict on `live | dead | uncertain`, a recommendation on `worth-implementing | not-worth | needs-deeper-PoC`, and (if recommended) the embed extractors it needs plus an estimate of implementation effort. **Operator decision gate**: after reading the sweep, operator selects 0-2 survivors. Each selected survivor gets its own sub-plan (e.g., 26-04 if one is picked, 26-04 + 26-05 if two).
3. **AnimeKai recovery (SCRAPER-HEAL-27).** Fills in the v3.0 Phase 19 escape-hatch carryover: provider methods in `services/scraper/internal/providers/animekai/client.go` go from stub-returning-`ErrProviderDown` to working scrapers; sidecar `POST /animekai-token` handler in `docker/megacloud-extractor/server.js` goes from HTTP 501 to a real MegaUp token generator. `SCRAPER_ANIMEKAI_ENABLED=true` verified end-to-end against live `anikai.to`. If R&D doesn't converge inside Phase 26's time budget (~14-21 days per the 2026-05-12 RESEARCH.md), the requirement stays open and Phase 26 ships without it (same escape-hatch pattern as v3.0 Phase 19).
4. **EnglishPlayer dropdown re-activation (SCRAPER-HEAL-28).** With expansion landed, the in-player source dropdown lights up. Single-option mode (one provider) hides the `<select>` to keep the UI clean; 2+ options show it. User preference persists per anime via `useWatchPreferences.preferredScraperProvider`. Each provider gets a `capitalizeProvider()` branch for human-readable display (`allanime` → "AllAnime", `animekai` → "AnimeKai", etc.).

**Out of scope:**

- WARP egress sidecar to revive VibePlayer (v3.2+ separate spec).
- MinIO segment archival (v3.2+ separate spec).
- 9anime / AniWave / Kaido / Zoro mirror resurrection (confirmed dead; do not waste cycles).
- Pre-populating the catalog with AllAnime metadata (the on-demand pattern stays per CLAUDE.md).

**Requirements covered:** SCRAPER-HEAL-25, SCRAPER-HEAL-26, SCRAPER-HEAL-27, SCRAPER-HEAL-28.

</domain>

<decisions>
## Implementation Decisions

### D1 — AllAnime lift is copy-with-adaptation, NOT a move

The catalog-side AllAnime parser is doing useful work for raw JP and we don't want to refactor its consumption flow at the same time as the scraper-side lift. The new `services/scraper/internal/providers/allanime/` is a sibling client that shares the same upstream (AllAnime GraphQL API at `api.allanime.day`) but presents the `domain.Provider` interface the scraper expects. Shared concerns (cache layer, base HTTP client) come from the scraper's existing `services/scraper/internal/domain/BaseHTTPClient` — the lift does NOT pull in the catalog's HTTP client.

Trade-off: code duplication of GraphQL queries. Acceptable because (a) GraphQL queries change slowly, (b) the catalog-side and scraper-side consumption contracts will diverge over time (raw JP cares about original audio + RU subs; scraper cares about EN sub catalog + multi-server resolution), (c) the alternative (extract to a shared lib) creates a new package boundary for the sake of three GraphQL strings.

### D2 — Operator decision gate before implementing survey candidates

SCRAPER-HEAL-26 ships the survey first, THEN the operator picks. We do NOT pre-commit to "implement all live candidates." Reason: each candidate is a ~3-7 day implementation; we don't know without the survey whether 0, 1, 2, or 4 candidates are even worth the engineering time. The survey gate prevents over-commitment.

If the survey produces zero live candidates, SCRAPER-HEAL-26 ships as research-only and the phase is done with two new providers (AllAnime + AnimeKai recovery if that converges, AllAnime alone if it doesn't). Acceptable outcome.

### D3 — AnimeKai recovery accepts a second escape-hatch

If the in-house MegaUp token generator doesn't converge in Phase 26 either, SCRAPER-HEAL-27 stays open and AnimeKai stays flag-default-off. v3.1 ships without it. This is NOT a failure — it's the documented contingency from v3.0 Phase 19 (RESEARCH.md §Convergence Probability Assessment scored full implementation at ~14-21 days vs ~3-4 days for the escape hatch).

If R&D does converge, AnimeKai joins the failover chain after AllAnime: `gogoanime → animepahe → allanime → animekai`.

### D4 — Dropdown UI: hide on single-option, show on 2+

Existing pattern from the 2026-05-12 EnglishPlayer (per the restored snapshot from Phase 24): the `<select>` is rendered with `v-if="providers.length > 1"` — single-option mode hides the chrome. Phase 26's expansion lights it up automatically without UI changes; only the i18n key for each new provider needs adding.

### D5 — `has_english` GORM column + browse filter activation lives here, NOT Phase 24

Phase 24's CONTEXT D4 explicitly deferred the browse filter to Phase 26. With AllAnime live (and optionally AnimeKai), opportunistic `SetHasEnglish` calls populate the column for any anime any user touches the EN tab for. Within ~7 days of Phase 26 ship the column has enough rows to make the filter useful. Add the filter row + composable widening + i18n key in the same sub-plan that lifts AllAnime (`26-01-PLAN.md`'s Wave 2 task) — kept close to the value source.

### D6 — Phase 26 ships in waves; each wave is independently releasable

- Wave 1: AllAnime lift (SCRAPER-HEAL-25 + browse-filter add per D5). Ships as the minimum lovable expansion.
- Wave 2: Survival sweep (SCRAPER-HEAL-26) — research artifact + decision gate.
- Wave 3a: Survey candidate #1 implementation (if operator picked one).
- Wave 3b: Survey candidate #2 implementation (if operator picked two).
- Wave 4: AnimeKai recovery (SCRAPER-HEAL-27) — may slip to v3.2 if R&D doesn't converge.
- Wave 5: Dropdown re-activation polish (SCRAPER-HEAL-28) — actually trivial since the infrastructure is in place from Phase 24; this wave is mostly just `capitalizeProvider` branches + i18n labels per added provider.

Wave 1 alone ships an observable improvement. Each subsequent wave is additive.

</decisions>

<open_questions>
- **Which 2026 candidates make the survey shortlist?** Answered by SCRAPER-HEAL-26 itself; not a planning blocker.
- **AnimeKai R&D budget**: ~14-21 days per the 2026-05-12 RESEARCH.md. Operator decides whether Phase 26 holds for the full budget or ships early on the other waves and carries AnimeKai to v3.2. Default proposed: ship early, carry AnimeKai if it slips past 7 days of effort.
- **Should AllAnime be registered before or after animepahe in the failover chain?** Default proposed: `gogoanime → animepahe → allanime → [animekai]`. Anitaku-flavored EN content from gogoanime is the highest user-perceived quality; animepahe is the dependable second-chance; AllAnime is the additional safety net. Re-orderable via `SCRAPER_SERVER_PRIORITY` if real-world performance suggests otherwise.
</open_questions>

<risks>
## Risks specific to this phase

- **AllAnime GraphQL persisted-query IDs change**: persisted-query hashes can rotate when AllAnime updates their schema. The catalog-side raw JP path will hit the same breakage at the same time, so the failure surface is shared. Mitigation: the existing raw-jp probe + scraper canary will both surface the issue immediately; the maintenance bot's Pattern 6/7 dispatch handles it.
- **Survey candidates rotate or change scoping rules during Phase 26 implementation**: e.g., if Miruro passes the survey but adds JS challenges by the time the implementation sub-plan ships. Mitigation: each candidate's implementation sub-plan starts with a "re-verify this is still live and unprotected" task — same shape as Phase 24 SCRAPER-HEAL-20.
- **AnimeKai's anti-bot posture tightens during R&D**: anikai.to may add Turnstile or Cloudflare challenges. The CI-rejected anti-bot deps list (`utls`, `flaresolverr`, etc.) still applies. If AnimeKai requires those, SCRAPER-HEAL-27 stays open and we live with the escape hatch. No scope creep.
- **The `has_english` column populates slowly**: opportunistic population requires users to actually visit anime pages with the EN tab. For never-visited anime, the column stays false and the browse filter misses them. Acceptable for v3.1; a backfill cron is a v3.2 polish item.
- **Provider expansion grows the failover budget past 8s**: Phase 21's hard ≤ 8s budget assumed up to 3 servers being probed. Adding a 4th provider × N servers each could blow the budget. Mitigation: the orchestrator's per-server budget is fixed independent of provider count; if a provider's first server probes in 2s and fails, the next provider starts immediately. Worst case is N providers × first-server-only ≤ 2s = still under 8s for ≤ 4 providers. Validate with a synthetic 4-provider test.
</risks>

<dependencies>
## Phase Dependencies

- **Hard dependency on:** v3.0 Phase 15-19 (scraper microservice + provider interface). v3.1 Phase 24 (EnglishPlayer restored with dropdown infrastructure) for SCRAPER-HEAL-28 to be observable; SCRAPER-HEAL-25/26/27 are backend-only and ship independently of Phase 24.
- **Soft dependency on:** v3.1 Phase 25 (audit findings) — Phase 26's increased provider activity will exercise the maintenance-bot dispatch more often; better to have W-INT-* closed first so the increased signal volume hits a clean pipeline.
- **No dependency on:** future v3.2 phases (WARP egress, MinIO archival) — those are downstream of Phase 26, not parallel.
- **Blocks:** nothing inside v3.1. Once Phase 26 ships (with or without AnimeKai converging), v3.1 is GREEN and the milestone re-audit can run.
</dependencies>

<plan_sketch>
## Plan Sketch (for `/gsd-plan-phase` to flesh out)

**Wave 1 — AllAnime Lift (SCRAPER-HEAL-25 + browse filter)**

- 26-01-PLAN.md: Create `services/scraper/internal/providers/allanime/` package — `client.go`, `dto.go`, `cache.go`, `client_test.go` against captured `testdata/allanime/*.json` goldens. Implement `domain.Provider`. Register in `main.go` after gogoanime. Build + test + redeploy scraper.
- 26-02-PLAN.md: Browse filter activation — `has_english bool` GORM field on `Anime` + `SetHasEnglish` repo method + `english` case in providers filter switch + opportunistic setter in catalog's `GetScraperEpisodes` + `useBrowseFilters` Provider union widening + `BrowseSidebar` row + i18n key. Verify against the live AllAnime catalog after opportunistic backfill kicks in.

**Wave 2 — 2026 Survival Sweep (SCRAPER-HEAL-26)**

- 26-03-PLAN.md: Research artifact `.planning/research/2026-05-19-en-source-survival.md`. Curl-based probes against each candidate's public endpoints; verdict matrix. **Operator decision gate at end.**

**Wave 3a/3b — Survey Candidate Implementation (conditional on Wave 2 decision)**

- 26-04-PLAN.md: Survey candidate #1 (if picked) — package + tests + registration + production smoke. Follows the same pattern as 26-01.
- 26-05-PLAN.md: Survey candidate #2 (if picked) — same.

**Wave 4 — AnimeKai Recovery (SCRAPER-HEAL-27)**

- 26-06-PLAN.md: AnimeKai client.go bodies + MegaUp token generator in `docker/megacloud-extractor/server.js` `/animekai-token` handler. Goldens captured fresh against `anikai.to`. `SCRAPER_ANIMEKAI_ENABLED=true` end-to-end verification. **If R&D doesn't converge within 7 days of effort, requirement stays open and Phase 26 ships without it.**

**Wave 5 — Dropdown Polish (SCRAPER-HEAL-28)**

- 26-07-PLAN.md: `capitalizeProvider` branches for `allanime` / `animekai` / picked-survey-candidates. i18n labels per provider. Verify dropdown light-up via Playwright e2e against a logged-in `ui_audit_bot` user. `/animeenigma-after-update` final invocation closes the milestone.
</plan_sketch>
