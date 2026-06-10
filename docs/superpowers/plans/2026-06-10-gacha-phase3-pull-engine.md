# Лудка (Gacha) — Phase 3: Pull Engine + Collection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The game itself — `POST /api/gacha/banners/{id}/pull` (x1/x10) that atomically debits «Энигмы», rolls rarity-weighted cards from the banner's pool with the x10 SR-floor and per-banner hard-pity@90 guarantees, records the collection (dupes = count++), plus player-facing `GET /api/gacha/banners` (active banners + my pity) and `GET /api/gacha/collection` (full album with owned flags for silhouettes).

**Architecture:** One DB transaction per pull request: conditional debit (`UPDATE … WHERE balance >= cost` — RowsAffected 0 ⇒ insufficient funds, no roll), ledger entry, pity row locked `FOR UPDATE` (concurrent pulls on one banner can't double-count), N rolls computed in-memory against the banner's card pool, pity write-back, collection upserts. Randomness is injected (`func(n int) int`) so tests are deterministic; production uses `math/rand/v2`. Tier weights come from config; when a banner lacks a tier, its weight redistributes to the nearest available tier BELOW (spec §5.3).

**Tech Stack:** Go 1.24, GORM tx + `clause.Locking`, sqlite tests (FOR UPDATE is a Postgres no-op there — acceptable; noted), config-driven economy knobs.

---

## Context for the implementer

- Spec §5.3–5.4 (rates, guarantees, pull algorithm), §4.6–4.7 (collection/pity tables), §6.2–6.4 (player payloads): `docs/superpowers/specs/2026-06-09-gacha-ludka-design.md`.
- Existing code to mirror: `services/gacha/internal/repo/wallet.go` (tx + OnConflict patterns), `wallet_test.go` (sqlite raw-DDL helper), `internal/transport/router.go` (route groups — note Phase 2 restructure: ONE `/api/gacha` Route containing public images, JWT wallet group, admin subtree), `internal/handler/*`.
- Phases 1–2 are LIVE; do not break them. Dirty-tree commit rules unchanged (path-scoped, no `-A`, never stage `go.work.sum`/compose/gateway files; trailers per Phase 1 plan).

## Economy config additions (`internal/config/config.go`)

```go
type EconomyConfig struct {
	StarterBonus int64
	// Phase 3 — pull engine knobs (spec §5.1/5.3).
	PullCostX1    int64 // GACHA_PULL_COST_X1, default 100
	PullCostX10   int64 // GACHA_PULL_COST_X10, default 900
	PityThreshold int   // GACHA_PITY_THRESHOLD, default 90 (the 90th pull without SSR is forced SSR)
	WeightN       int   // GACHA_WEIGHT_N, default 69
	WeightR       int   // GACHA_WEIGHT_R, default 22
	WeightSR      int   // GACHA_WEIGHT_SR, default 8
	WeightSSR     int   // GACHA_WEIGHT_SSR, default 1
}
```

Defaults in `Load()` via `getEnvInt`. Config test asserts the defaults.

## Domain additions (`internal/domain/collection.go`)

```go
// CollectionEntry — one row per (user, card); dupes bump Count (spec §4.6, decision #7).
type CollectionEntry struct {
	UserID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_card,priority:1" json:"user_id"`
	CardID          string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_card,priority:2" json:"card_id"`
	Count           int       `gorm:"not null;default:1" json:"count"`
	FirstObtainedAt time.Time `json:"first_obtained_at"`
}
func (CollectionEntry) TableName() string { return "gacha_collection" }

// PityCounter — per-(user,banner) pulls since last SSR (spec §4.7, decision #11).
type PityCounter struct {
	UserID        string `gorm:"type:uuid;not null;uniqueIndex:idx_user_banner,priority:1" json:"user_id"`
	BannerID      string `gorm:"type:uuid;not null;uniqueIndex:idx_user_banner,priority:2" json:"banner_id"`
	PullsSinceSSR int    `gorm:"not null;default:0" json:"pulls_since_ssr"`
}
func (PityCounter) TableName() string { return "gacha_pity" }
```

---

## Task 1: Domain + config (code-only)
Models above + EconomyConfig fields + `Load()` defaults + config test additions (`TestLoad_PullDefaults`: 100/900/90/69/22/8/1). Build + test. Commit `feat(gacha): pull-engine domain (collection, pity) + economy config knobs`.

## Task 2: Repo — Debit, pity, collection (TDD)

**File:** extend `internal/repo/wallet.go` (Debit) + new `internal/repo/pull.go` + tests.

Tests first (extend sqlite DDL helper with `gacha_collection`, `gacha_pity`):

```go
func TestDebit_SucceedsAndWritesLedger(t *testing.T)        // balance 300, debit 100 → balance 200, ledger delta -100
func TestDebit_InsufficientFundsNoChange(t *testing.T)      // balance 50, debit 100 → ErrInsufficientFunds; balance 50; NO ledger row
func TestPity_GetOrCreateIncrementSetReset(t *testing.T)    // starts 0; SavePity(5) → 5; per-banner isolation (banner B stays 0)
func TestCollectionUpsert_NewThenDupe(t *testing.T)         // first AddCards → count 1 + IsNew; same card again → count 2, FirstObtainedAt unchanged
```

Contracts:

```go
var ErrInsufficientFunds = apperrors.InvalidInput("insufficient balance") // or a dedicated constructor — match libs/errors style

// DebitTx debits amount inside the CALLER's tx. Conditional UPDATE
// gacha_wallets SET balance = balance - ? WHERE user_id = ? AND balance >= ?;
// RowsAffected==0 → ErrInsufficientFunds (tx rolls back). Then ledger insert
// (negative delta, reason pull_x1/pull_x10, ref = pullID for audit).
func DebitTx(tx *gorm.DB, userID string, amount int64, reason, ref string) error

// PullRepository owns pity + collection writes (all *Tx variants take the tx).
func (r *PullRepository) GetPityForUpdate(tx *gorm.DB, userID, bannerID string) (*domain.PityCounter, error) // SELECT ... FOR UPDATE (clause.Locking{Strength:"UPDATE"}); creates row if absent (OnConflict DoNothing + reselect)
func (r *PullRepository) SavePityTx(tx *gorm.DB, p *domain.PityCounter) error
func (r *PullRepository) AddToCollectionTx(tx *gorm.DB, userID string, cardIDs []string) (newIDs map[string]bool, counts map[string]int, err error)
// AddToCollectionTx: per card — INSERT ... ON CONFLICT (user_id,card_id) DO UPDATE SET count = count + 1; report which were new + resulting counts. A card appearing twice in one x10 counts twice.
func (r *PullRepository) ListCollection(ctx context.Context, userID string) ([]domain.CollectionEntry, error)
func (r *PullRepository) CardsByRarity(ctx context.Context, bannerID string) (map[domain.Rarity][]domain.Card, error) // enabled, non-deleted cards of the banner grouped by tier
```

Commit `feat(gacha): debit + pity + collection repository layer`.

## Task 3: Pull engine service (THE CORE — heavy TDD)

**File:** `internal/service/pull.go` + `pull_test.go`.

The roll logic is PURE and separately testable from the tx orchestration:

```go
// tierTable builds the cumulative weight table for the tiers that actually
// have cards in the pool. Missing tiers redistribute their weight to the
// nearest available tier BELOW (SSR→SR→R→N); if nothing below, to nearest above.
// (spec §5.3 "вес перераспределяется на ближайший доступный тир вниз")
func buildTierTable(weights map[domain.Rarity]int, available map[domain.Rarity][]domain.Card) []tierEntry

// rollOne picks a tier by weight (randInt(total)), then a uniform card from
// that tier. forceTier overrides the weighted pick (pity / SR-floor).
func rollOne(table []tierEntry, pool map[domain.Rarity][]domain.Card, randInt func(int) int, forceTier domain.Rarity) domain.Card
```

`PullService.Pull(ctx, userID, bannerID string, mode string) (*PullResult, error)`:
1. Load banner; must exist, be ActiveNow (reuse repo; pass `time.Now()` here), pool non-empty → else `InvalidInput`.
2. cost & count by mode ("x1"→1×100, "x10"→10×900 from config; unknown mode → InvalidInput).
3. In ONE `db.Transaction`:
   a. `DebitTx` (insufficient → error out, nothing happens).
   b. `GetPityForUpdate`.
   c. For each of N rolls: `pity.PullsSinceSSR++`; if `>= cfg.PityThreshold` → force SSR (if pool has SSR; else force highest available tier) ; roll; if rolled tier == SSR → `pity.PullsSinceSSR = 0`.
   d. x10 SR-floor: if mode x10 and no rolled card is SR/SSR → replace the LAST roll with a forced SR roll (if pool has SR; else SSR; if neither exists the floor is unsatisfiable — skip, log once). Pity recomputation: the replaced last roll was a non-SSR and stays non-SSR (SR), so no pity adjustment needed; if floor forced SSR (no SR in pool) reset pity.
   e. `AddToCollectionTx`, `SavePityTx`.
4. Return `PullResult{Cards []PulledCard, Balance int64, Pity int}` where `PulledCard{Card domain.Card; New bool; Count int}`.

`randInt func(int) int` is a `PullService` field — production `rand.IntN` (math/rand/v2), tests inject deterministic sequences.

**Tests (table-driven, deterministic rand):**
```go
func TestBuildTierTable_FullPoolMatchesWeights(t *testing.T)        // 69/22/8/1 cumulative
func TestBuildTierTable_MissingTierRedistributesDown(t *testing.T)  // no SSR → SR weight 9; only N → N weight 100
func TestPull_X1_DebitsRollsRecords(t *testing.T)                   // balance 300→200; 1 card; collection count 1; pity 1
func TestPull_InsufficientFunds_NothingHappens(t *testing.T)        // balance 50; pull x1 → error; balance 50, pity 0, collection empty, no ledger
func TestPull_PityForcesSSRAt90AndResets(t *testing.T)              // pity preloaded 89 → pull x1 rolls SSR regardless of rand; pity 0 after
func TestPull_SSRResetsPityMidBatch(t *testing.T)                   // x10 with rand forcing SSR at roll 3 → pity == 7 after (not 10)
func TestPull_X10_SRFloorUpgradesLast(t *testing.T)                 // rand always rolls N → last card upgraded to SR; 9 N + 1 SR
func TestPull_X10_NoFloorWhenSRPresent(t *testing.T)                // rand rolls SR at position 2 → no upgrade
func TestPull_PerBannerPityIsolation(t *testing.T)                  // pulls on banner A don't move banner B's counter
func TestPull_DupesIncrementCount(t *testing.T)                     // same card twice (1-card pool) → count 2, New only first time
func TestPull_InactiveBannerRejected(t *testing.T)                  // disabled banner → InvalidInput, no debit
func TestPull_Distribution_Sanity(t *testing.T)                     // real rand, 10_000 x1 rolls on full pool: each tier within ±30% of expected (loose — just catches inverted tables)
```

Commit `feat(gacha): pull engine — weighted rolls, x10 SR-floor, per-banner hard pity`.

## Task 4: Player-facing service bits + handlers + router (TDD-light)

- `PullService.ActiveBannersView(ctx, userID)` → for `GET /api/gacha/banners`: ActiveNow banners + per-banner `cards` (id/name/rarity/image_path + `owned bool` from the user's collection) + `my_pity` + `pity_threshold`.
- `PullService.CollectionView(ctx, userID)` → for `GET /api/gacha/collection`: ALL enabled non-deleted cards (the album, spec §6.4) each with `{card, owned, count, first_obtained_at?}` + per-rarity progress counters (`{"SSR": {owned: 3, total: 12}, ...}`).
- `internal/handler/pull.go`: `POST /banners/{id}/pull` (body `{"mode":"x1"|"x10"}`, UUID-validate id), `GET /banners`, `GET /collection` — all read userID from claims (401 if absent), mirror wallet handler style.
- Router: add the three routes to the EXISTING JWT-gated player group (where `/wallet` lives). Router test additions: all three → 401 without token.
- Commit `feat(gacha): player endpoints — pull, active banners view, collection album`.

## Task 5: main.go wiring (code-only)
AutoMigrate `CollectionEntry`, `PityCounter`; wire PullRepository + PullService (rand.IntN) + PullHandler; extend NewRouter. Build + full test. Commit `feat(gacha): wire pull engine (migrate, DI, routes)`.

## Task 6 (CONTROLLER): gateway player routes
Inside the gateway's `/gacha` dark-ship group (where `/wallet` is): add `r.Post("/banners/{id}/pull", proxyHandler.ProxyToGacha)`, `r.Get("/banners", ...)`, `r.Get("/collection", ...)`. Selective staging (dirty router.go). Dormant until bundle.

## Task 7 (CONTROLLER): deploy + live smoke
`make redeploy-gacha`; healthy; tables `gacha_collection`/`gacha_pity` exist; route smoke on :8093 (`POST /api/gacha/banners/x/pull` no-token → 401; with garbage id + no auth → 401 first). Full gameplay smoke (real pull) requires an admin JWT + seeded banner — defer to Phase 5 UI verification; the engine is covered by the deterministic test suite. Push.

---

## Self-Review

- **Spec coverage:** §5.1 costs ✅ (config T1), §5.3 rates+redistribution ✅ (T3 buildTierTable), x10 SR-floor ✅, pity@90 per-banner ✅ (T2 FOR UPDATE + T3), §5.4 atomic algorithm ✅ (one tx, debit-first), §4.6 collection ✅ (T2 upsert; dupes count++ per decision #7), §4.7 pity table ✅, §6.2 banner list payload ✅ (T4), §6.3 pull result NEW/×N ✅ (PulledCard), §6.4 album+silhouette data ✅ (CollectionView owned flags), randomness server-side ✅ (injected randInt, prod math/rand/v2).
- **Edge cases pinned:** insufficient funds (no side effects), inactive/empty banner, pool missing tiers (redistribution + floor fallbacks), pity reset on ANY SSR incl. forced, dupes within one x10.
- **Known gaps (accepted):** sqlite can't exercise FOR UPDATE row-locking (Postgres-only) — concurrency of simultaneous pulls relies on the lock; covered by design + noted for the deferred testcontainers suite. Distribution test is loose by design (no flaky CI).
- **Type consistency:** `domain.Rarity` reused; `PulledCard.Count` = post-upsert count from AddToCollectionTx; reasons reuse Phase 1 consts `pull_x1`/`pull_x10`.
