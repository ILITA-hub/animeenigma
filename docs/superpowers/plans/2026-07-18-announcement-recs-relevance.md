# Announcement Recs — Relevance Hardening + MAL Popularity — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stop the announces card from admitting continuations via genre-taste; gate standalone titles on rich attribute affinity (S5); add relative MAL popularity as a ranking booster; make the reason line honest.

**Architecture:** Two-service change on shared `animeenigma` Postgres. Catalog sources MAL popularity via the existing Jikan client and stores it on `animes`. Recs reads it in a new S9 signal, reworks the upcoming admission gate + ranking, and enriches the reason line. Frontend renders the new reason kinds.

**Tech Stack:** Go (GORM, libs/errors, libs/logger), Vue 3 + i18n, Jikan (MAL) API.

## Global Constraints

- New columns via GORM `AutoMigrate` only — never drop/alter existing columns.
- Recs reads `animes` directly; `mal_members` must exist before S9 runs live (catalog AutoMigrate creates it) — but S9 compiles/tests independently (raw SQL read).
- Jikan failures must NEVER fail the sync or the card (best-effort, logged).
- Signal contract: `ID() recs.SignalID`, `Precompute(ctx, UserID) error`, `Score(ctx, UserID, []AnimeID) (map[AnimeID]RawScore, error)`.
- Cache key suffix bump `:upcoming:v1 → :upcoming:v2`.
- i18n parity en/ru/ja with matching ICU placeholders; run `/frontend-verify` for FE.
- No time-unit effort metrics.

---

### Task 1: Jikan popularity fields

**Files:**
- Modify: `services/catalog/internal/parser/jikan/client.go` (`AnimeInfo` struct)
- Test: `services/catalog/internal/parser/jikan/client_test.go` (create if absent)

**Interfaces:**
- Produces: `AnimeInfo.Members int`, `.Favorites int`, `.Popularity int` populated from Jikan `/anime/{id}` JSON.

- [ ] Add to `AnimeInfo`: `Members int \`json:"members"\``, `Favorites int \`json:"favorites"\``, `Popularity int \`json:"popularity"\``.
- [ ] Test: feed a fixture JSON body (`{"data":{"mal_id":1,"members":123456,"favorites":789,"popularity":42,...}}`) through a stubbed `httptest` server; assert the three fields parse. Reuse the existing client construction pattern (override `baseURL`).
- [ ] `go test ./services/catalog/internal/parser/jikan/...` passes.
- [ ] Commit.

---

### Task 2: `mal_members`/`mal_favorites` on animes + sync population

**Files:**
- Modify: `services/catalog/internal/domain/anime.go` (or wherever `Anime` struct lives — grep `type Anime struct`)
- Modify: `services/catalog/internal/service/catalog_sync.go` (`SyncAnnouncements`)
- Test: `services/catalog/internal/service/catalog_sync_*_test.go`

**Interfaces:**
- Consumes: Task 1 `AnimeInfo.Members/.Favorites`; the existing `jikanClient` field on the sync's service struct (grep `jikanClient`).
- Produces: `domain.Anime.MalMembers int`, `.MalFavorites int` persisted for announced titles.

- [ ] Add `MalMembers int \`gorm:"default:0" json:"mal_members"\`` and `MalFavorites int \`gorm:"default:0" json:"mal_favorites"\`` to the `Anime` struct.
- [ ] In `SyncAnnouncements`, after an announced title is upserted and has a non-empty `mal_id`: call `jikanClient.GetAnimeByID(ctx, anime.MalID)`; on success, persist `mal_members = info.Members`, `mal_favorites = info.Favorites` (a targeted `Model(&Anime{}).Where("id = ?", id).Updates(...)` — do NOT clobber other columns). On error: `log.Warnw` and continue (best-effort). Respect the client's built-in rate limiter (no extra sleeps needed).
- [ ] Guard: only fetch for titles missing/zero `mal_members` OR always-refresh (choose always-refresh — popularity drifts; the daily cron makes it cheap). Keep the loop bounded to the announced pool the sync already walks.
- [ ] Test: mock the Jikan call (inject a stub) OR unit-test the persistence path — assert `mal_members` written; assert a Jikan error does not abort the sync (other titles still processed).
- [ ] `go test ./services/catalog/internal/service/...` for the touched tests passes.
- [ ] Commit.

---

### Task 3: S9 MAL popularity signal

**Files:**
- Create: `services/recs/internal/service/recs/signals/s9_mal_popularity.go`
- Test: `services/recs/internal/service/recs/signals/s9_mal_popularity_test.go`

**Interfaces:**
- Produces: `signals.S9MalPopularity` with `NewS9MalPopularity(db *gorm.DB) *S9MalPopularity`, `ID() = "s9"`.

- [ ] Implement the `SignalModule` contract. `Precompute` no-op. `Score`: one query `SELECT id, mal_members FROM animes WHERE id IN (candidates) AND mal_members > 0`; raw = `math.Log1p(float64(mal_members))`; omit zero/absent (normalizer treats absent as 0). Mirror S8's file-doc style.
- [ ] Test (sqlite in-mem like s8/s2 tests): three candidates with members `{0, 100, 1_000_000}` → the 0 omitted; the two others present with `log1p` values, the millions-title strictly greater. Assert `ID()=="s9"` and `Precompute` returns nil.
- [ ] `go test ./services/recs/internal/service/recs/signals/ -run S9` passes.
- [ ] Commit.

---

### Task 4: Continuation detector

**Files:**
- Create: `services/recs/internal/handler/upcoming_continuation.go`
- Test: `services/recs/internal/handler/upcoming_continuation_test.go`

**Interfaces:**
- Produces:
  - `looksLikeSequel(name, nameRU string) bool` — pure, regex-based.
  - `(h *UpcomingHandler) franchiseHasAiredSibling(ctx, ids []string) (map[string]bool, error)` — batched: for candidates with `franchise <> ''`, true when a sibling row in the same franchise has `status IN ('released','ongoing')`.

- [ ] `looksLikeSequel`: compile package-level regexps (case-insensitive) for the markers in the spec §1 (EN + RU). Return true on any match. Keep the roman-numeral rule anchored to a trailing standalone token to avoid false positives on titles that legitimately contain numerals mid-name.
- [ ] `franchiseHasAiredSibling`: load `(id, franchise)` for the candidate ids with non-empty franchise; then one query grouping franchises that have an aired sibling: `SELECT DISTINCT franchise FROM animes WHERE franchise IN (?) AND status IN ('released','ongoing')`. Map each candidate → whether its franchise is in that set. (A candidate that is itself released won't matter — announced pool only.)
- [ ] Tests: table of real announced names — `"Witch Watch 2nd Season"`, `"Dandadan 3rd Season"`, `"Kusuriya no Hitorigoto 3rd Season"`, `"Spy x Family Season 3"`, `"Frieren: Beyond Journey's End"` (NOT a sequel by name → false), `"Overlord IV"` (roman → true), a plain original `"Kimetsu Academy"` (false); RU `"Ван-Пис 2 сезон"` → true. Structural test with an in-mem DB: announced candidate in franchise `X` with a released sibling → true; franchise `Y` with only announced entries → false; empty franchise → false.
- [ ] `go test ./services/recs/internal/handler/ -run Continuation` passes.
- [ ] Commit.

---

### Task 5: Upcoming gate + ensemble + ordering + cache + reason rework

**Files:**
- Modify: `services/recs/internal/handler/upcoming.go`
- Modify: `services/recs/internal/handler/config` wiring for `RECS_UPCOMING_MIN_S5` (grep where `MinS8`/`MinS2` are loaded — likely `services/recs/internal/config/config.go`)
- Modify/create tests: `services/recs/internal/handler/upcoming_test.go`

**Interfaces:**
- Consumes: Task 3 `signals.NewS9MalPopularity`; Task 4 `looksLikeSequel` + `franchiseHasAiredSibling`; existing `signals.NewS7DroppedPenalty` (grep constructor).
- Produces: reworked `computeUpcoming`; `UpcomingConfig.MinS5 float64`; `UpcomingReason.Kind` values `franchise|attribute|anticipated|taste`.

- [ ] Wire `s7` and `s9` into `UpcomingHandler` (construct in `NewUpcomingHandler`).
- [ ] Ensemble → `{s8:0.40, s5:0.30, s9:0.15, s2:0.10, s7:-0.05}`.
- [ ] Add `MinS5` to `UpcomingConfig` + env `RECS_UPCOMING_MIN_S5` (default: a conservative small positive, e.g. `0.01` — flagged for calibration; document it's tuned at verification).
- [ ] Precompute `isContinuation` for the ranked pool once: `cont := looksLikeSequel(name,nameRU) || airedSibling[id]` (needs name/nameRU/franchise for pool ids — fold into a single lookup, or reuse hydrate fields; a small `SELECT id,name,name_ru,franchise` over ranked ids).
- [ ] New gate (spec §2): continuation ⇒ require `rawS8 >= MinS8` else skip; standalone ⇒ require `rawS8 >= MinS8 || rawS5 >= MinS5` else skip. Remove the S2 gate.
- [ ] Ordering: collect all eligible picks with a `franchise bool` (rawS8 ≥ upcomingFranchiseReasonMinS8) and `final`; sort `franchise DESC, final DESC`; take TopK.
- [ ] Cache suffix `:upcoming:v2`.
- [ ] Reason resolution (spec §5): franchise (existing seed) → else attribute (Task 6 helper; if Task 6 not yet merged, stub to `taste` and let Task 6 fill) → else anticipated (relative popularity high: `rawS9` in top of pool) → else taste.
- [ ] Tests: (a) a continuation candidate with `rawS8=0` but high genre/S5 is EXCLUDED; (b) a continuation with `rawS8≥MinS8` is INCLUDED with franchise reason; (c) a standalone with only genre overlap but `rawS5<MinS5` is EXCLUDED; (d) a standalone with `rawS5≥MinS5` INCLUDED; (e) franchise item sorts ahead of a higher-`final` taste item. Use fake signal stubs or seed the in-mem DB so raw scores are controllable (mirror existing upcoming_test fixtures).
- [ ] `go test ./services/recs/internal/handler/...` passes.
- [ ] Commit.

---

### Task 6: Attribute reason resolver

**Files:**
- Modify: `services/recs/internal/handler/upcoming.go` (reason resolver) OR new `upcoming_reason.go`
- Test: `services/recs/internal/handler/upcoming_reason_test.go`

**Interfaces:**
- Consumes: userID, candidate id.
- Produces: `attributeReason(ctx, userID, animeID) (*UpcomingReason, error)` returning `Kind:"attribute"` with a `Detail`/`SeedAnimeName` field naming the shared driver, or nil when none dominates.

- [ ] Add fields to `UpcomingReason` as needed: `Attribute string \`json:"attribute,omitempty"\`` (dimension: `studio|source|tag|genre`) and `AttributeName string \`json:"attribute_name,omitempty"\``.
- [ ] Implement a focused resolver independent of S5's JSONB: strongest shared **studio** first — the studio the user has watched most that this candidate also has (`anime_studios` ∩ user watch_history studios, ranked by count); else shared **material_source** (candidate's source that the user watches most). Return the winner; nil if neither shares.
- [ ] Wire into Task 5's reason order (attribute between franchise and anticipated).
- [ ] Tests: user who watched 5 shows by Studio A; candidate has Studio A → `attribute=studio, attribute_name="A"`. Candidate shares only a common source `manga` and the user's history is manga-heavy → `attribute=source`. No overlap → nil (falls through to anticipated/taste).
- [ ] `go test ./services/recs/internal/handler/ -run Reason` passes.
- [ ] Commit.

---

### Task 7: Frontend reason rendering + i18n

**Files:**
- Modify: `frontend/web/src/components/home/spotlight/cards/UpcomingForYouCard.vue`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json`
- Modify: `frontend/web/src/types/spotlight.ts` (reason shape, if typed there)

**Interfaces:**
- Consumes: reason JSON `{kind, seed_anime_name, user_score, attribute, attribute_name}`.

- [ ] Extend the reason type with `attribute?` / `attribute_name?`.
- [ ] `reasonLine` computed: `franchise` → existing seed sentence; `attribute` → `spotlight.upcomingForYou.reasonStudio`/`reasonSource`/`reasonTag` with `{name}`; `anticipated` → `spotlight.upcomingForYou.reasonAnticipated`; `taste` → existing `reasonTaste`.
- [ ] Add keys to en/ru/ja with matching ICU `{name}` placeholder. Example EN: `reasonStudio: "Same studio as your favorites — {name}"`, `reasonSource: "From a {name}, like you watch"`, `reasonAnticipated: "Highly anticipated right now"`.
- [ ] `/frontend-verify` green (DS-lint, i18n parity, build, touched vitest). Cascade-insensitive (text only) — no Chrome smoke needed.
- [ ] Commit.

---

## Post-execution (controller)

- Final whole-branch review.
- Deploy catalog+recs+web; run announcements-sync once (`POST /api/anime/announcements-sync`) to populate `mal_members`; hit `GET /api/users/recs/upcoming` as `tNeymik` — confirm Witch Watch 2nd Season gone, reasons honest; **calibrate `RECS_UPCOMING_MIN_S5`** from observed raw S5 spread.
- `/animeenigma-after-update` (changelog Trump-mode, land, deploy).
