# Relevance-Aware Notifications Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Hide `new_episode` notifications that are no longer relevant — the user has caught up to the advertised episode (anime-level, any combo) or is no longer `watching` the anime — at read time, with an hourly job that tombstones stale rows.

**Architecture:** A single shared SQL relevance predicate is applied (a) at read time in `repo.List` + `repo.UnreadCount` so stale rows never appear and the bell count stays consistent, and (b) hourly in a new `RelevanceInvalidationJob` that stamps a new `invalidated_at` column. The detector's `Upsert` clears `invalidated_at` on conflict so a re-fired episode revives a tombstoned row. All SQL is portable across Postgres (prod) and SQLite (tests).

**Tech Stack:** Go, GORM, Postgres (prod) / in-memory SQLite (tests), `promauto` Prometheus metrics, `robfig/cron`.

**Spec:** `docs/superpowers/specs/2026-05-27-notification-relevance-design.md`

---

## Background the implementer must know

- **Single shared DB (D-01):** the notifications service reads `anime_list` and `watch_history` (owned by the player service) through the same `*gorm.DB` handle via read-only view structs in `services/notifications/internal/repo/views.go`. No new connections.
- **Notification row** = `services/notifications/internal/domain/notification.go::UserNotification`. Payload is JSONB (`datatypes.JSON`) holding a `NewEpisodePayload` for `type='new_episode'`. Relevant payload fields: `anime_id` (string uuid), `latest_available_episode` (int), `first_unwatched_episode` (int).
- **Tests use in-memory SQLite** with hand-rolled DDL (see `job/detector_test.go::testDB`), NOT testcontainers. Therefore **all SQL in this feature must run on both Postgres and SQLite.** Use the `->>` JSON operator (both support `payload ->> 'key'`) and standard `CAST(x AS TEXT)` / `CAST(x AS INTEGER)` — never Postgres-only `::text` / `::int`, `NOW()`, or `INTERVAL`.
- **"Relevant" definition** (the predicate body, reused everywhere):
  - still watching: an `anime_list` row exists for `(user, anime)` with `status='watching'`, AND
  - not caught up: max watched episode for the anime (ANY combo) `< latest_available_episode` (fail-open if that field is NULL).

---

## File Structure

- **Modify** `services/notifications/internal/domain/notification.go` — add `InvalidatedAt *time.Time`.
- **Create** `services/notifications/internal/repo/relevance.go` — the shared relevance SQL fragments (`relevantBodySQL`, `relevanceReadClause()`, `notRelevantClause()`).
- **Modify** `services/notifications/internal/repo/notification.go` — apply read clause to `List` (rows + both counts) + `UnreadCount`; add `invalidated_at = NULL` to `Upsert` conflict update; base predicate gains `invalidated_at IS NULL`.
- **Create** `services/notifications/internal/repo/relevance_test.go` — repo-package SQLite harness + read-filter + revival tests.
- **Create** `services/notifications/internal/job/invalidation.go` — `RelevanceInvalidationJob`.
- **Create** `services/notifications/internal/job/invalidation_test.go` — job test.
- **Modify** `services/notifications/internal/job/metrics.go` — add `NotificationsStaleInvalidatedTotal` counter.
- **Modify** `services/notifications/internal/job/scheduler.go` — accept + run the invalidation job right after the detector.
- **Modify** `services/notifications/internal/job/detector_test.go` — extend `testDB` DDL with `invalidated_at` (used by the job test in the same package).
- **Modify** `services/notifications/internal/job/cleanup.go` — portable cutoff + reap `invalidated_at`.
- **Create** `services/notifications/internal/job/cleanup_test.go` — retention test (now portable).
- **Modify** `services/notifications/internal/repo/indexes.go` — tighten `idx_user_unread` (Postgres-only; drop+recreate).
- **Modify** `services/notifications/cmd/notifications-api/main.go` — construct the invalidation job + pass to `NewScheduler`.

---

## Task 1: Add `invalidated_at` to the domain model

**Files:**
- Modify: `services/notifications/internal/domain/notification.go`

- [ ] **Step 1: Add the field**

In `services/notifications/internal/domain/notification.go`, inside the `UserNotification` struct, add `InvalidatedAt` directly after `DismissedAt`:

```go
	ReadAt      *time.Time     `json:"read_at"`
	DismissedAt *time.Time     `gorm:"index" json:"dismissed_at"`
	InvalidatedAt *time.Time   `gorm:"index" json:"invalidated_at"`
	ClickedAt   *time.Time     `json:"clicked_at"`
	CreatedAt   time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/notifications && go build ./...`
Expected: builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add services/notifications/internal/domain/notification.go
git commit -m "feat(notifications): add invalidated_at column to UserNotification"
```

---

## Task 2: Repo-package SQLite test harness + JSON-SQL portability spike

This establishes the harness every repo test uses AND fails fast if `->>` / `CAST` don't work on the bundled SQLite driver.

**Files:**
- Create: `services/notifications/internal/repo/relevance_test.go`

- [ ] **Step 1: Write the harness + portability spike test**

Create `services/notifications/internal/repo/relevance_test.go`:

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// relevanceTestDB spins up an in-memory SQLite DB with the notifications
// table (including invalidated_at) plus the read-only source tables the
// relevance predicate joins against. Mirrors job/detector_test.go::testDB
// but uses the PARTIAL unique index so Upsert's ON CONFLICT ... WHERE
// dismissed_at IS NULL conflict target matches (revival test depends on it).
func relevanceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE user_notifications (
			id TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
			user_id TEXT NOT NULL,
			type TEXT NOT NULL,
			dedupe_key TEXT NOT NULL,
			payload TEXT NOT NULL,
			read_at DATETIME,
			dismissed_at DATETIME,
			invalidated_at DATETIME,
			clicked_at DATETIME,
			created_at DATETIME,
			updated_at DATETIME
		)`,
		`CREATE UNIQUE INDEX uk_user_dedupe ON user_notifications (user_id, dedupe_key)
		 WHERE dismissed_at IS NULL`,
		`CREATE TABLE anime_list (
			user_id TEXT, anime_id TEXT, status TEXT,
			PRIMARY KEY (user_id, anime_id)
		)`,
		`CREATE TABLE watch_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT, anime_id TEXT, episode_number INTEGER,
			player TEXT, language TEXT, watch_type TEXT, translation_id TEXT
		)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	return db
}

// seedNotif inserts a new_episode notification row with the given combo +
// latest episode encoded into the JSON payload.
func seedNotif(t *testing.T, db *gorm.DB, userID, animeID string, latestEp int) string {
	t.Helper()
	payload := `{"anime_id":"` + animeID + `","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":` + itoa(latestEp) + `,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`
	dedupe := "new_episode:" + animeID + ":kodik:ru:sub:1"
	now := time.Now().UTC()
	if err := db.Exec(
		`INSERT INTO user_notifications (user_id,type,dedupe_key,payload,created_at,updated_at)
		 VALUES (?,?,?,?,?,?)`,
		userID, "new_episode", dedupe, payload, now, now,
	).Error; err != nil {
		t.Fatalf("seed notif: %v", err)
	}
	var id string
	if err := db.Raw(`SELECT id FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&id).Error; err != nil {
		t.Fatalf("fetch seeded id: %v", err)
	}
	return id
}

func seedList(t *testing.T, db *gorm.DB, userID, animeID, status string) {
	t.Helper()
	if err := db.Exec(`INSERT INTO anime_list (user_id,anime_id,status) VALUES (?,?,?)`,
		userID, animeID, status).Error; err != nil {
		t.Fatalf("seed list: %v", err)
	}
}

func seedWatch(t *testing.T, db *gorm.DB, userID, animeID, player string, ep int) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO watch_history (user_id,anime_id,episode_number,player,language,watch_type,translation_id)
		 VALUES (?,?,?,?,?,?,?)`,
		userID, animeID, ep, player, "ru", "sub", "1").Error; err != nil {
		t.Fatalf("seed watch: %v", err)
	}
}

func itoa(n int) string {
	// tiny local strconv.Itoa to keep the payload string-build readable
	return func() string { return fmtInt(n) }()
}

// Test_JSONSQLPortability is a spike: confirms the bundled SQLite driver
// supports the `->>` operator and standard CAST forms the relevance
// predicate relies on. If this fails, the driver is too old and the
// whole approach needs revisiting BEFORE debugging the bigger queries.
func Test_JSONSQLPortability(t *testing.T) {
	db := relevanceTestDB(t)
	seedNotif(t, db, "u1", "anime-1", 7)

	var animeID string
	if err := db.Raw(
		`SELECT payload ->> 'anime_id' FROM user_notifications LIMIT 1`,
	).Scan(&animeID).Error; err != nil {
		t.Fatalf("->> operator failed (SQLite too old?): %v", err)
	}
	if animeID != "anime-1" {
		t.Fatalf("->> returned %q, want anime-1", animeID)
	}

	var ep int
	if err := db.Raw(
		`SELECT CAST(payload ->> 'latest_available_episode' AS INTEGER) FROM user_notifications LIMIT 1`,
	).Scan(&ep).Error; err != nil {
		t.Fatalf("CAST AS INTEGER failed: %v", err)
	}
	if ep != 7 {
		t.Fatalf("CAST returned %d, want 7", ep)
	}
	_ = context.Background()
}
```

Add the `fmtInt` helper at the bottom of the file (avoids importing strconv twice across helpers):

```go
import_strconv:
```

Replace the `itoa`/`fmtInt` shim above with a direct `strconv.Itoa`. Concretely: add `"strconv"` to the import block, delete the `itoa` and `fmtInt` shim, and in `seedNotif` use `strconv.Itoa(latestEp)`:

```go
	payload := `{"anime_id":"` + animeID + `","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":` + strconv.Itoa(latestEp) + `,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`
```

(The `itoa`/`fmtInt` indirection in the first draft is intentionally removed — use `strconv.Itoa` directly. Final imports: `context`, `strconv`, `testing`, `time`, the domain package, sqlite driver, gorm.)

- [ ] **Step 2: Run the spike test**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_JSONSQLPortability -v`
Expected: PASS. If FAIL with a `->>` syntax error, STOP — the SQLite driver predates 3.38; escalate before continuing.

- [ ] **Step 3: Commit**

```bash
git add services/notifications/internal/repo/relevance_test.go
git commit -m "test(notifications): repo SQLite harness + JSON-SQL portability spike"
```

---

## Task 3: Define the shared relevance SQL fragments

**Files:**
- Create: `services/notifications/internal/repo/relevance.go`

- [ ] **Step 1: Write the fragments**

Create `services/notifications/internal/repo/relevance.go`:

```go
package repo

// Relevance predicate fragments shared by the read path (List, UnreadCount)
// and the hourly RelevanceInvalidationJob, so the "is this notification still
// relevant?" rule lives in exactly ONE place.
//
// PORTABILITY: every construct here is valid on BOTH Postgres (prod) and
// SQLite (tests). Uses the `->>` JSON operator and standard-SQL CAST — never
// `::int`/`::text`. References the outer row via the literal table name
// `user_notifications` so the fragment works inside both a SELECT (List,
// Model(&UserNotification{})) and an UPDATE on the same table.
//
// A new_episode notification is RELEVANT iff:
//   (1) the user still has the anime as anime_list.status = 'watching', AND
//   (2) the user's max watched episode for the anime (ANY combo) is below the
//       advertised latest_available_episode (fail-open when that field is NULL).
const relevantBodySQL = `
EXISTS (
	SELECT 1 FROM anime_list al
	WHERE al.user_id = user_notifications.user_id
	  AND CAST(al.anime_id AS TEXT) = (user_notifications.payload ->> 'anime_id')
	  AND al.status = 'watching'
)
AND (
	CAST((user_notifications.payload ->> 'latest_available_episode') AS INTEGER) IS NULL
	OR COALESCE((
		SELECT MAX(wh.episode_number) FROM watch_history wh
		WHERE wh.user_id = user_notifications.user_id
		  AND CAST(wh.anime_id AS TEXT) = (user_notifications.payload ->> 'anime_id')
	), -1) < CAST((user_notifications.payload ->> 'latest_available_episode') AS INTEGER)
)`

// relevanceReadClause is the WHERE fragment added to user-facing reads. Rows
// of a non-new_episode type always pass (future types are filtered by their
// own logic, not this one).
func relevanceReadClause() string {
	return `(user_notifications.type <> 'new_episode' OR (` + relevantBodySQL + `))`
}

// notRelevantClause matches new_episode rows that are NO LONGER relevant —
// used by the invalidation job's UPDATE ... WHERE.
func notRelevantClause() string {
	return `user_notifications.type = 'new_episode' AND NOT (` + relevantBodySQL + `)`
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd services/notifications && go build ./...`
Expected: builds (unused functions are fine in Go only at package scope — these are package-level funcs, so no "declared but not used" error).

- [ ] **Step 3: Commit**

```bash
git add services/notifications/internal/repo/relevance.go
git commit -m "feat(notifications): shared relevance SQL predicate fragments"
```

---

## Task 4: Apply the relevance filter to `repo.List` (rows + both counts)

**Files:**
- Modify: `services/notifications/internal/repo/notification.go:53-103` (the `List` method)
- Test: `services/notifications/internal/repo/relevance_test.go`

- [ ] **Step 1: Write failing tests for List filtering**

Append to `services/notifications/internal/repo/relevance_test.go`:

```go
// helper: build repo + call List(unread) and return (rowCount, unread, total)
func listUnread(t *testing.T, db *gorm.DB, userID string) (int, int64, int64) {
	t.Helper()
	r := NewNotificationRepository(db)
	rows, unread, total, err := r.List(context.Background(), userID, ListUnread, 20, 0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	return len(rows), unread, total
}

func Test_List_WatchingBehind_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5) // watched 5, latest 7
	seedNotif(t, db, "u1", "anime-1", 7)
	n, unread, total := listUnread(t, db, "u1")
	if n != 1 || unread != 1 || total != 1 {
		t.Fatalf("want shown (1,1,1), got (%d,%d,%d)", n, unread, total)
	}
}

func Test_List_CaughtUp_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 7) // watched 7 == latest 7
	seedNotif(t, db, "u1", "anime-1", 7)
	n, unread, total := listUnread(t, db, "u1")
	if n != 0 || unread != 0 || total != 0 {
		t.Fatalf("want hidden (0,0,0), got (%d,%d,%d)", n, unread, total)
	}
}

func Test_List_CaughtUpOtherCombo_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "animelib", 7) // watched in a DIFFERENT player
	seedNotif(t, db, "u1", "anime-1", 7)             // notif combo is kodik
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (anime-level), got %d rows", n)
	}
}

func Test_List_Dropped_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "dropped")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (not watching), got %d rows", n)
	}
}

func Test_List_NotInList_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	// no anime_list row at all
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (not in list), got %d rows", n)
	}
}

func Test_List_PartialRange_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 7) // watched 7, latest 9 -> still behind
	seedNotif(t, db, "u1", "anime-1", 9)
	n, _, _ := listUnread(t, db, "u1")
	if n != 1 {
		t.Fatalf("want shown (newer eps unwatched), got %d rows", n)
	}
}

func Test_List_NeverWatched_Shows(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	// no watch_history rows -> COALESCE(-1) < 7 -> show
	seedNotif(t, db, "u1", "anime-1", 7)
	n, _, _ := listUnread(t, db, "u1")
	if n != 1 {
		t.Fatalf("want shown (never watched), got %d rows", n)
	}
}

func Test_List_Invalidated_Hidden(t *testing.T) {
	db := relevanceTestDB(t)
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5) // would otherwise show
	id := seedNotif(t, db, "u1", "anime-1", 7)
	if err := db.Exec(`UPDATE user_notifications SET invalidated_at=? WHERE id=?`,
		time.Now().UTC(), id).Error; err != nil {
		t.Fatalf("set invalidated: %v", err)
	}
	n, _, _ := listUnread(t, db, "u1")
	if n != 0 {
		t.Fatalf("want hidden (invalidated), got %d rows", n)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_List_ -v`
Expected: FAIL — `Test_List_CaughtUp_Hidden`, `Test_List_CaughtUpOtherCombo_Hidden`, `Test_List_Dropped_Hidden`, `Test_List_NotInList_Hidden`, `Test_List_Invalidated_Hidden` return rows because `List` doesn't filter yet.

- [ ] **Step 3: Add the filter to `List`**

In `services/notifications/internal/repo/notification.go`, modify the `List` method. Change the `base` query (currently lines ~69-71) and apply the relevance clause to all three queries:

```go
	// Base query: active, non-invalidated rows for this user, filtered to
	// still-relevant new_episode rows (other types pass through).
	base := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("user_id = ? AND dismissed_at IS NULL AND invalidated_at IS NULL", userID).
		Where(relevanceReadClause())
```

Leave the rest of the method unchanged — `rowsQuery`, the `ListUnread` `read_at IS NULL` branch, the `Order`/`Limit`/`Offset`, and both `Count` calls all derive from `base`, so they inherit the relevance + `invalidated_at` filter automatically.

- [ ] **Step 4: Run to verify they pass**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_List_ -v`
Expected: PASS (all 8).

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/repo/notification.go services/notifications/internal/repo/relevance_test.go
git commit -m "feat(notifications): filter stale notifications from List (rows + counts)"
```

---

## Task 5: Apply the relevance filter to `repo.UnreadCount`

**Files:**
- Modify: `services/notifications/internal/repo/notification.go:128-140` (the `UnreadCount` method)
- Test: `services/notifications/internal/repo/relevance_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relevance_test.go`:

```go
func Test_UnreadCount_ExcludesStale(t *testing.T) {
	db := relevanceTestDB(t)
	// relevant one
	seedList(t, db, "u1", "anime-1", "watching")
	seedWatch(t, db, "u1", "anime-1", "kodik", 5)
	seedNotif(t, db, "u1", "anime-1", 7)
	// caught-up one (should NOT count)
	seedList(t, db, "u1", "anime-2", "watching")
	seedWatch(t, db, "u1", "anime-2", "kodik", 9)
	seedNotif(t, db, "u1", "anime-2", 9)

	r := NewNotificationRepository(db)
	n, err := r.UnreadCount(context.Background(), "u1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if n != 1 {
		t.Fatalf("want unread=1 (stale excluded), got %d", n)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_UnreadCount_ExcludesStale -v`
Expected: FAIL — returns 2 (no filtering yet).

- [ ] **Step 3: Add the filter to `UnreadCount`**

In `services/notifications/internal/repo/notification.go`, modify `UnreadCount`:

```go
func (r *NotificationRepository) UnreadCount(
	ctx context.Context,
	userID string,
) (int64, error) {
	var n int64
	if err := r.db.WithContext(ctx).
		Model(&domain.UserNotification{}).
		Where("user_id = ? AND read_at IS NULL AND dismissed_at IS NULL AND invalidated_at IS NULL", userID).
		Where(relevanceReadClause()).
		Count(&n).Error; err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "count unread notifications")
	}
	return n, nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_UnreadCount_ExcludesStale -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/repo/notification.go services/notifications/internal/repo/relevance_test.go
git commit -m "feat(notifications): exclude stale notifications from UnreadCount"
```

---

## Task 6: Revive a tombstoned row on Upsert (clear `invalidated_at`)

**Files:**
- Modify: `services/notifications/internal/repo/notification.go:241-293` (the `Upsert` method, `DoUpdates` map)
- Test: `services/notifications/internal/repo/relevance_test.go`

- [ ] **Step 1: Write the failing test**

Append to `relevance_test.go`:

```go
func Test_Upsert_RevivesInvalidatedRow(t *testing.T) {
	db := relevanceTestDB(t)
	r := NewNotificationRepository(db)

	userID := "u1"
	dedupe := "new_episode:anime-1:kodik:ru:sub:1"
	p7 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":7,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)

	// First upsert (ep7), then tombstone it + mark read.
	if _, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p7); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if err := db.Exec(
		`UPDATE user_notifications SET invalidated_at=?, read_at=? WHERE user_id=? AND dedupe_key=?`,
		time.Now().UTC(), time.Now().UTC(), userID, dedupe).Error; err != nil {
		t.Fatalf("tombstone: %v", err)
	}

	// Re-fire with ep8 (same dedupe) -> must revive: invalidated_at + read_at cleared.
	p8 := []byte(`{"anime_id":"anime-1","anime_title":"X","first_unwatched_episode":8,` +
		`"latest_available_episode":8,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`)
	out, err := r.Upsert(context.Background(), userID, "new_episode", dedupe, p8)
	if err != nil {
		t.Fatalf("revive upsert: %v", err)
	}
	if out.InvalidatedAt != nil {
		t.Fatalf("expected invalidated_at cleared on revival, got %v", out.InvalidatedAt)
	}
	if out.ReadAt != nil {
		t.Fatalf("expected read_at cleared on revival, got %v", out.ReadAt)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_Upsert_RevivesInvalidatedRow -v`
Expected: FAIL — `invalidated_at` is still set (Upsert doesn't clear it yet).

- [ ] **Step 3: Add `invalidated_at = NULL` to the conflict update**

In `services/notifications/internal/repo/notification.go`, in `Upsert`'s `clause.OnConflict.DoUpdates` map, add the `invalidated_at` reset:

```go
		DoUpdates: clause.Assignments(map[string]interface{}{
			"payload":        datatypes.JSON(payload),
			"updated_at":     now,
			"read_at":        gorm.Expr("NULL"),
			"invalidated_at": gorm.Expr("NULL"), // revive a tombstoned row on re-fire
			"type":           ntype,
		}),
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/notifications && go test ./internal/repo/ -run Test_Upsert_RevivesInvalidatedRow -v`
Expected: PASS.

- [ ] **Step 5: Run the whole repo package (regression)**

Run: `cd services/notifications && go test ./internal/repo/ -count=1`
Expected: PASS (all repo tests).

- [ ] **Step 6: Commit**

```bash
git add services/notifications/internal/repo/notification.go services/notifications/internal/repo/relevance_test.go
git commit -m "feat(notifications): Upsert revives invalidated rows on re-fire"
```

---

## Task 7: `RelevanceInvalidationJob` + metric

**Files:**
- Modify: `services/notifications/internal/job/metrics.go`
- Create: `services/notifications/internal/job/invalidation.go`
- Modify: `services/notifications/internal/job/detector_test.go` (extend `testDB` DDL with `invalidated_at`)
- Create: `services/notifications/internal/job/invalidation_test.go`

- [ ] **Step 1: Add the metric**

In `services/notifications/internal/job/metrics.go`, add inside the `var (...)` block (after `NotificationsActiveUnreadGauge`):

```go
	// NotificationsStaleInvalidatedTotal counts notification rows the hourly
	// RelevanceInvalidationJob tombstoned (anime no longer 'watching', or the
	// user caught up to the advertised latest episode).
	NotificationsStaleInvalidatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "notifications_stale_invalidated_total",
			Help: "new_episode notifications tombstoned by the hourly relevance invalidation job.",
		},
	)
```

- [ ] **Step 2: Extend the job-package test DDL with `invalidated_at`**

In `services/notifications/internal/job/detector_test.go`, in `testDB`, add the `invalidated_at` column to the `user_notifications` DDL (after `dismissed_at DATETIME,`):

```go
			read_at DATETIME,
			dismissed_at DATETIME,
			invalidated_at DATETIME,
			clicked_at DATETIME,
```

Run: `cd services/notifications && go test ./internal/job/ -run Test_Detector -count=1`
Expected: PASS (existing detector tests still green with the extra column).

- [ ] **Step 3: Write the failing job test**

Create `services/notifications/internal/job/invalidation_test.go`:

```go
package job

import (
	"context"
	"strconv"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

func seedNotifJob(t *testing.T, db *gorm.DB, userID, animeID string, latestEp int) string {
	t.Helper()
	payload := `{"anime_id":"` + animeID + `","anime_title":"X","first_unwatched_episode":1,` +
		`"latest_available_episode":` + strconv.Itoa(latestEp) + `,"player":"kodik","language":"ru",` +
		`"watch_type":"sub","translation_id":"1","watch_url":"/x"}`
	dedupe := "new_episode:" + animeID + ":kodik:ru:sub:1"
	if err := db.Exec(
		`INSERT INTO user_notifications (user_id,type,dedupe_key,payload,created_at,updated_at)
		 VALUES (?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		userID, "new_episode", dedupe, payload).Error; err != nil {
		t.Fatalf("seed notif: %v", err)
	}
	var id string
	_ = db.Raw(`SELECT id FROM user_notifications WHERE user_id=? AND dedupe_key=?`,
		userID, dedupe).Scan(&id).Error
	return id
}

func invalidatedAtNull(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var n int64
	if err := db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE id=? AND invalidated_at IS NULL`, id).
		Scan(&n).Error; err != nil {
		t.Fatalf("check invalidated: %v", err)
	}
	return n == 1
}

func Test_InvalidationJob_TombstonesStaleKeepsRelevant(t *testing.T) {
	db := testDB(t) // from detector_test.go (same package)

	// Relevant: watching + behind -> must stay active.
	seedList(t, db, "u1", "anime-keep", "watching")
	seedWatch(t, db, "u1", "anime-keep", "kodik", "ru", "sub", "1", 5)
	keepID := seedNotifJob(t, db, "u1", "anime-keep", 7)

	// Stale (caught up): watching + watched latest -> tombstone.
	seedList(t, db, "u1", "anime-caught", "watching")
	seedWatch(t, db, "u1", "anime-caught", "kodik", "ru", "sub", "1", 7)
	caughtID := seedNotifJob(t, db, "u1", "anime-caught", 7)

	// Stale (dropped): not watching -> tombstone.
	seedList(t, db, "u1", "anime-drop", "dropped")
	dropID := seedNotifJob(t, db, "u1", "anime-drop", 7)

	job := NewRelevanceInvalidationJob(db, logger.Default())
	count, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("invalidation run: %v", err)
	}
	if count != 2 {
		t.Fatalf("want 2 tombstoned, got %d", count)
	}
	if !invalidatedAtNull(t, db, keepID) {
		t.Fatalf("relevant row was wrongly invalidated")
	}
	if invalidatedAtNull(t, db, caughtID) {
		t.Fatalf("caught-up row was not invalidated")
	}
	if invalidatedAtNull(t, db, dropID) {
		t.Fatalf("dropped row was not invalidated")
	}
}

func Test_InvalidationJob_Idempotent(t *testing.T) {
	db := testDB(t)
	seedList(t, db, "u1", "anime-drop", "dropped")
	_ = seedNotifJob(t, db, "u1", "anime-drop", 7)

	job := NewRelevanceInvalidationJob(db, logger.Default())
	first, _ := job.Run(context.Background())
	second, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if first != 1 {
		t.Fatalf("want first=1, got %d", first)
	}
	if second != 0 {
		t.Fatalf("want second=0 (already invalidated), got %d", second)
	}
}
```

Note: `seedList`/`seedWatch` already exist in `detector_test.go` in this package — reuse them. `seedWatch` there takes the full combo signature `(t, db, userID, animeID, player, language, watchType, translationID, ep)`.

- [ ] **Step 4: Run to verify it fails (won't compile yet)**

Run: `cd services/notifications && go test ./internal/job/ -run Test_InvalidationJob -v`
Expected: FAIL — `undefined: NewRelevanceInvalidationJob`.

- [ ] **Step 5: Implement the job**

Create `services/notifications/internal/job/invalidation.go`:

```go
package job

import (
	"context"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/repo"
	"gorm.io/gorm"
)

// RelevanceInvalidationJob stamps invalidated_at on active new_episode
// notifications that are no longer relevant — the user is no longer
// 'watching' the anime, or has caught up to the advertised latest episode
// (anime-level, any combo). Runs hourly right after the detector (so it sees
// freshly upserted rows) via Scheduler.
//
// The read path (repo.List / repo.UnreadCount) already hides these between
// runs using the same predicate; this job persists the state so storage is
// reclaimable (retention cleanup) and the hot-read partial index stays tight.
//
// Idempotent: invalidated_at IS NULL in the WHERE means already-tombstoned
// rows are skipped.
type RelevanceInvalidationJob struct {
	db  *gorm.DB
	log *logger.Logger
}

// NewRelevanceInvalidationJob constructs the job.
func NewRelevanceInvalidationJob(db *gorm.DB, log *logger.Logger) *RelevanceInvalidationJob {
	return &RelevanceInvalidationJob{db: db, log: log}
}

// Run executes the single UPDATE. Returns rows-affected for logging/metrics/
// the (future) admin endpoint.
func (j *RelevanceInvalidationJob) Run(ctx context.Context) (int64, error) {
	q := `UPDATE user_notifications SET invalidated_at = ?
	      WHERE dismissed_at IS NULL
	        AND invalidated_at IS NULL
	        AND ` + repo.NotRelevantClause()

	res := j.db.WithContext(ctx).Exec(q, time.Now().UTC())
	if res.Error != nil {
		return 0, apperrors.Wrap(res.Error, apperrors.CodeInternal, "relevance invalidation update")
	}
	if res.RowsAffected > 0 {
		NotificationsStaleInvalidatedTotal.Add(float64(res.RowsAffected))
	}
	if j.log != nil {
		j.log.Infow("relevance invalidation completed", "invalidated", res.RowsAffected)
	}
	return res.RowsAffected, nil
}
```

This references `repo.NotRelevantClause()` — exported. In `services/notifications/internal/repo/relevance.go`, rename `notRelevantClause` → `NotRelevantClause` (exported) so the job package can use it:

```go
// NotRelevantClause matches new_episode rows that are NO LONGER relevant —
// used by the invalidation job's UPDATE ... WHERE.
func NotRelevantClause() string {
	return `user_notifications.type = 'new_episode' AND NOT (` + relevantBodySQL + `)`
}
```

(`relevanceReadClause` stays unexported — only the repo package uses it.)

- [ ] **Step 6: Run to verify it passes**

Run: `cd services/notifications && go test ./internal/job/ -run Test_InvalidationJob -v`
Expected: PASS (both tests).

- [ ] **Step 7: Commit**

```bash
git add services/notifications/internal/job/metrics.go services/notifications/internal/job/invalidation.go services/notifications/internal/job/invalidation_test.go services/notifications/internal/job/detector_test.go services/notifications/internal/repo/relevance.go
git commit -m "feat(notifications): hourly RelevanceInvalidationJob + metric"
```

---

## Task 8: Wire the invalidation job into the Scheduler + DI

**Files:**
- Modify: `services/notifications/internal/job/scheduler.go`
- Modify: `services/notifications/cmd/notifications-api/main.go`

- [ ] **Step 1: Add the invalidator to the Scheduler struct + constructor**

In `services/notifications/internal/job/scheduler.go`:

Add the field to the struct:

```go
type Scheduler struct {
	cron        *cron.Cron
	detector    *NewEpisodeDetectorJob
	invalidator *RelevanceInvalidationJob
	cleanup     *DismissedRetentionCleanupJob
	gaugeRepo   *repo.UnreadGaugeRepository
	cfg         *config.DetectorConfig
	log         *logger.Logger

	jitter   time.Duration
	pollerWG sync.WaitGroup
	cancel   context.CancelFunc
}
```

Add the param to `NewScheduler` (after `detector`):

```go
func NewScheduler(
	detector *NewEpisodeDetectorJob,
	invalidator *RelevanceInvalidationJob,
	cleanup *DismissedRetentionCleanupJob,
	gaugeRepo *repo.UnreadGaugeRepository,
	cfg *config.DetectorConfig,
	log *logger.Logger,
) *Scheduler {
```

and set it in the returned struct literal:

```go
	return &Scheduler{
		detector:    detector,
		invalidator: invalidator,
		cleanup:     cleanup,
		gaugeRepo:   gaugeRepo,
		cfg:         cfg,
		log:         log,
		jitter:      jitter,
	}
```

- [ ] **Step 2: Run the invalidator after the detector**

In `scheduler.go`, modify `runDetector` to chain the invalidation pass on the same tick:

```go
func (s *Scheduler) runDetector(ctx context.Context) {
	if s.log != nil {
		s.log.Info("scheduled detector run starting")
	}
	if _, err := s.detector.Run(ctx); err != nil {
		// Detector logs its own structured error; nothing more to add.
		_ = err
	}
	// Retire notifications made stale by watches / list changes since the
	// last tick. Runs even if the detector errored — invalidation is
	// independent of new-episode detection.
	if s.invalidator != nil {
		if _, err := s.invalidator.Run(ctx); err != nil && s.log != nil {
			s.log.Errorw("relevance invalidation failed", "error", err)
		}
	}
}
```

- [ ] **Step 3: Wire DI in main.go**

In `services/notifications/cmd/notifications-api/main.go`, after the `detectorJob` construction (~line 99-108) and before `cleanupJob`, add:

```go
	invalidationJob := job.NewRelevanceInvalidationJob(db.DB, log)
```

Then update the `NewScheduler` call (~line 110) to pass it:

```go
	scheduler := job.NewScheduler(detectorJob, invalidationJob, cleanupJob, unreadGaugeRepo, &cfg.Detector, log)
```

- [ ] **Step 4: Verify build + full package tests**

Run: `cd services/notifications && go build ./... && go test ./... -count=1`
Expected: builds; all tests PASS. (The `NewScheduler` signature change has one caller — main.go — already updated. If any test constructs `NewScheduler`, update it too; none currently do.)

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/job/scheduler.go services/notifications/cmd/notifications-api/main.go
git commit -m "feat(notifications): run relevance invalidation after each detector tick"
```

---

## Task 9: Retention reaps invalidated rows (portable cutoff)

**Files:**
- Modify: `services/notifications/internal/job/cleanup.go`
- Create: `services/notifications/internal/job/cleanup_test.go`

- [ ] **Step 1: Write the failing test**

Create `services/notifications/internal/job/cleanup_test.go`:

```go
package job

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"gorm.io/gorm"
)

// insertNotifWithState inserts a bare notification row with explicit
// dismissed_at / invalidated_at timestamps (nil => NULL).
func insertNotifWithState(t *testing.T, db *gorm.DB, id string, dismissedAt, invalidatedAt *time.Time) {
	t.Helper()
	if err := db.Exec(
		`INSERT INTO user_notifications (id,user_id,type,dedupe_key,payload,dismissed_at,invalidated_at,created_at,updated_at)
		 VALUES (?,?,?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`,
		id, "u1", "new_episode", "dk-"+id, `{}`, dismissedAt, invalidatedAt,
	).Error; err != nil {
		t.Fatalf("insert: %v", err)
	}
}

func rowExists(t *testing.T, db *gorm.DB, id string) bool {
	t.Helper()
	var n int64
	_ = db.Raw(`SELECT COUNT(*) FROM user_notifications WHERE id=?`, id).Scan(&n).Error
	return n == 1
}

func Test_Retention_ReapsOldDismissedAndInvalidated(t *testing.T) {
	db := testDB(t) // from detector_test.go
	old := time.Now().UTC().AddDate(0, 0, -40)
	recent := time.Now().UTC().AddDate(0, 0, -1)

	insertNotifWithState(t, db, "old-dismissed", &old, nil)
	insertNotifWithState(t, db, "old-invalidated", nil, &old)
	insertNotifWithState(t, db, "recent-invalidated", nil, &recent)
	insertNotifWithState(t, db, "active", nil, nil)

	job := NewDismissedRetentionCleanupJob(db, 30, logger.Default())
	deleted, err := job.Run(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("want 2 deleted, got %d", deleted)
	}
	if rowExists(t, db, "old-dismissed") || rowExists(t, db, "old-invalidated") {
		t.Fatalf("old rows should be deleted")
	}
	if !rowExists(t, db, "recent-invalidated") || !rowExists(t, db, "active") {
		t.Fatalf("recent/active rows must survive")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/notifications && go test ./internal/job/ -run Test_Retention -v`
Expected: FAIL — the current query uses Postgres `NOW() - INTERVAL` (errors on SQLite) and ignores `invalidated_at`.

- [ ] **Step 3: Refactor `cleanup.go` to a portable cutoff + reap invalidated**

In `services/notifications/internal/job/cleanup.go`, replace the `Run` method body:

```go
func (j *DismissedRetentionCleanupJob) Run(ctx context.Context) (int64, error) {
	// Go-computed cutoff (portable across Postgres + SQLite; also retires the
	// old pgx INTERVAL-encoding workaround). Reaps rows retired either by the
	// user (dismissed_at) or by the relevance invalidation job (invalidated_at).
	cutoff := time.Now().UTC().AddDate(0, 0, -j.retentionDays)
	const q = `
		DELETE FROM user_notifications
		WHERE (dismissed_at   IS NOT NULL AND dismissed_at   < ?)
		   OR (invalidated_at IS NOT NULL AND invalidated_at < ?)
	`
	res := j.db.WithContext(ctx).Exec(q, cutoff, cutoff)
	if res.Error != nil {
		return 0, apperrors.Wrap(res.Error, apperrors.CodeInternal, "retention cleanup delete")
	}
	if j.log != nil {
		j.log.Infow("cleanup retention completed",
			"deleted", res.RowsAffected,
			"retention_days", j.retentionDays,
		)
	}
	return res.RowsAffected, nil
}
```

Add `"time"` to the imports in `cleanup.go` (it currently imports only `context`, the errors alias, logger, gorm). Update the doc comment above the type to mention `invalidated_at` reaping.

- [ ] **Step 4: Run to verify it passes**

Run: `cd services/notifications && go test ./internal/job/ -run Test_Retention -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/notifications/internal/job/cleanup.go services/notifications/internal/job/cleanup_test.go
git commit -m "feat(notifications): retention reaps invalidated rows (portable cutoff)"
```

---

## Task 10: Tighten `idx_user_unread` (Postgres-only)

The read path's base predicate now includes `invalidated_at IS NULL`; align the partial index. This is Postgres-only DDL (tests bypass `EnsureIndexes`), so there is no unit test — verify by reading + the runtime smoke in Task 11.

**Files:**
- Modify: `services/notifications/internal/repo/indexes.go`

- [ ] **Step 1: Update `EnsureIndexes`**

In `services/notifications/internal/repo/indexes.go`, change the `idx_user_unread` statement. Because a bare `CREATE INDEX IF NOT EXISTS` will NOT alter an existing index's predicate, drop then recreate. Leave `uk_user_dedupe` unchanged (it must keep matching invalidated rows so `Upsert` revival works):

```go
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_user_dedupe
		 ON user_notifications (user_id, dedupe_key)
		 WHERE dismissed_at IS NULL`,
		// Tightened to also exclude invalidated rows, matching the read-path
		// base predicate (dismissed_at IS NULL AND invalidated_at IS NULL).
		// DROP+CREATE because IF NOT EXISTS won't change an existing
		// predicate. Safe + idempotent on this small table.
		`DROP INDEX IF EXISTS idx_user_unread`,
		`CREATE INDEX IF NOT EXISTS idx_user_unread
		 ON user_notifications (user_id, created_at DESC)
		 WHERE dismissed_at IS NULL AND invalidated_at IS NULL`,
	}
```

Update the function doc comment to note the tightened predicate.

- [ ] **Step 2: Verify build**

Run: `cd services/notifications && go build ./...`
Expected: builds.

- [ ] **Step 3: Commit**

```bash
git add services/notifications/internal/repo/indexes.go
git commit -m "perf(notifications): tighten idx_user_unread to exclude invalidated rows"
```

---

## Task 11: Deploy + runtime verification

Per project memory: i18n/SQL/cache changes must be smoke-verified on the live service, not just in tests.

**Files:** none (operational)

- [ ] **Step 1: Full backend test + vet**

Run: `cd services/notifications && go test ./... -count=1 -race && go vet ./...`
Expected: all PASS, vet clean.

- [ ] **Step 2: Redeploy the service**

Run: `make redeploy-notifications`
Expected: builds + restarts; `make health` shows notifications healthy. AutoMigrate adds the `invalidated_at` column; `EnsureIndexes` recreates `idx_user_unread`.

- [ ] **Step 3: Confirm the column + index exist**

Run:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d user_notifications" | grep -E "invalidated_at|idx_user_unread"
```
Expected: `invalidated_at` column present; `idx_user_unread` partial index lists `dismissed_at IS NULL AND invalidated_at IS NULL`.

- [ ] **Step 4: Live smoke against the real affected account (`tNeymik`)**

The user `tNeymik` had 4 unread notifications, including caught-up ones. After deploy, the read filter should drop any the user has caught up on or dropped. Verify the API count matches the relevance predicate using a direct DB check vs. the endpoint. Mint a short-lived check via the existing API-key pattern OR inspect directly:

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "
SELECT
  substring(n.id::text,1,8) AS id8,
  n.payload->>'anime_title' AS title,
  n.payload->>'latest_available_episode' AS latest_ep,
  (SELECT MAX(wh.episode_number) FROM watch_history wh
     WHERE wh.user_id=n.user_id AND wh.anime_id::text=n.payload->>'anime_id') AS max_watched,
  (SELECT al.status FROM anime_list al
     WHERE al.user_id=n.user_id AND al.anime_id::text=n.payload->>'anime_id') AS list_status,
  n.invalidated_at
FROM user_notifications n JOIN users u ON u.id=n.user_id
WHERE u.username='tNeymik' AND n.dismissed_at IS NULL
ORDER BY n.created_at DESC;"
```
Expected: rows where `max_watched >= latest_ep` OR `list_status <> 'watching'` are the ones that should now be hidden; after the next hourly tick (or a manual detector/invalidation trigger) their `invalidated_at` is set.

- [ ] **Step 5: (Optional) trigger the hourly job immediately to confirm tombstoning**

The invalidation job runs after the detector on the hourly cron. To confirm without waiting, trigger the detector's admin run-once endpoint (which now also runs invalidation only if wired into that path) OR simply wait for the next tick and re-run the Step 4 query; tombstoned rows show a non-NULL `invalidated_at`. (Note: the admin `run-once` endpoint triggers the detector directly, not the Scheduler callback, so invalidation fires on the cron tick — waiting one hour, or temporarily setting `NOTIFICATIONS_DETECTOR_CRON` to `*/5 * * * *` for the smoke, confirms it.)

- [ ] **Step 6: Run the after-update skill**

Invoke `/animeenigma-after-update` to update the user-facing changelog (`frontend/web/public/changelog.json`), re-verify health, and commit/push. Changelog entry (enthusiastic + emoji tone): notifications now disappear once you've caught up on the episode or stop watching a show. 🔕✅

---

## Self-Review (completed during planning)

- **Spec coverage:** read-time filter (Tasks 4–5), `invalidated_at` column (Task 1), hourly invalidation job + metric (Tasks 7–8), Upsert revival (Task 6), retention reaping (Task 9), index tightening (Task 10), deploy smoke (Task 11). All spec sections mapped.
- **Type/name consistency:** `relevantBodySQL` (const), `relevanceReadClause()` (unexported, repo-only), `NotRelevantClause()` (exported, used by job). `NewRelevanceInvalidationJob` / `RelevanceInvalidationJob.Run` consistent across Tasks 7–8. `NotificationsStaleInvalidatedTotal` metric name consistent. `NewScheduler` signature change (Task 8) has exactly one production caller (main.go), updated in the same task.
- **Portability:** all runtime SQL uses `->>` + `CAST(... AS TEXT/INTEGER)` + bound `?` timestamps — verified-by-design against both Postgres and SQLite; Task 2 spike fails fast if the driver is too old. Postgres-only DDL (`EnsureIndexes`) is isolated to Task 10 with no SQLite test, by design.
- **Placeholders:** none — every code step shows complete code.
