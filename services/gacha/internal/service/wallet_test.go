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
	s.GetOrCreate(ctx, u) // no starter granted — service is disabled
	applied, err := s.Credit(ctx, u, 22, domain.ReasonEpisodeWatched, "a:1")
	if err != nil {
		t.Fatalf("disabled credit err: %v", err)
	}
	if applied {
		t.Fatal("disabled service must not apply credits")
	}
}
