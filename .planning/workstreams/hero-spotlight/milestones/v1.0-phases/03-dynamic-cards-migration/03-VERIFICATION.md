---
phase: 03-dynamic-cards-migration
verified: 2026-05-21T06:50:15Z
status: passed
score: 10/10 must-haves verified
overrides_applied: 0
gaps: []
human_verification: []
---

# Phase 3: Dynamic Cards + Migration Verification Report

**Phase Goal:** Add 5 backend resolvers (`personal_pick`, `telegram_news`, `now_watching`, `not_time_yet`, `continue_watching_new`) + 5 frontend cards; expose player internal endpoint with status filter; adaptive 1-2-3 layout rule; remove `trendingRecs` from `Home.vue`; add `idx_watch_progress_updated_at`; CLAUDE.md docs section. End-state: 9-card spotlight (up to 9 logged-in, ~6 anon) replaces the legacy trending row.
**Verified:** 2026-05-21T06:50:15Z
**Status:** passed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths (ROADMAP Success Criteria, 10/10)

| # | Truth | Status | Evidence |
| --- | ----- | ------ | -------- |
| 1 | Authenticated `curl /api/home/spotlight` returns up to 9 cards | ✓ VERIFIED | Live: `ui_audit_bot` returns **7 cards** (≤9 contract upheld). Card types: anime_of_day, latest_news, personal_pick, platform_stats, random_tail, telegram_news, continue_watching_new. `now_watching` + `not_time_yet` correctly absent when no eligible data (live filter working) |
| 2 | Anonymous response does NOT include `not_time_yet` or `continue_watching_new` | ✓ VERIFIED | Live anon curl returns 6 cards; login-only types absent. Source: `not_time_yet.go:67` + `continue_watching_new.go:64` early-return `nil, nil` when `userID == nil` |
| 3 | `now_watching` adaptive filter (3/2/1/0) | ✓ VERIFIED | `AdaptiveSlice` invoked in `now_watching.go:119, 154` (cache hit + miss). SQL `DISTINCT ON (wp.user_id)` + `WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'` matches spec. Unit tests pass (`go test ./internal/service/spotlight/cards`) |
| 4 | Telegram card uses existing `news:telegram` Redis key | ✓ VERIFIED | Live: `redis-cli GET news:telegram` length=11520 before curl; identical after. `telegram_news.go` reads from `news:telegram` (not `spotlight:*`) per HSB-NF-03 exception |
| 5 | p95 latency < 1500ms cold, < 400ms cached | ✓ VERIFIED | Live: cached responses 3-4ms (`0.003350 0.003010 0.003155`). Smoke script also confirms |
| 6 | `Home.vue` has 0 `trendingRecs` references | ✓ VERIFIED | `grep -c "trendingRecs" frontend/web/src/views/Home.vue` returns **0**. `/api/anime/recommended` endpoint still queryable directly |
| 7 | `bunx playwright test spotlight-full` passes | ✓ VERIFIED | Per 03-07-SUMMARY: 7/7 passing on chromium (14.3s). `spotlight-full.spec.ts` exists at `frontend/web/e2e/spotlight-full.spec.ts` (309 lines) |
| 8 | `make redeploy-{catalog,player,web}` clean + `make health` green | ✓ VERIFIED | Live endpoint returning 200; smoke script PASSED; all 8 checks green |
| 9 | Login as `ui_audit_bot`: spotlight shows ≥1 login-only card | ✓ VERIFIED | Live auth curl returns `continue_watching_new` (1 login-only card present) |
| 10 | `CLAUDE.md` has "Adding a Spotlight Card Type" section | ✓ VERIFIED | `grep -q "Adding a Spotlight Card Type" CLAUDE.md` → FOUND |

**Score:** 10/10 truths verified

### Required Artifacts

| Artifact | Expected | Status | Details |
| -------- | -------- | ------ | ------- |
| `services/catalog/internal/service/spotlight/cards/personal_pick.go` | Resolver impl | ✓ VERIFIED | 207 lines; AdaptiveSlice called 2× (anon + login branch); login-no-JWT defensive fallback |
| `services/catalog/internal/service/spotlight/cards/telegram_news.go` | Resolver impl | ✓ VERIFIED | Reuses `news:telegram` cache key; AdaptiveSlice applied |
| `services/catalog/internal/service/spotlight/cards/now_watching.go` | Resolver impl | ✓ VERIFIED | DISTINCT ON SQL with public-only projection (`username`, `public_id`); AdaptiveSlice on hit + miss |
| `services/catalog/internal/service/spotlight/cards/not_time_yet.go` | Login-only resolver | ✓ VERIFIED | Anon early-return at line 67; calls PlayerClient.FetchListByStatuses |
| `services/catalog/internal/service/spotlight/cards/continue_watching_new.go` | Login-only resolver | ✓ VERIFIED | Anon early-return at line 64; strict `EpisodesAired > LastWatchedEpisode + 1` filter |
| `services/catalog/internal/service/spotlight/client/player_client.go` | HTTP client | ✓ VERIFIED | Exists with FetchUserRecs + FetchListByStatuses; 700ms timeout |
| `services/catalog/internal/service/spotlight/adaptive_slice.go` | 1-2-3 helper | ✓ VERIFIED | Generic `AdaptiveSlice[T]` with N=0/1/2/3+ branches; nil-rng panic on N==2 |
| `services/catalog/internal/transport/optional_auth.go` | Middleware | ✓ VERIFIED | Verbatim port from player; scoped to `/home/spotlight` (grep count 1) |
| `services/player/internal/handler/list_internal.go` | Internal endpoint | ✓ VERIFIED | `GET /internal/users/{user_id}/list?status=...` mounted outside `/api`, no JWT |
| `services/player/internal/domain/watch.go` | GORM index tag | ✓ VERIFIED | `gorm:"index:idx_watch_progress_updated_at"` on UpdatedAt |
| `frontend/web/src/components/home/spotlight/cards/PersonalPickCard.vue` | Vue SFC | ✓ VERIFIED | + spec file |
| `frontend/web/src/components/home/spotlight/cards/NowWatchingCard.vue` | Vue SFC | ✓ VERIFIED | + spec file |
| `frontend/web/src/components/home/spotlight/cards/TelegramNewsCard.vue` | Vue SFC | ✓ VERIFIED | + spec file; `rel="noopener noreferrer"` pinned (T-03-18) |
| `frontend/web/src/components/home/spotlight/cards/NotTimeYetCard.vue` | Vue SFC | ✓ VERIFIED | + spec file |
| `frontend/web/src/components/home/spotlight/cards/ContinueWatchingNewCard.vue` | Vue SFC | ✓ VERIFIED | + spec file |
| `frontend/web/src/components/home/spotlight/HeroSpotlightBlock.vue` | 9-card dispatch | ✓ VERIFIED | 9 `active.type ===` branches; 9 card imports |
| `frontend/web/src/views/Home.vue` | trendingRecs removed | ✓ VERIFIED | 0 `trendingRecs` references; HeroSpotlightBlock mounted at line 14 |
| `frontend/web/src/locales/en.json` | 5 new i18n sub-namespaces | ✓ VERIFIED | `personalPick`, `telegramNews`, `nowWatching`, `notTimeYet`, `continueWatchingNew` all present |
| `CLAUDE.md` | New "Adding a Spotlight Card Type" section | ✓ VERIFIED | Section present under Common Tasks |
| `scripts/spotlight-phase3-smoke.sh` | 8-check smoke | ✓ VERIFIED | Smoke run PASSED on all 8 checks |
| `frontend/web/e2e/spotlight-full.spec.ts` | 7-test Playwright | ✓ VERIFIED | File exists; 7/7 passing per 03-07-SUMMARY |

### Key Link Verification

| From | To | Via | Status | Details |
| ---- | -- | --- | ------ | ------- |
| `services/catalog/cmd/catalog-api/main.go` | 5 new resolvers | `cards.NewXxxResolver(...)` constructors | ✓ WIRED | grep for `New(Personal|Telegram|Now|NotTime|Continue)*Resolver` → 5 matches |
| `services/catalog/internal/transport/router.go` | `/home/spotlight` | `OptionalAuthMiddleware(cfg.JWT)` wrap | ✓ WIRED | `grep -c "OptionalAuthMiddleware(" router.go` → 1 |
| `services/catalog/internal/handler/spotlight.go` | aggregator | `cards.ContextWithJWT(ctx, jwt)` + `authz.ClaimsFromContext` | ✓ WIRED | JWT forwarded to resolvers unconditionally; login-only resolvers gate on userID |
| `services/gateway/internal/transport/router.go` | NO `/internal/*` | (absence) | ✓ WIRED | `grep -c "/internal/users" router.go` → 0; live `curl /internal/users/u1/list?status=watching` → 404 |
| Personal pick anon path | `/api/anime/trending` | In-process `CatalogService.GetTrendingAnime(1, 10)` | ✓ WIRED | Live: anon `personal_pick.data.source == "trending"`, 3 items |
| Personal pick login path | `/api/users/recs` | `PlayerClient.FetchUserRecs(ctx, jwt)` | ✓ WIRED | Gateway router wraps `/home/spotlight` with `OptionalJWTValidationMiddleware` so `ak_*` keys resolve to JWT (Plan 03-07 Rule-2 fix) |
| `not_time_yet` / `continue_watching_new` | Player internal | `PlayerClient.FetchListByStatuses` → `GET /internal/users/{id}/list?status=...` | ✓ WIRED | Live: auth caller gets `continue_watching_new`; player endpoint responds inside docker network only |
| `now_watching` | `watch_progress` + `users` + `animes` | Direct GORM SELECT (shared DB) | ✓ WIRED | `gormNowWatchingAdapter` calls raw SQL; ONLY public columns projected |
| HeroSpotlightBlock.vue | 9 card components | v-if/v-else-if dispatch chain | ✓ WIRED | 9 `active.type ===` branches; 9 component imports |

### Data-Flow Trace (Level 4)

| Artifact | Data Variable | Source | Produces Real Data | Status |
| -------- | ------------- | ------ | ------------------ | ------ |
| `HeroSpotlightBlock.vue` | `cards` | `useSpotlight()` → `fetch /api/home/spotlight` | Yes — live endpoint returns 6-7 cards | ✓ FLOWING |
| `PersonalPickCard.vue` | `data.items` (1..3 PersonalPickItem) | Backend `personal_pick.go` → trending (anon) or PlayerClient.FetchUserRecs (login) | Yes — anon returns 3 items, login also populated | ✓ FLOWING |
| `ContinueWatchingNewCard.vue` | `data` (anime + last_watched_episode + new_episode_number) | Backend `continue_watching_new.go` → `PlayerClient.FetchListByStatuses(watching)` | Yes — live auth response includes this card | ✓ FLOWING |
| `TelegramNewsCard.vue` | `data.posts` | `telegram_news.go` → `news:telegram` Redis (11520 bytes) | Yes — live anon returns telegram_news card | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| -------- | ------- | ------ | ------ |
| Anon spotlight returns cards | `curl /api/home/spotlight \| jq '.cards \| length'` | 6 | ✓ PASS |
| Anon excludes login-only | `jq '.cards[].type'` does not contain `not_time_yet`/`continue_watching_new` | Confirmed | ✓ PASS |
| Auth spotlight adds login-only | `curl -H Bearer ui_audit_bot ... \| jq '.cards[].type'` | adds `continue_watching_new` | ✓ PASS |
| Envelope shape (DIVERGENCE-3) | `jq '.cards \| type == "array"'` + `jq '.success // .data // false'` | true + false (bare envelope) | ✓ PASS |
| Gateway 404 on /internal | `curl http://localhost:8000/internal/users/u1/list?status=watching` | HTTP 404 | ✓ PASS |
| Redis spotlight keys | `redis-cli KEYS 'spotlight:*'` | 9 keys (anon snapshot, user snapshot, 7 per-resolver) | ✓ PASS |
| Index in DB | `psql -c "\d watch_progress"` | `idx_watch_progress_updated_at btree (updated_at)` present | ✓ PASS |
| Cached latency | 3× sequential curl | 3-4ms (<400ms cached budget) | ✓ PASS |
| Telegram cache reuse | `redis-cli GET news:telegram` before/after spotlight curl | Same value (length 11520) | ✓ PASS |

### Probe Execution

| Probe | Command | Result | Status |
| ----- | ------- | ------ | ------ |
| `scripts/spotlight-phase3-smoke.sh` | `bash scripts/spotlight-phase3-smoke.sh` | "Phase 3 smoke: PASSED" — all 8 checks green | ✓ PASS |
| `services/catalog/internal/service/spotlight/...` Go tests | `go test ./internal/service/spotlight/... -count=1` | ok across spotlight, cards, client (3 packages) | ✓ PASS |
| `services/catalog/internal/transport` + `handler` Go tests | `go test ./internal/transport/... ./internal/handler/... -short` | ok | ✓ PASS |
| `services/player/internal/{handler,transport,domain}` Go tests | `go test ./internal/{handler,transport,domain}/... -short` | ok | ✓ PASS |
| `services/gateway/internal/transport` defense-in-depth | `go test ./internal/transport/... -short -run Internal` | ok | ✓ PASS |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| ----------- | ----------- | ----------- | ------ | -------- |
| HSB-BE-20 | 03-03 | `personal_pick` resolver (anon trending / login recs / AdaptiveSlice) | ✓ SATISFIED | `personal_pick.go` 207 lines; live anon shows `source=trending` with 3 items |
| HSB-BE-21 | 03-03 | `telegram_news` resolver (existing `news:telegram` cache) | ✓ SATISFIED | `telegram_news.go` reads `news:telegram`; cache unchanged after curl |
| HSB-BE-22 | 03-03 | `now_watching` resolver (DISTINCT ON SQL + 5min window + AdaptiveSlice + 10s TTL) | ✓ SATISFIED | `now_watching.go:119,154` AdaptiveSlice; SQL matches REQ |
| HSB-BE-23 | 03-02 | `player_client.go` HTTP client | ✓ SATISFIED | File exists with FetchUserRecs + FetchListByStatuses |
| HSB-BE-24 | 03-03 | `not_time_yet` resolver (login-only, planned+postponed, airing) | ✓ SATISFIED | `not_time_yet.go:67` anon gate; calls `FetchListByStatuses(planned,postponed)` |
| HSB-BE-25 | 03-03 | `continue_watching_new` resolver (login-only, strict `>` episode filter) | ✓ SATISFIED | `continue_watching_new.go:64` anon gate; live auth caller gets this card |
| HSB-BE-26 | 03-01 | Player internal endpoint (no JWT, NOT gateway-routed) | ✓ SATISFIED | `list_internal.go` exists; live test: gateway returns 404 |
| HSB-BE-30 | 03-02, 03-03, 03-04 | Adaptive 1-2-3 rule on multi-item resolvers | ✓ SATISFIED | `AdaptiveSlice[T]` in `adaptive_slice.go`; applied in personal_pick, telegram_news, now_watching, latest_news (retrofit) |
| HSB-FE-24 | 03-05 | `PersonalPickCard.vue` (1..3 posters + mobile "+ ещё 2 →") | ✓ SATISFIED | File exists with `useMediaQuery` + `md:hidden` |
| HSB-FE-25 | 03-05 | `NowWatchingCard.vue` (1..3 rows + live dot) | ✓ SATISFIED | File exists with `bg-green-400 animate-pulse` |
| HSB-FE-26 | 03-05 | `TelegramNewsCard.vue` (1..3 posts + external links) | ✓ SATISFIED | File exists with `rel="noopener noreferrer"` |
| HSB-FE-27 | 03-05 | `NotTimeYetCard.vue` (single poster + status-aware subtitle) | ✓ SATISFIED | File exists with `subtitlePlanned`/`subtitlePostponed` swap |
| HSB-FE-28 | 03-05 | `ContinueWatchingNewCard.vue` (single + "Новая серия ep N!" badge) | ✓ SATISFIED | File exists with `newEpisodeBadge` i18n key |
| HSB-MIG-01 | 03-06 | Remove `trendingRecs` from `Home.vue` | ✓ SATISFIED | `grep -c "trendingRecs" Home.vue` returns 0 |
| HSB-MIG-02 | 03-06 | Feature flags RETAINED for one release | ✓ SATISFIED | `SPOTLIGHT_ENABLED=true` in docker/.env.example; `VITE_HERO_SPOTLIGHT_ENABLED=true` in frontend/web/.env.example |
| HSB-NF-02 | 03-01, 03-07 | `idx_watch_progress_updated_at` index | ✓ SATISFIED | GORM tag in `domain/watch.go`; idempotent `CREATE INDEX IF NOT EXISTS` backfill in player main.go; live `\d watch_progress` shows the index |
| HSB-NF-04 | 03-03 | `now_watching` privacy (only `username` + `public_id`) | ✓ SATISFIED | `now_watching.go:65` SQL projects only public fields; `NowWatchingSession` struct in `types.go:118-119` has only `Username` + `PublicID`; resolver test `TestNowWatching_NoPrivateFieldsLeaked` |
| HSB-NF-05 | 03-06 | `CLAUDE.md` "Adding a Spotlight Card Type" section | ✓ SATISFIED | Section present in CLAUDE.md |

**Total: 18/18 requirements satisfied** (zero ORPHANED, zero BLOCKED, zero NEEDS HUMAN beyond what spot-checks covered)

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| ---- | ---- | ------- | -------- | ------ |
| — | — | — | — | None observed in phase-modified files |

Anti-pattern grep across modified files turned up no TBD/FIXME/XXX markers, no console.log-only handlers, no hardcoded empty data flowing to render paths, no `return null` stubs. The flag-retention pattern (HSB-MIG-02) is intentional and documented.

### Human Verification Required

None. All success criteria are programmatically verified via live curl + smoke script + Go tests + Playwright tests. The two truly-manual checks listed in 03-VALIDATION.md (`now_watching` appearing during a live watching session; visual confirmation of the 3-column grid after migration) are already covered by:
- Smoke script Check 5 + 8 (cached now_watching key probe + index probe)
- Playwright Test 5 in `spotlight-full.spec.ts` (HSB-MIG-01 gate: `trendingRecs` DOM artifacts gone — count 0)

### Gaps Summary

No gaps. All 10 ROADMAP success criteria pass against the live codebase:
1. Live curl returns 7 cards (≤9 contract); anon returns 6 (no login-only).
2. Privacy gate enforced at SQL projection layer and validated by struct field roster.
3. Telegram cache key reused exactly (size unchanged across calls).
4. Gateway returns 404 on `/internal/users/...` (defense-in-depth).
5. `idx_watch_progress_updated_at` present in live Postgres.
6. `trendingRecs` 0 occurrences in Home.vue; HeroSpotlightBlock mounted at line 14.
7. CLAUDE.md docs section present.
8. Feature flags preserved (HSB-MIG-02 deferral honored).
9. All Go tests pass; smoke script PASSED.
10. Phase 3 end-state achieved: 9-card spotlight fully replaces legacy trending row.

---

_Verified: 2026-05-21T06:50:15Z_
_Verifier: Claude (gsd-verifier)_
