# Clickstream Analytics Backend — Implementation Plan (Plan 1 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a new Go microservice `services/analytics/` that accepts batched browser clickstream events at `POST /collect`, buffers them in-process, and bulk-inserts them into Postgres with anonymous→identified identity stitching, 90-day retention, and an erasure path.

**Architecture:** Thin fire-and-forget HTTP ingestion (returns `204` fast, never blocks on the DB) → in-process bounded batcher (flush on 500 rows or 1s) → `EventStore` interface with a Postgres implementation (the seam a future ClickHouse impl swaps into). Identity resolution via a SQL view. Public `POST /api/analytics/collect` routed through the gateway (no JWT — anonymous users are tracked). This plan is the backend only; the frontend snippet (Plan 2) and distributed tracing (Plan 3) follow.

**Tech Stack:** Go 1.24, chi/v5 router, GORM (Postgres prod, SQLite in tests), `google/uuid`, `robfig/cron/v3`, Prometheus `promauto`, `libs/{logger,database,httputil,metrics,authz,cache}`. Spec: `docs/superpowers/specs/2026-06-02-analytics-tracing-design.md`.

**Port:** 8092 (8090 notifications, 8091 watch-together are taken).

---

## File Structure

**New service `services/analytics/`:**
- `go.mod`, `go.sum` — module `github.com/ILITA-hub/animeenigma/services/analytics`
- `Dockerfile` — workspace build, mirrors `services/notifications/Dockerfile`
- `cmd/analytics-api/main.go` — boot sequence
- `internal/config/config.go` — env config
- `internal/domain/event.go` — `Event` struct, `EventType` constants, `Validate()`
- `internal/domain/store.go` — `EventStore` interface
- `internal/service/ip.go` — daily-salted IP hashing
- `internal/repo/models.go` — GORM models (`Event`, `Identity`) + `AutoMigrateAll` + `EnsureIndexes` + `EnsureView`
- `internal/repo/postgres_store.go` — `EventStore` impl: `InsertBatch`, `UpsertIdentity`
- `internal/repo/resolve.go` — `ResolvePerson`, `EraseByUserID`, `EraseByAnonymousID`, `PurgeOlderThan`
- `internal/ingest/batcher.go` — in-process bounded buffer + flush loop
- `internal/observ/metrics.go` — Prometheus counters
- `internal/handler/collect.go` — `POST /collect`
- `internal/handler/admin.go` — `POST /internal/erase`
- `internal/job/purge.go` — daily 90-day purge cron
- `internal/transport/router.go` — chi router
- Co-located `_test.go` files for domain, service, repo, ingest, handler, job.

**Modified (gateway wiring):**
- `services/gateway/internal/config/config.go` — add `AnalyticsService` URL
- `services/gateway/internal/config/config_test.go` — env-override test
- `services/gateway/internal/service/proxy.go` — `case "analytics"`
- `services/gateway/internal/handler/proxy.go` — `ProxyToAnalytics`
- `services/gateway/internal/transport/router.go` — public `POST /api/analytics/collect`

**Modified (workspace + infra):**
- `go.work` — add `./services/analytics`
- every `services/*/Dockerfile` that copies go.work — add `COPY services/analytics/go.mod ...`
- `docker/docker-compose.yml` — `analytics` service block + `ANALYTICS_SERVICE_URL` on gateway
- `docker/.env.example` — document new vars

---

## Task 1: Scaffold the module and register it in the workspace

**Files:**
- Create: `services/analytics/go.mod`
- Create: `services/analytics/cmd/analytics-api/main.go` (temporary stub, replaced in Task 10)
- Modify: `go.work`
- Modify: every `services/*/Dockerfile` (add one COPY line)

- [ ] **Step 1: Create the module directory and go.mod**

```bash
cd /data/animeenigma
mkdir -p services/analytics/cmd/analytics-api
mkdir -p services/analytics/internal/{config,domain,service,repo,ingest,observ,handler,job,transport}
```

Create `services/analytics/go.mod`:

```
module github.com/ILITA-hub/animeenigma/services/analytics

go 1.24.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/google/uuid v1.6.0
	github.com/prometheus/client_golang v1.19.0
	github.com/robfig/cron/v3 v3.0.1
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
)
```

- [ ] **Step 2: Create a temporary main.go stub so the module compiles**

Create `services/analytics/cmd/analytics-api/main.go`:

```go
package main

func main() {}
```

- [ ] **Step 3: Add the module to go.work**

In `go.work`, add `./services/analytics` to the `use (` block, keeping alphabetical order (after `./services/`... — place it right after the opening of the services group, before `./services/auth`):

```
	./services/analytics
	./services/auth
```

- [ ] **Step 4: Add the COPY line to every sibling Dockerfile**

Every `services/*/Dockerfile` that copies `go.work` lists each module's go.mod so workspace `go mod download` resolves. Add the analytics line next to the others. Run this to insert it after the watch-together COPY line in each Dockerfile:

```bash
cd /data/animeenigma
for df in services/*/Dockerfile; do
  if grep -q 'COPY services/watch-together/go.mod' "$df" && ! grep -q 'COPY services/analytics/go.mod' "$df"; then
    sed -i '/COPY services\/watch-together\/go.mod/a COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/' "$df"
  fi
done
grep -l 'COPY services/analytics/go.mod' services/*/Dockerfile
```

Expected: lists most/all service Dockerfiles. (The analytics service's own Dockerfile is created in Task 10 and does not need this self-COPY beyond its own module.)

- [ ] **Step 5: Sync the workspace and verify it builds**

```bash
cd /data/animeenigma && go work sync && cd services/analytics && go build ./... && echo "BUILD OK"
```

Expected: `BUILD OK` (creates `go.sum`).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add go.work go.work.sum services/analytics/go.mod services/analytics/go.sum services/analytics/cmd services/*/Dockerfile
git commit -m "chore(analytics): scaffold analytics service module + workspace wiring"
```

---

## Task 2: Domain — Event type and validation

**Files:**
- Create: `services/analytics/internal/domain/event.go`
- Test: `services/analytics/internal/domain/event_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/domain/event_test.go`:

```go
package domain

import "testing"

func TestEvent_Validate(t *testing.T) {
	base := func() Event {
		return Event{
			EventType:   EventTypePageview,
			AnonymousID: "anon-1",
			SessionID:   "sess-1",
		}
	}

	t.Run("valid pageview passes", func(t *testing.T) {
		e := base()
		if err := e.Validate(); err != nil {
			t.Fatalf("expected valid, got %v", err)
		}
	})

	t.Run("missing anonymous_id fails", func(t *testing.T) {
		e := base()
		e.AnonymousID = ""
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for empty anonymous_id")
		}
	})

	t.Run("missing session_id fails", func(t *testing.T) {
		e := base()
		e.SessionID = ""
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for empty session_id")
		}
	})

	t.Run("unknown event_type fails", func(t *testing.T) {
		e := base()
		e.EventType = "bogus"
		if err := e.Validate(); err == nil {
			t.Fatal("expected error for unknown event_type")
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/domain/ -run TestEvent_Validate -v`
Expected: FAIL — `Event` / `EventTypePageview` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/domain/event.go`:

```go
// Package domain holds the analytics clickstream event model and the
// EventStore port. Events are produced by the browser snippet (Plan 2),
// validated here, and persisted via an EventStore implementation.
package domain

import (
	"fmt"
	"time"
)

// EventType enumerates the clickstream event kinds the snippet emits.
type EventType string

const (
	EventTypePageview  EventType = "pageview"
	EventTypeClick     EventType = "click"
	EventTypeHeartbeat EventType = "heartbeat"
	EventTypeIdentify  EventType = "identify"
	EventTypeCustom    EventType = "custom"
)

func (t EventType) valid() bool {
	switch t {
	case EventTypePageview, EventTypeClick, EventTypeHeartbeat, EventTypeIdentify, EventTypeCustom:
		return true
	default:
		return false
	}
}

// Event is one clickstream event. anonymous_id and session_id are always
// present; user_id is set only once the visitor is identified.
type Event struct {
	EventID     string
	EventType   EventType
	EventName   string
	AnonymousID string
	UserID      string // empty when unknown
	SessionID   string
	Timestamp   time.Time
	ReceivedAt  time.Time

	URL      string
	Path     string
	Referrer string
	Title    string

	ElSelector string
	ElText     string
	ElTag      string
	ElAttrs    string // JSON string, default "{}"

	ActiveMS int // heartbeat foreground time

	UserAgent  string
	DeviceType string
	ScreenW    int
	ScreenH    int
	IPHash     string

	TraceID    string
	Properties string // JSON string, default "{}"
}

// Validate enforces the always-present fields and a known event_type.
func (e Event) Validate() error {
	if e.AnonymousID == "" {
		return fmt.Errorf("anonymous_id is required")
	}
	if e.SessionID == "" {
		return fmt.Errorf("session_id is required")
	}
	if !e.EventType.valid() {
		return fmt.Errorf("unknown event_type: %q", e.EventType)
	}
	return nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/domain/ -run TestEvent_Validate -v`
Expected: PASS (all sub-tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/domain/
git commit -m "feat(analytics): domain Event type with validation"
```

---

## Task 3: IP hashing service (daily-salted, never store raw)

**Files:**
- Create: `services/analytics/internal/service/ip.go`
- Test: `services/analytics/internal/service/ip_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/service/ip_test.go`:

```go
package service

import (
	"testing"
	"time"
)

func TestHashIP(t *testing.T) {
	day := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)

	t.Run("deterministic within the same day", func(t *testing.T) {
		a := HashIP("203.0.113.7", "secret-salt", day)
		b := HashIP("203.0.113.7", "secret-salt", day.Add(3*time.Hour))
		if a != b {
			t.Fatalf("same ip+day must hash equal: %s != %s", a, b)
		}
	})

	t.Run("changes the next day", func(t *testing.T) {
		a := HashIP("203.0.113.7", "secret-salt", day)
		b := HashIP("203.0.113.7", "secret-salt", day.Add(24*time.Hour))
		if a == b {
			t.Fatal("ip hash must rotate daily")
		}
	})

	t.Run("does not contain the raw ip", func(t *testing.T) {
		h := HashIP("203.0.113.7", "secret-salt", day)
		if h == "" || len(h) != 64 {
			t.Fatalf("expected 64-char hex sha256, got %q", h)
		}
	})

	t.Run("empty ip yields empty hash", func(t *testing.T) {
		if HashIP("", "salt", day) != "" {
			t.Fatal("empty ip must yield empty hash")
		}
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/service/ -run TestHashIP -v`
Expected: FAIL — `HashIP` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/service/ip.go`:

```go
// Package service holds stateless helpers for the analytics service.
package service

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// HashIP returns sha256(ip + dailySalt) as lowercase hex, where dailySalt
// = configured salt + the UTC date. The daily rotation means the hash
// cannot be reversed to the raw IP across days, and we NEVER persist the
// raw IP. Empty ip → empty hash.
func HashIP(ip, salt string, now time.Time) string {
	if ip == "" {
		return ""
	}
	day := now.UTC().Format("2006-01-02")
	sum := sha256.Sum256([]byte(ip + "|" + salt + "|" + day))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/service/ -run TestHashIP -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/service/
git commit -m "feat(analytics): daily-salted IP hashing (never store raw IP)"
```

---

## Task 4: Postgres store — models, migration, indexes, view, InsertBatch, UpsertIdentity

**Files:**
- Create: `services/analytics/internal/domain/store.go`
- Create: `services/analytics/internal/repo/models.go`
- Create: `services/analytics/internal/repo/postgres_store.go`
- Test: `services/analytics/internal/repo/postgres_store_test.go`

- [ ] **Step 1: Define the EventStore port**

Create `services/analytics/internal/domain/store.go`:

```go
package domain

import (
	"context"
	"time"
)

// EventStore is the write port for clickstream persistence. The Postgres
// implementation backs v1; a ClickHouse implementation can replace it
// later without touching the ingestion path (see spec §3.2, deferred).
type EventStore interface {
	InsertBatch(ctx context.Context, events []Event) error
	UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error
}
```

- [ ] **Step 2: Write the failing test (sqlite-backed)**

Create `services/analytics/internal/repo/postgres_store_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

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

func TestInsertBatch(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	events := []domain.Event{
		{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now()},
		{EventID: "e2", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now(), ElSelector: "button#buy"},
	}
	if err := store.InsertBatch(ctx, events); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	var count int64
	db.Model(&Event{}).Count(&count)
	if count != 2 {
		t.Fatalf("expected 2 rows, got %d", count)
	}
}

func TestInsertBatch_Empty(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	if err := store.InsertBatch(context.Background(), nil); err != nil {
		t.Fatalf("empty batch must be a no-op, got %v", err)
	}
}

func TestUpsertIdentity(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	if err := store.UpsertIdentity(ctx, "a1", "u1", time.Now()); err != nil {
		t.Fatalf("UpsertIdentity: %v", err)
	}
	var count int64
	db.Model(&Identity{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 identity row, got %d", count)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/repo/ -run 'TestInsertBatch|TestUpsertIdentity' -v`
Expected: FAIL — `AutoMigrateAll`, `EnsureView`, `NewPostgresStore`, `Event`, `Identity` undefined.

- [ ] **Step 4: Write the models**

Create `services/analytics/internal/repo/models.go`:

```go
// Package repo holds the Postgres-backed EventStore and the GORM models.
package repo

import (
	"time"

	"gorm.io/gorm"
)

// Event is the GORM model for the analytics_events table. JSON-shaped
// columns (el_attrs, properties) are stored as text holding JSON for
// portability (sqlite tests + postgres prod); they can be ALTERed to
// jsonb later if JSON operators are needed.
type Event struct {
	EventID     string    `gorm:"column:event_id;primaryKey"`
	EventType   string    `gorm:"column:event_type;index:idx_evt_type_ts,priority:1"`
	EventName   string    `gorm:"column:event_name;default:''"`
	AnonymousID string    `gorm:"column:anonymous_id;index:idx_anon_ts,priority:1"`
	UserID      *string   `gorm:"column:user_id"`
	SessionID   string    `gorm:"column:session_id;index"`
	Timestamp   time.Time `gorm:"column:timestamp;index:idx_anon_ts,priority:2;index:idx_ts"`
	ReceivedAt  time.Time `gorm:"column:received_at"`

	URL      string `gorm:"column:url"`
	Path     string `gorm:"column:path"`
	Referrer string `gorm:"column:referrer;default:''"`
	Title    string `gorm:"column:title;default:''"`

	ElSelector string `gorm:"column:el_selector"`
	ElText     string `gorm:"column:el_text"`
	ElTag      string `gorm:"column:el_tag"`
	ElAttrs    string `gorm:"column:el_attrs;type:text;default:'{}'"`

	ActiveMS int `gorm:"column:active_ms"`

	UserAgent  string `gorm:"column:user_agent;default:''"`
	DeviceType string `gorm:"column:device_type;default:''"`
	ScreenW    int    `gorm:"column:screen_w;default:0"`
	ScreenH    int    `gorm:"column:screen_h;default:0"`
	IPHash     string `gorm:"column:ip_hash;default:''"`

	TraceID    string `gorm:"column:trace_id"`
	Properties string `gorm:"column:properties;type:text;default:'{}'"`
}

func (Event) TableName() string { return "analytics_events" }

// Identity maps an anonymous_id to a user_id at a point in time. Append
// only; the latest row per anonymous_id wins (see EnsureView).
type Identity struct {
	ID          uint      `gorm:"primaryKey"`
	AnonymousID string    `gorm:"column:anonymous_id;index:idx_ident_anon_ts,priority:1"`
	UserID      string    `gorm:"column:user_id"`
	Timestamp   time.Time `gorm:"column:timestamp;index:idx_ident_anon_ts,priority:2"`
}

func (Identity) TableName() string { return "analytics_identities" }

// AutoMigrateAll creates the service-owned tables.
func AutoMigrateAll(db *gorm.DB) error {
	return db.AutoMigrate(&Event{}, &Identity{})
}

// EnsureView creates analytics_events_resolved, which adds resolved_user_id
// and person_id (canonical identity: user if known, else anonymous). Uses a
// correlated subquery so it runs identically on sqlite and postgres.
func EnsureView(db *gorm.DB) error {
	return db.Exec(`CREATE VIEW IF NOT EXISTS analytics_events_resolved AS
SELECT e.*,
  COALESCE(e.user_id,
    (SELECT i.user_id FROM analytics_identities i
      WHERE i.anonymous_id = e.anonymous_id ORDER BY i.timestamp DESC LIMIT 1)
  ) AS resolved_user_id,
  COALESCE(e.user_id,
    (SELECT i.user_id FROM analytics_identities i
      WHERE i.anonymous_id = e.anonymous_id ORDER BY i.timestamp DESC LIMIT 1),
    e.anonymous_id
  ) AS person_id
FROM analytics_events e`).Error
}
```

- [ ] **Step 5: Write the store**

Create `services/analytics/internal/repo/postgres_store.go`:

```go
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"gorm.io/gorm"
)

// PostgresStore implements domain.EventStore against GORM (Postgres in
// prod, sqlite in tests).
type PostgresStore struct{ db *gorm.DB }

func NewPostgresStore(db *gorm.DB) *PostgresStore { return &PostgresStore{db: db} }

func toModel(e domain.Event) Event {
	m := Event{
		EventID: e.EventID, EventType: string(e.EventType), EventName: e.EventName,
		AnonymousID: e.AnonymousID, SessionID: e.SessionID,
		Timestamp: e.Timestamp, ReceivedAt: e.ReceivedAt,
		URL: e.URL, Path: e.Path, Referrer: e.Referrer, Title: e.Title,
		ElSelector: e.ElSelector, ElText: e.ElText, ElTag: e.ElTag, ElAttrs: e.ElAttrs,
		ActiveMS: e.ActiveMS, UserAgent: e.UserAgent, DeviceType: e.DeviceType,
		ScreenW: e.ScreenW, ScreenH: e.ScreenH, IPHash: e.IPHash,
		TraceID: e.TraceID, Properties: e.Properties,
	}
	if e.UserID != "" {
		uid := e.UserID
		m.UserID = &uid
	}
	if m.ElAttrs == "" {
		m.ElAttrs = "{}"
	}
	if m.Properties == "" {
		m.Properties = "{}"
	}
	return m
}

func (s *PostgresStore) InsertBatch(ctx context.Context, events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	rows := make([]Event, 0, len(events))
	for _, e := range events {
		rows = append(rows, toModel(e))
	}
	return s.db.WithContext(ctx).CreateInBatches(rows, 200).Error
}

func (s *PostgresStore) UpsertIdentity(ctx context.Context, anonymousID, userID string, ts time.Time) error {
	if anonymousID == "" || userID == "" {
		return nil
	}
	return s.db.WithContext(ctx).Create(&Identity{
		AnonymousID: anonymousID, UserID: userID, Timestamp: ts,
	}).Error
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/repo/ -run 'TestInsertBatch|TestUpsertIdentity' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/domain/store.go services/analytics/internal/repo/
git commit -m "feat(analytics): Postgres EventStore, models, migration, resolved view"
```

---

## Task 5: Identity resolution, erasure, and purge queries

**Files:**
- Create: `services/analytics/internal/repo/resolve.go`
- Test: `services/analytics/internal/repo/resolve_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/repo/resolve_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

func TestResolvePerson_StitchesAfterIdentify(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()

	// Anonymous event, then the visitor identifies.
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: time.Now()},
	})
	_ = store.UpsertIdentity(ctx, "a1", "u1", time.Now())

	person, err := ResolvePerson(ctx, db, "e1")
	if err != nil {
		t.Fatalf("ResolvePerson: %v", err)
	}
	if person != "u1" {
		t.Fatalf("expected anonymous event to resolve to u1, got %q", person)
	}
}

func TestEraseByUserID(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()
	uid := "u1"
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "e1", EventType: domain.EventTypeClick, AnonymousID: "a1", UserID: uid, SessionID: "s1", Timestamp: time.Now()},
	})
	_ = store.UpsertIdentity(ctx, "a1", uid, time.Now())

	if err := EraseByUserID(ctx, db, uid); err != nil {
		t.Fatalf("EraseByUserID: %v", err)
	}
	var ev, id int64
	db.Model(&Event{}).Count(&ev)
	db.Model(&Identity{}).Count(&id)
	if ev != 0 || id != 0 {
		t.Fatalf("erase left rows: events=%d identities=%d", ev, id)
	}
}

func TestPurgeOlderThan(t *testing.T) {
	db := newTestDB(t)
	store := NewPostgresStore(db)
	ctx := context.Background()
	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now()
	_ = store.InsertBatch(ctx, []domain.Event{
		{EventID: "old", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: old},
		{EventID: "new", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1", Timestamp: recent},
	})

	n, err := PurgeOlderThan(ctx, db, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("PurgeOlderThan: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}
	var count int64
	db.Model(&Event{}).Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row remaining, got %d", count)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/repo/ -run 'TestResolvePerson|TestErase|TestPurge' -v`
Expected: FAIL — `ResolvePerson`, `EraseByUserID`, `PurgeOlderThan` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/repo/resolve.go`:

```go
package repo

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// ResolvePerson returns the canonical person_id for an event via the
// analytics_events_resolved view (identified user if known, else anon).
func ResolvePerson(ctx context.Context, db *gorm.DB, eventID string) (string, error) {
	var personID string
	err := db.WithContext(ctx).
		Table("analytics_events_resolved").
		Where("event_id = ?", eventID).
		Select("person_id").
		Scan(&personID).Error
	return personID, err
}

// EraseByUserID deletes all events + identities for a user_id. Covers both
// directly-identified events and anonymous events stitched via identities.
func EraseByUserID(ctx context.Context, db *gorm.DB, userID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Anonymous ids that map to this user.
		var anonIDs []string
		if err := tx.Model(&Identity{}).Where("user_id = ?", userID).
			Distinct().Pluck("anonymous_id", &anonIDs).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).Delete(&Event{}).Error; err != nil {
			return err
		}
		if len(anonIDs) > 0 {
			if err := tx.Where("anonymous_id IN ?", anonIDs).Delete(&Event{}).Error; err != nil {
				return err
			}
		}
		return tx.Where("user_id = ?", userID).Delete(&Identity{}).Error
	})
}

// EraseByAnonymousID deletes all events + identities for an anonymous_id.
func EraseByAnonymousID(ctx context.Context, db *gorm.DB, anonymousID string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("anonymous_id = ?", anonymousID).Delete(&Event{}).Error; err != nil {
			return err
		}
		return tx.Where("anonymous_id = ?", anonymousID).Delete(&Identity{}).Error
	})
}

// PurgeOlderThan deletes events with timestamp < cutoff. Returns the count
// deleted. Backs the 90-day retention cron.
func PurgeOlderThan(ctx context.Context, db *gorm.DB, cutoff time.Time) (int64, error) {
	res := db.WithContext(ctx).Where("timestamp < ?", cutoff).Delete(&Event{})
	return res.RowsAffected, res.Error
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/repo/ -v`
Expected: PASS (all repo tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/repo/resolve.go services/analytics/internal/repo/resolve_test.go
git commit -m "feat(analytics): identity resolution, erasure, and retention purge queries"
```

---

## Task 6: In-process batcher

**Files:**
- Create: `services/analytics/internal/ingest/batcher.go`
- Test: `services/analytics/internal/ingest/batcher_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/ingest/batcher_test.go`:

```go
package ingest

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type fakeStore struct {
	mu        sync.Mutex
	inserted  []domain.Event
	identifies int
}

func (f *fakeStore) InsertBatch(_ context.Context, e []domain.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserted = append(f.inserted, e...)
	return nil
}
func (f *fakeStore) UpsertIdentity(_ context.Context, _, _ string, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.identifies++
	return nil
}
func (f *fakeStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.inserted)
}

func TestBatcher_FlushesOnInterval(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 1000, FlushInterval: 20 * time.Millisecond, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypePageview, AnonymousID: "a1", SessionID: "s1"})

	deadline := time.After(time.Second)
	for store.count() == 0 {
		select {
		case <-deadline:
			t.Fatal("event was not flushed within 1s")
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatcher_FlushesOnSize(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 2, FlushInterval: time.Hour, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1"})
	b.Enqueue(domain.Event{EventID: "e2", EventType: domain.EventTypeClick, AnonymousID: "a1", SessionID: "s1"})

	deadline := time.After(time.Second)
	for store.count() < 2 {
		select {
		case <-deadline:
			t.Fatalf("size-triggered flush failed, got %d", store.count())
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func TestBatcher_IdentifyUpsert(t *testing.T) {
	store := &fakeStore{}
	b := New(store, Config{MaxBatch: 1, FlushInterval: time.Hour, BufferSize: 100})
	b.Start()
	defer b.Stop()

	b.Enqueue(domain.Event{EventID: "e1", EventType: domain.EventTypeIdentify, AnonymousID: "a1", UserID: "u1", SessionID: "s1", Timestamp: time.Now()})

	deadline := time.After(time.Second)
	for store.count() < 1 {
		select {
		case <-deadline:
			t.Fatal("identify event not flushed")
		case <-time.After(5 * time.Millisecond):
		}
	}
	store.mu.Lock()
	got := store.identifies
	store.mu.Unlock()
	if got != 1 {
		t.Fatalf("expected 1 identity upsert, got %d", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/ingest/ -v`
Expected: FAIL — `New`, `Config`, batcher methods undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/ingest/batcher.go`:

```go
// Package ingest buffers clickstream events in memory and bulk-writes them
// to the EventStore. Best-effort durability: a full buffer drops events
// (analytics is not billing data — see spec §3.1). Redis Streams is the
// documented upgrade path.
package ingest

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type Config struct {
	MaxBatch      int           // flush when buffer reaches this many rows
	FlushInterval time.Duration // flush at least this often
	BufferSize    int           // channel capacity; full channel drops events
}

// Batcher accepts events via Enqueue and flushes them to the store.
type Batcher struct {
	store  domain.EventStore
	cfg    Config
	ch     chan domain.Event
	stop   chan struct{}
	done   chan struct{}
	log    *logger.Logger
	onDrop func() // hook for metrics; nil-safe
}

func New(store domain.EventStore, cfg Config) *Batcher {
	if cfg.MaxBatch <= 0 {
		cfg.MaxBatch = 500
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = time.Second
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10000
	}
	return &Batcher{
		store: store, cfg: cfg,
		ch:   make(chan domain.Event, cfg.BufferSize),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// WithLogger and WithDropHook are optional configuration.
func (b *Batcher) WithLogger(l *logger.Logger) *Batcher { b.log = l; return b }
func (b *Batcher) WithDropHook(f func()) *Batcher        { b.onDrop = f; return b }

// Enqueue adds an event without blocking. Returns false if the buffer is
// full (event dropped).
func (b *Batcher) Enqueue(e domain.Event) bool {
	select {
	case b.ch <- e:
		return true
	default:
		if b.onDrop != nil {
			b.onDrop()
		}
		return false
	}
}

func (b *Batcher) Start() {
	go b.run()
}

func (b *Batcher) run() {
	defer close(b.done)
	ticker := time.NewTicker(b.cfg.FlushInterval)
	defer ticker.Stop()
	buf := make([]domain.Event, 0, b.cfg.MaxBatch)

	flush := func() {
		if len(buf) == 0 {
			return
		}
		b.flush(buf)
		buf = buf[:0]
	}

	for {
		select {
		case e := <-b.ch:
			buf = append(buf, e)
			if len(buf) >= b.cfg.MaxBatch {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-b.stop:
			// Drain whatever is buffered, then exit.
			for {
				select {
				case e := <-b.ch:
					buf = append(buf, e)
				default:
					flush()
					return
				}
			}
		}
	}
}

func (b *Batcher) flush(events []domain.Event) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	batch := make([]domain.Event, len(events))
	copy(batch, events)
	if err := b.store.InsertBatch(ctx, batch); err != nil && b.log != nil {
		b.log.Errorw("analytics insert batch failed", "count", len(batch), "error", err)
	}
	// Identify events also upsert the anon→user mapping.
	for _, e := range batch {
		if e.EventType == domain.EventTypeIdentify && e.UserID != "" {
			if err := b.store.UpsertIdentity(ctx, e.AnonymousID, e.UserID, e.Timestamp); err != nil && b.log != nil {
				b.log.Errorw("analytics upsert identity failed", "anon", e.AnonymousID, "error", err)
			}
		}
	}
}

// Stop signals the flush loop to drain and exit, then waits for it.
func (b *Batcher) Stop() {
	close(b.stop)
	<-b.done
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/ingest/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/ingest/
git commit -m "feat(analytics): in-process batcher (size+interval flush, best-effort)"
```

---

## Task 7: Collect handler (`POST /collect`)

**Files:**
- Create: `services/analytics/internal/handler/collect.go`
- Test: `services/analytics/internal/handler/collect_test.go`

The wire contract (spec §5): the beacon POSTs a JSON envelope with `text/plain` content-type. The body is `{ anonymous_id, user_id, session_id, events: [{event_type, timestamp, ...}], ctx: {user_agent, screen_w, screen_h} }`.

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/handler/collect_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
)

type capturingSink struct {
	mu     sync.Mutex
	events []domain.Event
}

func (c *capturingSink) Enqueue(e domain.Event) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
	return true
}
func (c *capturingSink) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

func TestCollect_AcceptsBatch(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "test-salt")

	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"pageview","path":"/pricing"},
	  {"event_type":"click","path":"/pricing","el_selector":"button#buy"}
	],"ctx":{"user_agent":"UA","screen_w":1920,"screen_h":1080}}`

	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "text/plain")
	req.RemoteAddr = "203.0.113.7:5555"
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 2 {
		t.Fatalf("expected 2 events enqueued, got %d", sink.count())
	}
	sink.mu.Lock()
	first := sink.events[0]
	sink.mu.Unlock()
	if first.AnonymousID != "a1" || first.SessionID != "s1" {
		t.Fatalf("envelope fields not applied: %+v", first)
	}
	if first.IPHash == "" || strings.Contains(first.IPHash, "203.0.113.7") {
		t.Fatalf("ip must be hashed, got %q", first.IPHash)
	}
	if first.UserAgent != "UA" || first.ScreenW != 1920 {
		t.Fatalf("ctx not applied: %+v", first)
	}
	if first.EventID == "" {
		t.Fatal("event_id must be assigned")
	}
}

func TestCollect_RejectsMalformed(t *testing.T) {
	h := NewCollectHandler(&capturingSink{}, "salt")
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCollect_SkipsInvalidEventsButAcceptsRest(t *testing.T) {
	sink := &capturingSink{}
	h := NewCollectHandler(sink, "salt")
	// Second event has an unknown type and must be skipped, not 400.
	body := `{"anonymous_id":"a1","session_id":"s1","events":[
	  {"event_type":"pageview","path":"/"},
	  {"event_type":"bogus","path":"/"}
	]}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if sink.count() != 1 {
		t.Fatalf("expected 1 valid event, got %d", sink.count())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/handler/ -run TestCollect -v`
Expected: FAIL — `NewCollectHandler` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/handler/collect.go`:

```go
// Package handler holds the analytics HTTP handlers.
package handler

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
)

// Sink is the subset of the batcher the handler needs (enables a fake in
// tests).
type Sink interface {
	Enqueue(domain.Event) bool
}

// CollectHandler parses beacon batches and enqueues validated events.
type CollectHandler struct {
	sink Sink
	salt string
}

func NewCollectHandler(sink Sink, ipSalt string) *CollectHandler {
	return &CollectHandler{sink: sink, salt: ipSalt}
}

// wire types mirror the snippet contract (spec §5).
type wireCtx struct {
	UserAgent string `json:"user_agent"`
	ScreenW   int    `json:"screen_w"`
	ScreenH   int    `json:"screen_h"`
}

type wireEvent struct {
	EventType  string            `json:"event_type"`
	EventName  string            `json:"event_name"`
	Timestamp  time.Time         `json:"timestamp"`
	URL        string            `json:"url"`
	Path       string            `json:"path"`
	Referrer   string            `json:"referrer"`
	Title      string            `json:"title"`
	ElSelector string            `json:"el_selector"`
	ElText     string            `json:"el_text"`
	ElTag      string            `json:"el_tag"`
	ElAttrs    map[string]string `json:"el_attrs"`
	ActiveMS   int               `json:"active_ms"`
	TraceID    string            `json:"trace_id"`
	Properties json.RawMessage   `json:"properties"`
}

type wireEnvelope struct {
	AnonymousID string      `json:"anonymous_id"`
	UserID      string      `json:"user_id"`
	SessionID   string      `json:"session_id"`
	Events      []wireEvent `json:"events"`
	Ctx         wireCtx     `json:"ctx"`
}

func (h *CollectHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 256*1024)) // 256 KB cap
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}
	var env wireEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	now := time.Now().UTC()
	ipHash := service.HashIP(clientIP(r), h.salt, now)

	for _, we := range env.Events {
		ts := we.Timestamp
		if ts.IsZero() {
			ts = now
		}
		// Drop events with absurd clock skew (>1 day either way).
		if ts.After(now.Add(24*time.Hour)) || ts.Before(now.Add(-24*time.Hour)) {
			continue
		}
		attrs := "{}"
		if len(we.ElAttrs) > 0 {
			if b, err := json.Marshal(we.ElAttrs); err == nil {
				attrs = string(b)
			}
		}
		props := "{}"
		if len(we.Properties) > 0 {
			props = string(we.Properties)
		}
		ev := domain.Event{
			EventID:     uuid.NewString(),
			EventType:   domain.EventType(we.EventType),
			EventName:   we.EventName,
			AnonymousID: env.AnonymousID,
			UserID:      env.UserID,
			SessionID:   env.SessionID,
			Timestamp:   ts.UTC(),
			ReceivedAt:  now,
			URL:         we.URL, Path: we.Path, Referrer: we.Referrer, Title: we.Title,
			ElSelector: we.ElSelector, ElText: we.ElText, ElTag: we.ElTag, ElAttrs: attrs,
			ActiveMS:   we.ActiveMS,
			UserAgent:  env.Ctx.UserAgent, ScreenW: env.Ctx.ScreenW, ScreenH: env.Ctx.ScreenH,
			IPHash:  ipHash,
			TraceID: we.TraceID, Properties: props,
		}
		if err := ev.Validate(); err != nil {
			continue // skip the bad event, keep the rest
		}
		h.sink.Enqueue(ev)
	}

	w.WriteHeader(http.StatusNoContent)
}

func clientIP(r *http.Request) string {
	// The gateway sets X-Forwarded-For / X-Real-IP (chi middleware.RealIP
	// is applied upstream); fall back to RemoteAddr.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if comma := indexByte(xff, ','); comma > 0 {
			return trimSpace(xff[:comma])
		}
		return trimSpace(xff)
	}
	if rip := r.Header.Get("X-Real-IP"); rip != "" {
		return rip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/handler/ -run TestCollect -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/handler/collect.go services/analytics/internal/handler/collect_test.go
git commit -m "feat(analytics): POST /collect beacon handler (parse, hash IP, enqueue)"
```

---

## Task 8: Erasure handler (`POST /internal/erase`)

**Files:**
- Create: `services/analytics/internal/handler/admin.go`
- Test: `services/analytics/internal/handler/admin_test.go`

This is an internal endpoint (Docker-network-only; the gateway never proxies `/internal/*`). It accepts `{ "user_id": "..." }` or `{ "anonymous_id": "..." }`.

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/handler/admin_test.go`:

```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeEraser struct {
	byUser, byAnon string
}

func (f *fakeEraser) EraseByUserID(_ context.Context, id string) error      { f.byUser = id; return nil }
func (f *fakeEraser) EraseByAnonymousID(_ context.Context, id string) error { f.byAnon = id; return nil }

func TestErase_ByUserID(t *testing.T) {
	er := &fakeEraser{}
	h := NewAdminHandler(er)
	req := httptest.NewRequest(http.MethodPost, "/internal/erase", strings.NewReader(`{"user_id":"u1"}`))
	rec := httptest.NewRecorder()
	h.Erase(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if er.byUser != "u1" {
		t.Fatalf("expected erase by user u1, got %q", er.byUser)
	}
}

func TestErase_RequiresAnIdentifier(t *testing.T) {
	h := NewAdminHandler(&fakeEraser{})
	req := httptest.NewRequest(http.MethodPost, "/internal/erase", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.Erase(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/handler/ -run TestErase -v`
Expected: FAIL — `NewAdminHandler` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/handler/admin.go`:

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// Eraser is the erasure port (implemented by repo functions wrapped at
// wire-up time).
type Eraser interface {
	EraseByUserID(ctx context.Context, userID string) error
	EraseByAnonymousID(ctx context.Context, anonymousID string) error
}

type AdminHandler struct{ eraser Eraser }

func NewAdminHandler(e Eraser) *AdminHandler { return &AdminHandler{eraser: e} }

type eraseReq struct {
	UserID      string `json:"user_id"`
	AnonymousID string `json:"anonymous_id"`
}

// Erase implements the right-to-erasure path (spec §5).
func (h *AdminHandler) Erase(w http.ResponseWriter, r *http.Request) {
	var req eraseReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid json")
		return
	}
	switch {
	case req.UserID != "":
		if err := h.eraser.EraseByUserID(r.Context(), req.UserID); err != nil {
			httputil.Error(w, err)
			return
		}
	case req.AnonymousID != "":
		if err := h.eraser.EraseByAnonymousID(r.Context(), req.AnonymousID); err != nil {
			httputil.Error(w, err)
			return
		}
	default:
		httputil.BadRequest(w, "user_id or anonymous_id is required")
		return
	}
	httputil.OK(w, map[string]string{"status": "erased"})
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/handler/ -run TestErase -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/handler/admin.go services/analytics/internal/handler/admin_test.go
git commit -m "feat(analytics): internal erasure endpoint (right to erasure)"
```

---

## Task 9: 90-day purge cron job

**Files:**
- Create: `services/analytics/internal/job/purge.go`
- Test: `services/analytics/internal/job/purge_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/analytics/internal/job/purge_test.go`:

```go
package job

import (
	"context"
	"testing"
	"time"
)

type fakePurger struct {
	cutoff time.Time
	called int
}

func (f *fakePurger) PurgeOlderThan(_ context.Context, cutoff time.Time) (int64, error) {
	f.called++
	f.cutoff = cutoff
	return 3, nil
}

func TestPurgeJob_RunOnce(t *testing.T) {
	p := &fakePurger{}
	j := NewPurgeJob(p, 90, nil)
	n, err := j.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 purged, got %d", n)
	}
	// cutoff must be ~90 days ago.
	wantApprox := time.Now().Add(-90 * 24 * time.Hour)
	if diff := p.cutoff.Sub(wantApprox); diff > time.Minute || diff < -time.Minute {
		t.Fatalf("cutoff off by %v", diff)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd services/analytics && go test ./internal/job/ -v`
Expected: FAIL — `NewPurgeJob` undefined.

- [ ] **Step 3: Write the implementation**

Create `services/analytics/internal/job/purge.go`:

```go
// Package job holds the analytics background cron jobs.
package job

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// Purger deletes events older than a cutoff (implemented by repo).
type Purger interface {
	PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error)
}

// PurgeJob enforces the retention window (spec §5). Runs in-service (like
// the notifications cleanup cron) rather than the scheduler service, to
// avoid cross-service coupling.
type PurgeJob struct {
	purger        Purger
	retentionDays int
	log           *logger.Logger
}

func NewPurgeJob(p Purger, retentionDays int, log *logger.Logger) *PurgeJob {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	return &PurgeJob{purger: p, retentionDays: retentionDays, log: log}
}

// RunOnce executes a single purge pass and returns the rows deleted.
func (j *PurgeJob) RunOnce(ctx context.Context) (int64, error) {
	cutoff := time.Now().Add(-time.Duration(j.retentionDays) * 24 * time.Hour)
	n, err := j.purger.PurgeOlderThan(ctx, cutoff)
	if err != nil {
		if j.log != nil {
			j.log.Errorw("analytics purge failed", "error", err)
		}
		return 0, err
	}
	if j.log != nil {
		j.log.Infow("analytics purge complete", "deleted", n, "cutoff", cutoff)
	}
	return n, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd services/analytics && go test ./internal/job/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/analytics/internal/job/
git commit -m "feat(analytics): 90-day retention purge job"
```

---

## Task 10: Wire-up — metrics, config, router, main.go, Dockerfile, compose

**Files:**
- Create: `services/analytics/internal/observ/metrics.go`
- Create: `services/analytics/internal/config/config.go`
- Create: `services/analytics/internal/transport/router.go`
- Replace: `services/analytics/cmd/analytics-api/main.go`
- Create: `services/analytics/Dockerfile`
- Modify: `docker/docker-compose.yml`
- Modify: `docker/.env.example`

- [ ] **Step 1: Prometheus counters**

Create `services/analytics/internal/observ/metrics.go`:

```go
// Package observ holds analytics Prometheus metrics, auto-registered to the
// default registry that /metrics serves.
package observ

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	EventsReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_events_received_total",
		Help: "Clickstream events accepted at /collect.",
	})
	EventsDropped = promauto.NewCounter(prometheus.CounterOpts{
		Name: "analytics_events_dropped_total",
		Help: "Clickstream events dropped because the in-process buffer was full.",
	})
)
```

- [ ] **Step 2: Config**

Create `services/analytics/internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server        ServerConfig
	Database      database.Config
	IPSalt        string
	RetentionDays int
	PurgeCron     string
	MaxBatch      int
	FlushInterval time.Duration
	BufferSize    int
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8092),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "postgres"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "animeenigma"),
		},
		IPSalt:        getEnv("ANALYTICS_IP_SALT", "change-me-in-production"),
		RetentionDays: getEnvInt("ANALYTICS_RETENTION_DAYS", 90),
		PurgeCron:     getEnv("ANALYTICS_PURGE_CRON", "17 3 * * *"),
		MaxBatch:      getEnvInt("ANALYTICS_MAX_BATCH", 500),
		FlushInterval: getEnvDuration("ANALYTICS_FLUSH_INTERVAL", time.Second),
		BufferSize:    getEnvInt("ANALYTICS_BUFFER_SIZE", 10000),
	}, nil
}

func getEnv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func getEnvInt(k string, d int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return d
}
func getEnvDuration(k string, d time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if p, err := time.ParseDuration(v); err == nil {
			return p
		}
	}
	return d
}
```

> Note: confirm the field names on `database.Config` (`Host`, `Port`, `User`, `Password`, `DBName`) by reading `libs/database/`; adjust if the struct differs.

- [ ] **Step 3: Router**

Create `services/analytics/internal/transport/router.go`:

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter builds the analytics chi router.
//
//	GET  /health                 (public)
//	GET  /metrics                 (public, prom format)
//	POST /collect                 (public — anonymous users tracked)
//	POST /internal/erase          (internal — gateway never proxies /internal/*)
func NewRouter(
	collect *handler.CollectHandler,
	admin *handler.AdminHandler,
	log *logger.Logger,
	collector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(collector.Middleware)
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

	r.Post("/collect", collect.ServeHTTP)
	r.Post("/internal/erase", admin.Erase)

	return r
}
```

> Note: confirm `httputil.RequestLogger`, `httputil.Recoverer`, `httputil.CORS`, and `metrics.Collector`/`metrics.Handler` signatures against `services/notifications/internal/transport/router.go` (copied from there); adjust if they differ.

- [ ] **Step 4: main.go**

Replace `services/analytics/cmd/analytics-api/main.go`:

```go
// Package main is the analytics clickstream service entrypoint (port 8092).
//
// Boot: logger → config → database.New (auto-creates DB) → AutoMigrateAll +
// EnsureView → batcher.Start → purge cron → router → HTTP server with
// graceful shutdown (batcher drains on stop).
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
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/config"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/job"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/transport"
	"github.com/robfig/cron/v3"
)

// countingSink, repoEraser, and repoPurger are defined in adapters.go
// (same package main) — see Step 4's adapters.go block.

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load failed", "error", err)
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect failed", "error", err)
	}
	defer db.Close()

	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := repo.AutoMigrateAll(db.DB); err != nil {
		log.Fatalw("migrate failed", "error", err)
	}
	if err := repo.EnsureView(db.DB); err != nil {
		log.Fatalw("view create failed", "error", err)
	}

	store := repo.NewPostgresStore(db.DB)
	batcher := ingest.New(store, ingest.Config{
		MaxBatch:      cfg.MaxBatch,
		FlushInterval: cfg.FlushInterval,
		BufferSize:    cfg.BufferSize,
	}).WithLogger(log).WithDropHook(func() { observ.EventsDropped.Inc() })
	batcher.Start()

	// Collect handler wraps Enqueue to also bump the received counter.
	collectHandler := handler.NewCollectHandler(countingSink{batcher}, cfg.IPSalt)

	// Erasure adapter binds repo funcs to the Eraser interface.
	adminHandler := handler.NewAdminHandler(repoEraser{db: db})

	// Purge cron.
	purgeJob := job.NewPurgeJob(repoPurger{db: db}, cfg.RetentionDays, log)
	c := cron.New()
	_, _ = c.AddFunc(cfg.PurgeCron, func() {
		_, _ = purgeJob.RunOnce(context.Background())
	})
	c.Start()
	defer c.Stop()

	collector := metrics.NewCollector("analytics")
	router := transport.NewRouter(collectHandler, adminHandler, log, collector)

	srv := &http.Server{Addr: cfg.Server.Address(), Handler: router}
	go func() {
		log.Infow("analytics service listening", "addr", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("server error", "error", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	batcher.Stop() // drain remaining events
}
```

> **Wiring note:** `main.go` references `countingSink`, `repoEraser`, and `repoPurger`. Define them in a sibling file `services/analytics/cmd/analytics-api/adapters.go` (same `package main`):

```go
package main

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/ingest"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/observ"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
)

// countingSink bumps the received counter, then enqueues.
type countingSink struct{ b *ingest.Batcher }

func (c countingSink) Enqueue(e domain.Event) bool {
	observ.EventsReceived.Inc()
	return c.b.Enqueue(e)
}

// repoEraser adapts repo erase funcs to handler.Eraser.
type repoEraser struct{ db *database.DB }

func (r repoEraser) EraseByUserID(ctx context.Context, id string) error {
	return repo.EraseByUserID(ctx, r.db.DB, id)
}
func (r repoEraser) EraseByAnonymousID(ctx context.Context, id string) error {
	return repo.EraseByAnonymousID(ctx, r.db.DB, id)
}

// repoPurger adapts repo.PurgeOlderThan to job.Purger.
type repoPurger struct{ db *database.DB }

func (r repoPurger) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	return repo.PurgeOlderThan(ctx, r.db.DB, cutoff)
}
```

Confirm `database.DB` exposes `.DB` (the `*gorm.DB`) and `.Close()` — match `services/notifications` usage.

- [ ] **Step 5: Verify the whole service compiles and all unit tests pass**

Run:
```bash
cd /data/animeenigma/services/analytics && go build ./... && go test ./... 2>&1 | tail -20
```
Expected: build succeeds; all package tests PASS. Fix any signature mismatches against the notifications service (config/router/metrics helpers) until green.

- [ ] **Step 6: Dockerfile**

Create `services/analytics/Dockerfile` by copying `services/notifications/Dockerfile` and replacing every `notifications` token with `analytics` and the port `8090`→`8092`. Specifically the build line becomes `... -o /analytics-api ./cmd/analytics-api`, the binary copy `COPY --from=builder /analytics-api .`, `EXPOSE 8092`, and `CMD ["./analytics-api"]`. Keep the full `COPY services/*/go.mod` list (including the new `COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/` line) and `RUN cd services/analytics && go mod download`.

- [ ] **Step 7: docker-compose service block**

In `docker/docker-compose.yml`, add after the `watch-together` service block:

```yaml
  # Clickstream analytics ingestion (Plan 1). Postgres-backed events +
  # identity stitching; ClickHouse consolidation deferred. Port 8092.
  analytics:
    build:
      context: ..
      dockerfile: services/analytics/Dockerfile
    container_name: animeenigma-analytics
    restart: unless-stopped
    environment:
      SERVER_PORT: 8092
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: postgres
      DB_PASSWORD: postgres
      DB_NAME: animeenigma
      ANALYTICS_IP_SALT: ${ANALYTICS_IP_SALT:-dev-salt-change-in-production}
      ANALYTICS_RETENTION_DAYS: "90"
      ANALYTICS_PURGE_CRON: "17 3 * * *"
    ports:
      - "127.0.0.1:8092:8092"
    depends_on:
      postgres:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:8092/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Then add to the **gateway** service's `environment:` block in the same file:

```yaml
      ANALYTICS_SERVICE_URL: http://analytics:8092
```

- [ ] **Step 8: Document env vars**

In `docker/.env.example`, add:

```
# Analytics clickstream (Plan 1)
ANALYTICS_IP_SALT=change-me-in-production
```

- [ ] **Step 9: Commit**

```bash
cd /data/animeenigma
git add services/analytics docker/docker-compose.yml docker/.env.example
git commit -m "feat(analytics): wire service (config, router, main, Dockerfile, compose)"
```

---

## Task 11: Gateway routing for `POST /api/analytics/collect`

**Files:**
- Modify: `services/gateway/internal/config/config.go`
- Test: `services/gateway/internal/config/config_test.go`
- Modify: `services/gateway/internal/service/proxy.go`
- Modify: `services/gateway/internal/handler/proxy.go`
- Modify: `services/gateway/internal/transport/router.go`

- [ ] **Step 1: Write the failing config test**

Add to `services/gateway/internal/config/config_test.go`:

```go
// TestConfig_LoadAnalyticsServiceFromEnv asserts ANALYTICS_SERVICE_URL maps
// to ServiceURLs.AnalyticsService, with the docker default fallback.
func TestConfig_LoadAnalyticsServiceFromEnv(t *testing.T) {
	t.Setenv("ANALYTICS_SERVICE_URL", "http://test-an:9999")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Services.AnalyticsService, "http://test-an:9999"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestConfig_AnalyticsServiceDefault(t *testing.T) {
	t.Setenv("ANALYTICS_SERVICE_URL", "")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Services.AnalyticsService, "http://analytics:8092"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/gateway && go test ./internal/config/ -run Analytics -v`
Expected: FAIL — `AnalyticsService` field missing.

- [ ] **Step 3: Add the field and default**

In `services/gateway/internal/config/config.go`, add to the `ServiceURLs` struct (after `WatchTogetherService`):

```go
	AnalyticsService string
```

And in the `Load()` `Services: ServiceURLs{...}` literal (after the `WatchTogetherService` line):

```go
			AnalyticsService: getEnv("ANALYTICS_SERVICE_URL", "http://analytics:8092"),
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/gateway && go test ./internal/config/ -run Analytics -v`
Expected: PASS.

- [ ] **Step 5: Add the proxy service-name case**

In `services/gateway/internal/service/proxy.go`, add to the switch (after the `case "notifications":` block):

```go
	case "analytics":
		return s.serviceURLs.AnalyticsService, nil
```

- [ ] **Step 6: Add the proxy handler**

In `services/gateway/internal/handler/proxy.go`, add (after `ProxyToNotifications`):

```go
// ProxyToAnalytics proxies clickstream ingestion to the analytics service
// (Plan 1). Only POST /api/analytics/collect is exposed — it is PUBLIC (no
// JWT) so anonymous visitors are tracked. The internal erasure endpoint
// (/internal/erase) is Docker-network-only and never routed here.
func (h *ProxyHandler) ProxyToAnalytics(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "analytics")
}
```

- [ ] **Step 7: Register the public route**

In `services/gateway/internal/transport/router.go`, find the public route area (where unauthenticated `/api/*` routes are registered — e.g. near `/api/status` at line ~125, NOT inside a JWT-protected group). Add:

```go
	// Clickstream ingestion (Plan 1). PUBLIC — anonymous visitors tracked.
	// Per-IP rate limiting already applies to all /api/* paths. Only
	// /collect is exposed; /internal/erase is Docker-network-only.
	r.Post("/api/analytics/collect", proxyHandler.ProxyToAnalytics)
```

- [ ] **Step 8: Verify gateway builds and tests pass**

Run: `cd services/gateway && go build ./... && go test ./... 2>&1 | tail -15`
Expected: build OK; tests PASS.

- [ ] **Step 9: Commit**

```bash
cd /data/animeenigma
git add services/gateway/
git commit -m "feat(gateway): route public POST /api/analytics/collect to analytics service"
```

---

## Task 12: End-to-end smoke test and final verification

**Files:** none (verification only)

- [ ] **Step 1: Build and start the new service + gateway**

```bash
cd /data/animeenigma
make redeploy-analytics 2>/dev/null || docker compose -f docker/docker-compose.yml up -d --build analytics
make redeploy-gateway
```

Expected: both containers build and reach healthy. (If `make redeploy-analytics` is missing, the Makefile generates targets per compose service — confirm `make help | grep analytics`; otherwise use the raw compose command shown.)

- [ ] **Step 2: Health check**

```bash
curl -fsS http://localhost:8092/health && echo OK
```
Expected: `{"status":"ok"}` then `OK`.

- [ ] **Step 3: Post a clickstream batch through the gateway (anonymous)**

```bash
curl -i -X POST http://localhost:8000/api/analytics/collect \
  -H 'Content-Type: text/plain' \
  --data '{"anonymous_id":"smoke-a1","session_id":"smoke-s1","events":[{"event_type":"pageview","path":"/smoke"},{"event_type":"click","path":"/smoke","el_selector":"button#smoke"}],"ctx":{"user_agent":"smoke","screen_w":800,"screen_h":600}}'
```
Expected: `HTTP/1.1 204 No Content`.

- [ ] **Step 4: Verify rows landed (and IP is hashed, not raw)**

```bash
sleep 2
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -c \
  "SELECT event_type, path, el_selector, length(ip_hash) AS ip_hash_len, anonymous_id FROM analytics_events WHERE anonymous_id='smoke-a1' ORDER BY received_at;"
```
Expected: 2 rows (pageview + click), `ip_hash_len = 64`, no raw IP visible.

- [ ] **Step 5: Verify identity stitching**

```bash
# Identify the anonymous visitor, then confirm the earlier anon event resolves.
curl -s -X POST http://localhost:8000/api/analytics/collect -H 'Content-Type: text/plain' \
  --data '{"anonymous_id":"smoke-a1","user_id":"smoke-u1","session_id":"smoke-s1","events":[{"event_type":"identify"}]}'
sleep 2
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -c \
  "SELECT person_id FROM analytics_events_resolved WHERE anonymous_id='smoke-a1' AND event_type='pageview';"
```
Expected: `person_id = smoke-u1` (the prior anonymous pageview now resolves to the identified user).

- [ ] **Step 6: Verify erasure (internal endpoint, Docker-network-only)**

```bash
docker compose -f docker/docker-compose.yml exec -T analytics \
  wget -qO- --post-data='{"user_id":"smoke-u1"}' --header='Content-Type: application/json' \
  http://localhost:8092/internal/erase
sleep 1
docker compose -f docker/docker-compose.yml exec -T postgres \
  psql -U postgres -d animeenigma -c \
  "SELECT count(*) FROM analytics_events WHERE anonymous_id='smoke-a1';"
```
Expected: erase returns `{"status":"erased"}`; count = `0`.

- [ ] **Step 7: Confirm the internal erase endpoint is NOT reachable through the gateway**

```bash
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8000/api/analytics/internal/erase -d '{}'
curl -s -o /dev/null -w "%{http_code}\n" -X POST http://localhost:8000/internal/erase -d '{}'
```
Expected: `404` for both (gateway never routes `/internal/*` for analytics).

- [ ] **Step 8: Final commit (if any verification fixes were needed)**

```bash
cd /data/animeenigma
git add -A
git commit -m "test(analytics): end-to-end smoke verification of clickstream backbone" --allow-empty
```

---

## Acceptance Criteria (from spec §9, backend subset)

- [x] A click POSTed to `/api/analytics/collect` appears in `analytics_events` within seconds, carrying `anonymous_id`, `session_id`, `el_selector`, `path` (Task 12 Step 4).
- [x] After an `identify`, prior anonymous events resolve to the `user_id` via `analytics_events_resolved.person_id` (Task 12 Step 5).
- [x] No raw IP is stored; `ip_hash` is 64-char sha256 (Tasks 3, 12 Step 4).
- [x] Events older than the retention window are purged (Task 9; cron wired in Task 10).
- [x] Erasure-by-user works and the internal endpoint is not gateway-reachable (Task 12 Steps 6–7).
- [x] `VITE_ANALYTICS_ENABLED` / frontend integration — **out of scope, Plan 2**.
- [x] Funnel/retention dashboards — **out of scope, Plan 2/dashboards**.

## Notes & Deviations from Spec

- **In-service purge cron** instead of the spec's "scheduler service" purge — matches the notifications service pattern and avoids cross-service coupling. (Documented in `internal/job/purge.go`.)
- **`el_attrs`/`properties` stored as text-holding-JSON**, not `jsonb`, for sqlite/postgres test portability. ALTER to `jsonb` later if JSON operators are needed.
- **Client-asserted `user_id`** on `/collect` is trusted in v1 (small known-user group; analytics is not security-critical). A later hardening could derive `user_id` from a JWT when present.
- **Resolved view uses a correlated subquery**, not Postgres `LATERAL`, so the same DDL runs under sqlite tests and postgres prod.

## Follow-on plans (not in this plan)

- **Plan 2 — Frontend analytics snippet** (`frontend/web`): vanilla TS — anonymous_id, session, pageview (Vue Router), autocapture clicks, heartbeat, batching+sendBeacon, identify/reset, `VITE_ANALYTICS_ENABLED`, plus Grafana product-analytics dashboards over `analytics_events_resolved`.
- **Plan 3 — Distributed tracing**: wire `libs/tracing` in all services, `otelhttp` middleware, gateway `traceparent` propagation, OTel Collector + Tempo containers, Grafana Tempo datasource + trace→logs correlation, browser axios `traceparent` + click `trace_id` stamping.
