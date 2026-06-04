package repo

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newTestDB builds an in-memory sqlite DB with the analytics schema. Shared by
// resolve_test.go and the Postgres contract harness. The InsertBatch /
// UpsertIdentity / latest-wins assertions that used to live here have been
// folded into the backend-agnostic suite in store_contract_test.go
// (TestPostgresStore_Contract) so PG/sqlite and ClickHouse run the identical
// contract (AR-STORE-03).
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := AutoMigrateAll(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := EnsureView(db); err != nil {
		t.Fatalf("view: %v", err)
	}
	return db
}
