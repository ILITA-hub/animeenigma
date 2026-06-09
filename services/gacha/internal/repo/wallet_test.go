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
		// Mirror the production partial unique index used for credit idempotency.
		`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
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
