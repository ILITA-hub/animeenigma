package service

import (
	"context"
	"testing"
	"time"

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
	// Use raw SQL for SQLite compatibility (gen_random_uuid() is Postgres-only).
	stmts := []string{
		`CREATE TABLE gacha_wallets (
			user_id TEXT PRIMARY KEY,
			balance INTEGER NOT NULL DEFAULT 0,
			starter_granted INTEGER NOT NULL DEFAULT 0,
			daily_streak INTEGER NOT NULL DEFAULT 0,
			last_daily_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE gacha_ledger (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			delta INTEGER NOT NULL,
			reason TEXT NOT NULL,
			ref TEXT NOT NULL DEFAULT '',
			created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return NewWalletService(repo.NewWalletRepository(db), 300, 50, 10, 100, true, logger.Default())
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
	s.GetOrCreate(ctx, u) // no starter granted — service is disabled
	applied, err := s.Credit(ctx, u, 22, domain.ReasonEpisodeWatched, "a:1")
	if err != nil {
		t.Fatalf("disabled credit err: %v", err)
	}
	if applied {
		t.Fatal("disabled service must not apply credits")
	}
}

// --- Daily claim tests (Task 1, Phase 4) ---

// day0 is a fixed anchor date used across daily tests.
var day0 = time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

// newSvcNoStarter returns a WalletService with starterBonus=0 so daily tests
// can reason about balances without accounting for the starter grant.
func newSvcNoStarter(t *testing.T) *WalletService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	stmts := []string{
		`CREATE TABLE gacha_wallets (
			user_id TEXT PRIMARY KEY,
			balance INTEGER NOT NULL DEFAULT 0,
			starter_granted INTEGER NOT NULL DEFAULT 0,
			daily_streak INTEGER NOT NULL DEFAULT 0,
			last_daily_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE TABLE gacha_ledger (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			delta INTEGER NOT NULL,
			reason TEXT NOT NULL,
			ref TEXT NOT NULL DEFAULT '',
			created_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// starterBonus=0, dailyBase=50, step=10, cap=100, enabled=true
	return NewWalletService(repo.NewWalletRepository(db), 0, 50, 10, 100, true, logger.Default())
}

const dailyUser = "33333333-3333-3333-3333-333333333333"

// TestDaily_FirstClaim: first-ever daily claim → balance +50, streak=1,
// claimed=true, and last_daily_at updated to today's UTC date.
func TestDaily_FirstClaim(t *testing.T) {
	s := newSvcNoStarter(t)
	ctx := context.Background()
	// Ensure wallet exists with zero balance.
	if _, err := s.GetOrCreate(ctx, dailyUser); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	result, err := s.Daily(ctx, dailyUser, day0)
	if err != nil {
		t.Fatalf("Daily: %v", err)
	}
	if !result.Claimed {
		t.Fatal("first claim must be claimed=true")
	}
	if result.Amount != 50 {
		t.Errorf("amount = %d; want 50 (base only, streak=1 → bonus=0)", result.Amount)
	}
	if result.Streak != 1 {
		t.Errorf("streak = %d; want 1", result.Streak)
	}
	if result.Wallet == nil || result.Wallet.Balance != 50 {
		b := int64(0)
		if result.Wallet != nil {
			b = result.Wallet.Balance
		}
		t.Errorf("balance = %d; want 50", b)
	}

	// Probe the wallet row directly to check streak + last_daily_at.
	wantRef := day0.UTC().Format("2006-01-02")
	w, err2 := s.repo.GetOrCreate(ctx, dailyUser)
	if err2 != nil {
		t.Fatalf("re-read: %v", err2)
	}
	if w.DailyStreak != 1 {
		t.Errorf("wallet.daily_streak = %d; want 1", w.DailyStreak)
	}
	if w.LastDailyAt == nil {
		t.Fatal("last_daily_at must not be nil after claim")
	}
	gotRef := w.LastDailyAt.UTC().Format("2006-01-02")
	if gotRef != wantRef {
		t.Errorf("last_daily_at date = %q; want %q", gotRef, wantRef)
	}
}

// TestDaily_SecondClaimSameDayNoop: calling Daily twice on the same UTC day
// returns claimed=false on the second call without writing a new ledger row.
func TestDaily_SecondClaimSameDayNoop(t *testing.T) {
	s := newSvcNoStarter(t)
	ctx := context.Background()
	s.GetOrCreate(ctx, dailyUser)

	if _, err := s.Daily(ctx, dailyUser, day0); err != nil {
		t.Fatalf("first Daily: %v", err)
	}
	// Same day, one hour later.
	result, err := s.Daily(ctx, dailyUser, day0.Add(time.Hour))
	if err != nil {
		t.Fatalf("second Daily: %v", err)
	}
	if result.Claimed {
		t.Fatal("second same-day claim must be claimed=false")
	}
	// Balance must still be 50 (base only, no double-credit).
	w, _ := s.repo.GetOrCreate(ctx, dailyUser)
	if w.Balance != 50 {
		t.Errorf("balance = %d; want 50 (no double credit)", w.Balance)
	}
}

// TestDaily_ConsecutiveDayIncrementsStreak: claiming on day1 then day2 yields
// streak=2 and amount=60 (base 50 + step 10 × 1).
func TestDaily_ConsecutiveDayIncrementsStreak(t *testing.T) {
	s := newSvcNoStarter(t)
	ctx := context.Background()
	s.GetOrCreate(ctx, dailyUser)

	if _, err := s.Daily(ctx, dailyUser, day0); err != nil {
		t.Fatalf("day1 Daily: %v", err)
	}
	day1 := day0.Add(24 * time.Hour)
	result, err := s.Daily(ctx, dailyUser, day1)
	if err != nil {
		t.Fatalf("day2 Daily: %v", err)
	}
	if !result.Claimed {
		t.Fatal("day2 claim must be claimed=true")
	}
	if result.Streak != 2 {
		t.Errorf("streak = %d; want 2", result.Streak)
	}
	if result.Amount != 60 {
		t.Errorf("amount = %d; want 60 (50+10)", result.Amount)
	}
	if result.Wallet == nil || result.Wallet.Balance != 110 {
		b := int64(0)
		if result.Wallet != nil {
			b = result.Wallet.Balance
		}
		t.Errorf("balance = %d; want 110 (50+60)", b)
	}
}

// TestDaily_GapResetsStreak: claiming on day1 then skipping to day4 resets
// streak to 1 and awards base only (50).
func TestDaily_GapResetsStreak(t *testing.T) {
	s := newSvcNoStarter(t)
	ctx := context.Background()
	s.GetOrCreate(ctx, dailyUser)

	if _, err := s.Daily(ctx, dailyUser, day0); err != nil {
		t.Fatalf("day1 Daily: %v", err)
	}
	day4 := day0.Add(3 * 24 * time.Hour)
	result, err := s.Daily(ctx, dailyUser, day4)
	if err != nil {
		t.Fatalf("day4 Daily: %v", err)
	}
	if !result.Claimed {
		t.Fatal("day4 claim must be claimed=true")
	}
	if result.Streak != 1 {
		t.Errorf("streak = %d; want 1 (gap reset)", result.Streak)
	}
	if result.Amount != 50 {
		t.Errorf("amount = %d; want 50 (base only after reset)", result.Amount)
	}
}

// TestDaily_StreakBonusCaps: build streak to 15 via consecutive days, then
// claim one more day — the bonus caps at 100 and total = 150.
func TestDaily_StreakBonusCaps(t *testing.T) {
	s := newSvcNoStarter(t)
	ctx := context.Background()
	if _, err := s.GetOrCreate(ctx, dailyUser); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}

	// Build streak=15 by claiming 15 consecutive days leading up to day0.
	// day0 is the 16th claim; streak rises to 16 but bonus caps at
	// cap/step = 100/10 = 10 steps → bonus=100, total=150.
	for i := 0; i < 15; i++ {
		d := day0.Add(time.Duration(i-15) * 24 * time.Hour)
		if _, err := s.Daily(ctx, dailyUser, d); err != nil {
			t.Fatalf("preload day %d: %v", i, err)
		}
	}

	// 16th consecutive claim — bonus must be capped.
	result, err := s.Daily(ctx, dailyUser, day0)
	if err != nil {
		t.Fatalf("capped claim: %v", err)
	}
	if !result.Claimed {
		t.Fatal("capped claim must be claimed=true")
	}
	if result.Amount != 150 {
		t.Errorf("amount = %d; want 150 (50 base + 100 cap)", result.Amount)
	}
}
