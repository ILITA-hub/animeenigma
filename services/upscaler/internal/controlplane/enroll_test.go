package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestMain initialises the capability secret once for all controlplane tests.
// Using "test-secret" so VerifySession can actually validate minted handles.
func TestMain(m *testing.M) {
	capability.Init("test-secret")
	os.Exit(m.Run())
}

// --------------------------------------------------------------------------
// Handwritten fakes (fake-based orchestration path: Handle)
// --------------------------------------------------------------------------

// fakeTokenStore is an in-memory single-use token store.
//
//   - tokens[token] = false  → token is valid and not yet consumed
//   - tokens[token] = true   → token has already been consumed
//   - token absent           → token was never issued
type fakeTokenStore struct {
	mu         sync.Mutex
	tokens     map[string]bool // token → consumed?
	consumeErr error           // if set, Consume returns this error regardless
}

func newFakeStore(validTokens ...string) *fakeTokenStore {
	m := make(map[string]bool, len(validTokens))
	for _, t := range validTokens {
		m[t] = false
	}
	return &fakeTokenStore{tokens: m}
}

func (f *fakeTokenStore) Consume(_ context.Context, token string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.consumeErr != nil {
		return f.consumeErr
	}
	consumed, exists := f.tokens[token]
	if !exists || consumed {
		return ErrTokenNotFound
	}
	f.tokens[token] = true
	return nil
}

// fakeWorkerUpsert captures upserted workers in memory.
type fakeWorkerUpsert struct {
	mu      sync.Mutex
	workers []*domain.UpscaleWorker
	err     error // if set, Upsert returns this error
}

func (f *fakeWorkerUpsert) Upsert(_ context.Context, w *domain.UpscaleWorker) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.workers = append(f.workers, w)
	return nil
}

func (f *fakeWorkerUpsert) last() *domain.UpscaleWorker {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.workers) == 0 {
		return nil
	}
	return f.workers[len(f.workers)-1]
}

func (f *fakeWorkerUpsert) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.workers)
}

// --------------------------------------------------------------------------
// SQLite test-DB helper (mirrors repo/segment_sqlite_test.go).
// --------------------------------------------------------------------------

// ctlSQLiteOnce ensures the custom SQLite driver is registered exactly once per
// test binary (sql.Register panics on duplicate name).
var ctlSQLiteOnce sync.Once

// ctlGenRandomUUID returns a random UUID v4 string, the SQLite substitute for
// Postgres's gen_random_uuid() builtin.
func ctlGenRandomUUID() string {
	b := make([]byte, 16)
	rand.Read(b)                //nolint:gosec // test-only, non-cryptographic use
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func registerCtlSQLiteNow() {
	ctlSQLiteOnce.Do(func() {
		sql.Register("sqlite3_with_now_ctl", &sqlite3.SQLiteDriver{
			ConnectHook: func(conn *sqlite3.SQLiteConn) error {
				if err := conn.RegisterFunc("now", func() string {
					return time.Now().UTC().Format("2006-01-02 15:04:05")
				}, true); err != nil {
					return err
				}
				return conn.RegisterFunc("gen_random_uuid", ctlGenRandomUUID, false)
			},
		})
	})
}

// workersDDL / tokensDDL are the hand-written SQLite-compatible table defs.
// gen_random_uuid() is Postgres-only so tests supply IDs/tokens directly.
const workersDDL = `CREATE TABLE IF NOT EXISTS upscale_workers (
  worker_id          TEXT NOT NULL PRIMARY KEY,
  gpu_info           TEXT,
  image_version      TEXT,
  models_available   TEXT,
  status             TEXT NOT NULL DEFAULT 'idle',
  current_job_id     TEXT,
  current_segment    INTEGER,
  session_expires_at DATETIME,
  last_heartbeat_at  DATETIME,
  created_at         DATETIME
)`

const tokensDDL = `CREATE TABLE IF NOT EXISTS upscale_enroll_tokens (
  token       TEXT NOT NULL PRIMARY KEY,
  consumed_at DATETIME,
  created_at  DATETIME
)`

// openCtlTestDB opens a shared in-memory SQLite DB with the upscale_workers and
// upscale_enroll_tokens tables, and truncates them on cleanup so tests stay
// isolated within the same process.
func openCtlTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	registerCtlSQLiteNow()
	db, err := gorm.Open(&sqlite.Dialector{
		DriverName: "sqlite3_with_now_ctl",
		DSN:        "file:upscaler_ctlplane_test?mode=memory&cache=shared",
	}, &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("sqlite driver unavailable: %v", err)
	}
	for _, ddl := range []string{workersDDL, tokensDDL} {
		if err := db.Exec(ddl).Error; err != nil {
			t.Skipf("create table: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Exec("DELETE FROM upscale_workers")
		db.Exec("DELETE FROM upscale_enroll_tokens")
	})
	return db
}

// seedToken inserts a fresh (unconsumed) enroll token.
func seedToken(t *testing.T, db *gorm.DB, token string) {
	t.Helper()
	if err := db.Create(&domain.UpscaleEnrollToken{Token: token, CreatedAt: time.Now()}).Error; err != nil {
		t.Fatalf("seedToken: %v", err)
	}
}

// fetchToken returns the token row, or fails if it doesn't exist.
func fetchToken(t *testing.T, db *gorm.DB, token string) domain.UpscaleEnrollToken {
	t.Helper()
	var row domain.UpscaleEnrollToken
	if err := db.Where("token = ?", token).First(&row).Error; err != nil {
		t.Fatalf("fetchToken %q: %v", token, err)
	}
	return row
}

// countWorkers returns the number of rows in upscale_workers.
func countWorkers(t *testing.T, db *gorm.DB) int64 {
	t.Helper()
	var n int64
	if err := db.Model(&domain.UpscaleWorker{}).Count(&n).Error; err != nil {
		t.Fatalf("countWorkers: %v", err)
	}
	return n
}

// --------------------------------------------------------------------------
// Handle tests (fake-based orchestration)
// --------------------------------------------------------------------------

// TestHandle_ValidToken verifies the happy path:
//   - EnrollResponse carries a non-empty WorkerID + session triple
//   - The minted session verifies correctly with VerifySession
//   - The worker is upserted with the correct ID, status="idle", and a
//     derived SessionExpiresAt (I-2)
func TestHandle_ValidToken(t *testing.T) {
	store := newFakeStore("secret-token-1")
	workers := &fakeWorkerUpsert{}

	resp, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "secret-token-1"})
	if err != nil {
		t.Fatalf("Handle: unexpected error: %v", err)
	}
	if resp.WorkerID == "" {
		t.Error("EnrollResponse.WorkerID is empty, want non-empty UUID")
	}
	if resp.Handle == "" || resp.Exp == "" || resp.Sig == "" {
		t.Errorf("session triple (handle=%q exp=%q sig=%q) should be non-empty when capability is configured",
			resp.Handle, resp.Exp, resp.Sig)
	}
	if !VerifySession(resp.WorkerID, resp.Exp, resp.Sig, time.Now()) {
		t.Error("VerifySession returned false for freshly minted session")
	}

	w := workers.last()
	if w == nil {
		t.Fatal("no worker was upserted")
	}
	if w.WorkerID != resp.WorkerID {
		t.Errorf("upserted WorkerID = %q, want %q", w.WorkerID, resp.WorkerID)
	}
	if w.Status != "idle" {
		t.Errorf("upserted Status = %q, want %q", w.Status, "idle")
	}
	if w.SessionExpiresAt == nil {
		t.Error("upserted SessionExpiresAt is nil, want a derived expiry (I-2)")
	}
}

// TestHandle_ReusedToken verifies a token cannot be replayed through the
// fake-based orchestration path (the durable variant is covered by
// TestGormEnrollStore_ReusedToken).
func TestHandle_ReusedToken(t *testing.T) {
	store := newFakeStore("one-time-token")
	workers := &fakeWorkerUpsert{}

	if _, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "one-time-token"}); err != nil {
		t.Fatalf("first Handle call: %v", err)
	}
	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "one-time-token"})
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("second call: got %v, want ErrTokenNotFound", err)
	}
	if n := workers.count(); n != 1 {
		t.Errorf("upserted worker count = %d, want 1", n)
	}
}

// TestHandle_StoreError verifies fail-closed behaviour when the token store
// returns an unexpected error (e.g. Redis/DB outage): no worker is upserted.
func TestHandle_StoreError(t *testing.T) {
	storeErr := errors.New("store: connection refused")
	store := &fakeTokenStore{
		tokens:     map[string]bool{"tok": false},
		consumeErr: storeErr,
	}
	workers := &fakeWorkerUpsert{}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "tok"})
	if err == nil {
		t.Fatal("expected error from store failure, got nil")
	}
	if workers.last() != nil {
		t.Error("worker was upserted despite store error (fail-closed violation)")
	}
}

// TestHandle_DBError verifies that a worker-upsert (DB) error is propagated.
func TestHandle_DBError(t *testing.T) {
	store := newFakeStore("db-error-token")
	dbErr := errors.New("db: constraint violation")
	workers := &fakeWorkerUpsert{err: dbErr}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "db-error-token"})
	if err == nil {
		t.Fatal("expected error from DB failure, got nil")
	}
}

// TestHandle_CapabilityNotConfigured exercises the C-1 guard. We cannot un-set
// the global capability secret (Init is sync.Once-gated and already fired in
// TestMain), so we override the mintSession seam to return an empty triple,
// simulating an unconfigured capability, and restore it afterwards.
//
// Asserts: Handle returns the "capability not configured" error AND, crucially,
// no token is consumed and no worker is upserted (the guard fires before any
// side effect — the token is NOT burned).
func TestHandle_CapabilityNotConfigured(t *testing.T) {
	orig := mintSession
	mintSession = func(string, time.Duration) (string, string, string) { return "", "", "" }
	defer func() { mintSession = orig }()

	store := newFakeStore("c1-token")
	workers := &fakeWorkerUpsert{}

	_, err := Handle(context.Background(), store, workers, EnrollRequest{Token: "c1-token"})
	if err == nil {
		t.Fatal("expected C-1 error when capability is unconfigured, got nil")
	}
	if workers.last() != nil {
		t.Error("worker was upserted despite C-1 guard")
	}
	// Token must NOT have been consumed (guard fires before Consume).
	store.mu.Lock()
	consumed := store.tokens["c1-token"]
	store.mu.Unlock()
	if consumed {
		t.Error("token was consumed despite C-1 guard (token burned)")
	}
}

// --------------------------------------------------------------------------
// GormEnrollStore tests (production durable single-use + transactional rollback)
// --------------------------------------------------------------------------

// TestGormEnrollStore_ValidToken verifies the durable happy path: token consumed
// (ConsumedAt set), worker row created with status="idle" + SessionExpiresAt
// (I-2), and the minted session verifies.
func TestGormEnrollStore_ValidToken(t *testing.T) {
	db := openCtlTestDB(t)
	seedToken(t, db, "good-token")
	store := NewGormEnrollStore(db)

	resp, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "good-token"}, SessionTTL)
	if err != nil {
		t.Fatalf("EnrollTx: %v", err)
	}
	if resp.WorkerID == "" {
		t.Error("WorkerID is empty")
	}
	if !VerifySession(resp.WorkerID, resp.Exp, resp.Sig, time.Now()) {
		t.Error("VerifySession returned false for freshly minted session")
	}

	// Worker row exists with status="idle" + SessionExpiresAt set.
	var w domain.UpscaleWorker
	if err := db.Where("worker_id = ?", resp.WorkerID).First(&w).Error; err != nil {
		t.Fatalf("worker row not found: %v", err)
	}
	if w.Status != "idle" {
		t.Errorf("worker Status = %q, want %q", w.Status, "idle")
	}
	if w.SessionExpiresAt == nil {
		t.Error("worker SessionExpiresAt is nil, want a derived expiry (I-2)")
	}

	// Token now consumed.
	row := fetchToken(t, db, "good-token")
	if row.ConsumedAt == nil {
		t.Error("token ConsumedAt is nil after EnrollTx, want consumed")
	}
}

// TestGormEnrollStore_SelfEnroll proves the zero-config path: SelfEnroll mints a
// valid session and persists an idle worker WITHOUT any enroll token, and two
// calls yield two DISTINCT workers (one reusable image → many servers).
func TestGormEnrollStore_SelfEnroll(t *testing.T) {
	db := openCtlTestDB(t)
	store := NewGormEnrollStore(db)

	resp, err := store.SelfEnroll(context.Background(), SessionTTL)
	if err != nil {
		t.Fatalf("SelfEnroll: %v", err)
	}
	if resp.WorkerID == "" {
		t.Error("WorkerID is empty")
	}
	if !VerifySession(resp.WorkerID, resp.Exp, resp.Sig, time.Now()) {
		t.Error("VerifySession returned false for freshly self-enrolled session")
	}

	var w domain.UpscaleWorker
	if err := db.Where("worker_id = ?", resp.WorkerID).First(&w).Error; err != nil {
		t.Fatalf("worker row not found: %v", err)
	}
	if w.Status != "idle" {
		t.Errorf("worker Status = %q, want %q", w.Status, "idle")
	}
	if w.SessionExpiresAt == nil {
		t.Error("worker SessionExpiresAt is nil, want a derived expiry (I-2)")
	}

	// A second self-enroll yields a distinct worker (no collision).
	resp2, err := store.SelfEnroll(context.Background(), SessionTTL)
	if err != nil {
		t.Fatalf("second SelfEnroll: %v", err)
	}
	if resp2.WorkerID == resp.WorkerID {
		t.Error("two SelfEnroll calls returned the same WorkerID")
	}
	if got := countWorkers(t, db); got != 2 {
		t.Errorf("worker count = %d, want 2", got)
	}
}

// TestGormEnrollStore_SelfEnroll_CapabilityNotConfigured proves the C-1 guard:
// when the session triple is empty (capability secret unset), SelfEnroll rejects
// WITHOUT writing a worker row.
func TestGormEnrollStore_SelfEnroll_CapabilityNotConfigured(t *testing.T) {
	orig := mintSession
	mintSession = func(string, time.Duration) (string, string, string) { return "", "", "" }
	defer func() { mintSession = orig }()

	db := openCtlTestDB(t)
	store := NewGormEnrollStore(db)

	if _, err := store.SelfEnroll(context.Background(), SessionTTL); err == nil {
		t.Fatal("SelfEnroll: want error when capability not configured, got nil")
	}
	if got := countWorkers(t, db); got != 0 {
		t.Errorf("worker count = %d, want 0 (no row on C-1 rejection)", got)
	}
}

// TestGormEnrollStore_ReusedToken proves durable single-use: a second EnrollTx
// with the same token returns ErrTokenNotFound and only ONE worker is created.
func TestGormEnrollStore_ReusedToken(t *testing.T) {
	db := openCtlTestDB(t)
	seedToken(t, db, "replay-token")
	store := NewGormEnrollStore(db)

	if _, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "replay-token"}, SessionTTL); err != nil {
		t.Fatalf("first EnrollTx: %v", err)
	}
	_, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "replay-token"}, SessionTTL)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("second EnrollTx: got %v, want ErrTokenNotFound", err)
	}
	if n := countWorkers(t, db); n != 1 {
		t.Errorf("worker count = %d, want 1 (only the first enroll)", n)
	}
}

// TestGormEnrollStore_UnknownToken verifies an unissued token is rejected with
// ErrTokenNotFound and no worker row is created.
func TestGormEnrollStore_UnknownToken(t *testing.T) {
	db := openCtlTestDB(t)
	store := NewGormEnrollStore(db)

	_, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "never-issued"}, SessionTTL)
	if !errors.Is(err, ErrTokenNotFound) {
		t.Errorf("got %v, want ErrTokenNotFound", err)
	}
	if n := countWorkers(t, db); n != 0 {
		t.Errorf("worker count = %d, want 0 for unknown token", n)
	}
}

// TestGormEnrollStore_DBErrorOnConsume verifies fail-closed behaviour: when the
// DB is unreachable (connection closed), EnrollTx returns a NON-ErrTokenNotFound
// error and nothing is persisted.
func TestGormEnrollStore_DBErrorOnConsume(t *testing.T) {
	db := openCtlTestDB(t)
	seedToken(t, db, "db-down-token")
	store := NewGormEnrollStore(db)

	// Close the underlying connection so the tx fails with a real DB error.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB(): %v", err)
	}
	if cerr := sqlDB.Close(); cerr != nil {
		t.Fatalf("close: %v", cerr)
	}

	_, err = store.EnrollTx(context.Background(), EnrollRequest{Token: "db-down-token"}, SessionTTL)
	if err == nil {
		t.Fatal("expected a DB error when the connection is closed, got nil")
	}
	if errors.Is(err, ErrTokenNotFound) {
		t.Errorf("got ErrTokenNotFound, want a raw DB error (fail-closed)")
	}
}

// TestGormEnrollStore_UpserterFailure_RollsBack is the key I-3 rollback test.
// We force the worker upsert to fail WITHIN the tx (by dropping the
// upscale_workers table while upscale_enroll_tokens still exists) and assert:
//  1. EnrollTx returns an error.
//  2. The token is NOT consumed (the consume was rolled back — token not burned).
//  3. After recreating the workers table, a retry with the SAME token succeeds
//     (the token survived) and the token is then marked consumed.
func TestGormEnrollStore_UpserterFailure_RollsBack(t *testing.T) {
	db := openCtlTestDB(t)
	seedToken(t, db, "rollback-token")
	store := NewGormEnrollStore(db)

	// Force the in-tx worker upsert to fail: drop the workers table so the
	// Create errors ("no such table") while the consume UPDATE succeeds.
	if err := db.Exec("DROP TABLE upscale_workers").Error; err != nil {
		t.Fatalf("drop upscale_workers: %v", err)
	}

	_, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "rollback-token"}, SessionTTL)
	if err == nil {
		t.Fatal("expected an error when the worker upsert fails, got nil")
	}

	// I-3: the consume must have rolled back — token still unconsumed.
	row := fetchToken(t, db, "rollback-token")
	if row.ConsumedAt != nil {
		t.Fatalf("token ConsumedAt = %v after rolled-back upsert, want nil (token burned!)", row.ConsumedAt)
	}

	// Recreate the workers table and retry with the SAME token — must succeed.
	if err := db.Exec(workersDDL).Error; err != nil {
		t.Fatalf("recreate upscale_workers: %v", err)
	}
	resp, err := store.EnrollTx(context.Background(), EnrollRequest{Token: "rollback-token"}, SessionTTL)
	if err != nil {
		t.Fatalf("retry EnrollTx after rollback: %v (token was burned despite rollback)", err)
	}
	if resp.WorkerID == "" {
		t.Error("retry produced empty WorkerID")
	}
	if n := countWorkers(t, db); n != 1 {
		t.Errorf("worker count = %d after successful retry, want 1", n)
	}

	// Token now consumed by the successful retry.
	row = fetchToken(t, db, "rollback-token")
	if row.ConsumedAt == nil {
		t.Error("token ConsumedAt is nil after successful retry, want consumed")
	}
}
