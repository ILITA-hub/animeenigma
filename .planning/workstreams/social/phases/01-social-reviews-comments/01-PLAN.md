---
phase: 1
workstream: social
plan: 1
type: execute
wave: 1
depends_on: [0]
files_modified:
  - services/player/internal/domain/watch.go
  - services/player/internal/domain/comment.go
  - services/player/cmd/player-api/main.go
  - services/player/cmd/player-api/main_test.go
autonomous: false
requirements:
  - SOCIAL-01
  - SOCIAL-02
  - SOCIAL-NF-02

must_haves:
  truths:
    - "AnimeListEntry struct declares ReviewText (text NOT NULL DEFAULT '') and Username (varchar(32) NOT NULL DEFAULT '') columns."
    - "Comment table is created by GORM AutoMigrate on player service startup with both required composite indexes."
    - "After first `make redeploy-player`, postgres `\\d anime_list` shows review_text + username columns; `\\d comments` shows the new table; `\\d reviews` reports 'Did not find any relation named reviews'."
    - "Second `make redeploy-player` produces NO 'social migration' log line — idempotency guard short-circuits."
    - "Every row formerly in `reviews` has a corresponding row in `anime_list` with identical username + review_text. score is preserved when anime_list.score was already non-zero, otherwise copied from reviews.score."
    - "TestSocialMigration_Idempotent passes — running the bootstrap function twice against a fresh test DB produces zero data changes on the second run."
  artifacts:
    - path: "services/player/internal/domain/watch.go"
      provides: "Extended AnimeListEntry with ReviewText + Username fields and matching GORM tags"
      contains: "ReviewText"
    - path: "services/player/cmd/player-api/main.go"
      provides: "AutoMigrate(&domain.Comment{}) entry + one-shot social migration block gated by reviews-table-exists check"
      contains: "social migration"
    - path: "services/player/cmd/player-api/main_test.go"
      provides: "TestSocialMigration_Idempotent — invokes the migration helper twice against an in-memory sqlite DB seeded with reviews + anime_list rows; asserts second invocation makes zero writes"
      contains: "func TestSocialMigration_Idempotent"
  key_links:
    - from: "services/player/cmd/player-api/main.go"
      to: "services/player/internal/domain/comment.go"
      via: "AutoMigrate(&domain.Comment{}) in the existing AutoMigrate block"
      pattern: "domain.Comment"
    - from: "services/player/cmd/player-api/main.go"
      to: "PostgreSQL reviews table existence check"
      via: "information_schema.tables probe"
      pattern: "information_schema.tables"
    - from: "services/player/cmd/player-api/main.go"
      to: "anime_list rows (data write)"
      via: "INSERT ... ON CONFLICT (user_id, anime_id) DO UPDATE (raw SQL)"
      pattern: "ON CONFLICT \\(user_id, anime_id\\)"
---

<objective>
Land the schema half of the phase: extend `AnimeListEntry` with `ReviewText` + `Username` columns, create the new `comments` table via GORM AutoMigrate, and run a one-shot idempotent migration that copies every `reviews` row into `anime_list` then drops the `reviews` table. After this plan deploys, the database is in its target state and no Go code outside the migration block has changed (review wiring still references `&domain.Review{}` / `ReviewRepository` — that refactor is plan 02).

Purpose: provide the foundation. Plan 02 (reviews refactor) and plan 03 (comments CRUD) both depend on the new column shape existing in the live DB and on `domain.Comment` being AutoMigrated.

Output: schema diffs in postgres, migration block in main.go, comment.go domain struct upgraded from skeleton to production-ready, and a Go unit test proving the migration is idempotent.
</objective>

<execution_context>
@$HOME/.claude/get-shit-done/workflows/execute-plan.md
@$HOME/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/workstreams/social/phases/01-social-reviews-comments/01-SPEC.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md
@.planning/workstreams/social/phases/01-social-reviews-comments/01-00-SUMMARY.md
@services/player/cmd/player-api/main.go
@services/player/internal/domain/watch.go
@services/player/internal/domain/activity.go
@CLAUDE.md

<interfaces>
<!-- Reference structures from existing code -->
From services/player/internal/domain/watch.go:
- AnimeListEntry (lines 58-79): existing fields ID, UserID, AnimeID, Anime *AnimeInfo, Status, Score, Episodes, Notes, Tags, IsRewatching, Priority, MalID, StartedAt, CompletedAt, CreatedAt, UpdatedAt. Composite unique index `idx_user_anime` on (user_id, anime_id).
- Review struct (lines 105-119): kept INTACT in this plan (plan 02 will delete it after refactor lands).

From services/player/cmd/player-api/main.go:
- db.AutoMigrate(...) block starts at line 50, currently registers `&domain.Review{}` at line 56. Phase 3 backfill block at lines 192-213 — verbatim precedent for the bootstrap pattern.

Project conventions (libs/logger):
- log.Infow(msg, key, value, ...)
- log.Fatalw(msg, key, value, ...)
- log.Errorw(msg, key, value, ...)
</interfaces>
</context>

<tasks>

<task type="auto">
  <name>Task 1.1: Extend AnimeListEntry + finalize Comment domain struct</name>
  <files>services/player/internal/domain/watch.go, services/player/internal/domain/comment.go</files>
  <read_first>
    - services/player/internal/domain/watch.go (lines 58-119 — current AnimeListEntry + Review)
    - services/player/internal/domain/comment.go (from Wave 0 — verify struct shape)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Code Example 1, lines 700-740)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md (Schema decisions section)
  </read_first>
  <action>
    Edit `services/player/internal/domain/watch.go`:
    - Inside `AnimeListEntry` struct (lines 58-75), add two new fields after `Tags`:
      - `ReviewText string \`gorm:"type:text;not null;default:''" json:"review_text"\``
      - `Username   string \`gorm:"size:32;not null;default:''" json:"username"\``
    - Do NOT delete or modify the `Review` struct (lines 105-119) — plan 02 owns its removal. Do NOT delete `CreateReviewRequest` either.

    Edit `services/player/internal/domain/comment.go` (file already exists from Wave 0):
    - Confirm the struct matches RESEARCH.md Code Example 1 exactly. If Wave 0 omitted any tag, fix it now:
      - `ID string \`gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"\``
      - `UserID string \`gorm:"type:uuid;not null;index:idx_comments_user_created" json:"user_id"\``
      - `AnimeID string \`gorm:"type:uuid;not null;index:idx_comments_anime_created" json:"anime_id"\``
      - `Username string \`gorm:"size:32" json:"username"\``
      - `Body string \`gorm:"type:text;not null" json:"body"\``
      - `ParentID *string \`gorm:"type:uuid" json:"parent_id,omitempty"\``
      - `CreatedAt time.Time \`gorm:"not null;default:now();index:idx_comments_anime_created,sort:desc;index:idx_comments_user_created,sort:desc" json:"created_at"\``
      - `UpdatedAt time.Time \`gorm:"not null;default:now()" json:"updated_at"\``
      - `DeletedAt gorm.DeletedAt \`gorm:"index" json:"-"\``
    - Verify `CreateCommentRequest` still has no `ParentID` field (Pitfall 8).
  </action>
  <verify>
    <automated>cd services/player && go build ./... && grep -c 'ReviewText string' services/player/internal/domain/watch.go && grep -c 'Username   string' services/player/internal/domain/watch.go && grep -E 'idx_comments_anime_created,sort:desc' services/player/internal/domain/comment.go | wc -l</automated>
  </verify>
  <acceptance_criteria>
    - `services/player/internal/domain/watch.go` AnimeListEntry contains both `ReviewText string` and `Username   string` field declarations with the documented GORM tags. Verify: `grep -A1 'ReviewText string' services/player/internal/domain/watch.go | grep 'type:text;not null;default'` exits 0; same for `Username` with `size:32;not null;default`.
    - `services/player/internal/domain/comment.go` declares BOTH composite indexes via the `sort:desc` GORM directive. Verify: `grep -c 'idx_comments_anime_created' services/player/internal/domain/comment.go` outputs ≥ 2 (one per index participant column — user_id and created_at) — adjust grep to `grep 'idx_comments_anime_created,sort:desc'` which is the literal substring on the CreatedAt field.
    - `cd services/player && go build ./...` exits 0.
    - `awk '/CreateCommentRequest/,/^\}/' services/player/internal/domain/comment.go | grep -v '^#' | grep -c ParentID` outputs `0`.
  </acceptance_criteria>
  <done>AnimeListEntry carries the new columns; Comment domain struct is production-grade; the build is green.</done>
</task>

<task type="auto">
  <name>Task 1.2: Register Comment AutoMigrate + add idempotent social migration block in main.go</name>
  <files>services/player/cmd/player-api/main.go</files>
  <read_first>
    - services/player/cmd/player-api/main.go (full file — focus on lines 40-220)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-RESEARCH.md (Pattern 1, lines 266-344 — verbatim bootstrap pattern)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-CONTEXT.md (Migration decisions, lines 24-35)
    - services/player/internal/domain/comment.go (verify domain.Comment is importable from main)
  </read_first>
  <action>
    Edit `services/player/cmd/player-api/main.go` in two places:

    (1) Inside the existing `db.AutoMigrate(...)` block starting at line 50, ADD `&domain.Comment{},` immediately after `&domain.Review{},` (do NOT remove `&domain.Review{}` — plan 02 removes it once the migration drops the table at runtime). The new AutoMigrate creates the `comments` table and adds the new `anime_list.review_text` + `anime_list.username` columns automatically. Order matters: AnimeListEntry comes first so its new columns exist before the data copy.

    (2) Insert the social migration block AFTER `db.AutoMigrate(...)` completes (so the columns + comments table exist) and BEFORE the existing Phase-3 backfill block at line 192. Implement the block per RESEARCH.md Pattern 1, lines 293-344, verbatim minus formatting nits. Key requirements:
    - Idempotency guard: `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = 'public' AND table_name = 'reviews')` — only run if `reviewsExists` is true.
    - Step A: INSERT into anime_list FROM reviews ON CONFLICT (user_id, anime_id) DO UPDATE SET score = CASE WHEN anime_list.score = 0 THEN EXCLUDED.score ELSE anime_list.score END, review_text = EXCLUDED.review_text, username = COALESCE(NULLIF(EXCLUDED.username, ''), anime_list.username), updated_at = NOW(). Default status='completed' when creating new rows.
    - Step B: backfill empty `anime_list.username` from `users` JOIN.
    - Step C: `DROP TABLE reviews`.
    - Wrap each step in its own `if err := db.DB.Exec(...).Error; err != nil` block; on failure use `log.Fatalw` so the service crash-loops until ops intervenes (data integrity > availability for this one-shot).
    - Emit `log.Infow("social migration: merging reviews into anime_list")` at start and `log.Infow("social migration complete")` at end. Both messages MUST contain the literal substring `social migration` so the validation script `make logs-player | grep "social migration"` works.

    Extract the migration body into a package-level helper `runSocialMigration(db *gorm.DB, log *logger.Logger) error` placed near the bottom of main.go (above `main()` is fine; or in a sibling `migrate_social.go` file at the same package level — implementer's choice). The helper takes the `*gorm.DB` (not the project's `*database.DB` wrapper — pass `db.DB` so it's directly testable). This separation enables `TestSocialMigration_Idempotent` in task 1.3.

    Do NOT touch the reviewRepo / reviewService / reviewHandler wiring lines (lines 219-296) — plan 02 handles those.
  </action>
  <verify>
    <automated>cd services/player && go build ./... && grep -c 'runSocialMigration' services/player/cmd/player-api/main.go && grep -c 'ON CONFLICT (user_id, anime_id)' services/player/cmd/player-api/main.go && grep -c 'social migration' services/player/cmd/player-api/main.go</automated>
  </verify>
  <acceptance_criteria>
    - `services/player/cmd/player-api/main.go` contains the literal identifier `runSocialMigration` referenced at least twice (declaration + call site). Verify: `grep -c 'runSocialMigration' services/player/cmd/player-api/main.go` ≥ 2.
    - The AutoMigrate block now lists `&domain.Comment{}`. Verify: `grep -E '&domain\.Comment\{\}' services/player/cmd/player-api/main.go` exits 0.
    - The migration body contains the exact substrings: `information_schema.tables`, `ON CONFLICT (user_id, anime_id)`, `DROP TABLE reviews`, `social migration: merging reviews into anime_list`, `social migration complete`. Verify each via `grep -F`.
    - `cd services/player && go vet ./...` exits 0.
    - `cd services/player && go build ./...` exits 0.
    - The existing `&domain.Review{}` line is still present in the AutoMigrate block (plan 02 will remove it).
  </acceptance_criteria>
  <done>main.go runs the social migration exactly once per environment (gated by the reviews-table existence probe), then continues to start the player service normally.</done>
</task>

<task type="auto">
  <name>Task 1.3: Replace TestSocialMigration_Idempotent stub with real idempotency assertion</name>
  <files>services/player/cmd/player-api/main_test.go</files>
  <read_first>
    - services/player/cmd/player-api/main_test.go (from Wave 0 — the SKIP stub)
    - services/player/cmd/player-api/main.go (the `runSocialMigration` helper from task 1.2)
    - services/player/internal/handler/sync_test.go (setupSyncTestDB SQLite pattern)
    - .planning/workstreams/social/phases/01-social-reviews-comments/01-VALIDATION.md (01-Migrate-01 entry)
  </read_first>
  <action>
    Replace the `t.Skip(...)` body of `TestSocialMigration_Idempotent` with a real test:
    1. Setup: open in-memory SQLite (`sqlite.Open("file::memory:?cache=shared")` with `gorm.Open`); AutoMigrate `&domain.AnimeListEntry{}, &domain.Review{}, &domain.Comment{}, &domain.AnimeInfo{}`. Also create a minimal `users` table via raw SQL (since the player service doesn't own the `users` domain; SQLite raw `CREATE TABLE users (id TEXT PRIMARY KEY, username TEXT)` is sufficient).
    2. Seed: insert 2 review rows + 1 anime_list row that overlaps (same user_id, anime_id, score=7) + 1 user row matching one of the review user_ids.
    3. First invocation: call `runSocialMigration(db, testLogger)`. Assert returns nil. Assert the `reviews` table no longer exists (SELECT from sqlite_master). Assert `anime_list` now has 2 rows (one preserving score=7 from the original anime_list row, one new with score=reviews.score). Assert both rows have non-empty `username` and `review_text`.
    4. Reseed the `reviews` table NOT — it has been dropped. The idempotency check looks at `information_schema.tables` (postgres) but SQLite uses `sqlite_master`; the helper needs to abstract this. Solution: the helper accepts a `tableExists(db, name) bool` function-typed parameter, OR uses a dialect-aware probe: `if db.Migrator().HasTable("reviews")` (GORM's Migrator API works on both backends). Use the GORM Migrator approach. Document this choice in a comment in `runSocialMigration`.
    5. Second invocation: call `runSocialMigration(db, testLogger)` again. Assert returns nil. Assert `anime_list` count unchanged (still 2). Assert no panic / fatal log.

    Use `testify/assert` (`assert.NoError`, `assert.Equal`, `assert.False`) which is already in go.mod. Use a noop `*logger.Logger` constructed via `logger.New(...)` with WARN level so test output isn't noisy.

    The test is hermetic — no docker, no real postgres, no network.
  </action>
  <verify>
    <automated>cd services/player && go test ./cmd/player-api/ -run TestSocialMigration_Idempotent -v -count=1</automated>
  </verify>
  <acceptance_criteria>
    - `go test ./cmd/player-api/ -run TestSocialMigration_Idempotent -v -count=1` exits 0 with `--- PASS: TestSocialMigration_Idempotent`.
    - `grep -c 't.Skip' services/player/cmd/player-api/main_test.go` outputs `0` (no skips remain in this file's TestSocialMigration function).
    - The test contains the substring `runSocialMigration` invoked at least twice (first + second pass).
    - The test contains the literal `assert.False` or `require.False` check confirming the `reviews` table was dropped (via `db.Migrator().HasTable("reviews")` returning false).
  </acceptance_criteria>
  <done>The idempotency property is encoded in a hermetic Go test that runs in &lt; 5 seconds and passes on every CI invocation.</done>
</task>

<task type="checkpoint:human-verify" gate="blocking">
  <name>Checkpoint 1.4: Deploy + verify schema state in live postgres</name>
  <what-built>
    Domain extension + AutoMigrate registration + idempotent social migration helper. After `make redeploy-player` runs, postgres should show the new column shape and the comments table; the second restart should produce no migration log.
  </what-built>
  <action>Manual verification gate — implementer pauses execution and the human runs the steps in &lt;how-to-verify&gt; below, then types the resume signal. No automated work in this task.</action>
  <how-to-verify>
    1. Run `make redeploy-player` from repo root. Wait until `make logs-player | tail -20` shows the boot is complete.
    2. Confirm migration ran exactly once: `make logs-player | grep "social migration"` MUST show both `social migration: merging reviews into anime_list` AND `social migration complete` exactly one time each.
    3. Inspect schema:
       - `docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c "\d anime_list"` MUST show `review_text` (text, not null, default '') and `username` (varchar(32), not null, default '') columns.
       - Same exec target with `-c "\d comments"` MUST show the table with columns `id, user_id, anime_id, username, body, parent_id, created_at, updated_at, deleted_at` and indexes `idx_comments_anime_created` + `idx_comments_user_created`.
       - Same exec target with `-c "\d reviews"` MUST return `Did not find any relation named "reviews".`
    4. Restart again: `make redeploy-player`. Check `make logs-player | grep "social migration" | wc -l` → MUST output `2` (the SAME two lines from step 2 are still in the logs from the prior boot — they were preserved in container log volume; the SECOND boot adds NOTHING new). To verify idempotency at the boot level, watch live logs: `make logs-player &` then `make redeploy-player` again and confirm no new "social migration" lines appear during the fresh boot.
    5. Spot-check data: pick one user_id that had a `reviews` row before deploy (best effort — query a pre-deploy backup or rely on the test harness). Run `SELECT user_id, anime_id, score, review_text, username FROM anime_list WHERE review_text != '' LIMIT 5;` and confirm score + review_text + username are populated.
  </how-to-verify>
  <resume-signal>Type "approved" if all five verification steps pass. If `\d reviews` still shows the table, or migration log is missing, or the second boot re-runs the migration, describe the failure and the planner will produce a revision.</resume-signal>
</task>

</tasks>

<threat_model>
## Trust Boundaries

| Boundary | Description |
|----------|-------------|
| db.Exec ← migration body | Migration runs raw SQL with hardcoded statements; no user input. |

## STRIDE Threat Register

| Threat ID | Category | Component | Disposition | Mitigation Plan |
|-----------|----------|-----------|-------------|-----------------|
| T-1-Migration | Tampering / Data integrity | runSocialMigration | mitigate | Idempotency guard via `db.Migrator().HasTable("reviews")` prevents double-copy on retry; `log.Fatalw` on any step failure prevents partial migrations (service crash-loops until ops resolves). Forward-only — no rollback path; backup is the restore mechanism per SPEC. |
| T-1-V5 | Input validation | AnimeListEntry.ReviewText / Username | mitigate | New columns are `NOT NULL DEFAULT ''` so legacy rows are valid pre-migration; `size:32` cap on username matches existing `reviews.username` cap so no row will fail to copy due to truncation. |
| T-1-V13 | API & Web Service | (none directly — schema only) | accept | Plan 02 lands the API-shape consumer of these columns. |
</threat_model>

<verification>
- `cd services/player && go build ./...` exits 0
- `cd services/player && go test -run TestSocialMigration_Idempotent -v ./cmd/player-api/` passes
- `make redeploy-player` followed by `\d anime_list` shows review_text + username
- `\d comments` shows the new table with both indexes
- `\d reviews` returns "Did not find any relation"
- `make logs-player | grep "social migration complete"` returns exactly one occurrence per deploy cycle
</verification>

<success_criteria>
The database is in its target shape and the migration is provably idempotent (unit + manual smoke). Subsequent plans can `INSERT INTO comments` and `SELECT review_text FROM anime_list` against the live schema with no further migration work.
</success_criteria>

<output>
After completion, create `.planning/workstreams/social/phases/01-social-reviews-comments/01-01-SUMMARY.md` documenting: the new AnimeListEntry shape, the runSocialMigration helper signature, postgres schema diffs (output of `\d anime_list` post-deploy), and how plan 02 will assume the new schema.
</output>
