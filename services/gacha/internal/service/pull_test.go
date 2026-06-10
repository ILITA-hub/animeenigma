package service

import (
	"context"
	"testing"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// ─── pure-helper test fixtures ──────────────────────────────────────────────

func card(id string, r domain.Rarity) domain.Card {
	return domain.Card{ID: id, Name: id, ImagePath: "cards/" + id + ".webp", Rarity: r, Enabled: true}
}

func defaultWeights() map[domain.Rarity]int {
	return map[domain.Rarity]int{
		domain.RarityN:   69,
		domain.RarityR:   22,
		domain.RaritySR:  8,
		domain.RaritySSR: 1,
	}
}

// fullPool has at least one card of every tier.
func fullPool() map[domain.Rarity][]domain.Card {
	return map[domain.Rarity][]domain.Card{
		domain.RarityN:   {card("n1", domain.RarityN)},
		domain.RarityR:   {card("r1", domain.RarityR)},
		domain.RaritySR:  {card("sr1", domain.RaritySR)},
		domain.RaritySSR: {card("ssr1", domain.RaritySSR)},
	}
}

// tierWeight finds the weight assigned to a tier in the cumulative table.
func tierWeight(table []tierEntry, r domain.Rarity) int {
	prev := 0
	for _, e := range table {
		if e.rarity == r {
			return e.cumulative - prev
		}
		prev = e.cumulative
	}
	return 0
}

func TestBuildTierTable_FullPoolMatchesWeights(t *testing.T) {
	table := buildTierTable(defaultWeights(), fullPool())
	want := map[domain.Rarity]int{
		domain.RarityN:   69,
		domain.RarityR:   22,
		domain.RaritySR:  8,
		domain.RaritySSR: 1,
	}
	for r, w := range want {
		if got := tierWeight(table, r); got != w {
			t.Errorf("tier %s weight = %d; want %d", r, got, w)
		}
	}
	// Cumulative total == 100.
	if total := table[len(table)-1].cumulative; total != 100 {
		t.Errorf("cumulative total = %d; want 100", total)
	}
}

func TestBuildTierTable_MissingTierRedistributesDown(t *testing.T) {
	// No SSR: its weight (1) goes DOWN to SR → SR = 8 + 1 = 9.
	noSSR := map[domain.Rarity][]domain.Card{
		domain.RarityN:  {card("n1", domain.RarityN)},
		domain.RarityR:  {card("r1", domain.RarityR)},
		domain.RaritySR: {card("sr1", domain.RaritySR)},
	}
	table := buildTierTable(defaultWeights(), noSSR)
	if got := tierWeight(table, domain.RaritySR); got != 9 {
		t.Errorf("SR weight (no SSR) = %d; want 9", got)
	}
	if got := tierWeight(table, domain.RaritySSR); got != 0 {
		t.Errorf("SSR weight (absent) = %d; want 0", got)
	}
	if total := table[len(table)-1].cumulative; total != 100 {
		t.Errorf("cumulative total = %d; want 100", total)
	}

	// Only N present: ALL weight (everything above redistributes down to N) = 100.
	onlyN := map[domain.Rarity][]domain.Card{
		domain.RarityN: {card("n1", domain.RarityN)},
	}
	table2 := buildTierTable(defaultWeights(), onlyN)
	if got := tierWeight(table2, domain.RarityN); got != 100 {
		t.Errorf("N weight (only N) = %d; want 100", got)
	}

	// Only SSR present: everything below has nothing below → redistributes UP to SSR = 100.
	onlySSR := map[domain.Rarity][]domain.Card{
		domain.RaritySSR: {card("ssr1", domain.RaritySSR)},
	}
	table3 := buildTierTable(defaultWeights(), onlySSR)
	if got := tierWeight(table3, domain.RaritySSR); got != 100 {
		t.Errorf("SSR weight (only SSR) = %d; want 100", got)
	}
}

// ─── tx-orchestration test fixtures ─────────────────────────────────────────

func newPullSvcDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE gacha_wallets (
			user_id TEXT PRIMARY KEY, balance INTEGER NOT NULL DEFAULT 0,
			starter_granted INTEGER NOT NULL DEFAULT 0, daily_streak INTEGER NOT NULL DEFAULT 0,
			last_daily_at DATETIME, created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE gacha_ledger (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))), user_id TEXT NOT NULL,
			delta INTEGER NOT NULL, reason TEXT NOT NULL, ref TEXT NOT NULL DEFAULT '', created_at DATETIME)`,
		`CREATE UNIQUE INDEX idx_ledger_dedup ON gacha_ledger(user_id, reason, ref) WHERE ref <> ''`,
		`CREATE TABLE gacha_collection (
			user_id TEXT NOT NULL, card_id TEXT NOT NULL, count INTEGER NOT NULL DEFAULT 1, first_obtained_at DATETIME)`,
		`CREATE UNIQUE INDEX idx_user_card ON gacha_collection(user_id, card_id)`,
		`CREATE TABLE gacha_pity (user_id TEXT NOT NULL, banner_id TEXT NOT NULL, pulls_since_ssr INTEGER NOT NULL DEFAULT 0)`,
		`CREATE UNIQUE INDEX idx_user_banner ON gacha_pity(user_id, banner_id)`,
		`CREATE TABLE gacha_cards (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, source_title TEXT, image_path TEXT NOT NULL,
			back_path TEXT NOT NULL DEFAULT '',
			rarity TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 0, created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE gacha_banners (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, description TEXT, art_path TEXT,
			backdrop_path TEXT NOT NULL DEFAULT '',
			is_standard INTEGER NOT NULL DEFAULT 0, enabled INTEGER NOT NULL DEFAULT 0,
			active_from DATETIME, active_to DATETIME, sort_order INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME, updated_at DATETIME, deleted_at DATETIME)`,
		`CREATE TABLE gacha_banner_cards (banner_id TEXT NOT NULL, card_id TEXT NOT NULL)`,
		`CREATE UNIQUE INDEX idx_banner_card ON gacha_banner_cards(banner_id, card_id)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("DDL: %v", err)
		}
	}
	return db
}

const (
	svcUser = "11111111-1111-1111-1111-111111111111"
	bannerA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	bannerB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func seedBalance(t *testing.T, db *gorm.DB, bal int64) {
	t.Helper()
	if err := db.Exec(`INSERT INTO gacha_wallets (user_id, balance) VALUES (?, ?)`, svcUser, bal).Error; err != nil {
		t.Fatalf("seed wallet: %v", err)
	}
}

func seedBanner(t *testing.T, db *gorm.DB, id string, enabled bool) {
	t.Helper()
	if err := db.Exec(`INSERT INTO gacha_banners (id, name, enabled, is_standard) VALUES (?, ?, ?, 1)`,
		id, "Banner "+id, enabled).Error; err != nil {
		t.Fatalf("seed banner: %v", err)
	}
}

func seedCard(t *testing.T, db *gorm.DB, bannerID, cardID string, r domain.Rarity) {
	t.Helper()
	if err := db.Exec(`INSERT INTO gacha_cards (id, name, image_path, rarity, enabled) VALUES (?, ?, ?, ?, 1)`,
		cardID, cardID, "cards/"+cardID+".webp", r).Error; err != nil {
		t.Fatalf("seed card: %v", err)
	}
	if err := db.Exec(`INSERT INTO gacha_banner_cards (banner_id, card_id) VALUES (?, ?)`, bannerID, cardID).Error; err != nil {
		t.Fatalf("link card: %v", err)
	}
}

func testEconomy() config.EconomyConfig {
	return config.EconomyConfig{
		PullCostX1: 100, PullCostX10: 900, PityThreshold: 90,
		WeightN: 69, WeightR: 22, WeightSR: 8, WeightSSR: 1,
	}
}

// seqRand returns a deterministic randInt that walks the given sequence,
// clamping each value into [0, n) so callers can pass a tier-selecting cursor
// regardless of the actual modulus.
func seqRand(seq []int) func(int) int {
	i := 0
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		v := seq[i%len(seq)]
		i++
		return v % n
	}
}

// constRand always returns a value that lands in the N tier (cumulative pick 0).
func constRand(v int) func(int) int { return func(n int) int { if n <= 0 { return 0 }; return v % n } }

func newPullSvc(t *testing.T, db *gorm.DB, randInt func(int) int) *PullService {
	t.Helper()
	pullRepo := repo.NewPullRepository(db)
	bannerRepo := repo.NewBannerRepository(db)
	contentRepo := repo.NewContentRepository(db)
	return NewPullService(pullRepo, bannerRepo, contentRepo, testEconomy(), randInt, logger.Default())
}

func getBalance(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var b int64
	db.Raw(`SELECT balance FROM gacha_wallets WHERE user_id = ?`, svcUser).Scan(&b)
	return b
}

func getPity(t *testing.T, db *gorm.DB, bannerID string) int {
	t.Helper()
	var p int
	db.Raw(`SELECT pulls_since_ssr FROM gacha_pity WHERE user_id = ? AND banner_id = ?`, svcUser, bannerID).Scan(&p)
	return p
}

func TestPull_X1_DebitsRollsRecords(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 300)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	// rand always lands in N tier.
	svc := newPullSvc(t, db, constRand(0))

	res, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %d; want 1", len(res.Cards))
	}
	if res.Balance != 200 {
		t.Errorf("balance = %d; want 200", res.Balance)
	}
	if getBalance(t, db) != 200 {
		t.Errorf("db balance = %d; want 200", getBalance(t, db))
	}
	if res.Cards[0].Count != 1 || !res.Cards[0].New {
		t.Errorf("card New/Count = %v/%d; want true/1", res.Cards[0].New, res.Cards[0].Count)
	}
	if res.Pity != 1 || getPity(t, db, bannerA) != 1 {
		t.Errorf("pity = %d (db %d); want 1", res.Pity, getPity(t, db, bannerA))
	}
}

func TestPull_InsufficientFunds_NothingHappens(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 50)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	svc := newPullSvc(t, db, constRand(0))

	_, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err == nil {
		t.Fatal("expected insufficient funds error")
	}
	if !apperrors.Is(err, repo.ErrInsufficientFunds) {
		t.Errorf("err = %v; want ErrInsufficientFunds", err)
	}
	if getBalance(t, db) != 50 {
		t.Errorf("balance = %d; want 50 (unchanged)", getBalance(t, db))
	}
	if getPity(t, db, bannerA) != 0 {
		t.Errorf("pity = %d; want 0 (no roll)", getPity(t, db, bannerA))
	}
	var ledger, coll int64
	db.Raw(`SELECT COUNT(*) FROM gacha_ledger`).Scan(&ledger)
	db.Raw(`SELECT COUNT(*) FROM gacha_collection`).Scan(&coll)
	if ledger != 0 || coll != 0 {
		t.Errorf("ledger=%d collection=%d; want 0/0 (no side effects)", ledger, coll)
	}
}

func TestPull_PityForcesSSRAt90AndResets(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 300)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerA, "ssr1", domain.RaritySSR)
	// Preload pity 89 → the next (90th) pull is forced SSR regardless of rand.
	db.Exec(`INSERT INTO gacha_pity (user_id, banner_id, pulls_since_ssr) VALUES (?, ?, 89)`, svcUser, bannerA)
	// rand always lands in N tier — but pity forces SSR.
	svc := newPullSvc(t, db, constRand(0))

	res, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if res.Cards[0].Card.Rarity != domain.RaritySSR {
		t.Errorf("forced rarity = %s; want SSR", res.Cards[0].Card.Rarity)
	}
	if res.Pity != 0 || getPity(t, db, bannerA) != 0 {
		t.Errorf("pity = %d; want 0 after forced SSR", res.Pity)
	}
}

func TestPull_SSRResetsPityMidBatch(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerA, "r1", domain.RarityR)
	seedCard(t, db, bannerA, "sr1", domain.RaritySR)
	seedCard(t, db, bannerA, "ssr1", domain.RaritySSR)
	// Build a rand sequence: roll #3 (index 2) lands in SSR tier; others in N.
	// Tier table cumulative: N[0,69) R[69,91) SR[91,99) SSR[99,100).
	// To land SSR, randInt(100) must return 99. To land N, return 0.
	// Each roll: first call selects tier, second call selects card within tier.
	seq := []int{
		0, 0, // roll1 N, card pick
		0, 0, // roll2 N, card pick
		99, 0, // roll3 SSR, card pick (resets pity)
		0, 0, // roll4 N
		0, 0, // roll5 N
		0, 0, // roll6 N
		0, 0, // roll7 N
		0, 0, // roll8 N
		0, 0, // roll9 N
		0, 0, // roll10 N
	}
	svc := newPullSvc(t, db, seqRand(seq))

	res, err := svc.Pull(context.Background(), svcUser, bannerA, "x10")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Cards) != 10 {
		t.Fatalf("cards = %d; want 10", len(res.Cards))
	}
	// SSR at roll 3 reset pity to 0; rolls 4-10 (7 rolls) increment → 7.
	if res.Pity != 7 {
		t.Errorf("pity = %d; want 7 (reset at roll 3, +7 after)", res.Pity)
	}
}

func TestPull_X10_SRFloorUpgradesLast(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerA, "r1", domain.RarityR)
	seedCard(t, db, bannerA, "sr1", domain.RaritySR)
	seedCard(t, db, bannerA, "ssr1", domain.RaritySSR)
	// rand always lands in N → no SR+ naturally → floor upgrades the LAST roll.
	svc := newPullSvc(t, db, constRand(0))

	res, err := svc.Pull(context.Background(), svcUser, bannerA, "x10")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if len(res.Cards) != 10 {
		t.Fatalf("cards = %d; want 10", len(res.Cards))
	}
	nCount, srCount := 0, 0
	for _, c := range res.Cards {
		switch c.Card.Rarity {
		case domain.RarityN:
			nCount++
		case domain.RaritySR:
			srCount++
		}
	}
	if nCount != 9 || srCount != 1 {
		t.Errorf("N=%d SR=%d; want 9 N + 1 SR (floor upgrade)", nCount, srCount)
	}
	if res.Cards[9].Card.Rarity != domain.RaritySR {
		t.Errorf("last card rarity = %s; want SR (floor upgrades LAST)", res.Cards[9].Card.Rarity)
	}
}

func TestPull_X10_NoFloorWhenSRPresent(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerA, "r1", domain.RarityR)
	seedCard(t, db, bannerA, "sr1", domain.RaritySR)
	seedCard(t, db, bannerA, "ssr1", domain.RaritySSR)
	// roll #2 lands SR (cumulative 91..99 → randInt(100)=91), rest N.
	seq := []int{
		0, 0, // roll1 N
		91, 0, // roll2 SR
		0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, // rolls 3-10 N
	}
	svc := newPullSvc(t, db, seqRand(seq))

	res, err := svc.Pull(context.Background(), svcUser, bannerA, "x10")
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	// Last card must still be N (no floor upgrade because an SR already present).
	if res.Cards[9].Card.Rarity != domain.RarityN {
		t.Errorf("last card = %s; want N (no upgrade — SR already present)", res.Cards[9].Card.Rarity)
	}
	srCount := 0
	for _, c := range res.Cards {
		if c.Card.Rarity == domain.RaritySR {
			srCount++
		}
	}
	if srCount != 1 {
		t.Errorf("SR count = %d; want exactly 1 (the natural one)", srCount)
	}
}

func TestPull_PerBannerPityIsolation(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	seedBanner(t, db, bannerB, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerB, "n2", domain.RarityN)
	svc := newPullSvc(t, db, constRand(0))

	if _, err := svc.Pull(context.Background(), svcUser, bannerA, "x1"); err != nil {
		t.Fatalf("pull A: %v", err)
	}
	if _, err := svc.Pull(context.Background(), svcUser, bannerA, "x1"); err != nil {
		t.Fatalf("pull A2: %v", err)
	}
	if getPity(t, db, bannerA) != 2 {
		t.Errorf("banner A pity = %d; want 2", getPity(t, db, bannerA))
	}
	if getPity(t, db, bannerB) != 0 {
		t.Errorf("banner B pity = %d; want 0 (isolated)", getPity(t, db, bannerB))
	}
}

func TestPull_DupesIncrementCount(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	// Single-card pool → every roll is the same card.
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	svc := newPullSvc(t, db, constRand(0))

	res1, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err != nil {
		t.Fatalf("pull1: %v", err)
	}
	if !res1.Cards[0].New || res1.Cards[0].Count != 1 {
		t.Errorf("pull1 New/Count = %v/%d; want true/1", res1.Cards[0].New, res1.Cards[0].Count)
	}
	res2, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err != nil {
		t.Fatalf("pull2: %v", err)
	}
	if res2.Cards[0].New {
		t.Error("pull2 must NOT be New (dupe)")
	}
	if res2.Cards[0].Count != 2 {
		t.Errorf("pull2 count = %d; want 2", res2.Cards[0].Count)
	}
}

func TestPull_InactiveBannerRejected(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, false) // disabled
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	svc := newPullSvc(t, db, constRand(0))

	_, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err == nil {
		t.Fatal("expected InvalidInput for inactive banner")
	}
	if appErr, ok := apperrors.IsAppError(err); !ok || appErr.Code != apperrors.CodeInvalidInput {
		t.Errorf("err = %v; want InvalidInput", err)
	}
	if getBalance(t, db) != 1000 {
		t.Errorf("balance = %d; want 1000 (no debit on rejection)", getBalance(t, db))
	}
}

func TestPull_EmptyPoolRejected(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true) // active but no cards
	svc := newPullSvc(t, db, constRand(0))

	_, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
	if err == nil {
		t.Fatal("expected InvalidInput for empty pool")
	}
	if getBalance(t, db) != 1000 {
		t.Errorf("balance = %d; want 1000 (no debit)", getBalance(t, db))
	}
}

func TestPull_UnknownModeRejected(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 1000)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	svc := newPullSvc(t, db, constRand(0))

	if _, err := svc.Pull(context.Background(), svcUser, bannerA, "x5"); err == nil {
		t.Fatal("expected InvalidInput for unknown mode")
	}
	if getBalance(t, db) != 1000 {
		t.Errorf("balance = %d; want 1000", getBalance(t, db))
	}
}

func TestPull_Distribution_Sanity(t *testing.T) {
	db := newPullSvcDB(t)
	seedBalance(t, db, 10_000_000)
	seedBanner(t, db, bannerA, true)
	seedCard(t, db, bannerA, "n1", domain.RarityN)
	seedCard(t, db, bannerA, "r1", domain.RarityR)
	seedCard(t, db, bannerA, "sr1", domain.RaritySR)
	seedCard(t, db, bannerA, "ssr1", domain.RaritySSR)
	svc := newPullSvc(t, db, NewSecureRand()) // production rand

	const n = 10_000
	counts := map[domain.Rarity]int{}
	for i := 0; i < n; i++ {
		res, err := svc.Pull(context.Background(), svcUser, bannerA, "x1")
		if err != nil {
			t.Fatalf("pull %d: %v", i, err)
		}
		counts[res.Cards[0].Card.Rarity]++
	}
	// Expected: N 69% R 22% SR 8% SSR 1% (pity won't fire in 10k single pulls
	// because SSR appears far more often than every 90 — but even if it does,
	// it only raises SSR slightly). Loose ±30% bound just catches inverted tables.
	expect := map[domain.Rarity]float64{
		domain.RarityN: 0.69, domain.RarityR: 0.22, domain.RaritySR: 0.08,
	}
	for r, p := range expect {
		got := float64(counts[r]) / float64(n)
		lo, hi := p*0.7, p*1.3
		if got < lo || got > hi {
			t.Errorf("tier %s freq = %.3f; want within [%.3f, %.3f]", r, got, lo, hi)
		}
	}
	if counts[domain.RaritySSR] == 0 {
		t.Error("SSR never rolled in 10k pulls — table likely broken")
	}
}

// guard against unused import when time helpers trimmed.
var _ = time.Now
