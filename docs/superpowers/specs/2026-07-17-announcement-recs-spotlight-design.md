# Announcement-Aware Recommendations + "Upcoming for you" Spotlight Card

**Date:** 2026-07-17
**Status:** Approved (owner, this session)
**Origin:** admin feedback `2026-06-27T13-49-02_tNeymik_telegram` ("сделать нотификации анонсов") — notifications half explicitly deferred by owner this session; this spec covers the recs + announcement-recs + Spotlight surface. The future notification producer will reuse the matching engine built here.

**Metrics:** UXΔ = +3 (Better) · CDI = 0.06 * 21 · MVQ = Griffin 85%/80%

## Goals

1. **S8 franchise/sequel signal** in the recs engine — "a new entry in a franchise you scored highly" is, per owner, the single most valued matcher for announcements, and improves general recs too.
2. **Announcement discovery** — featured `anons` titles enter the catalog automatically (today they only arrive via calendar-sync-with-a-date or user search).
3. **Announcement matching** ("announce recs") — per-user scoring of announced titles using the signals that work for unaired content (S8 + S5 + S2), exposed as `GET /api/users/recs/upcoming`.
4. **`upcoming_for_you` Spotlight card** — login-only 10th card showing the user's best announcement match with two actions: *Add to Plan to Watch* and *Dismiss (never show this title again)*.
5. **Quality pass** — fix the broken spotlight→recs client (personal_pick personalized path 404s since the 2026-06-11 recs extraction), rebalance ensemble weights for S8, bump cache keys, reconcile the S11 spec divergence as intentional.

## Non-Goals (deferred)

- Announcement **notifications** (bell/toast/Telegram) — future session; will consume `UpcomingForUser` + the dismissals table built here.
- `announced → ongoing` transition detection.
- S9/S10 signals; tags (AniList) backfill changes beyond what exists.

## Architecture (Approach A — recs-owned matching)

```
scheduler (daily) ──HTTP──> catalog POST /api/anime/announcements-sync
                              └─ Shikimori GraphQL: status=anons, order=popularity, limit N
                              └─ import missing (genres+studios) + inline franchise fetch
                              └─ refresh stale existing announced rows

user ──> gateway ──> catalog GET /api/anime/spotlight (OptionalAuth)
             spotlight aggregator ── upcoming_for_you resolver (login-only)
                              └─HTTP, JWT-forwarded──> recs GET /api/users/recs/upcoming
                                        └─ pool: status='announced', not hidden/deleted,
                                          not in user's anime_list, not dismissed
                                        └─ announcement ensemble {S8:0.5, S5:0.3, S2:0.2}
                                        └─ floor threshold; top-K with match reason

card CTA "Add to plan"  ──> existing watchlist API (status=plan_to_watch)
card CTA "Dismiss"      ──> recs POST /api/users/recs/upcoming/dismiss {anime_id}
```

## Component detail

### 1. Catalog: announcement discovery sync

- New method on `catalog_sync.go` service: `SyncAnnouncements(ctx)`. Shikimori GraphQL query `animes(status: "anons", order: popularity, limit: N, page: 1)` (new parser method beside `GetTrendingAnime`; N sourced from the endpoint's `?limit=` query param, default 30, max 100 — **shipped as a request-time query param, not an env var**; see below). Popularity ordering IS the "featured" gate — only titles the community already anticipates get imported/refreshed here.
- Partition existing/missing by shikimori_id (reuse the calendar-sync partition/import pattern, including genres+studios persistence).
- **Franchise enrichment inline**: for imported/refreshed rows with `FranchiseChecked=false`, call `GetAnimeFranchise` (client exists) and set `Franchise`/`FranchiseChecked`. Pool is ≤N so inline is fine; respect Shikimori rate limits (sequential, small N).
- Endpoint `POST /api/anime/announcements-sync` (same auth posture as `calendar-sync`); scheduler job (daily, off-peak, minute jittered per fleet convention) in `services/scheduler/internal/jobs/` + registration in scheduler main. **Shipped:** two query params, both request-time (not env-configured) — `?limit=` (default 30, max 100, anons import size) and `?seed_backfill=` (default 40, max 200, franchise-enrichment backfill pool size for existing deduped rows); the scheduler job POSTs with no query string, so daily runs use the defaults.
- No new columns. `created_at` of the imported row is the "discovered at" proxy; matching does not need an announcement date.

### 2. Recs: S8 franchise signal

- `services/recs/internal/service/recs/signals/s8_franchise.go`, implements `SignalModule` (`ID()="S8"`).
- Request-time (no persistence): load user's franchise affinity — one query joining `anime_list` (score > 0) × `animes` (franchise ≠ ''): `franchise → MAX(score)`. Candidates joined to their franchise; raw score `clamp((maxScore−5)/5, 0, 1)`; candidates without franchise or unknown franchise → 0.
- Positive-only by design: low-scored/dropped franchises contribute 0 here — negative pressure remains S7's job (no double-penalty).
- **Ensemble (logged-in)** rebalanced: `{S1:0.27, S2:0.17, S3:0.17, S4:0.09, S5:0.17, S8:0.13, S7:−0.15}` (S7 stays appended LAST in every registry — established invariant). Anonymous ensemble unchanged.
- **Cache key bump**: `UserTopNKeySuffix` v4 → **v5**. Public trending key unchanged (v2).

### 3. Recs: announcement matching + dismissals

- New domain/service path `UpcomingForUser(ctx, userID) ([]UpcomingMatch, error)`:
  - Pool query (recs DB, same shared Postgres): `animes WHERE status='announced' AND hidden=false AND deleted_at IS NULL` minus `anime_list` rows for the user (any status) minus `rec_announcement_dismissals` rows for the user.
  - Score via a dedicated ensemble instance `{S8:0.50, S5:0.30, S2:0.20}` over the announced pool (pool-level min-max is fine — pool has multiple members; if pool size < 2, fall back to raw scores).
  - **Floor — shipped as a raw-score OR-gate, not the combined-score floor sketched above**: `RECS_UPCOMING_MIN_SCORE` was replaced before landing by two independent raw-score thresholds, `RECS_UPCOMING_MIN_S8` (default 0.2) and `RECS_UPCOMING_MIN_S2` (default 0.3) — a candidate passes if `raw_s8 >= RECS_UPCOMING_MIN_S8` (franchise affinity) **OR** `raw_s2 >= RECS_UPCOMING_MIN_S2` (genre Jaccard), evaluated on pre-normalization scores. Rationale: per-pool min-max normalization makes a combined-score floor meaningless for small announcement pools (a floor on normalized scores would always let the pool's own weakest members through relative to each other, defeating the "rarity is emergent" goal); gating on raw scores keeps the floor an absolute, pool-size-independent bar. Below floor still returns empty ⇒ card doesn't render.
  - Returns top-K (default 3) `UpcomingMatch{Anime (hydrated: id, names, poster, year/season, kind, score, franchise), MatchScore, Reason}`. `Reason` = `{kind:"franchise", seed_anime_id, seed_anime_name, user_score}` when S8 dominated (raw S8 ≥ 0.4), else `{kind:"taste"}`.
- **HTTP**: `GET /api/users/recs/upcoming` (JWT required — 401 for anon) → `{success, data:{items:[…]}}`. Cached `recs:user:<uid>:upcoming:v1`, TTL 6h. Cache busted by dismiss/list-add? No — dismiss handler busts the key explicitly; list-add propagates within TTL (acceptable; card FE optimistically advances locally).
- **Dismiss**: `POST /api/users/recs/upcoming/dismiss {anime_id}` (JWT) → insert `rec_announcement_dismissals(user_id, anime_id, created_at)` (GORM AutoMigrate, unique (user_id, anime_id) idempotent) + bust the upcoming cache key. Dismissal is permanent per title.
- Gateway: `/api/users/recs/upcoming*` rides the existing `/api/users/recs` proxy prefix to recs:8094 (verify prefix matching covers subpaths; extend if exact-match).

### 4. Spotlight card `upcoming_for_you` (5-anchor recipe + guidelines doc)

- **Resolver** (`cards/upcoming_for_you.go`): login-only — anon/no-JWT → `nil` card (pattern: now_watching). Calls a new `RecsClient` method `FetchUpcoming` forwarding the ctx JWT to `http://recs:8094/api/users/recs/upcoming`. Empty items → nil card (absent slide). Card payload `UpcomingForYouData{Items[≤3]}` (each: anime id/name/russian, poster_url, year, season, kind, reason fields). Multi-item so the FE can advance locally after add/dismiss without a spotlight refetch.
- **Client fix (bug)**: spotlight's recs fetch (`FetchUserRecs`, used by personal_pick) still targets `player:8083` where the route no longer exists → personalized personal_pick has silently degraded to trending fallback since 2026-06-11. Split/point recs calls to a `defaultRecsBaseURL = "http://recs:8094"`, keep player list calls on player:8083. personal_pick regains personalization. **Shipped without an env override** — `defaultRecsBaseURL` is a Go const in `recs_client.go`; `NewRecsClient("", nil, log)` in catalog's `main.go` always resolves to it. `SPOTLIGHT_RECS_URL` was dropped from the plan: every other spotlight client (player, notifications) also hardcodes its in-cluster default rather than reading an env var, so this stays consistent with precedent instead of adding a one-off knob.
- **SFC** (`frontend/web/src/components/spotlight/UpcomingForYouCard.vue`): wraps `SpotlightCardShell`, accent **cyan**, kicker icon `CalendarClock` (lucide, NAMED import), backdrop `poster-blur` with `SpotlightPoster`. Body: title (font-display), season/year + kind plain text, why-line — franchise: t('spotlight.upcomingForYou.reasonFranchise', {seed, score}); taste: t('…reasonTaste'). Single-root invariant; kicker font-medium.
- **CTAs** (`#cta` slot, `buttonVariants` styled `<button>`s — actions, not links): **Add to Plan to Watch** → existing watchlist store/API set status `plan_to_watch`, success → advance to next item / hide card locally; **Dismiss** → `POST …/dismiss`, same advance. Both idempotent-safe and error-toasted via existing patterns.
- **Wiring**: `cardTokens` entry (accent+kickerKey+icon), dispatch in `HeroSpotlightBlock` (+ `cardImageUrls()` poster prefetch at the same bucket), FE type in `types/spotlight.ts`, i18n keys in en/ru/ja (locale-parity test), DI in catalog main.go (resolver registered in the aggregator list — verify snapshot cache shape tolerance per the known cache-shape-migration gotcha; additive card types were designed to be tolerated, confirm).

### 5. Quality pass (rides along)

- `personal_pick` recs client fix (above) — the one behavioral bug fix.
- S11 divergence: **keep code behavior** (exclude candidates with ANY anime_list row — recommending a planned/on-hold title is redundant); annotate `2026-05-03-rec-engine-design.md` S11 section as superseded-by-implementation.
- S2 stays genres-only; studios remain S5's dimension (owner decision) — no code change, note in spec only.
- Cache versions after this spec: user topN **v5**, public trending v2, upcoming **v1**.

## Error handling

- Discovery sync: per-title import failures logged + skipped (batch continues); Shikimori failures → job error, next daily tick retries. Franchise fetch failure → leave `FranchiseChecked=false` (retried next sync).
- `UpcomingForUser`: signal errors degrade to that signal contributing zero (existing ensemble semantics); empty pool/below-floor → empty items, 200.
- Spotlight resolver: recs HTTP error/timeout → nil card (aggregator already tolerates absent resolvers; deadline via existing per-card budget).
- FE CTAs: failed add/dismiss → existing toast error pattern, card does NOT advance.

## Testing

- **Recs**: unit tests for S8 (affinity derivation, clamp, no-franchise zero, registry order S7-last), announcement pool exclusions (list rows, dismissals), floor/reason selection, dismiss idempotency + cache bust. Suite conventions of services/recs (sqlite/testcontainers per existing).
- **Catalog**: parser test for the anons GraphQL query (httptest, existing style); sync partition/import/franchise-enrich tests; resolver tests with fakes (cards/fakes_test.go pattern) — anon→nil, empty→nil, items→payload, recs-error→nil.
- **FE**: card component test (reasons, CTA advance/hide, error no-advance), locale-parity, `/frontend-verify` (DS-lint, i18n, real build) before landing.
- **E2E-ish**: manual verify on deploy via ui_audit_bot flow + real anime data (verify-streams convention: test with ACTUAL data).

## Rollout

Single worktree → land on main → `/animeenigma-after-update` (redeploys catalog, recs, scheduler, web; changelog Trump-mode). **Shipped env/config surface** (documented in `docs/environment-variables.md`; supersedes the knob list originally sketched here): `RECS_UPCOMING_TOPK` (default 3), `RECS_UPCOMING_MIN_S8` (default 0.2), `RECS_UPCOMING_MIN_S2` (default 0.3), scheduler `ANNOUNCEMENTS_SYNC_CRON` (default `"23 5 * * *"`). The announcements-sync size knobs are request-time query params (`?limit=`, default 30; `?seed_backfill=`, default 40) on `POST /api/anime/announcements-sync`, not env vars — there is no `ANNOUNCEMENTS_SYNC_LIMIT`. There is no `SPOTLIGHT_RECS_URL` — the spotlight recs client's `http://recs:8094` default is a Go const, not env-overridable (see §4 above).
