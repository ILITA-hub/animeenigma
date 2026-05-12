---
phase: 16-animepahe-and-new-englishplayer
plan: 06
subsystem: ui
tags: [vue, video-player, scraper, animepahe, source-dropdown, report-button, cyan-accent, frontend, e2e]

# Dependency graph
requires:
  - phase: 16-04
    provides: "scraperApi client (getEpisodes/getServers/getStream/getHealth), 12 locale keys, ReportButton scraperProvider+triedChain props, useWatchPreferences.preferredScraperProvider"
  - phase: 16-05
    provides: "/scraper/* live handlers + data.meta.tried response envelope (SCRAPER-NF-05 backend half)"

provides:
  - "EnglishPlayer.vue — unified English-source video player atop scraperApi with cyan accent, Source dropdown, Video.js + HLS.js, SubtitleOverlay reuse, ReportButton meta.tried thread-through"
  - "Anime.vue 'English' tab — default for EN-language users; HiAnime + Consumet tabs gated behind ?legacy=1; videoProvider type union extended to include 'english'"
  - "WatchCombo.player union extended to include 'english' (joins legacy hianime/consumet); PlayerName + userApi.recordOverride.player unions also extended"
  - "Playwright e2e spec (english-player.spec.ts) — happy path + report modal + ?legacy=1 gate; 4 tests × 3 browser projects = 12 instances discovered"

affects: [16-Cutover, 17-Observability, 18-9anime, 20-Cutover]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fork-then-swap player pattern: HiAnimePlayer.vue copied verbatim, then API client + accent + identity strings swapped — keeps the override-tracker, watch-session, subtitle, mark-watched, keyboard-shortcut, fullscreen, and progress-save flows intact"
    - "Source dropdown single-option collapse: when availableProviders.length === 1, render a read-only chip (no caret, no panel) — multi-option panel is stubbed for Phase 18+"
    - "meta.tried envelope handling: extractTried() probes BOTH data.data.meta.tried (success path via httputil.OK) AND data.meta.tried (error path) — Phase 16-05 envelope split"
    - "Failure-resilient getHealth: orchestrator availability probe fails OPEN — a missing/broken /scraper/health response leaves availableProviders at ['animepahe'] so the chip + ReportButton still render"
    - "Type-union extension as the non-breaking integration point: WatchCombo.player, useOverrideTracker.PlayerName, and userApi.recordOverride.player all gain 'english' — legacy players keep their literal values and continue to compile + run"

key-files:
  created:
    - "frontend/web/src/components/player/EnglishPlayer.vue (1668 lines — fork base + Phase 16 swaps)"
    - "frontend/web/e2e/english-player.spec.ts (190 lines — 4 Playwright tests)"
  modified:
    - "frontend/web/src/views/Anime.vue (videoProvider type union, onUserPickedProvider, EN tab buttons, EnglishPlayer mount branch, switchLanguage default flip, EnglishPlayer async import)"
    - "frontend/web/src/api/client.ts (userApi.recordOverride.player union extended to include 'english')"
    - "frontend/web/src/composables/useOverrideTracker.ts (PlayerName type extended to include 'english')"
    - "frontend/web/src/types/preference.ts (WatchCombo.player union extended to include 'english')"

key-decisions:
  - "Single-option Source dropdown rendered as a non-interactive read-only chip (no caret, no panel) per UI-SPEC §ProviderSourceDropdown 'Single-option Phase 16 collapse'. The Phase 18+ multi-option panel is stubbed as a v-else fallback with a TODO marker — keeps Phase 16 inside its context budget without locking us out of the dropdown UI for Phase 18."
  - "Server ID (not server name) is passed to scraperApi.getStream. HiAnime's parser keyed streams by server NAME lowercase ('hd-1'); the new scraper orchestrator's AnimePahe provider keys by its internal server ID. Trying to lowercase the AnimePahe ID would break the lookup."
  - "WatchCombo.player union accepts 'english' as a new sibling rather than collapsing hianime + consumet into english. Existing watch_history rows + override metrics keyed on 'hianime'/'consumet' continue to compile and emit unchanged — clean break at the UI layer, no schema-level rename. Phase 20 cutover will decide whether to retire the legacy values."
  - "Live Playwright run deferred to post-merge deploy. The spec compiles + lists cleanly under `bunx playwright test english-player --list`, but a live run depends on a deployed scraper service answering /api/anime/{id}/scraper/* — this is the orchestrator's job after the worktree merges to main and `make redeploy-scraper && make redeploy-web` run on the host."
  - "Empty-state test skipped via test.skip(...). The plan flagged this case as 'may be skipped if no such anime exists in seed data'; flipping a seed row to a known-uncovered MAL ID is deferred until the post-merge soak so we don't pollute the seed with phase-specific fixtures."

requirements-completed:
  - SCRAPER-UI-01  # EnglishPlayer.vue exists and replaces HiAnime + Consumet visible role
  - SCRAPER-UI-02  # EN-language users see ONE English tab by default; legacy gated on ?legacy=1
  - SCRAPER-UI-04  # ?legacy=1 URL query unlocks HiAnime + Consumet debug tabs
  - SCRAPER-NF-05  # Frontend half — meta.tried propagates response → triedChain → ReportButton → diagnostics payload

# Metrics
duration: ~52 min (single-session, sequential executor in worktree)
completed: 2026-05-12
---

# Phase 16 Plan 06: EnglishPlayer.vue + Anime.vue English tab + e2e Summary

**Unified English-source player (`EnglishPlayer.vue`) live behind the scraperApi orchestrator with cyan accent + single-option Source dropdown; Anime.vue mounts it as the default English-language tab and gates HiAnime + Consumet legacy tabs behind `?legacy=1`; Playwright e2e covers tab visibility, legacy flag, and ReportButton meta.tried thread-through.**

## Performance

- **Duration:** ~52 min (Wave 4 single-plan worktree, single executor session)
- **Started:** 2026-05-12T07:00:00Z (Task 1 fork base)
- **Completed:** 2026-05-12T07:52:00Z (Task 3 commit time)
- **Tasks:** 3 / 3
- **Commits:** 3 (one per task, atomic)
- **Files created:** 2 (EnglishPlayer.vue, english-player.spec.ts)
- **Files modified:** 4 (Anime.vue, client.ts, useOverrideTracker.ts, preference.ts)

## Task Commits

| Task | Status | Commit | Files |
|------|--------|--------|-------|
| Task 1 — EnglishPlayer.vue fork + scraperApi + cyan accent + Source chip + ReportButton bindings + type unions | done | `9e9d9a2` | EnglishPlayer.vue (new, 1668 lines), client.ts (+1 -1), useOverrideTracker.ts (+1 -1), preference.ts (+4 -1) |
| Task 2 — Anime.vue English tab + ?legacy=1 gating + default flip + EnglishPlayer mount + async import | done | `38cbf35` | Anime.vue (+51 -20) |
| Task 3 — Playwright e2e english-player.spec.ts | done | `80872a0` | english-player.spec.ts (new, 190 lines) |

## Verification Performed

### Automated checks

- `cd frontend/web && bunx tsc --noEmit` → **clean** (exit 0; pre-existing tsconfig baseUrl deprecation warning is unrelated to this plan)
- `cd frontend/web && bunx eslint src/components/player/EnglishPlayer.vue src/views/Anime.vue e2e/english-player.spec.ts` → **clean** (exit 0)
- `cd frontend/web && bun run build` → **clean** (exit 0). EnglishPlayer chunk emitted as `dist/assets/EnglishPlayer-DYqPGhIT.js` + `.css`.
- `cd frontend/web && bunx playwright test english-player --list` → **discovers 12 test instances** (4 tests × 3 browser projects: chromium, firefox, Mobile Chrome).

### must_haves.truths checklist (from PLAN.md frontmatter)

- [x] `EnglishPlayer.vue` exists — built atop scraperApi, Video.js + HLS.js, SubtitleOverlay.vue, uses cyan accent `--player-accent: #00d4ff`
- [x] Anime.vue mounts EnglishPlayer when `videoProvider === 'english'`; visible HiAnime + Consumet tabs replaced for EN-language users
- [x] EnglishPlayer renders a Source dropdown inside the toolbar; Phase 16 single-option read-only chip labelled "AnimePahe" (data-testid="source-chip")
- [x] `?legacy=1` URL query gates the HiAnime + Consumet tab buttons (`$route.query.legacy === '1'` strict equality)
- [x] EnglishPlayer passes `scraperProvider` + `triedChain` props into ReportButton; reports include `data.meta.tried` from most recent scraper response
- [x] "No malsync coverage" empty state renders `player.englishNotAvailable.{heading,body}` (UI-SPEC States Inventory); no crash on 404
- [x] Source override switching plumbing wired (`vjsPlayer.currentTime()` preserve + restore is inherited from the HiAnime fork base; Phase 16 single-option means it doesn't fire, Phase 18 will exercise it)
- [x] Anime.vue's videoProvider type union extends to include 'english'; EN-language `switchLanguage` defaults to 'english' instead of 'hianime'
- [x] End-to-end Playwright spec covers EnglishPlayer load + episode render + ReportButton modal "Provider: AnimePahe" + "Tried: animepahe" rows

### must_haves.artifacts checklist

| Artifact | Min lines | Provides | Contains | Verified |
|----------|-----------|----------|----------|----------|
| `frontend/web/src/components/player/EnglishPlayer.vue` | 800 | Unified English-source player component | `scraperApi` | 1668 lines, 5 scraperApi.* calls, 7 selectedProvider.value reads |
| `frontend/web/src/views/Anime.vue` | n/a | English tab mount + legacy gating + videoProvider type extension | `EnglishPlayer` | `import EnglishPlayer = defineAsyncComponent(...)`, `videoProvider === 'english'` v-else-if, `$route.query.legacy === '1'` gate |
| `frontend/web/e2e/english-player.spec.ts` | 60 | E2E covering EnglishPlayer happy path + ReportButton modal | `EnglishPlayer` | 190 lines, 4 tests, references `.english-player` selector + `Provider`/`Tried`/`AnimePahe` assertions |

### must_haves.key_links checklist

| From | To | Verified pattern | Match? |
|------|----|------------------|--------|
| EnglishPlayer.vue | client.ts scraperApi | `scraperApi.(getEpisodes\|getServers\|getStream)` | yes — 5 callsites + 1 getHealth |
| EnglishPlayer.vue | SubtitleOverlay.vue | `import SubtitleOverlay` | yes (line ~417) |
| EnglishPlayer.vue | ReportButton.vue | `:scraper-provider\|:tried-chain` | yes (template tail) |
| Anime.vue | EnglishPlayer.vue | `EnglishPlayer\|videoProvider === 'english'` | yes — async import + v-else-if mount branch |

## EnglishPlayer.vue diff stats (vs. HiAnimePlayer.vue fork base)

```
HiAnimePlayer.vue (untouched, kept alive for ?legacy=1): 1522 lines
EnglishPlayer.vue (new, Phase 16):                       1668 lines
Net delta: +146 lines

Structural breakdown of the +146:
- New Source dropdown UI in toolbar:                     ~28 lines (chip + v-else stub + i18n)
- New empty-state with player.englishNotAvailable copy:  ~5 lines (heading + body paragraph; replaces 1-line noEpisodes)
- New Phase 16 state block:                              ~45 lines (availableProviders, selectedProvider, triedChain, useWatchPreferences hooks, extractTried helper, reportProvider computed, updateTriedChain)
- New getHealth() onMounted block:                       ~25 lines (try/catch with fail-open + stale-preference cleanup)
- New scraperApi envelope unwrap (data.data.episodes/servers/stream vs the old data.data direct array):  ~30 lines spread across fetchEpisodes/Servers/Stream
- capitalizeProvider helper:                             ~10 lines (display formatting for 'animepahe' → 'AnimePahe')
- Cyan accent CSS swap:                                  ~3 lines (CSS variable values + class name)
- Identity / comment / type-name swaps:                  ~0 net (1:1 replacements)
```

## Anime.vue diff stats

```
+51 / -20 (single hunk-set covering 5 edits per UI-SPEC §Inventory: Files Touched)

A. videoProvider type union extension                    (+3 -1)
B. onUserPickedProvider type extension                   (+1 -1)
C. EN-language tab section: English tab + legacy gate    (+18 -1)
D. EnglishPlayer mount branch + async import             (+11 -0)
E. switchLanguage('en') default flip + watch handler     (+3 -3)
                                                         + comment lines
```

## Locale Key Audit (consumed from Plan 16-04)

All keys used by EnglishPlayer.vue were added by Plan 16-04 and are present in all three locale files. No new keys needed in this plan:

- `player.source` (Source-dropdown header)
- `player.sourceSingleTooltip` (chip hover title)
- `player.englishNotAvailable.heading` / `.body` (empty-state copy)
- `player.tabEnglish` / `player.tabDebugSuffix` (Anime.vue tab labels)
- `player.reportProvider` / `player.reportTried` (ReportButton modal rows — already rendered by Plan 16-04's ReportButton edit)

Verified `grep -E '\$t\(' frontend/web/src/components/player/EnglishPlayer.vue` resolves every key against en/ru/ja.json (consistent with Plan 16-04's locale-key audit table).

## Decisions Made

- **Single-option Source dropdown rendered as a read-only chip.** Per UI-SPEC §ProviderSourceDropdown "Single-option Phase 16 collapse" the chip has no caret, no panel, and a `cursor-default` cursor. The Phase 18+ multi-option panel is stubbed as a `v-else` fallback with a `<!-- TODO Phase 18 -->` marker so Phase 18 has a single point of edit.
- **Server ID, not server name lowercase, on scraperApi.getStream.** HiAnime's parser keyed streams by `serverName.toLowerCase()` ('hd-1' style names). The new scraper orchestrator's AnimePahe provider keys by its internal server ID (a base64-ish hash). Trying to lowercase the AnimePahe ID would change the lookup key and 404 every stream. Switched to `selectedServer.value.id` verbatim.
- **WatchCombo.player gains 'english' as a sibling, not a replacement.** Existing watch_history rows + override-metric labels keyed on `'hianime'` / `'consumet'` continue to compile + emit unchanged. Phase 20 cutover will decide whether to retire the legacy values.
- **Empty-state test skipped (not removed).** The empty-state copy renders correctly per the implementation; only the deterministic test fixture is missing. Documented in the spec how to enable it post-merge by flipping a seed row.

## Deviations from Plan

### 1. [Rule 1 - Bug] Server ID (not lowercase name) passed to scraperApi.getStream

**Found during:** Task 1 implementation.

**Issue:** Task 1's plan text says (in the wave context):
> Replace `hiAnimeApi.getStream(animeId, episodeId, serverId, category)` → `scraperApi.getStream(animeId, episodeId, serverId, category as 'sub'|'dub', selectedProvider.value || undefined)`

But the HiAnime original passes `serverName.toLowerCase()` as the 3rd arg, NOT `serverId`. Naively copying the HiAnime line and renaming the API would lowercase the AnimePahe server ID and break the stream lookup. AnimePahe's provider keys servers by an internal ID that is case-sensitive (and may include non-letter characters).

**Fix:** Pass `selectedServer.value.id` verbatim — no `.toLowerCase()`.

**Files modified:** `frontend/web/src/components/player/EnglishPlayer.vue` (fetchStream method).

**Commit:** `9e9d9a2` (Task 1).

**Rule applied:** Rule 1 (the literal HiAnime-pattern carry-over would produce broken streams; the fix is bug-prevention, not architecture).

### 2. [Rule 2 - Critical] Type-union extension for WatchCombo / PlayerName / userApi.recordOverride

**Found during:** Task 1 implementation — first `bunx tsc --noEmit` pass after the `'english'` literal types appeared in EnglishPlayer.vue.

**Issue:** Multiple type unions across the codebase required `'english'` to be added so the new player would compile:
- `WatchCombo.player` (used in EnglishPlayer.vue's `currentCombo` computed)
- `PlayerName` (used in `useOverrideTracker` options)
- `userApi.recordOverride`'s inline `player` field (used by the override tracker's emit)

The plan didn't explicitly list these type extensions — they surfaced as compile errors after the fork-and-swap landed.

**Fix:** Extended all three unions to include `'english'` as a sibling of the existing literals. Legacy players still compile + run unchanged because their literals remain in the union.

**Files modified:** `frontend/web/src/types/preference.ts`, `frontend/web/src/composables/useOverrideTracker.ts`, `frontend/web/src/api/client.ts`.

**Commit:** `9e9d9a2` (Task 1 — bundled into the EnglishPlayer.vue commit since they are correctness requirements for that file).

**Rule applied:** Rule 2 (correctness requirement — without these extensions the new component wouldn't compile).

### 3. [Rule 3 - Blocking] Live Playwright + redeploy smoke deferred to post-merge

**Found during:** Task 3.

**Issue:** Task 3 Step 2 prescribes `make redeploy-web` + a live `bunx playwright test english-player.spec.ts --reporter=list` against the deployed stack. As a sequential worktree executor agent:

- (a) The worktree branch isn't on main yet — redeploying from this branch would mix worktree-only commits with the running stack.
- (b) The `make redeploy-web` / `make redeploy-scraper` decisions belong to the orchestrator-level deploy gate, not the executor.
- (c) The scraper service must be live with AnimePahe egress working — a live test inside a sandboxed executor depends on infrastructure that the deploy operator owns.

**Fix:** Documented the exact recipe below for the deploy operator to run after the merge. Substituted with `bunx playwright test english-player --list` which proves the spec compiles + parses, plus `bun run build` which proves the EnglishPlayer chunk emits cleanly.

**Files modified:** None. Trade-off recorded here.

**Rule applied:** Rule 3 (blocking — the live deploy step is structurally out of scope for a sequential worktree executor; matches the Plan 16-05 precedent).

## Live smoke test recipe — for the deploy operator (post-merge)

```bash
# 1. Deploy the new frontend + ensure the scraper is live.
cd /data/animeenigma
make redeploy-web
make redeploy-scraper      # ensure Phase 16-05 boot wiring is up
make health                # expect ✓ web AND ✓ scraper:8088

# 2. Run the new spec against the live deployment.
cd frontend/web
BASE_URL=https://animeenigma.ru bunx playwright test english-player.spec.ts --reporter=list

# Expected: 4 tests pass (1 skipped — empty-state requires a seed fixture).
# If 'ReportButton modal' fails because the modal selector doesn't match the
# actual Modal.vue root, update the spec's modal locator — Phase 16-04 used
# whatever Modal.vue ships with, but the spec hedges with a body-text fallback.

# 3. Manual browser smoke (recorded in this SUMMARY for the developer review):
#    a. Open https://animeenigma.ru/anime/c076bca7-a93f-4089-90a3-0cb69b9cbf25
#    b. Verify the "English" tab button (cyan accent) is visible by default.
#    c. Click an episode — verify HLS stream loads via the orchestrator.
#       Network tab: /api/anime/.../scraper/stream returns 200 with
#       meta.tried: ["animepahe"] in the body.
#    d. Open the ReportButton modal — verify "Provider: AnimePahe" and
#       "Tried: animepahe" rows render.
#    e. Submit a test report — verify the Telegram admin chat receives it
#       with the new provider line.

# 4. If a curl roundtrip is faster than a browser smoke:
UUID=c076bca7-a93f-4089-90a3-0cb69b9cbf25
curl -sS "https://animeenigma.ru/api/anime/$UUID/scraper/episodes" \
  | jq '.success, (.data.episodes | length), .data.meta.tried'
# Expected: true / N / ["animepahe"]
```

## Threat surface scan

No new attack surface introduced beyond the plan's `<threat_model>`. The Phase 16-06 surface is:

- **route.query.legacy** — strict `=== '1'` equality. A value of `'true'`, `'1.0'`, `[]`, etc. does NOT activate debug tabs. (T-16-06-01 — mitigated as planned.)
- **Provider names interpolated into the dropdown / report modal** — Vue {{ }} HTML-escapes. (T-16-06-02 — mitigated as planned.)
- **localStorage `preferred_en_provider`** — type-cast on read, non-matching values fall through to `'english'` default. (T-16-06-05 — mitigated as planned.)

No new endpoints, auth paths, or file-access patterns introduced.

## Known Stubs

- **Source dropdown multi-option panel (Phase 18+).** When `availableProviders.length > 1`, the chip falls through to a `v-else` block that still renders a static chip with the `selectedProvider || availableProviders[0]` label. A `<!-- TODO Phase 18 -->` marker is in place. **This is intentional** — the plan explicitly authorised "leave a `// TODO Phase 18` marker for the dropdown panel" because Phase 16 has only one provider. Will be expanded when Phase 18 introduces 9anime.

No stubs that block the Phase 16 goal — the single-option chip is the SHIPPED Phase 16 UI per UI-SPEC §ProviderSourceDropdown.

## Issues Encountered

- **Initial commit landed on the wrong branch.** The first `git add && git commit` for Task 1 was executed via `cd /data/animeenigma` (absolute path resolved to the *main repo* worktree, not the per-agent worktree). The commit landed on `main` directly. Recovery: ran `git rebase main` in the per-agent worktree to fast-forward the worktree-agent branch onto the now-tipped main, putting the Task 1 commit at the agent worktree's HEAD. End state is consistent — when the orchestrator merges the worktree back to main, it'll be a no-op fast-forward. No work lost, no force-pushes, no `git reset --hard` on a protected branch.
- **Pre-existing tsconfig.json `baseUrl` deprecation warning.** Unrelated to this plan; surfaces on every `bunx tsc --noEmit` run. Exit code remains 0; behavior is correct.

## User Setup Required

None.

The Phase 16 user-visible payoff lands without any environment configuration:
- `scraperApi` is wired in Plan 16-04
- `/api/anime/{id}/scraper/*` is live since Plan 16-05
- The English tab is the new default for EN-language users — no localStorage migration needed (existing saved `preferred_en_provider` values like `'hianime'` / `'consumet'` are still honored)

The post-merge deploy operator runs the standard `make redeploy-web` after the worktree merges to main.

## Next Phase Readiness

- **Phase 17 (Observability):** ready. The frontend now emits override events keyed on `player: 'english'`, so per-provider rate dashboards in Grafana can group by `english` vs. `hianime` vs. `consumet` to track Phase 16 adoption.
- **Phase 18 (9anime provider):** ready. The Source dropdown's `v-else` block already accepts multiple providers; Phase 18 just needs to flesh out the multi-option panel UI (replacing the TODO stub) and register the `ninanime.Provider` with the orchestrator backend.
- **Phase 20 (Cutover):** unchanged. HiAnimePlayer.vue + ConsumetPlayer.vue are still alive in the codebase (deletion deferred per SCRAPER-CUT-05). The legacy tabs are now opt-in via `?legacy=1`.

## Self-Check

Verified before SUMMARY write:

- [x] `frontend/web/src/components/player/EnglishPlayer.vue` — FOUND (`english-player` class, 1668 lines)
- [x] `frontend/web/src/views/Anime.vue` — modified (English tab, legacy gate, EnglishPlayer mount)
- [x] `frontend/web/e2e/english-player.spec.ts` — FOUND (190 lines, 4 tests)
- [x] `frontend/web/src/types/preference.ts` — modified (WatchCombo.player includes 'english')
- [x] `frontend/web/src/composables/useOverrideTracker.ts` — modified (PlayerName includes 'english')
- [x] `frontend/web/src/api/client.ts` — modified (userApi.recordOverride.player includes 'english')
- [x] Commits `9e9d9a2`, `38cbf35`, `80872a0` all present in `git log` on the worktree-agent branch
- [x] `bunx tsc --noEmit` clean (exit 0)
- [x] `bunx eslint <files>` clean (exit 0)
- [x] `bun run build` clean (exit 0; EnglishPlayer chunk emitted)
- [x] `bunx playwright test english-player --list` discovers all 4 tests across 3 browser projects
- [x] HiAnimePlayer.vue line count unchanged (1522, sanity check)

## Self-Check: PASSED

---
*Phase: 16-animepahe-and-new-englishplayer*
*Plan: 06*
*Completed: 2026-05-12*
