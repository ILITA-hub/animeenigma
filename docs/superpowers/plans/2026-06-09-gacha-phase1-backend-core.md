# Лудка (Gacha) — Phase 1: Backend Core (Wallet + Ledger) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a new `services/gacha/` Go microservice (port 8093) that owns the «Энигмы» currency: a per-user wallet, an append-only ledger with idempotent credits, a one-time starter bonus, an internal credit endpoint, and a public balance endpoint — fully wired into docker-compose, the gateway, and the Makefile, with `make health` green.

**Architecture:** Mirrors the `services/notifications` service skeleton (chi router + GORM + `libs/*`). Balance is a denormalized field on `gacha_wallets`, kept in lockstep with an append-only `gacha_ledger` inside one DB transaction. Idempotency is enforced by a unique index `(user_id, reason, ref)` + `INSERT … ON CONFLICT DO NOTHING`, so a duplicate credit (same episode credited twice) is a silent no-op. Cards/banners/pulls are LATER phases — Phase 1 ships only the economy substrate.

**Tech Stack:** Go 1.24, chi/v5, GORM (Postgres), `libs/{database,cache,logger,errors,httputil,metrics,authz,tracing}`, Docker Compose, Prometheus metrics.

---

## Phase Series (context)

This is **Plan 1 of 5**. Each is independently shippable:

1. **Phase 1 — Backend core: wallet + ledger + credit** ← THIS PLAN
2. Phase 2 — Cards + Groups + Banners domain & admin API (gacha-cards MinIO bucket, CRUD)
3. Phase 3 — Pull engine (rarity weights, x10 SR-floor, per-banner hard pity) + collection storage
4. Phase 4 — Currency earning hooks (player episode-watched → internal credit; daily streak; theme/reaction optional)
5. Phase 5 — Frontend (navbar balance, `/gacha` banner list, spin screen, profile collection w/ silhouettes), behind `VITE_GACHA_ENABLED`

Design spec: `docs/superpowers/specs/2026-06-09-gacha-ludka-design.md`.

---

## File Structure (Phase 1)

**New service `services/gacha/`:**

| File | Responsibility |
|------|----------------|
| `go.mod` | Module + replace directives for `libs/*` |
| `Dockerfile` | Multi-stage build (mirror notifications) |
| `cmd/gacha-api/main.go` | Boot: config → DB → AutoMigrate → wire → HTTP server + graceful shutdown |
| `internal/config/config.go` | Env config (port 8093, DB, Redis, JWT, GACHA_* economy knobs) |
| `internal/config/config_test.go` | Config defaults + overrides |
| `internal/domain/wallet.go` | `Wallet` GORM model + `LedgerEntry` model + reason consts |
| `internal/repo/wallet.go` | Wallet get-or-create, balance read, atomic credit (tx: ledger insert + balance bump) |
| `internal/repo/wallet_test.go` | Idempotency, atomicity, starter-once (sqlite in-mem) |
| `internal/service/wallet.go` | `WalletService`: GetOrCreate (+ starter grant), Credit, Balance |
| `internal/service/wallet_test.go` | Starter-bonus-once, credit dedup, negative-guard |
| `internal/handler/wallet.go` | `GET /api/gacha/wallet` (JWT) |
| `internal/handler/internal.go` | `POST /internal/gacha/credit` (Docker-network-only) |
| `internal/transport/router.go` | chi router + `AuthMiddleware` (copied pattern) |

**Modified (wiring):**

| File | Change |
|------|--------|
| `go.work` | add `./services/gacha` to `use (...)` |
| `services/*/Dockerfile` (ALL service Dockerfiles) | add `COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/` |
| `docker/docker-compose.yml` | new `gacha` service block (port 8093) + `GACHA_SERVICE_URL` env on gateway |
| `services/gateway/internal/config/config.go` | add `GachaService` to `ServiceURLs` + env load |
| `services/gateway/internal/handler/proxy.go` | add `ProxyToGacha` |
| `services/gateway/internal/transport/router.go` | mount `/api/gacha/*` → `ProxyToGacha` (JWT-gated group) |
| `services/gateway/internal/service/proxy.go` (proxy map) | add `"gacha"` → `cfg.Services.GachaService` |
| `Makefile` | add `gacha` to `SERVICES :=`; add health-check line |

---

## Task 1: Scaffold module + register in workspace

**Files:**
- Create: `services/gacha/go.mod`
- Modify: `go.work`

- [ ] **Step 1: Create `services/gacha/go.mod`**

```
module github.com/ILITA-hub/animeenigma/services/gacha

go 1.24.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0-00010101000000-000000000000
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/prometheus/client_golang v1.23.2
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)
```

- [ ] **Step 2: Add gacha to `go.work`**

In `go.work`, inside the `use (...)` block, add a line (keep alphabetical-ish grouping near the other services):

```
	./services/gacha
```

- [ ] **Step 3: Sync workspace**

Run: `cd /data/animeenigma && go work sync`
Expected: no error (it resolves the new module; `go.work.sum` may update).

- [ ] **Step 4: Commit**

```bash
git add services/gacha/go.mod go.work go.work.sum
git commit -m "feat(gacha): scaffold gacha service module + register in go.work"
```

---

## Task 2: Config package

**Files:**
- Create: `services/gacha/internal/config/config.go`
- Test: `services/gacha/internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

`services/gacha/internal/config/config_test.go`:

```go
package config

import (
	"os"
	"testing"
)

func TestLoad_RequiresJWTSecret(t *testing.T) {
	os.Unsetenv("JWT_SECRET")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when JWT_SECRET is unset")
	}
}

func TestLoad_Defaults(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	defer os.Unsetenv("JWT_SECRET")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.Port != 8093 {
		t.Errorf("port = %d; want 8093", cfg.Server.Port)
	}
	if cfg.Database.Database != "animeenigma" {
		t.Errorf("db = %q; want animeenigma", cfg.Database.Database)
	}
	if cfg.Economy.StarterBonus != 300 {
		t.Errorf("starter = %d; want 300", cfg.Economy.StarterBonus)
	}
	if !cfg.Enabled {
		t.Error("Enabled default = false; want true")
	}
}

func TestLoad_StarterBonusOverride(t *testing.T) {
	os.Setenv("JWT_SECRET", "x")
	os.Setenv("GACHA_STARTER_BONUS", "500")
	defer func() { os.Unsetenv("JWT_SECRET"); os.Unsetenv("GACHA_STARTER_BONUS") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Economy.StarterBonus != 500 {
		t.Errorf("starter = %d; want 500", cfg.Economy.StarterBonus)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/gacha && go test ./internal/config/ -run TestLoad -v`
Expected: FAIL — package/`Load` undefined (compile error).

- [ ] **Step 3: Write `services/gacha/internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

// Config is the gacha service configuration.
type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
	Economy  EconomyConfig

	// Enabled is the backend dark-ship toggle (GACHA_ENABLED). When false,
	// the internal credit endpoint no-ops with 200 (so producers don't
	// error) and the service still boots. Frontend is gated separately by
	// VITE_GACHA_ENABLED. Default true.
	Enabled bool
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

// EconomyConfig holds tunable currency knobs (spec §5). Numeric balance is
// in whole «Энигмы».
type EconomyConfig struct {
	StarterBonus int64 // one-time grant on first wallet access (default 300)
}

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			// Port 8093: next free after analytics:8092 (8087 maintenance,
			// 8089 library, 8090 notifications, 8091 watch-together).
			Port: getEnvInt("SERVER_PORT", 8093),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		Economy: EconomyConfig{
			StarterBonus: int64(getEnvInt("GACHA_STARTER_BONUS", 300)),
		},
		Enabled: getEnvBool("GACHA_ENABLED", true),
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "t", "true", "y", "yes", "on":
		return true
	case "0", "f", "false", "n", "no", "off":
		return false
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/gacha && go test ./internal/config/ -run TestLoad -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/gacha/internal/config/
git commit -m "feat(gacha): config package (port 8093, economy knobs, GACHA_ENABLED)"
```

---

## Task 3: Domain models

**Files:**
- Create: `services/gacha/internal/domain/wallet.go`

- [ ] **Step 1: Write `services/gacha/internal/domain/wallet.go`**

```go
// Package domain holds the gacha service's persisted models and value types.
package domain

import "time"

// Credit/debit reasons recorded on every LedgerEntry. The (user_id, reason,
// ref) unique index uses these — keep them stable strings.
const (
	ReasonStarter        = "starter"
	ReasonEpisodeWatched = "episode_watched"
	ReasonDaily          = "daily"
	ReasonTitleCompleted = "title_completed"
	ReasonPullX1         = "pull_x1"
	ReasonPullX10        = "pull_x10"
)

// Wallet is one row per user. Balance is denormalized from the ledger and
// updated in the same transaction as each LedgerEntry insert.
type Wallet struct {
	UserID         string    `gorm:"type:uuid;primaryKey" json:"user_id"`
	Balance        int64     `gorm:"not null;default:0" json:"balance"`
	StarterGranted bool      `gorm:"not null;default:false" json:"starter_granted"`
	DailyStreak    int       `gorm:"not null;default:0" json:"daily_streak"`
	LastDailyAt    *time.Time `json:"last_daily_at,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Wallet) TableName() string { return "gacha_wallets" }

// LedgerEntry is the append-only source of truth for every balance change.
// Delta is positive for credits, negative for debits. Ref is an optional
// idempotency discriminator (e.g. "<anime_id>:<episode>"); when non-empty,
// the (UserID, Reason, Ref) unique index makes a duplicate insert a no-op.
type LedgerEntry struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:uuid;not null;index:idx_ledger_user_created" json:"user_id"`
	Delta     int64     `gorm:"not null" json:"delta"`
	Reason    string    `gorm:"size:32;not null" json:"reason"`
	Ref       string    `gorm:"size:128;not null;default:''" json:"ref"`
	CreatedAt time.Time `gorm:"index:idx_ledger_user_created" json:"created_at"`
}

func (LedgerEntry) TableName() string { return "gacha_ledger" }
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/gacha && go build ./internal/domain/`
Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add services/gacha/internal/domain/
git commit -m "feat(gacha): wallet + ledger domain models and reason constants"
```

---

## Task 4: Wallet repository (idempotent atomic credit)

**Files:**
- Create: `services/gacha/internal/repo/wallet.go`
- Test: `services/gacha/internal/repo/wallet_test.go`

> **Idempotency mechanism:** the partial unique index `(user_id, reason, ref)
> WHERE ref <> ''` is created in `EnsureIndexes` (Task 8 wires it at boot).
> In tests we create the same index on sqlite so `ON CONFLICT DO NOTHING`
> behaves identically.

- [ ] **Step 1: Write the failing test**

`services/gacha/internal/repo/wallet_test.go`:

```go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.Wallet{}, &domain.LedgerEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Mirror the production partial unique index used for credit idempotency.
	if err := db.Exec(
		`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	).Error; err != nil {
		t.Fatalf("index: %v", err)
	}
	return db
}

const testUser = "11111111-1111-1111-1111-111111111111"

func TestCredit_IncrementsBalanceAndWritesLedger(t *testing.T) {
	db := newTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()

	if _, err := r.GetOrCreate(ctx, testUser); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	applied, err := r.Credit(ctx, testUser, 22, domain.ReasonEpisodeWatched, "anime-1:1")
	if err != nil {
		t.Fatalf("Credit: %v", err)
	}
	if !applied {
		t.Fatal("first credit should be applied")
	}
	w, _ := r.GetOrCreate(ctx, testUser)
	if w.Balance != 22 {
		t.Errorf("balance = %d; want 22", w.Balance)
	}
}

func TestCredit_DuplicateRefIsNoop(t *testing.T) {
	db := newTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()
	r.GetOrCreate(ctx, testUser)

	if _, err := r.Credit(ctx, testUser, 22, domain.ReasonEpisodeWatched, "anime-1:1"); err != nil {
		t.Fatalf("credit 1: %v", err)
	}
	applied, err := r.Credit(ctx, testUser, 22, domain.ReasonEpisodeWatched, "anime-1:1")
	if err != nil {
		t.Fatalf("credit 2: %v", err)
	}
	if applied {
		t.Fatal("duplicate (user,reason,ref) credit must NOT be applied")
	}
	w, _ := r.GetOrCreate(ctx, testUser)
	if w.Balance != 22 {
		t.Errorf("balance = %d; want 22 (no double credit)", w.Balance)
	}
}

func TestCredit_EmptyRefAlwaysApplies(t *testing.T) {
	db := newTestDB(t)
	r := NewWalletRepository(db)
	ctx := context.Background()
	r.GetOrCreate(ctx, testUser)

	// Empty ref is exempt from the partial index → both apply.
	r.Credit(ctx, testUser, 50, domain.ReasonDaily, "")
	r.Credit(ctx, testUser, 50, domain.ReasonDaily, "")
	w, _ := r.GetOrCreate(ctx, testUser)
	if w.Balance != 100 {
		t.Errorf("balance = %d; want 100 (empty ref not deduped)", w.Balance)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/gacha && go test ./internal/repo/ -v`
Expected: FAIL — `NewWalletRepository` / `Credit` undefined.

- [ ] **Step 3: Write `services/gacha/internal/repo/wallet.go`**

```go
package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WalletRepository wraps gacha_wallets + gacha_ledger access.
type WalletRepository struct {
	db *gorm.DB
}

func NewWalletRepository(db *gorm.DB) *WalletRepository { return &WalletRepository{db: db} }

// GetOrCreate returns the user's wallet, inserting a zero-balance row on
// first access. Concurrent first-access is safe: ON CONFLICT DO NOTHING on
// the PK, then re-read.
func (r *WalletRepository) GetOrCreate(ctx context.Context, userID string) (*domain.Wallet, error) {
	w := domain.Wallet{UserID: userID}
	if err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&w).Error; err != nil {
		return nil, err
	}
	var out domain.Wallet
	if err := r.db.WithContext(ctx).First(&out, "user_id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// Credit atomically appends a ledger entry and bumps the wallet balance by
// delta. Returns applied=false (no error) when a non-empty ref collides with
// an existing (user,reason,ref) row — the idempotent no-op path. The ledger
// insert and balance update share one transaction so they can never diverge.
func (r *WalletRepository) Credit(
	ctx context.Context, userID string, delta int64, reason, ref string,
) (applied bool, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		entry := domain.LedgerEntry{UserID: userID, Delta: delta, Reason: reason, Ref: ref}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entry)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			// Duplicate (user,reason,ref) — idempotent no-op.
			return nil
		}
		applied = true
		return tx.Model(&domain.Wallet{}).
			Where("user_id = ?", userID).
			UpdateColumn("balance", gorm.Expr("balance + ?", delta)).Error
	})
	return applied, err
}

// MarkStarterGranted flips starter_granted to true and returns whether THIS
// call did the flip (false if it was already true). Used to grant the
// starter bonus exactly once. Atomic compare-and-set via WHERE guard.
func (r *WalletRepository) MarkStarterGranted(ctx context.Context, userID string) (didGrant bool, err error) {
	res := r.db.WithContext(ctx).Model(&domain.Wallet{}).
		Where("user_id = ? AND starter_granted = ?", userID, false).
		Update("starter_granted", true)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/gacha && go test ./internal/repo/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/gacha/internal/repo/
git commit -m "feat(gacha): wallet repo with idempotent atomic credit + starter CAS"
```

---

## Task 5: Wallet service (starter-bonus-once, credit gating)

**Files:**
- Create: `services/gacha/internal/service/wallet.go`
- Test: `services/gacha/internal/service/wallet_test.go`

- [ ] **Step 1: Write the failing test**

`services/gacha/internal/service/wallet_test.go`:

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSvc(t *testing.T) *WalletService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.Wallet{}, &domain.LedgerEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db.Exec(`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`)
	return NewWalletService(repo.NewWalletRepository(db), 300, true, logger.Default())
}

const u = "22222222-2222-2222-2222-222222222222"

func TestGetOrCreate_GrantsStarterOnce(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()

	w1, err := s.GetOrCreate(ctx, u)
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if w1.Balance != 300 {
		t.Errorf("balance = %d; want 300 (starter)", w1.Balance)
	}
	w2, _ := s.GetOrCreate(ctx, u)
	if w2.Balance != 300 {
		t.Errorf("balance = %d; want 300 (starter NOT granted twice)", w2.Balance)
	}
}

func TestCredit_AddsAndDedups(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	s.GetOrCreate(ctx, u) // balance 300

	s.Credit(ctx, u, 22, domain.ReasonEpisodeWatched, "a:1")
	s.Credit(ctx, u, 22, domain.ReasonEpisodeWatched, "a:1") // dup
	w, _ := s.GetOrCreate(ctx, u)
	if w.Balance != 322 {
		t.Errorf("balance = %d; want 322 (300 + one 22)", w.Balance)
	}
}

func TestCredit_RejectsNonPositive(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	s.GetOrCreate(ctx, u)
	if _, err := s.Credit(ctx, u, 0, domain.ReasonDaily, ""); err == nil {
		t.Fatal("expected error for non-positive credit")
	}
}

func TestCredit_DisabledServiceNoops(t *testing.T) {
	s := newSvc(t)
	s.enabled = false
	ctx := context.Background()
	s.GetOrCreate(ctx, u) // starter still 0 because disabled? see note
	applied, err := s.Credit(ctx, u, 22, domain.ReasonEpisodeWatched, "a:1")
	if err != nil {
		t.Fatalf("disabled credit err: %v", err)
	}
	if applied {
		t.Fatal("disabled service must not apply credits")
	}
}
```

> Note: with `enabled=false`, `GetOrCreate` still creates the wallet row but
> skips the starter grant, and `Credit` is a no-op returning `(false, nil)`.
> This is the backend dark-ship path — producers calling `/internal/gacha/credit`
> get a clean 200 and nothing changes.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/gacha && go test ./internal/service/ -v`
Expected: FAIL — `NewWalletService` undefined.

- [ ] **Step 3: Write `services/gacha/internal/service/wallet.go`**

```go
package service

import (
	"context"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
)

// WalletService is the economy use-case layer over WalletRepository.
type WalletService struct {
	repo         *repo.WalletRepository
	starterBonus int64
	enabled      bool
	log          *logger.Logger
}

func NewWalletService(r *repo.WalletRepository, starterBonus int64, enabled bool, log *logger.Logger) *WalletService {
	return &WalletService{repo: r, starterBonus: starterBonus, enabled: enabled, log: log}
}

// GetOrCreate returns the user's wallet, granting the one-time starter bonus
// on first creation (only when the service is enabled). The starter grant is
// guarded by an atomic compare-and-set on starter_granted, so two concurrent
// first-accesses grant exactly once.
func (s *WalletService) GetOrCreate(ctx context.Context, userID string) (*domain.Wallet, error) {
	w, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.enabled && !w.StarterGranted && s.starterBonus > 0 {
		didGrant, err := s.repo.MarkStarterGranted(ctx, userID)
		if err != nil {
			return nil, err
		}
		if didGrant {
			if _, err := s.repo.Credit(ctx, userID, s.starterBonus, domain.ReasonStarter, "starter"); err != nil {
				return nil, err
			}
			s.log.Infow("granted gacha starter bonus", "user_id", userID, "amount", s.starterBonus)
		}
		// Re-read so the returned wallet reflects the grant.
		return s.repo.GetOrCreate(ctx, userID)
	}
	return w, nil
}

// Credit adds delta «Энигмы» to a user's wallet under (reason, ref). Returns
// applied=false when the credit was a deduped no-op or the service is
// disabled. delta must be positive (spend paths use a separate debit method
// in a later phase).
func (s *WalletService) Credit(ctx context.Context, userID string, delta int64, reason, ref string) (bool, error) {
	if delta <= 0 {
		return false, apperrors.BadRequest("credit amount must be positive")
	}
	if !s.enabled {
		return false, nil
	}
	// Ensure the wallet row exists before crediting (no starter side-effects
	// on the hot credit path — GetOrCreate at repo level, not service level).
	if _, err := s.repo.GetOrCreate(ctx, userID); err != nil {
		return false, err
	}
	return s.repo.Credit(ctx, userID, delta, reason, ref)
}
```

> **Verify `apperrors.BadRequest` exists** — Run: `grep -rn "func BadRequest" libs/errors/`. If the constructor has a different name (e.g. `Invalid`/`InvalidArgument`), use that name instead and update the import usage. (Spec/CLAUDE.md show `errors.NotFound`/`errors.Wrap`; confirm the 400 helper before relying on the exact name.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/gacha && go test ./internal/service/ -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add services/gacha/internal/service/
git commit -m "feat(gacha): wallet service with one-time starter bonus + credit gating"
```

---

## Task 6: HTTP handlers (public wallet + internal credit)

**Files:**
- Create: `services/gacha/internal/handler/wallet.go`
- Create: `services/gacha/internal/handler/internal.go`

- [ ] **Step 1: Write `services/gacha/internal/handler/wallet.go`**

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
)

// WalletHandler serves the authenticated wallet endpoint.
type WalletHandler struct {
	svc *service.WalletService
	log *logger.Logger
}

func NewWalletHandler(svc *service.WalletService, log *logger.Logger) *WalletHandler {
	return &WalletHandler{svc: svc, log: log}
}

// GetWallet handles GET /api/gacha/wallet. Returns the caller's wallet,
// creating it (and granting the starter bonus) on first access. User identity
// comes from the JWT claims the AuthMiddleware put on the context.
func (h *WalletHandler) GetWallet(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims.UserID == "" {
		httputil.Unauthorized(w)
		return
	}
	wallet, err := h.svc.GetOrCreate(r.Context(), claims.UserID)
	if err != nil {
		h.log.Errorw("get wallet failed", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, wallet)
}
```

> **Verify claims accessor** — Run: `grep -rn "func ClaimsFromContext\|UserID" libs/authz/*.go | head`. The router template uses `authz.ContextWithClaims`; confirm the read-side accessor name (`ClaimsFromContext`) and the claims field that holds the user UUID (`UserID` vs `Subject`/`Sub`). Adjust both lines if the real names differ.

- [ ] **Step 2: Write `services/gacha/internal/handler/internal.go`**

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
)

// InternalHandler serves /internal/gacha/* — Docker-network-only (the gateway
// never proxies /internal/*). Producer services (player, themes, …) credit
// «Энигмы» here. No auth middleware, same model as notifications' internal
// handler (D-05).
type InternalHandler struct {
	svc *service.WalletService
	log *logger.Logger
}

func NewInternalHandler(svc *service.WalletService, log *logger.Logger) *InternalHandler {
	return &InternalHandler{svc: svc, log: log}
}

// CreditRequest is the body of POST /internal/gacha/credit.
type CreditRequest struct {
	UserID string `json:"user_id"`
	Amount int64  `json:"amount"`
	Reason string `json:"reason"`
	Ref    string `json:"ref"`
}

// Credit handles POST /internal/gacha/credit. Idempotent on (user_id, reason,
// ref). Returns { "applied": bool } — applied=false means a duplicate ref or
// disabled service, which producers treat as success (non-fatal).
func (h *InternalHandler) Credit(w http.ResponseWriter, r *http.Request) {
	var req CreditRequest
	if err := httputil.Bind(r, &req); err != nil {
		httputil.Error(w, err)
		return
	}
	if req.UserID == "" || req.Reason == "" {
		httputil.BadRequest(w, "user_id and reason are required")
		return
	}
	applied, err := h.svc.Credit(r.Context(), req.UserID, req.Amount, req.Reason, req.Ref)
	if err != nil {
		h.log.Errorw("internal credit failed",
			"user_id", req.UserID, "reason", req.Reason, "ref", req.Ref, "error", err)
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"applied": applied})
}

// Health handles GET /internal/health.
func (h *InternalHandler) Health(w http.ResponseWriter, _ *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok"})
}
```

> **Verify httputil helpers** — Run: `grep -rn "func OK\|func Error\|func Bind\|func BadRequest\|func Unauthorized" libs/httputil/*.go`. The router/handler templates above use `OK`, `Error`, `Bind`, `Unauthorized`; confirm `BadRequest(w, msg)` signature (it may be `httputil.Error(w, apperrors.BadRequest(msg))` instead). Adjust the one call if needed.

- [ ] **Step 3: Verify it compiles**

Run: `cd services/gacha && go build ./internal/handler/`
Expected: success (fix any helper-name mismatches flagged above).

- [ ] **Step 4: Commit**

```bash
git add services/gacha/internal/handler/
git commit -m "feat(gacha): public wallet handler + internal idempotent credit handler"
```

---

## Task 7: Router

**Files:**
- Create: `services/gacha/internal/transport/router.go`

- [ ] **Step 1: Write `services/gacha/internal/transport/router.go`**

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the chi router for the gacha service.
//
//	GET  /health                  (public)
//	GET  /metrics                 (public, prom)
//	POST /internal/gacha/credit   (internal — gateway never proxies)
//	GET  /internal/health         (internal)
//	GET  /api/gacha/wallet        (JWT)
func NewRouter(
	walletHandler *handler.WalletHandler,
	internalHandler *handler.InternalHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, req *http.Request) {
		metrics.Handler().ServeHTTP(w, req)
	})

	// Internal — no middleware, Docker-network-only.
	r.Post("/internal/gacha/credit", internalHandler.Credit)
	r.Get("/internal/health", internalHandler.Health)

	// Public — JWT-gated.
	r.Route("/api/gacha", func(r chi.Router) {
		r.Use(AuthMiddleware(jwtConfig))
		r.Get("/wallet", walletHandler.GetWallet)
	})

	return r
}

// AuthMiddleware validates the JWT access token and puts claims on the
// context. Copied from services/notifications/internal/transport/router.go
// (project convention — every service re-validates the gateway's JWT).
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	jwtManager := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}
			claims, err := jwtManager.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}
			ctx := authz.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/gacha && go build ./internal/transport/`
Expected: success.

- [ ] **Step 3: Commit**

```bash
git add services/gacha/internal/transport/
git commit -m "feat(gacha): chi router (wallet + internal credit + JWT middleware)"
```

---

## Task 8: main.go boot + index creation

**Files:**
- Create: `services/gacha/cmd/gacha-api/main.go`

- [ ] **Step 1: Write `services/gacha/cmd/gacha-api/main.go`**

```go
// Package main is the gacha service entrypoint (port 8093) — owns the
// «Энигмы» economy: wallets + an idempotent ledger. Cards/banners/pulls
// arrive in later phases. Mirrors services/notifications boot sequence.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/service"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "gacha")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := db.AutoMigrate(&domain.Wallet{}, &domain.LedgerEntry{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Partial unique index for credit idempotency — GORM can't express the
	// WHERE clause, so create it explicitly. IF NOT EXISTS = no-op on reboot.
	if err := db.DB.Exec(
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_gacha_ledger_dedup
		 ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	).Error; err != nil {
		log.Fatalw("failed to create ledger dedup index", "error", err)
	}

	walletRepo := repo.NewWalletRepository(db.DB)
	walletSvc := service.NewWalletService(walletRepo, cfg.Economy.StarterBonus, cfg.Enabled, log)

	walletHandler := handler.NewWalletHandler(walletSvc, log)
	internalHandler := handler.NewInternalHandler(walletSvc, log)

	metricsCollector := metrics.NewCollector("gacha")
	router := transport.NewRouter(walletHandler, internalHandler, cfg.JWT, log, metricsCollector)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("gacha")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting gacha service",
			"address", cfg.Server.Address(),
			"db_name", cfg.Database.Database,
			"enabled", cfg.Enabled,
		)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down gacha service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("gacha service stopped")
}
```

- [ ] **Step 2: Build the whole service**

Run: `cd services/gacha && go build ./...`
Expected: success. Fix any helper-name mismatches surfaced by the `grep` verification notes in Tasks 5–6.

- [ ] **Step 3: Run all gacha unit tests**

Run: `cd services/gacha && go test ./... -count=1`
Expected: PASS (config, repo, service).

- [ ] **Step 4: Commit**

```bash
git add services/gacha/cmd/
git commit -m "feat(gacha): service entrypoint — boot, AutoMigrate, dedup index, HTTP server"
```

---

## Task 9: Dockerfile

**Files:**
- Create: `services/gacha/Dockerfile`

- [ ] **Step 1: Write `services/gacha/Dockerfile`** (mirror notifications; copies the full workspace module set)

```dockerfile
# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Copy go.work and all module files
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum* ./libs/logger/
COPY libs/tracing/go.mod libs/tracing/go.sum* ./libs/tracing/
COPY libs/errors/go.mod libs/errors/go.sum* ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum* ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum* ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum* ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum* ./libs/httputil/
COPY libs/pagination/go.mod libs/pagination/go.sum* ./libs/pagination/
COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
COPY libs/animeparser/go.mod libs/animeparser/go.sum* ./libs/animeparser/
COPY libs/videoutils/go.mod libs/videoutils/go.sum* ./libs/videoutils/
COPY libs/idmapping/go.mod libs/idmapping/go.sum* ./libs/idmapping/
COPY libs/kodikextract/go.mod libs/kodikextract/go.sum* ./libs/kodikextract/
COPY libs/metrics/go.mod libs/metrics/go.sum* ./libs/metrics/
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
COPY services/themes/go.mod services/themes/go.sum* ./services/themes/
COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/
COPY services/watch-together/go.mod services/watch-together/go.sum* ./services/watch-together/
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/
COPY services/maintenance/go.mod services/maintenance/go.sum* ./services/maintenance/
COPY services/library/go.mod services/library/go.sum* ./services/library/
COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/

RUN cd services/gacha && go mod download

# Copy source
COPY libs/ ./libs/
COPY services/gacha/ ./services/gacha/

# Build
RUN cd services/gacha && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gacha-api ./cmd/gacha-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /gacha-api .

EXPOSE 8093

CMD ["./gacha-api"]
```

- [ ] **Step 2: Commit**

```bash
git add services/gacha/Dockerfile
git commit -m "feat(gacha): Dockerfile (multi-stage, workspace module set)"
```

---

## Task 10: Add gacha go.mod COPY to EVERY service Dockerfile

> **Why this is mandatory:** every service Dockerfile copies the root `go.work`,
> which now `use`s `./services/gacha`. `go mod download` validates ALL workspace
> modules exist, so a Dockerfile missing the gacha `go.mod` COPY fails to build
> that service ("cannot load module … listed in go.work file"). This is the
> same gotcha documented for new `libs/` modules — it applies to new services too.

**Files (modify each):** `services/auth/Dockerfile`, `services/catalog/Dockerfile`, `services/streaming/Dockerfile`, `services/player/Dockerfile`, `services/rooms/Dockerfile`, `services/scraper/Dockerfile`, `services/scheduler/Dockerfile`, `services/gateway/Dockerfile`, `services/themes/Dockerfile`, `services/notifications/Dockerfile`, `services/watch-together/Dockerfile`, `services/analytics/Dockerfile`, `services/maintenance/Dockerfile`, `services/library/Dockerfile`.

- [ ] **Step 1: Find every Dockerfile that copies the workspace service set**

Run: `grep -rln "services/library/go.mod" services/*/Dockerfile`
Expected: a list of the service Dockerfiles (the ones participating in the workspace build).

- [ ] **Step 2: Add the gacha COPY line after the library COPY in each**

For every file listed, add this line immediately after the `services/library/go.mod` COPY line:

```dockerfile
COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/
```

(The `services/gacha/Dockerfile` already has it from Task 9 — skip it.) Sidecars without go.work (`services/animepahe-resolver`, `services/megacloud-extractor`) are exempt — they don't copy `go.work`.

- [ ] **Step 3: Verify all got it**

Run: `grep -rln "services/gacha/go.mod" services/*/Dockerfile | wc -l`
Expected: count equals the Task-10 file list **plus** the gacha Dockerfile itself.

- [ ] **Step 4: Commit**

```bash
git add services/*/Dockerfile
git commit -m "build(gacha): add gacha go.mod COPY to all workspace service Dockerfiles"
```

---

## Task 11: Wire docker-compose

**Files:**
- Modify: `docker/docker-compose.yml`

- [ ] **Step 1: Add the gacha service block**

Insert after the `watch-together` service block (keep service blocks grouped):

```yaml
  # workstream gacha (Лудка), Phase 1 — «Энигмы» economy substrate on port
  # 8093 (next free after analytics:8092). Wallet + idempotent ledger only;
  # cards/banners/pulls land in later phases. GACHA_ENABLED=false dark-ships
  # the backend (credit endpoint no-ops, service still boots).
  gacha:
    build:
      context: ..
      dockerfile: services/gacha/Dockerfile
    container_name: animeenigma-gacha
    restart: unless-stopped
    environment:
      SERVER_PORT: 8093
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: postgres
      DB_PASSWORD: postgres
      DB_NAME: animeenigma
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      REDIS_HOST: redis
      GACHA_ENABLED: "true"
      GACHA_STARTER_BONUS: "300"
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8093:8093"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8093/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

- [ ] **Step 2: Add `GACHA_SERVICE_URL` to the gateway service env**

In the `gateway` service's `environment:` block (near `NOTIFICATIONS_SERVICE_URL`/`WATCH_TOGETHER_SERVICE_URL`), add:

```yaml
      GACHA_SERVICE_URL: http://gacha:8093
```

- [ ] **Step 3: Validate compose syntax**

Run: `docker compose -f docker/docker-compose.yml config >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 4: Commit**

```bash
git add docker/docker-compose.yml
git commit -m "build(gacha): docker-compose service block (8093) + gateway GACHA_SERVICE_URL"
```

---

## Task 12: Wire gateway routing

**Files:**
- Modify: `services/gateway/internal/config/config.go`
- Modify: `services/gateway/internal/handler/proxy.go`
- Modify: `services/gateway/internal/transport/router.go`
- Modify: `services/gateway/internal/service/proxy.go` (the name→URL resolver)

- [ ] **Step 1: Add `GachaService` to `ServiceURLs` + load env**

In `services/gateway/internal/config/config.go`, add to the `ServiceURLs` struct (after `AnalyticsService`/`WatchTogetherService`):

```go
	// GachaService — workstream gacha (Лудка), Phase 1. Port 8093.
	// Exposes /api/gacha/* (JWT). /internal/gacha/* is Docker-network-only
	// and never routed here.
	GachaService string
```

And in the `Services: ServiceURLs{ ... }` literal in `Load()`:

```go
			GachaService: getEnv("GACHA_SERVICE_URL", "http://gacha:8093"),
```

- [ ] **Step 2: Find how the proxy resolves the service name → URL**

Run: `grep -rn "\"notifications\"\|NotificationsService\|case \"" services/gateway/internal/service/proxy.go`
Expected: shows the map/switch from short name (e.g. `"notifications"`) to `cfg.Services.NotificationsService`. Add the analogous `"gacha"` → `cfg.Services.GachaService` entry there, mirroring the notifications line exactly.

- [ ] **Step 3: Add `ProxyToGacha` handler**

In `services/gateway/internal/handler/proxy.go`, after `ProxyToNotifications`:

```go
// ProxyToGacha proxies requests to the gacha service (workstream gacha /
// Лудка, Phase 1). Only /api/gacha/* is exposed; /internal/gacha/* is
// reachable solely inside the Docker network (the gateway never registers a
// route under /internal/* for it).
func (h *ProxyHandler) ProxyToGacha(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "gacha")
}
```

- [ ] **Step 4: Mount the `/api/gacha/*` route group (JWT-gated)**

In `services/gateway/internal/transport/router.go`, near the notifications route group, add a JWT-protected group. (Find the notifications group — `grep -n "Route(\"/notifications\"" services/gateway/internal/transport/router.go` — and mirror its auth middleware usage.)

```go
		// Gacha (Лудка) routes — workstream gacha, Phase 1. All JWT-gated:
		// the лудка is logged-in-only (guests are blocked at the API). The
		// internal credit endpoint (/internal/gacha/credit) is NOT registered
		// here — Docker-network-only.
		r.Route("/gacha", func(r chi.Router) {
			r.Use(<AUTH_MIDDLEWARE>) // same middleware the /notifications group uses
			r.Get("/wallet", proxyHandler.ProxyToGacha)
		})
```

Replace `<AUTH_MIDDLEWARE>` with the exact JWT middleware reference the existing `/notifications` group uses (read those lines first so the symbol matches — e.g. `AuthMiddleware(cfg.JWT)` or a shared `authMW`).

- [ ] **Step 5: Build the gateway**

Run: `cd services/gateway && go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add services/gateway/
git commit -m "feat(gacha): gateway routing — /api/gacha/* → gacha:8093 (JWT-gated)"
```

---

## Task 13: Makefile + health wiring

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Add `gacha` to the `SERVICES` list**

In `Makefile`, find the line starting `SERVICES :=` and append `gacha`:

```make
SERVICES := auth catalog streaming player rooms scheduler gateway themes scraper library notifications watch-together gacha
```

This gives `make redeploy-gacha` and `make logs-gacha` via the generic `redeploy-%` / `logs-%` targets.

- [ ] **Step 2: Add a health-check line**

In the `health` target (find: `grep -n "notifications:8090" Makefile`), add after the notifications line:

```make
	@curl -sf http://localhost:8093/health > /dev/null && echo "✓ gacha:8093" || echo "✗ gacha:8093"
```

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build(gacha): Makefile SERVICES entry + health-check line"
```

---

## Task 14: Build, deploy, verify end-to-end

- [ ] **Step 1: Workspace-wide build sanity**

Run: `cd /data/animeenigma && go build ./services/gacha/... && go build ./services/gateway/...`
Expected: success.

- [ ] **Step 2: Deploy the new service**

Run: `make redeploy-gacha`
Expected: image builds, container `animeenigma-gacha` starts. (First build is slow — full module download.)

- [ ] **Step 3: Redeploy gateway (picks up routing + env)**

Run: `make redeploy-gateway`
Expected: gateway rebuilds and restarts.

- [ ] **Step 4: Health check**

Run: `make health`
Expected: includes `✓ gacha:8093`.

- [ ] **Step 5: Smoke the internal credit endpoint (Docker-network-only)**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T gacha \
  wget -qO- --post-data='{"user_id":"33333333-3333-3333-3333-333333333333","amount":22,"reason":"episode_watched","ref":"smoke:1"}' \
  --header='Content-Type: application/json' \
  http://localhost:8093/internal/gacha/credit
```
Expected: `{"applied":true}`. Run the SAME command again → `{"applied":false}` (idempotent dedup proven live).

- [ ] **Step 6: Confirm the wallet row + balance in Postgres**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -c \
  "SELECT user_id, balance, starter_granted FROM gacha_wallets WHERE user_id='33333333-3333-3333-3333-333333333333';"
```
Expected: one row, `balance = 22` (the internal credit path does NOT grant the starter bonus — that's the `GetOrCreate` service path used by the JWT wallet endpoint, exercised in Phase 5).

- [ ] **Step 7: Verify the gateway blocks the wallet endpoint without a JWT**

Run: `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8000/api/gacha/wallet`
Expected: `401`.

- [ ] **Step 8: Final commit (if any wiring tweaks were needed)**

```bash
git add -A services/gacha docker Makefile services/gateway
git commit -m "build(gacha): Phase 1 deploy verification + wiring fixups" || echo "nothing to commit"
```

> **Note on `/animeenigma-after-update`:** Phase 1 ships no user-facing UI (dark
> backend substrate), so the changelog entry is optional here — defer the
> Russian-Trump-mode changelog to Phase 5 when the лудка becomes visible. Still
> push after committing (realtime-backup convention).

---

## Self-Review

**Spec coverage (Phase 1 scope only):**
- ✅ New `services/gacha/` service, port 8093 — Tasks 1–9.
- ✅ `gacha_wallets` + `gacha_ledger` tables (spec §4.4, §4.5) — Task 3, migrated Task 8.
- ✅ Idempotent credit via `(user_id, reason, ref)` unique index — Tasks 4, 8; proven live Task 14 Step 5.
- ✅ One-time starter bonus (spec §5.2, decision #18 cold-start) — Task 5.
- ✅ Internal credit endpoint (spec §3.3, §8) — Task 6, router Task 7.
- ✅ Public wallet endpoint, JWT-gated / logged-in-only (decision #19) — Tasks 6, 7, 12.
- ✅ Backend dark-ship flag `GACHA_ENABLED` (decision #20) — Task 2, honored in service Task 5.
- ✅ Gateway routing, compose, Makefile, all-Dockerfiles gotcha — Tasks 10–13.
- ⏭ Cards / groups / banners / pulls / collection / earning-hooks / frontend — explicitly DEFERRED to Phases 2–5 (see Phase Series).

**Placeholder scan:** The four `<...>`-style spots are *deliberate verification gates* (helper/middleware symbol names that must be confirmed against the real `libs/` + gateway code before use), each paired with an exact `grep` command and a fallback. They are not lazy placeholders — they exist because inventing a wrong symbol name would compile-fail, and the real names live in code this plan shouldn't blind-guess. Resolve each at its step.

**Type consistency:** `WalletRepository.Credit(ctx, userID, delta, reason, ref) (bool, error)` is used identically in service + tests. `WalletService.GetOrCreate`/`Credit` signatures match handler call sites. `CreditRequest` JSON fields (`user_id`/`amount`/`reason`/`ref`) match the live smoke payload in Task 14 and the future player producer (Phase 4). `domain.Reason*` constants are the single source for reason strings across repo/service/handler.

**Known follow-ups for Phase 2+:** debit path (spending on pulls) is intentionally absent — pulls in Phase 3 add a transactional `Debit` that checks balance ≥ cost. The wallet's `daily_streak`/`last_daily_at` columns exist now but are only populated in Phase 4 (daily-claim).
