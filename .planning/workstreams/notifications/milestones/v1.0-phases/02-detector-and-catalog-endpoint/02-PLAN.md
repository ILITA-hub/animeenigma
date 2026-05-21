---
phase: 02-detector-and-catalog-endpoint
plan: 02
type: execute
workstream: notifications
milestone: v1.0
wave: 1
depends_on:
  - 01-notifications-foundation
files_modified:
  # Catalog — new internal endpoint + parser adapters
  - services/catalog/internal/handler/internal_episodes.go
  - services/catalog/internal/service/episodes_lookup.go
  - services/catalog/internal/parser/kodik/latest_episode.go
  - services/catalog/internal/parser/animelib/latest_episode.go
  - services/catalog/internal/transport/router.go
  - services/catalog/cmd/catalog-api/main.go
  # Notifications service — detector + cleanup + metrics + admin trigger
  - services/notifications/go.mod
  - services/notifications/internal/config/config.go
  - services/notifications/internal/job/doc.go
  - services/notifications/internal/job/hotcombos.go
  - services/notifications/internal/job/detector.go
  - services/notifications/internal/job/cleanup.go
  - services/notifications/internal/job/scheduler.go
  - services/notifications/internal/job/metrics.go
  - services/notifications/internal/repo/snapshot.go
  - services/notifications/internal/repo/maxwatched.go
  - services/notifications/internal/repo/unread_gauge.go
  - services/notifications/internal/service/catalog_client.go
  - services/notifications/internal/service/payload_builder.go
  - services/notifications/internal/handler/admin.go
  - services/notifications/internal/transport/router.go
  - services/notifications/cmd/notifications-api/main.go
  # Infra + tooling
  - docker/docker-compose.yml
  - docker/.env.example
  - Makefile
autonomous: true
requirements:
  - NOTIF-DET-01
  - NOTIF-DET-02
  - NOTIF-DET-03
  - NOTIF-DET-04
  - NOTIF-DET-05
  - NOTIF-DET-06
  - NOTIF-DET-07
  - NOTIF-DET-08
  - NOTIF-DET-09
  - NOTIF-DET-10
  - NOTIF-NF-01
  - NOTIF-NF-02

must_haves:
  truths:
    - "GET /internal/anime/{shikimori_id}/episodes?player=kodik|animelib&translation_id=&watch_type=&language= returns {latest_available_episode:N, checked_at:ISO}; a second call within 5 minutes is served from Redis (no parser hit)"
    - "Starting from empty parser_episode_snapshots + user_notifications, manually running the detector once populates snapshot rows for every active hot combo and inserts ZERO user_notifications rows (bootstrap protection per NOTIF-DET-06)"
    - "After seeding watch_history to ep 5 + manually setting that combo's snapshot.latest_episode=5, running the detector with the parser returning 6 inserts EXACTLY one user_notifications row with the dedupe_key 'new_episode:{anime_id}:kodik:ru:dub:{translation_id}', payload.first_unwatched_episode=6, payload.latest_available_episode=6, payload.watch_url non-null"
    - "Re-running the same detector pass against the same upstream is a no-op — no new rows, payload byte-equal, read_at stays NULL only if it was already NULL (NOTIF-DET-10 idempotency)"
    - "Bumping the parser stub to return 8 UPSERTs the existing row: payload.latest_available_episode becomes 8, payload.first_unwatched_episode stays 6, read_at is reset to NULL so the toast re-fires (NOTIF-DET-08 aggregation)"
    - "Manually UPDATE'ing one notification's dismissed_at to NOW() - INTERVAL '31 days' and triggering the cleanup cron deletes that row; a peer row with dismissed_at = NOW() - INTERVAL '29 days' is untouched (NOTIF-DET-09)"
    - "/metrics on :8090 exposes all six NOTIF-NF-01 series after one detector run: notifications_created_total, notifications_detector_runs_total, notifications_detector_duration_seconds, notifications_detector_combos_scanned, notifications_detector_parser_failures_total, notifications_active_unread_gauge"
    - "Detector run logs one INFO line via libs/logger carrying combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures (NOTIF-NF-02); per-combo failures log WARN with anime_id+player+translation_id context; no usernames in any log line"
    - "Parser-low-episode does NOT lower snapshot; catalog 5xx/timeout on a single combo skips that combo and continues the run (NOTIF-DET-10)"
  artifacts:
    - path: "services/catalog/internal/handler/internal_episodes.go"
      provides: "GET /internal/anime/{shikimori_id}/episodes — routes to kodik|animelib by ?player; 400 for any other player value in v1.0"
      contains: "InternalEpisodesHandler"
    - path: "services/catalog/internal/service/episodes_lookup.go"
      provides: "EpisodesLookupService — Redis-cached (5min TTL) wrapper around the per-player LatestEpisode adapters"
      contains: "notifications:episodes:"
    - path: "services/catalog/internal/parser/kodik/latest_episode.go"
      provides: "LatestEpisodeForTranslation(shikimoriID, translationID) — picks the matching translation from Search and returns its LastEpisode/EpisodesCount"
      contains: "LatestEpisodeForTranslation"
    - path: "services/catalog/internal/parser/animelib/latest_episode.go"
      provides: "LatestEpisodeForTeam(slug, teamID, watchType) — reads GetEpisodes, filters PlayerData by team_id+translation_type, returns highest episode number"
      contains: "LatestEpisodeForTeam"
    - path: "services/notifications/internal/job/hotcombos.go"
      provides: "Collect(ctx) []Combo — single SQL DISTINCT join over watch_history + anime_list (status='watching') + animes (status='ongoing') filtered to translation_id != ''"
      contains: "SELECT DISTINCT"
    - path: "services/notifications/internal/job/detector.go"
      provides: "NewEpisodeDetectorJob.Run(ctx) error — orchestrates steps 1..6 per design doc with errgroup worker pool (cap 5, per-call 10s timeout) and bootstrap protection"
      contains: "errgroup"
    - path: "services/notifications/internal/job/cleanup.go"
      provides: "DismissedRetentionCleanupJob.Run(ctx) error — single DELETE statement, 30-day retention on dismissed_at"
      contains: "INTERVAL '30 days'"
    - path: "services/notifications/internal/job/scheduler.go"
      provides: "Scheduler — wraps *cron.Cron, registers detector at '0 * * * *' (with ±5min boot-time jitter) and cleanup at '30 3 * * *'; Start/Stop lifecycle"
      contains: "cron.New"
    - path: "services/notifications/internal/job/metrics.go"
      provides: "Six promauto-registered metrics matching NOTIF-NF-01 names + labels"
      contains: "notifications_detector_runs_total"
    - path: "services/notifications/internal/repo/snapshot.go"
      provides: "SnapshotRepository.BulkLoad + BulkUpsert — per-combo map round-trips for detector"
      contains: "BulkUpsert"
    - path: "services/notifications/internal/repo/maxwatched.go"
      provides: "MaxWatchedRepository.ForCombos(ctx, combos) — single GROUP BY query over watch_history view returning map[Combo][UserID]MaxEp"
      contains: "MAX(episode_number)"
    - path: "services/notifications/internal/repo/unread_gauge.go"
      provides: "ActiveUnreadCount(ctx) — SELECT COUNT(*) WHERE dismissed_at IS NULL AND read_at IS NULL; polled by metrics goroutine every 5min"
      contains: "ActiveUnreadCount"
    - path: "services/notifications/internal/service/catalog_client.go"
      provides: "EpisodeChecker interface + HTTPEpisodeChecker — thin client over GET /internal/anime/{shikimori_id}/episodes; per-call timeout 10s; testable via interface"
      contains: "type EpisodeChecker interface"
    - path: "services/notifications/internal/service/payload_builder.go"
      provides: "BuildNewEpisodePayload(combo, anime, maxWatched, latestAvail) -> NewEpisodePayload + WatchURL formatter per design doc"
      contains: "watch?player="
    - path: "services/notifications/internal/handler/admin.go"
      provides: "POST /internal/detector/run-once — fires the detector synchronously and returns {combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures}; gateway-non-routing"
      contains: "RunOnce"
  key_links:
    - from: "services/notifications/internal/job/detector.go"
      to: "services/notifications/internal/repo/notification.go::Upsert"
      via: "Detector calls notifService.Upsert() directly (in-process), NOT POST /internal/notifications over HTTP — decision D-DET-01 below"
      pattern: "notifService\\.Upsert|repo\\.Upsert"
    - from: "services/notifications/internal/job/detector.go"
      to: "services/notifications/internal/service/catalog_client.go::EpisodeChecker"
      via: "Detector depends on EpisodeChecker interface (not concrete HTTPEpisodeChecker) so unit tests inject a stub returning canned (latest, error) per combo"
      pattern: "EpisodeChecker"
    - from: "services/notifications/internal/job/detector.go"
      to: "services/notifications/internal/job/metrics.go"
      via: "Per-run: increments notifications_detector_runs_total{outcome}; observes notifications_detector_duration_seconds; sets notifications_detector_combos_scanned gauge; per-failure increments notifications_detector_parser_failures_total{player}; per-upsert increments notifications_created_total{type='new_episode',producer='detector'}"
      pattern: "DetectorRunsTotal|DetectorDuration|ParserFailures|CreatedTotal"
    - from: "services/catalog/internal/handler/internal_episodes.go"
      to: "services/catalog/internal/service/episodes_lookup.go"
      via: "Handler validates player param ∈ {kodik, animelib}; on success calls EpisodesLookupService.LatestAvailable which checks Redis then dispatches to the matching parser adapter"
      pattern: "EpisodesLookupService|LatestAvailable"
    - from: "services/notifications/internal/job/scheduler.go"
      to: "github.com/robfig/cron/v3 v3.0.1"
      via: "Match scheduler service's pinned cron version (services/scheduler/go.mod) so the workspace resolves a single cron module across services"
      pattern: "robfig/cron/v3"
    - from: "services/notifications/cmd/notifications-api/main.go"
      to: "services/notifications/internal/job/scheduler.go::Start"
      via: "Boot wires NewEpisodeDetectorJob, DismissedRetentionCleanupJob, ActiveUnreadGaugePoller (5min ticker) inside Scheduler.Start(ctx); gated by NOTIFICATIONS_DETECTOR_ENABLED env (default true; rollback toggle)"
      pattern: "NOTIFICATIONS_DETECTOR_ENABLED|scheduler\\.Start"
---

<objective>
Make notifications real. Catalog gains exactly one new internal endpoint
`GET /internal/anime/{shikimori_id}/episodes?player=&translation_id=&watch_type=&language=`
with a 5-minute Redis cache (key `notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}`).
The notifications service gains a `NewEpisodeDetectorJob` running at
`0 * * * *` with ±5min boot-time jitter, a `DismissedRetentionCleanupJob`
at `30 3 * * *`, a single boot-time cron instance wiring both, the six
NOTIF-NF-01 Prometheus metrics, structured `libs/logger` lines per
NOTIF-NF-02, and an internal `POST /internal/detector/run-once` admin
trigger plus `make run-detector-once` Makefile shortcut for verification.

**End-state** (mirrors ROADMAP SC1..SC7):
After seeding `watch_history` for `ui_audit_bot` to ep 5 of an anime and
inserting a `parser_episode_snapshots` row with `latest_episode = 5`,
running `make run-detector-once` against a parser stubbed to return
`latest_episode = 6` produces exactly one `user_notifications` row with
the design-doc payload + dedupe key + watch URL. The first detector run
against truly-empty snapshot tables emits zero notifications (bootstrap
protection). Cleanup deletes only rows with `dismissed_at` older than 30
days.

**Out of scope (explicit deferrals to v1.0.x or v1.1):**
- HiAnime/Consumet/Scraper player branches (REQUIREMENTS line 152) —
  the catalog endpoint returns 400 for any `player` value outside
  {kodik, animelib}; the detector's hot-combo query naturally filters
  those rows because `watch_history.player` for English playback won't
  match the catalog's accepted set.
- Per-user notification preferences (NOTIF-DET-* are unconditional).
- Frontend rendering — Phase 3.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/notifications/PROJECT.md
@.planning/workstreams/notifications/REQUIREMENTS.md
@.planning/workstreams/notifications/ROADMAP.md
@.planning/workstreams/notifications/phases/01-notifications-foundation/SUMMARY.md
@.planning/workstreams/notifications/phases/02-detector-and-catalog-endpoint/02-CONTEXT.md
@docs/superpowers/specs/2026-05-11-notifications-engine-design.md
@CLAUDE.md

@services/notifications/cmd/notifications-api/main.go
@services/notifications/internal/config/config.go
@services/notifications/internal/transport/router.go
@services/notifications/internal/handler/internal.go
@services/notifications/internal/service/notification.go
@services/notifications/internal/repo/notification.go
@services/notifications/internal/repo/views.go
@services/notifications/internal/domain/notification.go
@services/notifications/internal/domain/snapshot.go
@services/notifications/internal/job/doc.go
@services/notifications/go.mod

@services/catalog/internal/transport/router.go
@services/catalog/internal/handler/internal_cache.go
@services/catalog/internal/parser/kodik/client.go
@services/catalog/internal/parser/animelib/client.go
@services/catalog/internal/service/catalog.go
@services/scheduler/internal/service/job.go
@services/scheduler/go.mod
@libs/cache/cache.go
@libs/metrics/scheduler.go

<interfaces>
<!-- Existing notifications-service contracts the detector will reuse (extracted 2026-05-21) -->

<!-- Producer path — already shipped in Phase 1 (services/notifications/internal/service/notification.go) -->
type UpsertRequest struct {
    UserID    string          `json:"user_id"`
    Type      string          `json:"type"`
    DedupeKey string          `json:"dedupe_key"`
    Payload   json.RawMessage `json:"payload"`
}
func (s *NotificationService) Upsert(ctx, req UpsertRequest) (*domain.UserNotification, error)
func NewEpisodeDedupeKey(animeID, player, language, watchType, translationID string) string
// Returns: "new_episode:<anime_id>:<player>:<language>:<watch_type>:<translation_id>"

<!-- Notification model with NewEpisodePayload — services/notifications/internal/domain/notification.go -->
type NewEpisodePayload struct {
    AnimeID                string `json:"anime_id"`
    ShikimoriID            string `json:"shikimori_id,omitempty"`
    AnimeTitle             string `json:"anime_title"`
    AnimePosterURL         string `json:"anime_poster_url,omitempty"`
    FirstUnwatchedEpisode  int    `json:"first_unwatched_episode"`
    LatestAvailableEpisode int    `json:"latest_available_episode"`
    Player                 string `json:"player"`
    Language               string `json:"language"`
    WatchType              string `json:"watch_type"`
    TranslationID          string `json:"translation_id"`
    TranslationTitle       string `json:"translation_title,omitempty"`
    WatchURL               string `json:"watch_url"`
}

<!-- Read-only views — already shipped in Phase 1 (services/notifications/internal/repo/views.go) -->
type WatchHistoryView struct {
    UserID, AnimeID, Player, Language, WatchType, TranslationID string
    EpisodeNumber int
}
// TableName() == "watch_history"
type AnimeListView struct { UserID, AnimeID, Status string }
// TableName() == "anime_list"
type AnimeView struct {
    ID, ShikimoriID, Status, Name, NameRU, PosterURL string
}
// TableName() == "animes"

<!-- Snapshot row — already shipped in Phase 1 (services/notifications/internal/domain/snapshot.go) -->
type ParserEpisodeSnapshot struct {
    ID, AnimeID, Player, Language, WatchType, TranslationID string
    LatestEpisode int
    CheckedAt, UpdatedAt time.Time
}
// Composite uniqueIndex uk_combo on (anime_id, player, language, watch_type, translation_id)

<!-- Cache primitive — libs/cache/cache.go -->
type Cache interface {
    Get(ctx, key, dest interface{}) error
    Set(ctx, key, value interface{}, ttl time.Duration) error
    Delete(ctx, keys ...string) error
}
func New(cfg Config) (*RedisCache, error)
// Get returns cache.ErrNotFound on miss.

<!-- Cron library version to pin in services/notifications/go.mod -->
// github.com/robfig/cron/v3 v3.0.1 — matches services/scheduler/go.mod:14

<!-- Worker pool primitive — already an indirect dep -->
// golang.org/x/sync/errgroup — services/notifications/go.mod already pulls
// golang.org/x/sync v0.18.0 transitively, so adding errgroup needs no
// new require beyond bumping it from indirect to direct.

<!-- Existing kodik adapter — services/catalog/internal/parser/kodik/client.go -->
func (c *Client) SearchByShikimoriID(shikimoriID string) ([]SearchResult, error)
type SearchResult struct {
    ...
    LastEpisode   int          `json:"last_episode,omitempty"`
    EpisodesCount int          `json:"episodes_count,omitempty"`
    Translation   *Translation `json:"translation"`
    Seasons       map[string]*Season
}
type Translation struct { ID int; Title, Type string; EpisodesCount int }

<!-- Existing animelib adapter — services/catalog/internal/parser/animelib/client.go -->
func (c *Client) Search(query string) ([]SearchResult, error)
func (c *Client) GetEpisodes(animeID int) ([]Episode, error)
func (c *Client) GetEpisodeStreams(episodeID int) (*EpisodeDetail, error)
type Episode struct { ID int; Number, Name string }
type EpisodeDetail struct { ID int; Number, Name string; Players []PlayerData }
type PlayerData struct {
    Team            Team            // {ID, Name}
    TranslationType TranslationType // {ID, Label}  (2 == "Озвучка"/dub, 1 == "Субтитры"/sub)
    ...
}
// Existing CatalogService.findAnimeLibID(ctx, *domain.Anime) (int, error)
// already maps anime -> animelib internal numeric id via search+match.

<!-- Catalog router — current internal route precedent (services/catalog/internal/transport/router.go ~line 54) -->
// if internalCacheHandler != nil {
//     r.Post("/internal/cache/invalidate/raw/{shikimoriId}", internalCacheHandler.InvalidateRaw)
// }
// Mirror that style for the new internal episodes route. No middleware —
// gateway never proxies /internal/* (Phase 1 D-05).

<!-- Catalog cache key + TTL — REQUIREMENTS NOTIF-DET-01 mandates the literal key + 5min TTL -->
// key:  notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}
// ttl:  5 * time.Minute
// Use s.cache.Get / s.cache.Set — same RedisCache the rest of catalog already
// has injected (services/catalog/internal/service/catalog.go:41).
</interfaces>
</context>

<decisions>

### D-DET-01: Detector calls `notifService.Upsert` IN-PROCESS, NOT `POST /internal/notifications` over HTTP

**Choice:** The detector imports `services/notifications/internal/service.NotificationService` directly and calls `Upsert(ctx, UpsertRequest{...})` per combo. No `http.Client` round-trip to its own `:8090/internal/notifications`.

**Rationale:**
- The detector runs in the same Go process that owns the service struct — the producer endpoint is just a thin HTTP wrapper around the exact same `service.Upsert`. A self-loopback HTTP call would add ~1ms per UPSERT, a JSON marshal+unmarshal pair, and three new failure modes (DNS, TCP, timeout) for zero correctness benefit.
- HTTP-self-call was historically useful when the producer needed JWT/auth, but the producer endpoint is explicitly gateway-non-routing with no middleware (Phase 1 D-05) — there is nothing to "exercise on the wire" that isn't already exercised by the seed script in Phase 1's SC4.
- Future external producers (a hypothetical `social` service in v1.1) will still use `POST /internal/notifications` because they're in a different process. That contract stays intact.
- Bonus: in-process calls participate in the same `context.Context` cancellation tree as the worker pool, so a SIGTERM during a run cleanly aborts pending UPSERTs.

**Risk surface trimmed:** No HTTP client to mock in unit tests, no port to misconfigure, no Docker-internal DNS to fail.

### D-DET-02: Catalog `/internal/episodes` endpoint mirrors `internal_cache.go` precedent — no middleware

**Choice:** Mount `r.Get("/internal/anime/{shikimoriId}/episodes", h.GetLatestEpisode)` directly on the catalog root router with NO middleware (no auth, no admin, no rate-limit).

**Rationale:**
- `services/catalog/internal/transport/router.go` already mounts `POST /internal/cache/invalidate/raw/{shikimoriId}` exactly this way (line ~54) — the inline comment explicitly states "nginx/gateway does NOT proxy /internal/* — so the route is reachable only from within the docker network. Mirrors the precedent set by services/auth/internal/transport/router.go's /internal/resolve-api-key."
- Phase 1 PLAN D-05 codified this as the project's "internal" pattern. Adding bespoke middleware would diverge for no security gain.

### D-DET-03: Players in v1.0 catalog endpoint = `{kodik, animelib}` only; other values → HTTP 400

**Choice:** REQUIREMENTS NOTIF-DET-01 line 59 + REQUIREMENTS line 152 ("HiAnime/Consumet specific notifications ... belongs in a v1.0.x patch, not v1.0 scope") gate this. Catalog endpoint validates `player` against an explicit allowlist; anything else returns `400 Bad Request` with body `{"error": "player not supported by detector in v1.0"}`.

**Rationale:**
- Scraper service has `GetEpisodes` but it routes through `MAL ID -> orchestrator -> per-provider plugin`; the "latest available episode" is not directly exposed in a stable contract today. Adding it would introduce a second cross-service hop and a second cache layer that the design doc deliberately scoped out.
- Hanime (`services/catalog/internal/parser/hanime/`) exists but its `GetVideo` flow is search-then-pick-best — there is no clean "episode count per (anime, translation_id)" surface yet.
- The detector's hot-combo SQL filters on `wh.translation_id != ''`; English-player rows in `watch_history` won't carry a translation_id that matches the {kodik, animelib} catalog dispatch — they will naturally drop out of the combo set, so the 400 path is defensive-only.
- v1.0.x can add scraper later by extending the catalog handler's switch + adding `scraper.LatestEpisodeForAnime(malID, prefer) (int, error)` without touching the detector.

### D-DET-04: New parser adapter methods (`LatestEpisodeForTranslation` / `LatestEpisodeForTeam`) instead of reusing existing `GetTranslations` / `GetEpisodes`

**Choice:** Add small adapter files alongside the existing parser clients:
- `services/catalog/internal/parser/kodik/latest_episode.go` → `func (c *Client) LatestEpisodeForTranslation(shikimoriID string, translationID int) (int, error)` — wraps `SearchByShikimoriID` and replays the same `LastEpisode > EpisodesCount > sum(Seasons.Episodes)` precedence the existing `GetTranslations` uses (kodik/client.go:522-538).
- `services/catalog/internal/parser/animelib/latest_episode.go` → `func (c *Client) LatestEpisodeForTeam(slug string, teamID int, watchType string) (int, error)` — wraps `GetEpisodes(animelibID)` then `GetEpisodeStreams(episode.ID)` for each, filters `PlayerData` where `Team.ID == teamID` and `TranslationType.ID` matches watch_type (2=dub/voice, 1=sub), returns the highest episode number that has at least one matching PlayerData.

**Rationale:**
- Existing `kodik.GetTranslations` returns ALL translations, doing extra work; the detector wants exactly one. A focused adapter is cheaper per call (one search, one filter, one int).
- Existing `animelib.GetEpisodes` only returns episode IDs/numbers — it does NOT tell you which team has which episode. The detector must reach into `GetEpisodeStreams` to discover team availability. Hiding that loop inside a single adapter keeps the detector code linear.
- Keeping the adapters in the parser package (not in catalog/service/) lets them be unit-tested against the same fixtures the parser tests already use.

**Performance note:** AnimeLib's `GetEpisodeStreams` is one HTTP per episode — for a 24-episode show that's 24 calls per combo. Acceptable per design-doc §Constraints (5-worker concurrency cap, 5-min cache absorbs repeats). If this becomes hot in production, v1.0.x can add an upstream batch endpoint or a smarter "newest-first early-exit" — both are local optimisations that do not change the public interface.

### D-DET-05: Admin trigger endpoint = `POST /internal/detector/run-once` (gateway-non-routing), not `/admin/...` under JWT

**Choice:** Mount the manual-trigger handler at `/internal/detector/run-once` on the notifications service root router, no middleware. Add a `make run-detector-once` Makefile target that shells `docker compose exec -T notifications wget -qO- --post-data='' http://localhost:8090/internal/detector/run-once`.

**Rationale:**
- Phase 1 D-05 model: internal routes are protected by gateway-non-routing + the Docker network boundary. The `/admin/*` path under JWT would require minting an admin JWT for manual testing — overkill for an internal trigger.
- `ui_audit_bot` does not have an admin role wired today (it's an automation user, not an admin per MEMORY.md). Putting the trigger behind admin-JWT would block the very script SC2/SC3 verification needs.
- Returns a structured JSON summary `{combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures}` so the make-target output is grep-able for the verification matrix.
- The endpoint is synchronous (blocks until the run completes) so the make-target exits when the detector exits — easy for CI/manual verification.

### D-DET-06: Cleanup cron test path is a SQL `UPDATE ... dismissed_at` + `make run-cleanup-once`

**Choice:** Add a sibling `POST /internal/cleanup/run-once` + `make run-cleanup-once` target. ROADMAP SC6 requires that "manually UPDATEing one notification's dismissed_at to NOW() - INTERVAL '31 days' and triggering the cleanup cron deletes that row". The make-target makes "triggering the cleanup cron" a one-line command rather than waiting until 03:30 the next day.

**Rationale:** Same line of reasoning as D-DET-05. Both run-once endpoints share the same handler file (`handler/admin.go`). Cleanup is a single DELETE statement — no input, no concurrency — so the endpoint body is `cleanupJob.Run(ctx)` + return rows-deleted count.

### D-DET-07: Mock parser injection via `EpisodeChecker` interface (not via HTTP-mock-server)

**Choice:** Define a tiny interface in `services/notifications/internal/service/catalog_client.go`:
```
type EpisodeChecker interface {
    LatestEpisode(ctx context.Context, combo job.Combo) (int, error)
}
```
Production = `HTTPEpisodeChecker` wrapping a 10s-timeout `http.Client` against `CATALOG_URL`. Tests = stub returning `(latest int, err error)` from a `map[Combo]int` fixture.

**Rationale:**
- ROADMAP SC3..SC5 explicitly require running the detector "with the parser mocked to return latest_episode = 6" / "now returning latest_episode = 8". Stubbing the interface is one struct per test; standing up an httptest.Server is three lines of fixture-setup per test plus a real TCP socket.
- Detector unit tests (in `services/notifications/internal/job/detector_test.go`) inject the stub via the same constructor field as production; integration-style tests in `_test.go` files can still use httptest if needed for the HTTP-checker variant.
- This is the canonical "Hexagonal port" shape — the detector orchestrator does not care whether the integer came from HTTP, gRPC, or a fixture.

</decisions>

<touch_list>

**Catalog — new internal endpoint + parser adapters:**

- `services/catalog/internal/parser/kodik/latest_episode.go` *(new)* — `LatestEpisodeForTranslation(shikimoriID string, translationID int) (int, error)`. Wraps `SearchByShikimoriID`, finds the result whose `Translation.ID == translationID`, computes the episode count via the same fallback ladder as `GetTranslations` (LastEpisode → EpisodesCount → sum of Seasons.Episodes → 1 if type=="anime"). Returns 0 + `apperrors.NotFound` when no matching translation exists.
- `services/catalog/internal/parser/animelib/latest_episode.go` *(new)* — `LatestEpisodeForTeam(slug string, teamID int, watchType string) (int, error)`. Calls `Search` (or reuses the existing `findAnimeLibID` helper if exported) to map slug→animelibID, then `GetEpisodes(animelibID)` to enumerate, then `GetEpisodeStreams(episode.ID)` per episode (or batched via goroutine pool inside the adapter — cap 5). Returns the highest `Episode.Number` (parsed as int) that has at least one PlayerData with matching team + watch_type.
- `services/catalog/internal/service/episodes_lookup.go` *(new)* — `type EpisodesLookupService struct { cache *cache.RedisCache; kodik *kodik.Client; animelib *animelib.Client; animeRepo *repo.AnimeRepository; log *logger.Logger }`. Method `LatestAvailable(ctx, shikimoriID, player, translationID, watchType, language string) (latest int, checkedAt time.Time, err error)`:
  1. Build cache key `fmt.Sprintf("notifications:episodes:%s:%s:%s:%s", shikimoriID, player, translationID, watchType)`.
  2. `cache.Get` — if hit, return cached `{latest, checkedAt}`.
  3. Dispatch on `player`:
     - `"kodik"`: parse `translationID` to int, call `kodik.LatestEpisodeForTranslation`.
     - `"animelib"`: resolve `shikimoriID` → `domain.Anime` via `animeRepo.GetByShikimoriID` → `animelibID` via existing `findAnimeLibID` (export it if needed), then `animelib.LatestEpisodeForTeam(slug, teamID, watchType)`.
     - default: return `apperrors.InvalidInput("player not supported by detector in v1.0")`.
  4. On success, `cache.Set` with TTL `5 * time.Minute`.
  5. Return `(latest, time.Now().UTC(), nil)`.
- `services/catalog/internal/handler/internal_episodes.go` *(new)* — `type InternalEpisodesHandler struct { svc *service.EpisodesLookupService; log *logger.Logger }`. Method `GetLatestEpisode(w, r)`:
  - Read `shikimoriId` via `chi.URLParam`; validate against the same `shikimoriIDPattern` regex as `internal_cache.go`.
  - Read `player`, `translation_id`, `watch_type`, `language` via `r.URL.Query()`. Validate `player ∈ {kodik, animelib}`; else 400.
  - Call `svc.LatestAvailable(...)`. On `apperrors.InvalidInput` return 400; on `apperrors.NotFound` return 404; on any other return 500.
  - Response: `httputil.OK(w, struct { LatestAvailableEpisode int `json:"latest_available_episode"`; CheckedAt time.Time `json:"checked_at"` })`.
- `services/catalog/internal/transport/router.go` *(modify)* — insert after the existing `internalCacheHandler` block:
  ```
  if internalEpisodesHandler != nil {
      r.Get("/internal/anime/{shikimoriId}/episodes", internalEpisodesHandler.GetLatestEpisode)
  }
  ```
  Add `internalEpisodesHandler *handler.InternalEpisodesHandler` to `NewRouter`'s signature.
- `services/catalog/cmd/catalog-api/main.go` *(modify)* — construct `episodesLookupSvc := service.NewEpisodesLookupService(redisCache, kodikClient, animelibClient, animeRepo, log)`, then `internalEpisodesHandler := handler.NewInternalEpisodesHandler(episodesLookupSvc, log)`, then pass into `transport.NewRouter(...)`.

**Notifications service — detector + cleanup + metrics + admin:**

- `services/notifications/go.mod` *(modify)* — add direct require for `github.com/robfig/cron/v3 v3.0.1` (match scheduler service version) and `golang.org/x/sync v0.18.0` (already indirect — promote to direct). Run `cd services/notifications && go mod tidy`.
- `services/notifications/internal/config/config.go` *(modify)* — add to `Config`:
  ```
  type DetectorConfig struct {
      Enabled        bool          // NOTIFICATIONS_DETECTOR_ENABLED, default true (rollback toggle, D-RB-01)
      Cron           string        // NOTIFICATIONS_DETECTOR_CRON, default "0 * * * *"
      CleanupCron    string        // NOTIFICATIONS_CLEANUP_CRON, default "30 3 * * *"
      RetentionDays  int           // NOTIFICATIONS_RETENTION_DAYS, default 30
      WorkerLimit    int           // NOTIFICATIONS_DETECTOR_WORKER_LIMIT, default 5
      ParserTimeout  time.Duration // NOTIFICATIONS_PARSER_TIMEOUT, default 10s
      UnreadGaugeEvery time.Duration // NOTIFICATIONS_UNREAD_GAUGE_INTERVAL, default 5m
      CatalogURL     string        // CATALOG_URL, default http://catalog:8081
  }
  ```
  Wire under `Config.Detector`. Add `Redis cache.Config` (Host/Port from existing `REDIS_HOST`/`REDIS_PORT`) so the detector's catalog client doesn't need it but the gauge poller's metrics goroutine can be cleanly shut down.
- `services/notifications/internal/job/doc.go` *(modify)* — replace the placeholder body with a real package-level doc comment describing the job graph (detector + cleanup + unread-gauge poller + scheduler), reference design-doc §Detection Flow.
- `services/notifications/internal/job/hotcombos.go` *(new)* — `type Combo struct { AnimeID, ShikimoriID, Player, Language, WatchType, TranslationID string }`. `type HotCombosCollector struct { db *gorm.DB; log *logger.Logger }`. Method `Collect(ctx) ([]Combo, error)`:
  ```sql
  SELECT DISTINCT
      wh.anime_id, a.shikimori_id, wh.player, wh.language,
      wh.watch_type, wh.translation_id
  FROM watch_history wh
  JOIN anime_list al ON al.user_id = wh.user_id AND al.anime_id = wh.anime_id
  JOIN animes a ON a.id = wh.anime_id
  WHERE al.status = 'watching'
    AND a.status = 'ongoing'
    AND wh.translation_id != '';
  ```
  via `db.WithContext(ctx).Raw(...).Scan(&combos)`.
- `services/notifications/internal/job/metrics.go` *(new)* — six `promauto.NewCounterVec` / `NewHistogramVec` / `NewGauge` matching the names + labels from REQUIREMENTS NOTIF-NF-01 exactly:
  ```
  NotificationsCreatedTotal                = promauto.NewCounterVec(... "notifications_created_total" ..., []string{"type", "producer"})
  NotificationsDetectorRunsTotal           = promauto.NewCounterVec(... "notifications_detector_runs_total" ..., []string{"outcome"})
  NotificationsDetectorDurationSeconds     = promauto.NewHistogram(...)
  NotificationsDetectorCombosScanned       = promauto.NewGauge(...)
  NotificationsDetectorParserFailuresTotal = promauto.NewCounterVec(..., []string{"player"})
  NotificationsActiveUnreadGauge           = promauto.NewGauge(...)
  ```
- `services/notifications/internal/repo/snapshot.go` *(modify — replace placeholder)* — add methods:
  - `BulkLoad(ctx, combos []job.Combo) (map[job.Combo]int, error)` — one `SELECT ... WHERE (anime_id, player, language, watch_type, translation_id) IN ((?,?,?,?,?), ...)` via parameterized GORM `Where("(anime_id, player, language, watch_type, translation_id) IN ?", tuples)`. Returns combo→latestEpisode; missing combos absent from map (caller treats as 0 for bootstrap).
  - `BulkUpsert(ctx, updates map[job.Combo]int) error` — builds `[]ParserEpisodeSnapshot` from the map, then `db.Clauses(clause.OnConflict{Columns: ... uk_combo cols ..., DoUpdates: clause.Assignments(map[string]interface{}{"latest_episode": ..., "checked_at": time.Now(), "updated_at": gorm.Expr("NOW()")})}).Create(&rows)`. Batch insert in chunks of 200 to stay under Postgres' bind-param limit.
  - **Import cycle warning:** `job.Combo` lives in `internal/job/` while this repo is `internal/repo/`. Either (a) define `Combo` in `internal/domain/combo.go` and import it from both packages, or (b) define a local repo-side `ComboKey` type that the detector translates. Pick **(a)** — `Combo` is a domain concept (it's the natural key of `parser_episode_snapshots`).
- `services/notifications/internal/repo/maxwatched.go` *(new)* — `MaxWatchedRepository.ForCombos(ctx, combos []domain.Combo) (map[domain.Combo]map[string]int, error)`. One SQL:
  ```sql
  SELECT wh.user_id, wh.anime_id, wh.player, wh.language, wh.watch_type, wh.translation_id,
         MAX(wh.episode_number) AS max_watched
  FROM watch_history wh
  WHERE (wh.anime_id, wh.player, wh.language, wh.watch_type, wh.translation_id) IN ?
  GROUP BY 1,2,3,4,5,6;
  ```
  Returns `combo -> userID -> maxWatched`. Skip combos with zero rows.
- `services/notifications/internal/repo/unread_gauge.go` *(new)* — `ActiveUnreadCount(ctx) (int64, error)` runs `SELECT COUNT(*) FROM user_notifications WHERE dismissed_at IS NULL AND read_at IS NULL`. Called by the gauge poller goroutine.
- `services/notifications/internal/service/catalog_client.go` *(new)* — `type EpisodeChecker interface { LatestEpisode(ctx, combo domain.Combo) (int, error) }`. `type HTTPEpisodeChecker struct { baseURL string; client *http.Client; log *logger.Logger }`. `LatestEpisode` builds `GET {baseURL}/internal/anime/{shikimori_id}/episodes?player=X&translation_id=Y&watch_type=Z&language=L`, sets a per-call `context.WithTimeout(ctx, 10s)`, parses `{latest_available_episode, checked_at}`, returns 0 + `apperrors.NotFound` on 404, 0 + wrapped error on 5xx/timeout.
- `services/notifications/internal/service/payload_builder.go` *(new)* — `BuildNewEpisodePayload(combo domain.Combo, anime *repo.AnimeView, maxWatched, latestAvail int, translationTitle string) ([]byte, error)`. Marshals `domain.NewEpisodePayload` with:
  - `first_unwatched_episode = maxWatched + 1`
  - `latest_available_episode = latestAvail`
  - `watch_url = fmt.Sprintf("/anime/%s/watch?player=%s&episode=%d&translation=%s", combo.AnimeID, combo.Player, maxWatched+1, combo.TranslationID)` (mirrors the design-doc example URL pattern)
  - `anime_title` = `anime.NameRU` if non-empty else `anime.Name` (Russian-first per project convention).
- `services/notifications/internal/job/detector.go` *(new)* — `type NewEpisodeDetectorJob struct { hotCombos *HotCombosCollector; checker service.EpisodeChecker; snapshots *repo.SnapshotRepository; maxWatched *repo.MaxWatchedRepository; animeRepo *repo.AnimeRepo /* read-only via AnimeView */; notif *service.NotificationService; cfg *config.DetectorConfig; log *logger.Logger }`. `Run(ctx) (RunReport, error)`:
  1. `start := time.Now()` + structured "detector run start" log.
  2. `combos, err := hotCombos.Collect(ctx)` — on error: log + metric `outcome="failed"` + return.
  3. `snapshotMap, err := snapshots.BulkLoad(ctx, combos)` — on error: log + return.
  4. Set gauge `NotificationsDetectorCombosScanned.Set(float64(len(combos)))`.
  5. Build `latestPerCombo := make(map[domain.Combo]int)` + `parserFailures := atomic.Int64{}` + `var mu sync.Mutex`. Use `errgroup.WithContext(ctx)` + `g.SetLimit(cfg.WorkerLimit)`. For each combo: `g.Go(func() error { ctx, cancel := context.WithTimeout(ctx, cfg.ParserTimeout); defer cancel(); latest, err := checker.LatestEpisode(ctx, combo); if err != nil { log.Warnw("parser failed", "combo", combo, "error", err); ParserFailures.WithLabelValues(combo.Player).Inc(); parserFailures.Add(1); return nil /* skip, don't abort */ }; mu.Lock(); latestPerCombo[combo] = latest; mu.Unlock(); return nil })`. `g.Wait()`.
  6. **Diff + bootstrap protection (NOTIF-DET-06):** `affected := []domain.Combo{}`; `snapUpdates := map[domain.Combo]int{}`. For each combo in latestPerCombo:
     - `prev, hadSnapshot := snapshotMap[combo]`
     - `if !hadSnapshot { snapUpdates[combo] = latest; /* bootstrap: snapshot only, no notification */ continue }`
     - `if latest <= prev { snapUpdates[combo] = max(prev, latest) /* never lower */ ; continue }`
     - `snapUpdates[combo] = latest; affected = append(affected, combo)`
  7. **BulkUpsert snapshots** (`snapshots.BulkUpsert(ctx, snapUpdates)`) — do this BEFORE notifications so a mid-run crash doesn't replay notifications. Idempotent UPSERT.
  8. If `len(affected) == 0`: emit success metric + log "no affected combos" + return.
  9. `maxByCombo, err := maxWatched.ForCombos(ctx, affected)`.
  10. For each combo in affected, for each (userID, maxWatched) in maxByCombo[combo]:
      - `firstUnwatched := maxWatched + 1`
      - `if firstUnwatched > latestPerCombo[combo] { continue }` — defensive race guard (NOTIF-DET-07)
      - resolve `anime *AnimeView` (one query per UNIQUE animeID; memoize across combos sharing animeID)
      - `payload, _ := BuildNewEpisodePayload(combo, anime, maxWatched, latestPerCombo[combo], translationTitleFor(combo))`
      - `dedupeKey := service.NewEpisodeDedupeKey(combo.AnimeID, combo.Player, combo.Language, combo.WatchType, combo.TranslationID)`
      - `_, err := notif.Upsert(ctx, service.UpsertRequest{ UserID: userID, Type: "new_episode", DedupeKey: dedupeKey, Payload: payload })` — D-DET-01 in-process call
      - on success: `NotificationsCreatedTotal.WithLabelValues("new_episode", "detector").Inc()`; `upserted++`
  11. Determine outcome: `parserFailures==0 → "success"`; `parserFailures > 0 && upserted > 0 → "partial"`; `upserted == 0 && parserFailures > 0 → "failed"`. Metric+log accordingly.
  12. Return `RunReport{CombosScanned, AffectedCombos, NotificationsUpserted, DurationMs, ParserFailures}`.
- `services/notifications/internal/job/cleanup.go` *(new)* — `type DismissedRetentionCleanupJob struct { db *gorm.DB; retentionDays int; log *logger.Logger }`. `Run(ctx) (deleted int64, err error)`:
  ```
  res := db.WithContext(ctx).Exec(
      "DELETE FROM user_notifications WHERE dismissed_at IS NOT NULL AND dismissed_at < NOW() - (? || ' days')::interval",
      cfg.RetentionDays,
  )
  return res.RowsAffected, res.Error
  ```
  Log INFO with `deleted` count.
- `services/notifications/internal/job/scheduler.go` *(new)* — `type Scheduler struct { cron *cron.Cron; detector *NewEpisodeDetectorJob; cleanup *DismissedRetentionCleanupJob; gaugeRepo *repo.UnreadGaugeRepository; cfg *config.DetectorConfig; log *logger.Logger }`. `Start(ctx) error`:
  1. `s.cron = cron.New()`.
  2. Compute boot-time jitter: `jitter := time.Duration(rand.Intn(11)-5) * time.Minute` (range -5..+5min); store on Scheduler so a synthetic `make run-detector-once` ignores it.
  3. `s.cron.AddFunc(cfg.Cron, func() { time.Sleep(jitter if positive else 0); s.runDetector(ctx) })`. (Negative jitter is honored only by skipping the first tick — robfig/cron has no negative-offset API; the simplest implementation is `if jitter < 0 { time.Sleep(-jitter) /* runs after, not before */ }` — effectively a +5..-5 randomized boot offset that re-anchors to the hour after the first tick).
  4. `s.cron.AddFunc(cfg.CleanupCron, func() { s.runCleanup(ctx) })`.
  5. Start gauge poller goroutine: `go s.pollUnreadGauge(ctx, cfg.UnreadGaugeEvery)`.
  6. `s.cron.Start()`. Log INFO with schedules + jitter.
  `Stop()`: `s.cron.Stop()` — waits for in-flight jobs.
  `runDetector` / `runCleanup` are private wrappers that call the underlying Run + record metrics in the consistent shape (mirror `services/scheduler/internal/service/job.go` lines 49-90 for the pattern).
- `services/notifications/internal/handler/admin.go` *(new)* — `type AdminHandler struct { detector *job.NewEpisodeDetectorJob; cleanup *job.DismissedRetentionCleanupJob; log *logger.Logger }`. Two handlers:
  - `RunDetectorOnce(w, r)` — calls `detector.Run(r.Context())`, returns the `RunReport` JSON.
  - `RunCleanupOnce(w, r)` — calls `cleanup.Run(r.Context())`, returns `{deleted: N}`.
  No middleware (gateway-non-routing per D-DET-05).
- `services/notifications/internal/transport/router.go` *(modify)* — add `adminHandler *handler.AdminHandler` parameter; mount under the internal routes block:
  ```
  r.Post("/internal/notifications", internalHandler.CreateNotification)
  r.Get("/internal/health", internalHandler.Health)
  // New for Phase 2:
  if adminHandler != nil {
      r.Post("/internal/detector/run-once", adminHandler.RunDetectorOnce)
      r.Post("/internal/cleanup/run-once", adminHandler.RunCleanupOnce)
  }
  ```
- `services/notifications/cmd/notifications-api/main.go` *(modify)* — after the existing handler/service wiring:
  1. Construct `cache.New(cfg.Redis)` (NEW — Phase 1 didn't need Redis; this is required for the catalog client to share the Redis client if we later add a shared connection pool, but for now Redis is consumed only by catalog; we still wire it for the future gauge poller). Skip if `cfg.Redis.Host == ""`.
  2. Construct `episodeChecker := service.NewHTTPEpisodeChecker(cfg.Detector.CatalogURL, log)`.
  3. Construct `hotCombos := job.NewHotCombosCollector(db.DB, log)`.
  4. Construct `snapshotRepo := repo.NewSnapshotRepository(db.DB)` (already partially exists).
  5. Construct `maxWatchedRepo := repo.NewMaxWatchedRepository(db.DB)`.
  6. Construct `unreadGaugeRepo := repo.NewUnreadGaugeRepository(db.DB)`.
  7. Construct `animeReadRepo := repo.NewAnimeViewRepository(db.DB)` — minimal shim, just `GetByID(ctx, id) (*AnimeView, error)` reading the views table.
  8. Construct `detectorJob := job.NewEpisodeDetectorJob(hotCombos, episodeChecker, snapshotRepo, maxWatchedRepo, animeReadRepo, notifService, &cfg.Detector, log)`.
  9. Construct `cleanupJob := job.NewDismissedRetentionCleanupJob(db.DB, cfg.Detector.RetentionDays, log)`.
  10. Construct `scheduler := job.NewScheduler(detectorJob, cleanupJob, unreadGaugeRepo, &cfg.Detector, log)`.
  11. `if cfg.Detector.Enabled { if err := scheduler.Start(context.Background()); err != nil { log.Fatalw(...) } }` else log "detector disabled by NOTIFICATIONS_DETECTOR_ENABLED=false" (rollback toggle).
  12. Construct `adminHandler := handler.NewAdminHandler(detectorJob, cleanupJob, log)`; pass into router.
  13. On SIGINT/SIGTERM: `scheduler.Stop()` BEFORE `srv.Shutdown(ctx)` so in-flight jobs finish.

**Infra + tooling:**

- `docker/docker-compose.yml` *(modify)* — add to notifications service `environment:` block:
  ```
  CATALOG_URL: http://catalog:8081
  NOTIFICATIONS_DETECTOR_ENABLED: "true"
  NOTIFICATIONS_DETECTOR_CRON: "0 * * * *"
  NOTIFICATIONS_CLEANUP_CRON: "30 3 * * *"
  NOTIFICATIONS_RETENTION_DAYS: "30"
  NOTIFICATIONS_DETECTOR_WORKER_LIMIT: "5"
  NOTIFICATIONS_PARSER_TIMEOUT: "10s"
  NOTIFICATIONS_UNREAD_GAUGE_INTERVAL: "5m"
  ```
  Add `catalog` to `notifications.depends_on:` so the detector's catalog client has a target when the service boots (with `condition: service_started`).
- `docker/.env.example` *(modify)* — append the 7 new `NOTIFICATIONS_*` and `CATALOG_URL` lines with brief inline `# ...` comments.
- `Makefile` *(modify)* — add two new shortcuts near the existing `redeploy-%` block:
  ```
  .PHONY: run-detector-once run-cleanup-once
  run-detector-once: ## Trigger the notifications detector synchronously (Phase 2 verification)
      @docker compose -f docker/docker-compose.yml exec -T notifications \
          wget -qO- --post-data='' http://localhost:8090/internal/detector/run-once

  run-cleanup-once: ## Trigger the notifications retention cleanup synchronously
      @docker compose -f docker/docker-compose.yml exec -T notifications \
          wget -qO- --post-data='' http://localhost:8090/internal/cleanup/run-once
  ```

</touch_list>

<tasks>

<task type="auto">
  <name>Task 1: Catalog internal /episodes endpoint + parser adapters + Redis cache (Wave 1, independent of notifications-service changes)</name>
  <files>services/catalog/internal/parser/kodik/latest_episode.go, services/catalog/internal/parser/animelib/latest_episode.go, services/catalog/internal/service/episodes_lookup.go, services/catalog/internal/handler/internal_episodes.go, services/catalog/internal/transport/router.go, services/catalog/cmd/catalog-api/main.go</files>
  <action>
Implement NOTIF-DET-01 entirely inside the catalog service so notifications service code in Task 2 can compile against a real HTTP target (or the catalog test fixtures, via the EpisodeChecker interface stub).

(1) `services/catalog/internal/parser/kodik/latest_episode.go` — add `func (c *Client) LatestEpisodeForTranslation(shikimoriID string, translationID int) (int, error)`. Body: call `c.SearchByShikimoriID(shikimoriID)`; iterate results, find one where `r.Translation != nil && r.Translation.ID == translationID`; compute episode count using the exact same fallback ladder as `GetTranslations` (`r.LastEpisode → r.EpisodesCount → sum(season.Episodes) → 1 if r.Type=="anime"`). Return 0 + `fmt.Errorf("kodik: no translation %d for shikimori %s", translationID, shikimoriID)` when no result matches.

(2) `services/catalog/internal/parser/animelib/latest_episode.go` — add `func (c *Client) LatestEpisodeForTeam(animelibID int, teamID int, watchType string) (int, error)`. Body: call `c.GetEpisodes(animelibID)`; for each episode (newest-first by parsed `Number`) call `c.GetEpisodeStreams(ep.ID)`; for each `PlayerData` in the response, check `pd.Team.ID == teamID` and a watch-type match (`watchType=="dub" → pd.TranslationType.ID==2`, `watchType=="sub" → pd.TranslationType.ID==1`). Return the FIRST (highest) episode number where any PlayerData matches. Use `errgroup.WithContext` cap 5 to fan out GetEpisodeStreams calls; early-exit once a match is found and all greater-numbered episodes have been checked. Take `animelibID` (an int) NOT a slug — the caller (episodes_lookup.go) resolves slug→animelibID via the existing CatalogService.findAnimeLibID flow.

(3) `services/catalog/internal/service/episodes_lookup.go` — implement `EpisodesLookupService` per the touch list. Use the existing `s.cache` (`*cache.RedisCache`) field signature mirroring `services/catalog/internal/service/catalog.go:41`. Cache key MUST be literal `notifications:episodes:{shikimori_id}:{player}:{translation_id}:{watch_type}` (NOTIF-DET-01 mandates that exact shape). TTL = `5 * time.Minute`. For `animelib`: look up the catalog `Anime` row via `s.animeRepo.GetByShikimoriID(ctx, shikimoriID)`, then resolve animelibID via the existing `s.catalogService.findAnimeLibID(...)` helper — if findAnimeLibID is currently unexported, leave it unexported and either (a) inject `s.catalogService` and call it via a thin wrapper method exported just for this package, or (b) duplicate the 5-line search-then-pick logic locally with a TODO comment. Pick **(a)** — duplication invites drift.

For `translationID`: parse via `strconv.Atoi`; on parse error return 400-shaped `apperrors.InvalidInput`. Validate `watchType ∈ {"sub", "dub"}` for animelib; for kodik watch_type is informational (Kodik's per-translation Type field already encodes voice/sub).

(4) `services/catalog/internal/handler/internal_episodes.go` — implement `InternalEpisodesHandler` per the touch list. Reuse `shikimoriIDPattern` from `internal_cache.go` (extract it to a shared file if needed — `internal/handler/internal_common.go` — to avoid duplication; if internal_cache.go's regex is private package-scoped, just declare a sibling `var` with the same regex literal).

(5) `services/catalog/internal/transport/router.go` — add `internalEpisodesHandler *handler.InternalEpisodesHandler` to `NewRouter`'s signature; inside the `if internalCacheHandler != nil {...}` block (or a new parallel block), register:
```
if internalEpisodesHandler != nil {
    r.Get("/internal/anime/{shikimoriId}/episodes", internalEpisodesHandler.GetLatestEpisode)
}
```

(6) `services/catalog/cmd/catalog-api/main.go` — construct the new service and handler in the existing wiring sequence (look at how `internalCacheHandler` is constructed and follow the same shape). Pass into `transport.NewRouter(...)`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/catalog &amp;&amp; go build ./... &amp;&amp; go vet ./...</automated>
  </verify>
  <done>Catalog builds + vets clean. `grep -c "internal/anime/{shikimoriId}/episodes" services/catalog/internal/transport/router.go` is 1. `grep -c "notifications:episodes:" services/catalog/internal/service/episodes_lookup.go` is at least 1. Hitting the endpoint after redeploy: `curl -fsS 'http://localhost:8081/internal/anime/57466/episodes?player=animelib&translation_id=9999&watch_type=dub&language=ru'` returns 200 with `latest_available_episode` + `checked_at` keys (parser may legitimately 404 the stub combo — accept 404 as proof the dispatch reached the parser). Second identical call within 5 minutes does NOT hit the parser (verify via `make logs-catalog | grep animelib` — only ONE parser-side log line per shikimori+combo in the 5-minute window).</done>
</task>

<task type="auto">
  <name>Task 2: Notifications service detector — combo collector, snapshot/maxwatched repos, EpisodeChecker, payload builder, detector orchestrator, metrics</name>
  <files>services/notifications/go.mod, services/notifications/internal/config/config.go, services/notifications/internal/domain/combo.go, services/notifications/internal/job/doc.go, services/notifications/internal/job/hotcombos.go, services/notifications/internal/job/metrics.go, services/notifications/internal/repo/snapshot.go, services/notifications/internal/repo/maxwatched.go, services/notifications/internal/repo/anime_view.go, services/notifications/internal/service/catalog_client.go, services/notifications/internal/service/payload_builder.go, services/notifications/internal/job/detector.go</files>
  <action>
Build the detector machinery as a single coherent commit so Task 3's scheduler + admin endpoint wires it without forward-referencing.

(0) `services/notifications/go.mod` — add direct require for `github.com/robfig/cron/v3 v3.0.1` (match `services/scheduler/go.mod:14`). Promote `golang.org/x/sync` to a direct require at `v0.18.0`. Run `cd services/notifications && go mod tidy`.

(1) `services/notifications/internal/config/config.go` — add the `DetectorConfig` struct + `Detector DetectorConfig` field on `Config` per the touch list. Defaults: `Enabled=true, Cron="0 * * * *", CleanupCron="30 3 * * *", RetentionDays=30, WorkerLimit=5, ParserTimeout=10s, UnreadGaugeEvery=5m, CatalogURL="http://catalog:8081"`. Add `getEnvBool` helper if not present.

(2) `services/notifications/internal/domain/combo.go` *(new)* — `type Combo struct { AnimeID, ShikimoriID, Player, Language, WatchType, TranslationID string }` (D-DET-04 import-cycle fix — domain-level type both job/ and repo/ can import).

(3) `services/notifications/internal/job/doc.go` — replace placeholder with a real package doc explaining the four files in this package and the design-doc cross-reference.

(4) `services/notifications/internal/job/hotcombos.go` — `type HotCombosCollector struct { db *gorm.DB; log *logger.Logger }`. `func (c *HotCombosCollector) Collect(ctx) ([]domain.Combo, error)` runs the DISTINCT join in the touch list via `c.db.WithContext(ctx).Raw(...).Scan(&combos)`. Wrap errors with `apperrors.Wrap(err, apperrors.CodeInternal, "collect hot combos")`.

(5) `services/notifications/internal/job/metrics.go` — register all six metrics via `promauto.NewCounterVec` / `NewHistogram` / `NewGauge`. Names + labels MUST match NOTIF-NF-01 exactly:
- `notifications_created_total{type,producer}`
- `notifications_detector_runs_total{outcome}`
- `notifications_detector_duration_seconds` (histogram, buckets `[0.1, 0.5, 1, 5, 10, 30, 60, 300, 600]` matching libs/metrics/scheduler.go pattern)
- `notifications_detector_combos_scanned` (gauge)
- `notifications_detector_parser_failures_total{player}`
- `notifications_active_unread_gauge` (gauge)

Pattern reference: `libs/metrics/scheduler.go` shows the exact promauto + ops shape used elsewhere in the project.

(6) `services/notifications/internal/repo/snapshot.go` — extend the existing stub: add `BulkLoad(ctx, combos []domain.Combo) (map[domain.Combo]int, error)` + `BulkUpsert(ctx, updates map[domain.Combo]int) error`. For BulkLoad, build the IN-tuple via GORM `Where("(anime_id, player, language, watch_type, translation_id) IN ?", tuples)` where `tuples := [][]interface{}{{combo.AnimeID, combo.Player, combo.Language, combo.WatchType, combo.TranslationID}, ...}`. For BulkUpsert, use `clause.OnConflict{ Columns: []clause.Column{{Name: "anime_id"}, {Name: "player"}, {Name: "language"}, {Name: "watch_type"}, {Name: "translation_id"}}, DoUpdates: clause.Assignments(map[string]interface{}{"latest_episode": gorm.Expr("EXCLUDED.latest_episode"), "checked_at": time.Now(), "updated_at": gorm.Expr("NOW()")}) }`. Chunk inserts at 200 rows.

(7) `services/notifications/internal/repo/maxwatched.go` *(new)* — `type MaxWatchedRepository struct { db *gorm.DB }`. `ForCombos(ctx, []domain.Combo) (map[domain.Combo]map[string]int, error)`. Use the GROUP BY query from the touch list. Returns combo → userID → maxEpisode. Use the existing `WatchHistoryView` table via `db.Table("watch_history").Select(...).Where(...).Scan(&rows)`.

(8) `services/notifications/internal/repo/anime_view.go` *(new)* — `type AnimeViewRepository struct { db *gorm.DB }`. `GetByID(ctx, animeID string) (*AnimeView, error)` — selects from `animes` via the existing `AnimeView` projection. NotFound → `apperrors.NotFound("anime")`.

(9) `services/notifications/internal/service/catalog_client.go` *(new)* — `type EpisodeChecker interface { LatestEpisode(ctx, combo domain.Combo) (int, error) }`. `type HTTPEpisodeChecker struct { baseURL string; client *http.Client; log *logger.Logger }`. Constructor: `NewHTTPEpisodeChecker(baseURL string, log *logger.Logger) *HTTPEpisodeChecker { return &HTTPEpisodeChecker{ baseURL: baseURL, client: &http.Client{ Timeout: 10 * time.Second }, log: log } }`. `LatestEpisode` builds `url := fmt.Sprintf("%s/internal/anime/%s/episodes?player=%s&translation_id=%s&watch_type=%s&language=%s", baseURL, combo.ShikimoriID, combo.Player, combo.TranslationID, combo.WatchType, combo.Language)` (URL-escape params), GETs, on 404 returns 0 + `apperrors.NotFound`, on 5xx/timeout returns 0 + wrapped error, on 200 unmarshals and returns `LatestAvailableEpisode`.

(10) `services/notifications/internal/service/payload_builder.go` *(new)* — implement `BuildNewEpisodePayload` per the touch list. Translation title is currently NOT something the detector has cheap access to (it would require an extra parser hit per combo). For v1.0 pass it as empty string; the frontend NotificationCard already handles `translation_title` being optional (design doc payload spec marks it omitempty). Add `// TODO(v1.0.x): populate translation_title via a per-player title resolver.` Then `dedupeKey := service.NewEpisodeDedupeKey(...)` and payload as a marshaled `domain.NewEpisodePayload`.

(11) `services/notifications/internal/job/detector.go` — implement `NewEpisodeDetectorJob` + `Run(ctx) (RunReport, error)` per the touch list. RunReport struct: `type RunReport struct { CombosScanned, AffectedCombos, NotificationsUpserted int; ParserFailures int; DurationMs int64; Outcome string }`. Use `golang.org/x/sync/errgroup` for the worker pool. Implement bootstrap-protection branch (NOTIF-DET-06) and the never-lower-snapshot invariant (NOTIF-DET-10) exactly as specified in the touch list (steps 6 + 7). After `g.Wait()`, do snapshot UPSERT BEFORE notification UPSERTs (this ordering ensures a mid-run crash replays parser calls — idempotent — but doesn't replay UPSERTs against a now-newer snapshot).

Per-combo memoize: build `animeByID := map[string]*repo.AnimeView{}` to avoid re-fetching anime metadata when multiple combos share an animeID (a typical user has 3 combos × ~24 episodes for one anime → 1 anime fetch instead of 3).

Outcome derivation EXACTLY: `0 parser failures && >=0 upserts → "success"`; `>0 parser failures && >0 upserts → "partial"`; `>0 parser failures && 0 upserts → "failed"`. Reflect into `NotificationsDetectorRunsTotal.WithLabelValues(outcome).Inc()` and the structured INFO log (NOTIF-NF-02 fields: `combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures`).

Per-combo failures log via `log.Warnw("parser failed", "anime_id", combo.AnimeID, "player", combo.Player, "translation_id", combo.TranslationID, "error", err)` — no user_id leak (user_id is not bound to a single combo in the parser-call phase anyway).
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/notifications &amp;&amp; go build ./... &amp;&amp; go vet ./...</automated>
  </verify>
  <done>Notifications service builds + vets clean. `grep -c "notifications_detector_runs_total\|notifications_created_total\|notifications_detector_parser_failures_total\|notifications_active_unread_gauge" services/notifications/internal/job/metrics.go` is 6 (one per series — or 4 distinct strings if you bundle the histogram/gauge without _total suffix, adjust accordingly). `go.mod` lists `github.com/robfig/cron/v3 v3.0.1` and `golang.org/x/sync v0.18.0` as direct requires. Bootstrap-protection branch is explicitly commented in `detector.go` with a reference to NOTIF-DET-06.</done>
</task>

<task type="auto">
  <name>Task 3: Scheduler + cleanup job + unread-gauge poller + admin trigger endpoint + main.go wiring</name>
  <files>services/notifications/internal/job/cleanup.go, services/notifications/internal/job/scheduler.go, services/notifications/internal/repo/unread_gauge.go, services/notifications/internal/handler/admin.go, services/notifications/internal/transport/router.go, services/notifications/cmd/notifications-api/main.go, docker/docker-compose.yml, docker/.env.example, Makefile</files>
  <action>
Wire the detector into a runnable scheduler + expose the manual-trigger endpoints + update infra + add Makefile shortcuts.

(1) `services/notifications/internal/job/cleanup.go` — implement `DismissedRetentionCleanupJob` per the touch list. Single Exec, INFO log with rows-affected. Use parameterized interval to avoid SQL injection on `retentionDays`.

(2) `services/notifications/internal/repo/unread_gauge.go` — `type UnreadGaugeRepository struct { db *gorm.DB }`. `ActiveUnreadCount(ctx) (int64, error)` — `SELECT COUNT(*) FROM user_notifications WHERE dismissed_at IS NULL AND read_at IS NULL`. Backed by the existing `idx_user_unread` partial index from Phase 1, so this is cheap.

(3) `services/notifications/internal/job/scheduler.go` — implement `Scheduler` per the touch list. Use `services/scheduler/internal/service/job.go` as the structural reference for cron-call wrapping (metric increment + duration histogram + last-success gauge) but USE the new `NotificationsDetectorRunsTotal` / `NotificationsDetectorDurationSeconds` metrics, NOT the scheduler service's metrics. Boot-time jitter: compute once in the constructor via `time.Duration(rand.Intn(11)-5) * time.Minute`. Gauge poller: simple `time.Ticker(cfg.UnreadGaugeEvery)` in a goroutine, on each tick: `n, err := gaugeRepo.ActiveUnreadCount(ctx); if err == nil { NotificationsActiveUnreadGauge.Set(float64(n)) }`. Goroutine listens to `ctx.Done()` for clean shutdown.

(4) `services/notifications/internal/handler/admin.go` — implement `AdminHandler` per the touch list. Two POST handlers, no middleware. JSON responses.

(5) `services/notifications/internal/transport/router.go` — add `adminHandler *handler.AdminHandler` parameter to `NewRouter`; mount the two run-once routes under the existing internal routes block (BEFORE the `/api/notifications/*` Route block).

(6) `services/notifications/cmd/notifications-api/main.go` — extend the existing boot sequence to construct everything per the touch list. Order matters: cache → checker → repos → jobs → scheduler → admin handler → router → server start → if cfg.Detector.Enabled: scheduler.Start. Shutdown: `scheduler.Stop()` BEFORE `srv.Shutdown(ctx)`.

(7) `docker/docker-compose.yml` — add the 7 new env vars + `CATALOG_URL` to the notifications service block. Add `catalog` to `depends_on` (the existing notifications block from Phase 1 lists `postgres, redis` — add `catalog`).

(8) `docker/.env.example` — append the 7 new `NOTIFICATIONS_*` vars + `CATALOG_URL` with inline comments.

(9) `Makefile` — add the two new `.PHONY` targets per the touch list, with help comments compatible with `make help`.
  </action>
  <verify>
    <automated>cd /data/animeenigma/services/notifications &amp;&amp; go build ./... &amp;&amp; go vet ./... &amp;&amp; docker compose -f docker/docker-compose.yml config --services | grep -c '^notifications$' &amp;&amp; grep -c 'NOTIFICATIONS_DETECTOR_ENABLED' docker/docker-compose.yml &amp;&amp; grep -c 'run-detector-once' Makefile</automated>
  </verify>
  <done>Service builds + vets clean. docker-compose lists notifications, the 7 new env vars are present, Makefile has both new targets. Restart locally: `make redeploy-notifications` succeeds; `make logs-notifications | head -50` shows "scheduler started" with the two cron expressions + jitter offset.</done>
</task>

<task type="auto">
  <name>Task 4: End-to-end verification — bootstrap protection, diff fire, idempotency, aggregation, cleanup, metrics, internal-only routing</name>
  <files>(no source edits — verification-only task; fix-forward into the appropriate prior task on regressions)</files>
  <action>
Run the full success-criteria gauntlet against the live containers. This task is the gate; any failing sub-step is fix-forward into the originating task (1, 2, or 3).

**Setup**:
- `make redeploy-catalog` (picks up internal endpoint + parser adapters).
- `make redeploy-notifications` (picks up scheduler + jobs).
- Wait ~5s, `make health` → both services healthy.
- Resolve `UI_AUDIT_USER_ID`: `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "SELECT id FROM users WHERE username='ui_audit_bot'"`.
- Pick a real ongoing anime present in DB: `SELECT id, shikimori_id, name FROM animes WHERE status='ongoing' LIMIT 5` — record `ANIME_ID`, `SHIKIMORI_ID`.
- Reset state: `DELETE FROM parser_episode_snapshots; DELETE FROM user_notifications;`.

**SC1 — Catalog endpoint + Redis cache (NOTIF-DET-01)**:
```
docker compose -f docker/docker-compose.yml exec -T catalog wget -qO- \
  "http://localhost:8081/internal/anime/${SHIKIMORI_ID}/episodes?player=animelib&translation_id=9999&watch_type=dub&language=ru" | jq
```
Expect: JSON with `latest_available_episode` (int) + `checked_at` (ISO). Re-run within 5 min; grep `make logs-catalog | tail -200 | grep -c "animelib: get_episodes"` shows the count incremented by 1 across BOTH calls (one parser hit; second served from Redis).

**SC2 — Bootstrap protection (NOTIF-DET-06)**:
```
# Seed watch_history so a hot combo exists
INSERT INTO watch_history (user_id, anime_id, episode_number, player, language, watch_type, translation_id, ...)
VALUES ('${UI_AUDIT_USER_ID}', '${ANIME_ID}', 5, 'animelib', 'ru', 'dub', '9999', NOW(), NOW());
INSERT INTO anime_list (user_id, anime_id, status, ...)
VALUES ('${UI_AUDIT_USER_ID}', '${ANIME_ID}', 'watching', NOW(), NOW())
ON CONFLICT DO NOTHING;
# Tables still empty
DELETE FROM parser_episode_snapshots; DELETE FROM user_notifications;
make run-detector-once
# Verify
SELECT COUNT(*) FROM parser_episode_snapshots; -- expect >= 1
SELECT COUNT(*) FROM user_notifications;       -- expect 0
```

**SC3 — First real notification fires (NOTIF-DET-07 + NOTIF-DET-08)**:
```
# Snapshot row from SC2 is at the parser's "current" episode count, call it N.
# Inject a stale snapshot at N-1 so the next detector run sees a diff.
UPDATE parser_episode_snapshots SET latest_episode = latest_episode - 1
 WHERE anime_id = '${ANIME_ID}' AND player='animelib' AND translation_id='9999';
make run-detector-once
SELECT user_id, dedupe_key,
       payload->>'first_unwatched_episode' AS first_un,
       payload->>'latest_available_episode' AS latest,
       payload->>'watch_url' AS url
  FROM user_notifications WHERE user_id = '${UI_AUDIT_USER_ID}';
```
Expect EXACTLY 1 row, `dedupe_key = 'new_episode:${ANIME_ID}:animelib:ru:dub:9999'`, `first_un = 6`, `latest = <parser current>`, `url` non-null and shaped `/anime/{ANIME_ID}/watch?player=animelib&episode=6&translation=9999`.

**SC4 — Idempotency (NOTIF-DET-10)**:
```
make run-detector-once   # re-run with unchanged upstream
SELECT COUNT(*), MAX(updated_at)::text, read_at FROM user_notifications WHERE user_id = '${UI_AUDIT_USER_ID}';
```
Expect: COUNT stays 1; `updated_at` may bump; `read_at` stays NULL only if it was NULL before (UPSERT clears read_at — design decision; SC4 in ROADMAP allows this).

**SC5 — Aggregation re-fire (NOTIF-DET-08)**:
This requires bumping the parser's view of "latest" — easiest: re-poll via the cache key + manually `cache.Set` to bypass the parser, OR rely on parser actually showing a higher episode. Pragmatic SC5: directly UPDATE the snapshot down by 2 (simulating "parser saw 8 vs snapshot's 6"):
```
UPDATE parser_episode_snapshots SET latest_episode = latest_episode - 2
 WHERE anime_id = '${ANIME_ID}' AND player='animelib' AND translation_id='9999';
make run-detector-once
SELECT payload->>'first_unwatched_episode' AS first_un,
       payload->>'latest_available_episode' AS latest,
       read_at FROM user_notifications WHERE user_id = '${UI_AUDIT_USER_ID}';
```
Expect: COUNT still 1 (UPSERT, not INSERT). `latest_available_episode` increased by 2. `first_unwatched_episode` STAYS at 6. `read_at` reset to NULL.

**SC6 — Cleanup retention (NOTIF-DET-09)**:
```
# Insert two test rows with different dismissed_at ages
INSERT INTO user_notifications (id, user_id, type, dedupe_key, payload, dismissed_at, created_at, updated_at)
VALUES (gen_random_uuid(), '${UI_AUDIT_USER_ID}', 'new_episode', 'test:old', '{}'::jsonb, NOW() - INTERVAL '31 days', NOW() - INTERVAL '31 days', NOW() - INTERVAL '31 days'),
       (gen_random_uuid(), '${UI_AUDIT_USER_ID}', 'new_episode', 'test:young', '{}'::jsonb, NOW() - INTERVAL '29 days', NOW() - INTERVAL '29 days', NOW() - INTERVAL '29 days');
make run-cleanup-once
SELECT dedupe_key FROM user_notifications WHERE dedupe_key IN ('test:old', 'test:young');
```
Expect: only `test:young` remains.

**SC7 — All six metrics exposed (NOTIF-NF-01)**:
```
curl -s http://localhost:8090/metrics | grep -E '^notifications_(created_total|detector_runs_total|detector_duration_seconds|detector_combos_scanned|detector_parser_failures_total|active_unread_gauge)' | wc -l
```
Expect: >=6 distinct series (some may be families e.g. `_count`/`_sum`/`_bucket` for the histogram — that's fine; the requirement is presence, not series count). Inspect `notifications_active_unread_gauge` value matches the COUNT(*) from SQL within ~5min lag.

**Internal-only routing**:
```
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/internal/anime/${SHIKIMORI_ID}/episodes
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/internal/detector/run-once
```
Both expect 404. From inside the network, both expected 200/202.

**NOTIF-NF-02 structured log spot-check**:
```
make logs-notifications | grep -E "detector run (started|completed)" | tail -3
```
Each "completed" line contains all of: combos_scanned, affected_combos, notifications_upserted, duration_ms, parser_failures. No `username=`, `email=`, or other PII fields.

If ANY sub-step fails: identify which task originated the defect (1=catalog, 2=detector core, 3=scheduler/admin) and fix-forward IN PLACE — do not split into a gap plan unless the deviation is structural.
  </action>
  <verify>
    <automated>set -euo pipefail; UNREAD_BEFORE=$(docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -tAc "SELECT COUNT(*) FROM user_notifications WHERE dismissed_at IS NULL AND read_at IS NULL"); RESULT=$(docker compose -f docker/docker-compose.yml exec -T notifications wget -qO- --post-data='' http://localhost:8090/internal/detector/run-once); echo "$RESULT" | jq -e .combos_scanned > /dev/null; curl -fsS http://localhost:8090/metrics | grep -qE '^notifications_detector_runs_total\b'; curl -fsS http://localhost:8090/metrics | grep -qE '^notifications_active_unread_gauge\b'; CODE=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8000/internal/detector/run-once); test "$CODE" = "404"</automated>
  </verify>
  <done>All 7 ROADMAP success criteria pass. The detector run-once endpoint returns a structured RunReport. All six Prometheus series are visible. The internal routes return 404 via the gateway. SUMMARY.md captures verbatim SQL + curl outputs.</done>
</task>

</tasks>

<verification>

## Verification matrix — ROADMAP Phase 2 success criteria → exact runner commands

| SC | Requirement | Command(s) | Pass criterion |
|---|---|---|---|
| **SC1** | NOTIF-DET-01 — catalog endpoint + 5min Redis cache | `docker compose exec -T catalog wget -qO- 'http://localhost:8081/internal/anime/${SID}/episodes?player=animelib&translation_id=9999&watch_type=dub&language=ru'` then immediately re-run; `make logs-catalog \| grep -c animelib_get_episodes` | First call returns 200 with `latest_available_episode` + `checked_at`; second call within 5 min does NOT increment the parser-log counter |
| **SC2** | NOTIF-DET-06 — bootstrap protection | `DELETE FROM parser_episode_snapshots; DELETE FROM user_notifications; make run-detector-once; SELECT COUNT(*) FROM parser_episode_snapshots; SELECT COUNT(*) FROM user_notifications;` | snapshots ≥ 1, notifications = 0 |
| **SC3** | NOTIF-DET-07, NOTIF-DET-08 — first real fire | Seed watch_history@ep5 + UPDATE snapshot.latest_episode = current-1, then `make run-detector-once`; SELECT row | EXACTLY 1 row, dedupe_key `new_episode:{anime_id}:animelib:ru:dub:9999`, `first_unwatched=6`, `latest_available={current}`, `watch_url` non-null |
| **SC4** | NOTIF-DET-10 — idempotency | `make run-detector-once` again; SELECT COUNT(*) | COUNT stays 1; no new rows |
| **SC5** | NOTIF-DET-08 — aggregation re-fire | `UPDATE parser_episode_snapshots SET latest_episode = latest_episode - 2`; `make run-detector-once`; SELECT payload | COUNT still 1; `latest_available` +2; `first_unwatched` stays 6; `read_at` becomes NULL |
| **SC6** | NOTIF-DET-09 — retention cleanup | Insert two test rows with `dismissed_at = NOW()-31d` and `NOW()-29d`; `make run-cleanup-once`; SELECT remaining | Only the 29-day row survives |
| **SC7** | NOTIF-NF-01 — six metrics live | `curl -s http://localhost:8090/metrics \| grep -E '^notifications_' \| awk '{print $1}' \| sort -u` | All six series names present |

## Cross-cutting checks (not numbered in ROADMAP but required)

- **Internal-only routing (D-DET-02 + D-DET-05):** `curl http://localhost:8000/internal/anime/SID/episodes` → 404. `curl http://localhost:8000/internal/detector/run-once` → 404. Both return 200 via `docker compose exec` inside the network.
- **NOTIF-NF-02 structured logs:** every "detector run completed" INFO line carries the five required fields; no username/email leakage. WARN lines on parser failure carry anime_id+player+translation_id.
- **NOTIF-DET-10 failure isolation:** simulate by killing the catalog container mid-run (`docker compose stop catalog`) and triggering the detector — the run completes with `outcome="failed"` if all combos failed or `outcome="partial"` if some snapshot rows existed without diff, no panic in logs.
- **Rollback toggle (D-RB-01):** `docker compose run -e NOTIFICATIONS_DETECTOR_ENABLED=false notifications` startup logs show "detector disabled by NOTIFICATIONS_DETECTOR_ENABLED=false" and the scheduler is not started.

</verification>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|---|---|
| host → gateway | Public HTTP (port 8000). Gateway never proxies `/internal/*` (D-DET-02). |
| gateway → notifications:8090 | Docker-internal; only `/api/notifications/*` is reachable from gateway. |
| notifications → catalog:8081/internal/* | Docker-internal; no auth (Phase 1 D-05 pattern). |
| catalog → kodik/animelib parser (upstream) | Outbound HTTPS to third-party APIs. |
| postgres ↔ notifications/catalog | Docker network; shared `animeenigma` DB; both services hold a single GORM handle. |

## STRIDE Threat Register

| ID | Category | Component | Disposition | Mitigation |
|---|---|---|---|---|
| T-02-01 | **S**poofing | Detector calls `POST /internal/notifications` (NOT — we chose in-process per D-DET-01). N/A | accept | In-process call has no spoofing surface; no HTTP layer to authenticate. |
| T-02-02 | **T**ampering | `parser_episode_snapshots.latest_episode` modified by attacker who reaches Postgres | accept | DB access is Docker-internal; user_notifications already has the same exposure post-Phase-1. Snapshot tampering would cause spurious notifications or suppressed ones — not data loss. |
| T-02-03 | **R**epudiation | Detector creates notification for a user; user claims they never asked for it | accept | Notifications are clearly opt-out (dismiss button) and require the user to be in `anime_list.status='watching'` (consent surface). |
| T-02-04 | **I**nformation disclosure | Catalog `/internal/episodes` leaks ongoing-anime episode counts to attacker who reaches it | accept | Already public via `/api/anime/{id}/animelib/episodes` (no auth). New endpoint reveals nothing new. |
| T-02-05 | **I**nformation disclosure | Detector logs include user_id (PII) | mitigate | user_id is opaque UUID (not PII per project convention; CLAUDE.md "username/email is PII, user_id is fine"). WARN lines from parser failures explicitly OMIT user_id (the failure is per-combo, not per-user). |
| T-02-06 | **D**enial of service | Malicious shikimori_id triggers parser thrash via uncached cache key | mitigate | Catalog endpoint validates `shikimoriIDPattern` (alphanumeric + `_-` only) and `player` allowlist. Parser timeout 10s + worker pool cap 5 caps blast radius even on legitimate calls. |
| T-02-07 | **D**enial of service | Run-once admin endpoint hit in a loop saturates parsers | mitigate | Gateway never proxies `/internal/*`; loop attacker must already be inside Docker. Cache TTL 5min absorbs repeated calls naturally. Optionally add a simple in-process semaphore on the admin handler if production logs show abuse (out of scope for v1.0). |
| T-02-08 | **D**enial of service | Cron job runs while DB is offline mid-night, returns errors forever | mitigate | Failure isolation per NOTIF-DET-10: errors logged and recorded as metric `notifications_detector_runs_total{outcome="failed"}`; next hour retries. Cleanup is a single statement — a Postgres outage just defers it. |
| T-02-09 | **E**levation of privilege | Anyone reaching `:8090/internal/detector/run-once` from inside Docker triggers a detector run | accept | Same risk as `:8090/internal/notifications` from Phase 1 (already accepted via Phase 1 D-05). Docker network boundary is the trust gate. |

</threat_model>

<risks>

## Phase-2-specific risks + mitigations

1. **R-02-01 — Parser rate-limit storm under cron**: 5-worker concurrency × ~30 hot combos × hourly = ~30 catalog calls per run. Mitigated by: Redis cache TTL 5min absorbs same-combo repeats within a run AND across the run window; `errgroup.SetLimit(5)` caps parallelism; per-call timeout 10s; failure logged + skipped, not retried in-loop (next hour is the retry).
2. **R-02-02 — Race between detector and user dismissing**: detector starts run → user dismisses notification mid-run → UPSERT tries to update the dismissed row. Mitigated by: the partial unique index `uk_user_dedupe WHERE dismissed_at IS NULL` ensures the UPSERT no longer matches the dismissed row, so the UPSERT becomes an INSERT — a fresh notification. The user sees a brand-new badge, which is actually correct behavior (the underlying episode availability really did change after they dismissed).
3. **R-02-03 — Cron-during-restart**: SIGTERM mid-run. Mitigated by: scheduler.Stop() called BEFORE srv.Shutdown(ctx) in main.go; errgroup respects ctx cancellation; pending HTTP calls to catalog abort cleanly via the per-call context. Snapshot UPSERT happens BEFORE notification UPSERTs, so worst case: snapshot moved forward, some notifications not yet fired — next hour catches them.
4. **R-02-04 — Cache invalidation when admin manually injects a snapshot row**: admin UPDATEs `parser_episode_snapshots.latest_episode = 5` to test SC3; the catalog Redis cache still holds the old `latest_available_episode`. The 5-min TTL bounds the staleness; or invalidate via `docker compose exec redis redis-cli DEL "notifications:episodes:${SID}:animelib:9999:dub"`. The SC3 sequence in Task 4 explicitly modifies the SNAPSHOT (which lives in Postgres, not Redis), so this race only matters if the verifier ALSO tries to inject "fake parser output" through the catalog client — which Task 4 explicitly avoids.
5. **R-02-05 — Scheduler running before DB ready**: docker-compose `depends_on` only enforces `service_started`, not health. Notifications service might boot, AutoMigrate, then immediately fire a cron tick before player/catalog rows exist. Mitigated by: hot-combo collector returns empty slice when joins find nothing → detector logs "no combos" and exits cleanly. Boot-time jitter ±5min adds extra slack.
6. **R-02-06 — AnimeLib `LatestEpisodeForTeam` is N+1 HTTP calls per combo**: discussed in D-DET-04. Worst-case 24 episodes × 30 combos = 720 catalog→animelib calls per run. Mitigated by: 5-min Redis cache (one combo = one cache key for the whole run); inner errgroup cap 5 inside the adapter; v1.0.x upstream-batch optimization is local to the adapter file.
7. **R-02-07 — Detector flagged "partial" on a fresh DB where most snapshots are bootstrap (no notifications)**: outcome rule says `>0 parser failures && 0 upserts = failed`, but `0 parser failures && 0 upserts` = `success` (bootstrap path is normal). Verify the outcome derivation in detector.go matches: `parserFailures==0 → success` regardless of upserted count. Add unit test if not obvious.
8. **R-02-08 — Cron expression parse error at boot**: typo in `NOTIFICATIONS_DETECTOR_CRON` would cause `cron.AddFunc` to return an error. Mitigated by: scheduler.Start returns the error to main.go which Fatalws — service refuses to boot rather than running with a silent disabled cron.
9. **R-02-09 — `gorm.io/datatypes` JSONB driver mismatch on the snapshot table**: the snapshot has no JSONB column, only ints/strings/timestamps — no JSONB driver in play for snapshot UPSERT. No risk.

</risks>

<rollback>

## Rollback plan

**Level 0 — runtime toggle (zero-downtime)**:
Set `NOTIFICATIONS_DETECTOR_ENABLED=false` in `docker/.env` and `make restart-notifications`. Service stays up, cron not started, no detector fires, no cleanup fires. Public API + producer endpoint continue to work, so any seeded notifications from Phase 1 still render normally.

**Level 1 — endpoint disable**:
Revert `services/catalog/internal/transport/router.go` to NOT register `/internal/anime/{shikimoriId}/episodes`. Notifications service detector will start getting 404 from the catalog client → `outcome="failed"` per run; no notifications created. Same effect as Level 0 but the catalog change is opt-in.

**Level 2 — full revert**:
`git revert` the three Phase-2 commits (one per task). Notifications service reverts to Phase-1 state (CRUD API + producer endpoint, no scheduler). Migrations are forward-only but additive — `parser_episode_snapshots` already existed in Phase 1; any rows from Phase-2 runs are inert without the detector reading them. `user_notifications` rows created by the detector during Phase-2 testing can be left in place (the public API treats them like any other notification) or DELETE'd via `DELETE FROM user_notifications WHERE created_at > '${PHASE_2_DEPLOY_TIMESTAMP}'`.

**D-RB-01 (rollback toggle)**: The `NOTIFICATIONS_DETECTOR_ENABLED` env defaults to `true` (Phase 2 ships ON). The toggle EXISTS specifically to make Level 0 possible without a code revert.

</rollback>

<success_criteria>

Phase 2 is complete when:
- [ ] All 7 ROADMAP SC1..SC7 verification commands pass (recorded verbatim in SUMMARY).
- [ ] `go build ./...` + `go vet ./...` are green for both catalog and notifications services.
- [ ] All 6 NOTIF-NF-01 metric series visible at `:8090/metrics` after one detector run.
- [ ] At least one "detector run completed" INFO line carries all five NOTIF-NF-02 fields with no PII.
- [ ] `/internal/*` routes on both services return 404 via the gateway.
- [ ] Boot-time jitter is logged on `make logs-notifications` for forensic reproducibility.
- [ ] Rollback toggle `NOTIFICATIONS_DETECTOR_ENABLED=false` verified to skip scheduler start.
- [ ] `make run-detector-once` and `make run-cleanup-once` both work and return structured JSON.

</success_criteria>

<output>

After Phase 2 completion, the executor writes
`.planning/workstreams/notifications/phases/02-detector-and-catalog-endpoint/SUMMARY.md`
following the same shape as Phase 1's SUMMARY: frontmatter (status, completed, executor_branch, score, commits, requirements_resolved), verification matrix table with PASS/FAIL per SC, deviations-from-plan section, risks-materialized section, decisions honored (D-DET-01..07) with one-line evidence each.

</output>

## Score (per project convention)

- **UXΔ:** `0 (Ambiguous)` — pure backend; user-visible bell + toast lands in Phase 3. Aggregate is zero, sub-signals (task-completion, error-rate, satisfaction) are all unmoved until Phase 3 mounts the UI. The phase is necessary scaffolding, not direct UX delivery.
- **CDI:** `0.06 × 13` — Distribution Spread: catalog (2 parser adapters + 1 service + 1 handler + 1 router edit + main.go), notifications (2 new repos, 3 new job files, 1 service file, 1 handler, transport edit, main.go, config, go.mod), infra (docker-compose, env, Makefile) ≈ 15 components in a repo with ~250 backend files → ~0.06. Coherence Shift: low (additive, no schema-breaking change; cron pattern already exists in scheduler service so it extends the established pattern). Effort: 13 Fibonacci — multi-subsystem feature with the most subtle correctness work in v1.0 (bootstrap protection invariant, idempotency UPSERT, failure isolation, N+1 parser fan-out). Cleanly 13, not 8 — bootstrap-protection + idempotency invariants alone earn a Fib step.
- **MVQ:** `Kraken 80%/88%` — Kraken (deep-sea, many tentacles, latent power) is the right shape: invisible from the surface but reaches into catalog, notifications, scheduler-pattern, Redis cache, and Prometheus simultaneously. 80% match — could be a Griffin (graceful + reliable + visible) but the visibility is deferred until Phase 3, so Kraken's "latent power" fits better. 88% slop-resistance — bootstrap-protection guard + UPSERT idempotency + worker-pool failure isolation + never-lower-snapshot invariant + rollback toggle make accidental defects (false positives, notification storms) extremely unlikely to escape Task 4's verification gauntlet.
