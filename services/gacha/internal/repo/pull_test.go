package repo

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newPullTestDB builds an in-memory sqlite DB with the wallet/ledger tables
// PLUS the Phase-3 collection/pity/card tables, using raw DDL (gen_random_uuid
// is Postgres-only; sqlite uses lower(hex(randomblob(16)))).
func newPullTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
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
		`CREATE TABLE gacha_collection (
			user_id TEXT NOT NULL,
			card_id TEXT NOT NULL,
			count INTEGER NOT NULL DEFAULT 1,
			first_obtained_at DATETIME
		)`,
		`CREATE UNIQUE INDEX idx_user_card ON gacha_collection(user_id, card_id)`,
		`CREATE TABLE gacha_pity (
			user_id TEXT NOT NULL,
			banner_id TEXT NOT NULL,
			pulls_since_ssr INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE UNIQUE INDEX idx_user_banner ON gacha_pity(user_id, banner_id)`,
		`CREATE TABLE gacha_cards (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			name TEXT NOT NULL,
			source_title TEXT,
			image_path TEXT NOT NULL,
			rarity TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME,
			updated_at DATETIME,
			deleted_at DATETIME
		)`,
		`CREATE TABLE gacha_banner_cards (
			banner_id TEXT NOT NULL,
			card_id TEXT NOT NULL
		)`,
		`CREATE UNIQUE INDEX idx_banner_card ON gacha_banner_cards(banner_id, card_id)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return db
}

const (
	bannerA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bannerB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	cardX   = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	cardY   = "dddddddd-dddd-dddd-dddd-dddddddddddd"
)

func seedWallet(t *testing.T, db *gorm.DB, balance int64) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO gacha_wallets (user_id, balance) VALUES (?, ?)`, testUser, balance,
	).Error; err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
}

func TestDebit_SucceedsAndWritesLedger(t *testing.T) {
	db := newPullTestDB(t)
	seedWallet(t, db, 300)
	ctx := context.Background()

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return DebitTx(tx, testUser, 100, domain.ReasonPullX1, "pull-1")
	})
	if err != nil {
		t.Fatalf("DebitTx: %v", err)
	}

	var balance int64
	db.Raw(`SELECT balance FROM gacha_wallets WHERE user_id = ?`, testUser).Scan(&balance)
	if balance != 200 {
		t.Errorf("balance = %d; want 200", balance)
	}
	var delta int64
	var n int64
	db.Raw(`SELECT COUNT(*), COALESCE(SUM(delta),0) FROM gacha_ledger WHERE user_id = ?`, testUser).Row().Scan(&n, &delta)
	if n != 1 {
		t.Errorf("ledger rows = %d; want 1", n)
	}
	if delta != -100 {
		t.Errorf("ledger delta = %d; want -100", delta)
	}
}

func TestDebit_InsufficientFundsNoChange(t *testing.T) {
	db := newPullTestDB(t)
	seedWallet(t, db, 50)
	ctx := context.Background()

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return DebitTx(tx, testUser, 100, domain.ReasonPullX1, "pull-1")
	})
	if err == nil {
		t.Fatal("expected ErrInsufficientFunds, got nil")
	}
	if !apperrors.Is(err, ErrInsufficientFunds) {
		t.Errorf("error = %v; want ErrInsufficientFunds", err)
	}

	var balance int64
	db.Raw(`SELECT balance FROM gacha_wallets WHERE user_id = ?`, testUser).Scan(&balance)
	if balance != 50 {
		t.Errorf("balance = %d; want 50 (unchanged)", balance)
	}
	var n int64
	db.Raw(`SELECT COUNT(*) FROM gacha_ledger WHERE user_id = ?`, testUser).Scan(&n)
	if n != 0 {
		t.Errorf("ledger rows = %d; want 0 (no side effects)", n)
	}
}

func TestPity_GetOrCreateIncrementSetReset(t *testing.T) {
	db := newPullTestDB(t)
	r := NewPullRepository(db)
	ctx := context.Background()

	// First access creates the row at 0.
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.GetPityForUpdate(tx, testUser, bannerA)
		if err != nil {
			return err
		}
		if p.PullsSinceSSR != 0 {
			t.Errorf("fresh pity = %d; want 0", p.PullsSinceSSR)
		}
		p.PullsSinceSSR = 5
		return r.SavePityTx(tx, p)
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}

	// Re-read: 5 persisted.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.GetPityForUpdate(tx, testUser, bannerA)
		if err != nil {
			return err
		}
		if p.PullsSinceSSR != 5 {
			t.Errorf("pity = %d; want 5", p.PullsSinceSSR)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx2: %v", err)
	}

	// Per-banner isolation: banner B is independent and stays 0.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		p, err := r.GetPityForUpdate(tx, testUser, bannerB)
		if err != nil {
			return err
		}
		if p.PullsSinceSSR != 0 {
			t.Errorf("banner B pity = %d; want 0 (isolated)", p.PullsSinceSSR)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx3: %v", err)
	}
}

func TestCollectionUpsert_NewThenDupe(t *testing.T) {
	db := newPullTestDB(t)
	r := NewPullRepository(db)
	ctx := context.Background()

	// First obtain — new, count 1.
	var firstObtained time.Time
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newIDs, counts, err := r.AddToCollectionTx(tx, testUser, []string{cardX})
		if err != nil {
			return err
		}
		if !newIDs[cardX] {
			t.Error("first obtain should be new")
		}
		if counts[cardX] != 1 {
			t.Errorf("count = %d; want 1", counts[cardX])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx1: %v", err)
	}
	db.Raw(`SELECT first_obtained_at FROM gacha_collection WHERE user_id = ? AND card_id = ?`, testUser, cardX).Scan(&firstObtained)

	time.Sleep(2 * time.Millisecond)

	// Same card again — dupe, count 2, NOT new, first_obtained unchanged.
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		newIDs, counts, err := r.AddToCollectionTx(tx, testUser, []string{cardX})
		if err != nil {
			return err
		}
		if newIDs[cardX] {
			t.Error("dupe must NOT be new")
		}
		if counts[cardX] != 2 {
			t.Errorf("count = %d; want 2", counts[cardX])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx2: %v", err)
	}
	var after time.Time
	db.Raw(`SELECT first_obtained_at FROM gacha_collection WHERE user_id = ? AND card_id = ?`, testUser, cardX).Scan(&after)
	if !after.Equal(firstObtained) {
		t.Errorf("first_obtained_at changed: %v -> %v", firstObtained, after)
	}
}

// TestCollectionUpsert_SameCardTwiceInOneBatch covers a single x10 that yields
// the same card twice: count must end at 2.
func TestCollectionUpsert_SameCardTwiceInOneBatch(t *testing.T) {
	db := newPullTestDB(t)
	r := NewPullRepository(db)
	ctx := context.Background()

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, counts, err := r.AddToCollectionTx(tx, testUser, []string{cardX, cardX})
		if err != nil {
			return err
		}
		if counts[cardX] != 2 {
			t.Errorf("count = %d; want 2 (same card twice in batch)", counts[cardX])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("tx: %v", err)
	}
	var count int
	db.Raw(`SELECT count FROM gacha_collection WHERE user_id = ? AND card_id = ?`, testUser, cardX).Scan(&count)
	if count != 2 {
		t.Errorf("persisted count = %d; want 2", count)
	}
}
