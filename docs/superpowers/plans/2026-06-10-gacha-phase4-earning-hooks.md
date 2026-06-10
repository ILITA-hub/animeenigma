# Лудка (Gacha) — Phase 4: Earning Hooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire the «Энигмы» income side: (1) `player` fires non-blocking internal credits when a user watches an episode (22) or completes a title (80) — idempotent gacha-side; (2) gacha gains `POST /api/gacha/daily` — daily claim with a consecutive-day streak bonus, idempotent per UTC day via the existing ledger unique index.

**Architecture:** Player gets a tiny fire-and-forget `GachaCreditProducer` (buffered chan 256 + worker goroutine + drop-on-full WARN + 3s HTTP timeout — same resilience contract as the analytics effect producer: a gacha outage can NEVER affect MarkEpisodeWatched). Dedup lives gacha-side on `(user_id, reason, ref)`: player fires on EVERY call; duplicates no-op. Daily claim is a single gacha-side transaction: streak math + ledger insert (ref = `YYYY-MM-DD` UTC ⇒ the unique index makes same-day double-claims impossible even under races) + wallet streak fields.

**Tech Stack:** Go, httptest-faked producer tests, sqlite repo tests (existing helpers).

---

## Context for the implementer

- Spec §5.2 (amounts): episode 22 (ref `anime:ep`), title complete 80 (ref anime), daily base 50 + streak +10/consecutive-day capped at +100 (max total 150). Decision #18: NO retroactive backfill — only events after deploy.
- Hook points in `services/player/internal/service/list.go`:
  - `MarkEpisodeWatched` (~292) — after the (idempotent) `progressRepo.MarkCompleted` call: fire `episode_watched` credit with ref `fmt.Sprintf("%s:%d", animeID, req.Episode)`. Fire on EVERY invocation (gacha dedups) — both the auto-create branch and the main path (put the fire AFTER the final MarkCompleted so one call site covers the main path; the auto-create branch `return`s early — add the fire there too).
  - `UpdateEntry` — where `req.Status == "completed"` sets `entry.CompletedAt` (~209): fire `title_completed` credit, ref = animeID.
- Existing gacha internal endpoint: `POST /internal/gacha/credit` body `{"user_id","amount","reason","ref"}` → `{"applied":bool}`; reasons `episode_watched`/`title_completed`/`daily` already exist as consts in `services/gacha/internal/domain/wallet.go`.
- Wallet daily fields already exist (Phase 1): `gacha_wallets.daily_streak`, `last_daily_at *time.Time`.
- Player service layout: config `services/player/internal/config/config.go`, DI in `services/player/cmd/player-api/main.go:371` (`NewListService(...)` — extend its param list with the producer).
- Dirty-tree rules unchanged: path-scoped commits; never -A; never stage go.work.sum / compose / gateway. Trailers per Phase 1 plan. Player + gacha are SEPARATE services — separate commits.

## File Structure

| File | Responsibility |
|------|----------------|
| `services/player/internal/service/gacha_credit.go` | `GachaCreditProducer` — Start/Stop, chan 256, drop-on-full, POST /internal/gacha/credit |
| `services/player/internal/service/gacha_credit_test.go` | httptest: payload shape; drop-on-full doesn't block; Stop drains |
| `services/player/internal/service/list.go` (modify) | fire episode/title credits (nil-safe producer field) |
| `services/player/internal/config/config.go` (modify) | `GachaInternalURL` (env `GACHA_INTERNAL_URL`, default `http://gacha:8093`), `GachaCreditEpisode` int64 (env `GACHA_CREDIT_EPISODE`, 22), `GachaCreditTitle` (env `GACHA_CREDIT_TITLE`, 80), `GachaCreditEnabled` bool (env `GACHA_CREDIT_ENABLED`, default true) |
| `services/player/cmd/player-api/main.go` (modify) | construct/Start/defer-Stop producer; pass to NewListService |
| `services/gacha/internal/repo/wallet.go` (modify) | `DailyClaimTx` — one tx: ledger(ref=date, OnConflict DoNothing → already-claimed) + balance + streak fields |
| `services/gacha/internal/service/wallet.go` (modify) | `Daily(ctx, userID, now time.Time)` — streak math, calls repo |
| `services/gacha/internal/service/wallet_test.go` (modify) | daily tests below |
| `services/gacha/internal/handler/wallet.go` (modify) | `POST /api/gacha/daily` handler |
| `services/gacha/internal/transport/router.go` (modify) | route in the JWT player group |
| `services/gacha/internal/config/config.go` (modify) | `DailyBase` (env `GACHA_DAILY_BASE`, 50), `DailyStreakStep` (10), `DailyStreakCap` (100) |
| gateway router (CONTROLLER) | `r.Post("/daily", ProxyToGacha)` in the dark-ship group |
| compose (CONTROLLER if dirty) | player env: GACHA_INTERNAL_URL + amounts |

---

## Task 1: Gacha — daily claim (TDD)

Streak semantics (UTC dates): let `today = now.UTC` date, `last = last_daily_at` date.
- `last == today` → already claimed: return current wallet, `claimed=false`, no writes.
- `last == yesterday` → `streak = streak + 1`; else → `streak = 1`.
- `bonus = min(streak-1, cap/step) * step` (defaults: min(streak-1,10)*10, so day1=+0 … day11+=+100 cap). `amount = base + bonus`.
- ONE tx (`DailyClaimTx(tx…)` or a Transaction inside the repo): insert ledger `{delta:+amount, reason:"daily", ref:today.Format("2006-01-02")}` with OnConflict DoNothing — RowsAffected==0 ⇒ already claimed (race-safe even if last_daily_at was stale) → return claimed=false; else update `balance += amount`, `daily_streak = streak`, `last_daily_at = now`.
- Service `Daily(ctx, userID, now)` takes `now` as param (deterministic tests); handler passes `time.Now()`. Disabled service (`!enabled`) → claimed=false no-op. Wallet row ensured first (GetOrCreate).

Tests (`wallet_test.go`):
```go
func TestDaily_FirstClaim(t *testing.T)            // balance +50, streak 1, claimed=true, ledger ref=date
func TestDaily_SecondClaimSameDayNoop(t *testing.T)// claimed=false, balance unchanged, single ledger row
func TestDaily_ConsecutiveDayIncrementsStreak(t *testing.T) // day1 then day2 → streak 2, +60
func TestDaily_GapResetsStreak(t *testing.T)       // day1 then day4 → streak 1, +50
func TestDaily_StreakBonusCaps(t *testing.T)       // streak 15 preloaded, claim next day → bonus capped +100 (total 150)
```

Handler `ClaimDaily` (claims→401 pattern), route `r.Post("/daily", walletHandler.ClaimDaily)` in the JWT player group, router test 401-without-token. Config knobs + defaults test.

Commit (gacha paths only): `feat(gacha): daily claim — streak bonus, UTC-day idempotent via ledger ref`.

## Task 2: Player — GachaCreditProducer (TDD)

```go
// gacha_credit.go
type GachaCreditProducer struct { url string; episodeAmt, titleAmt int64; ch chan creditMsg; client *http.Client; log *logger.Logger; wg sync.WaitGroup; enabled bool }
func NewGachaCreditProducer(url string, episodeAmt, titleAmt int64, enabled bool, log *logger.Logger) *GachaCreditProducer // chan cap 256, client timeout 3s
func (p *GachaCreditProducer) Start()                       // one worker goroutine: POST {user_id,amount,reason,ref}; log WARN on non-200/err; never retry (gacha dedups make retries pointless)
func (p *GachaCreditProducer) Stop()                        // close chan, wait worker (drains remaining)
func (p *GachaCreditProducer) EpisodeWatched(userID, animeID string, episode int) // non-blocking select{ch<-…default: WARN drop}
func (p *GachaCreditProducer) TitleCompleted(userID, animeID string)
```
Nil-receiver-safe: all public methods no-op when `p == nil || !p.enabled` (so tests constructing ListService without it stay untouched).

Tests: httptest server captures N posted bodies (assert user_id/amount/reason/ref exact); flood 1000 msgs with a slow server → no blocking (send loop completes fast, drops logged); Stop drains pending.

Commit (player paths): `feat(player): non-blocking gacha credit producer (episode/title earning)`.

## Task 3: Player — wire hooks + DI + config

- `ListService` gains `gachaCredit *GachaCreditProducer` field (last NewListService param; pass nil in existing tests that break — prefer updating call sites mechanically).
- `MarkEpisodeWatched`: after final `MarkCompleted` → `s.gachaCredit.EpisodeWatched(userID, animeID, req.Episode)`; same call in the auto-create branch before its return.
- `UpdateEntry`: in the `req.Status == "completed"` CompletedAt-setting branch (only when newly setting it — i.e. the `else if req.Status == "completed"` arm, NOT when CompletedAt was already preserved) → `s.gachaCredit.TitleCompleted(userID, animeID)`. (gacha-side ref=animeID dedups re-completions anyway; rewatches don't double-pay — decision #7-adjacent, spec §5.2 "разовый".)
- Config fields + main.go construct/Start/defer Stop + pass to NewListService.
- `cd /data/animeenigma && go build ./services/player/... && go test ./services/player/internal/service/... -count=1` green.
- Commit (player paths): `feat(player): fire gacha credits on episode watched / title completed`.

## Task 4 (CONTROLLER): compose env + gateway route + deploy + smoke

- compose `player` env: `GACHA_INTERNAL_URL: http://gacha:8093` (+ amounts if non-default). Selective staging if compose dirty.
- gateway: `r.Post("/daily", proxyHandler.ProxyToGacha)` in the dark-ship player group. Build, commit (dormant).
- `make redeploy-gacha` + `make redeploy-player`; both healthy; smoke: daily claim direct on :8093 → 401 without token; player logs show producer started; live end-to-end credit check via psql after marking an episode (can reuse the ui_audit_bot API key через gateway player routes — episode mark fires producer → gacha_ledger row appears with reason=episode_watched).
- Push; update memory.

---

## Self-Review
- §5.2 amounts/dedup ✅ (episode 22 ref anime:ep; title 80 ref anime; daily 50+streak cap100 ref date — ALL ride the Phase-1 unique index). Non-blocking producer ✅ (§3.3 contract). No backfill ✅ (#18 — hooks only fire on new events). Optional sources (themes/reactions/WT) explicitly DEFERRED per spec §5.2.
- Race-safety: same-day double daily claim collapses on the ledger unique index even with stale wallet reads; episode/title double-fires collapse likewise.
- Type consistency: reuses `domain.ReasonEpisodeWatched/ReasonTitleCompleted/ReasonDaily` consts; producer payload matches `handler.CreditRequest` json exactly.
- Known gap: player→gacha hook untestable end-to-end in unit tests (two services) — covered by the live smoke in Task 4.
