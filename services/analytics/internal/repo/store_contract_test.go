package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	tcclickhouse "github.com/testcontainers/testcontainers-go/modules/clickhouse"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

// storeHarness bundles a store under test with backend-neutral inspection
// hooks so one contract body can run against both the GORM/sqlite store and a
// real ClickHouse store.
type storeHarness struct {
	store domain.EventStore
	// countEvents returns how many event rows are persisted.
	countEvents func(t *testing.T) int64
	// resolveUser returns the resolved user_id for an anonymous_id via the
	// backend's resolved view (latest-wins identity stitching).
	resolveUser func(t *testing.T, anonymousID string) string
}

// runEventStoreContract is the single backend-agnostic EventStore contract.
// Both TestPostgresStore_Contract (sqlite) and TestClickHouseStore_Contract
// (real ClickHouse via testcontainers) drive this identical body, proving
// parity (AR-STORE-03).
func runEventStoreContract(t *testing.T, newStore func(t *testing.T) storeHarness) {
	t.Run("InsertBatch_persists", func(t *testing.T) {
		h := newStore(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Millisecond)
		events := []domain.Event{
			{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: now},
			{EventID: "e2", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1", Timestamp: now, ElSelector: "button#buy"},
		}
		if err := h.store.InsertBatch(ctx, events); err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		if got := h.countEvents(t); got != 2 {
			t.Fatalf("expected 2 rows, got %d", got)
		}
	})

	t.Run("InsertBatch_empty_noop", func(t *testing.T) {
		h := newStore(t)
		if err := h.store.InsertBatch(context.Background(), nil); err != nil {
			t.Fatalf("empty batch must be a no-op, got %v", err)
		}
		if got := h.countEvents(t); got != 0 {
			t.Fatalf("expected 0 rows after empty batch, got %d", got)
		}
	})

	t.Run("UpsertIdentity_latest_wins", func(t *testing.T) {
		h := newStore(t)
		ctx := context.Background()
		now := time.Now().UTC().Truncate(time.Millisecond)

		// One event for the anon visitor, then two identities — the later one wins.
		if err := h.store.InsertBatch(ctx, []domain.Event{
			{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "anon-x", SessionID: "s1", Timestamp: now},
		}); err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		if err := h.store.UpsertIdentity(ctx, "anon-x", "user-old", now); err != nil {
			t.Fatalf("UpsertIdentity old: %v", err)
		}
		if err := h.store.UpsertIdentity(ctx, "anon-x", "user-new", now.Add(time.Second)); err != nil {
			t.Fatalf("UpsertIdentity new: %v", err)
		}
		if got := h.resolveUser(t, "anon-x"); got != "user-new" {
			t.Fatalf("expected latest identity user-new, got %q", got)
		}
	})

	t.Run("UpsertIdentity_empty_noop", func(t *testing.T) {
		h := newStore(t)
		ctx := context.Background()
		if err := h.store.UpsertIdentity(ctx, "", "u", time.Now()); err != nil {
			t.Fatalf("empty anon must be a no-op, got %v", err)
		}
		if err := h.store.UpsertIdentity(ctx, "a", "", time.Now()); err != nil {
			t.Fatalf("empty user must be a no-op, got %v", err)
		}
	})
}

// --- Postgres/sqlite backend -------------------------------------------------

func newSqliteTestDB(t *testing.T) *gorm.DB {
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

func TestPostgresStore_Contract(t *testing.T) {
	runEventStoreContract(t, func(t *testing.T) storeHarness {
		db := newSqliteTestDB(t)
		return storeHarness{
			store: NewPostgresStore(db),
			countEvents: func(t *testing.T) int64 {
				var n int64
				if err := db.Model(&Event{}).Count(&n).Error; err != nil {
					t.Fatalf("count events: %v", err)
				}
				return n
			},
			resolveUser: func(t *testing.T, anonymousID string) string {
				var uid string
				err := db.Table("analytics_events_resolved").
					Where("anonymous_id = ?", anonymousID).
					Select("resolved_user_id").
					Limit(1).
					Scan(&uid).Error
				if err != nil {
					t.Fatalf("resolve user: %v", err)
				}
				return uid
			},
		}
	})
}

// --- ClickHouse backend (real container, gated by -short) --------------------

func newCHContainerConn(t *testing.T) driver.Conn {
	t.Helper()
	ctx := context.Background()

	const (
		dbName = "analytics"
		user   = "analytics"
		pass   = "changeme"
	)
	ch, err := tcclickhouse.Run(ctx,
		"clickhouse/clickhouse-server:24.3",
		tcclickhouse.WithDatabase(dbName),
		tcclickhouse.WithUsername(user),
		tcclickhouse.WithPassword(pass),
	)
	if err != nil {
		t.Fatalf("start clickhouse container: %v", err)
	}
	t.Cleanup(func() { _ = ch.Terminate(ctx) })

	host, err := ch.ConnectionHost(ctx)
	if err != nil {
		t.Fatalf("connection host: %v", err)
	}
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{host},
		Auth: clickhouse.Auth{Database: dbName, Username: user, Password: pass},
	})
	if err != nil {
		t.Fatalf("open clickhouse: %v", err)
	}
	if err := conn.Ping(ctx); err != nil {
		t.Fatalf("ping clickhouse: %v", err)
	}
	if err := EnsureSchema(ctx, conn); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestClickHouseStore_Contract(t *testing.T) {
	if testing.Short() {
		t.Skip("requires docker (ClickHouse testcontainer)")
	}
	conn := newCHContainerConn(t)
	runEventStoreContract(t, func(t *testing.T) storeHarness {
		// Fresh data per sub-test: truncate between runs so counts are isolated.
		ctx := context.Background()
		if err := conn.Exec(ctx, "TRUNCATE TABLE events"); err != nil {
			t.Fatalf("truncate events: %v", err)
		}
		if err := conn.Exec(ctx, "TRUNCATE TABLE identities"); err != nil {
			t.Fatalf("truncate identities: %v", err)
		}
		return storeHarness{
			store: NewClickHouseStore(conn),
			countEvents: func(t *testing.T) int64 {
				var n uint64
				row := conn.QueryRow(ctx, "SELECT count() FROM events")
				if err := row.Scan(&n); err != nil {
					t.Fatalf("count events: %v", err)
				}
				return int64(n)
			},
			resolveUser: func(t *testing.T, anonymousID string) string {
				var uid *string
				row := conn.QueryRow(ctx,
					"SELECT resolved_user_id FROM events_resolved WHERE anonymous_id = ? LIMIT 1",
					anonymousID)
				if err := row.Scan(&uid); err != nil {
					t.Fatalf("resolve user: %v", err)
				}
				if uid == nil {
					return ""
				}
				return *uid
			},
		}
	})
}
