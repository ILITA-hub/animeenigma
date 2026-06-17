// Package migrations exposes the library service's SQL migration
// files as Go strings via go:embed.
//
// The companion files (001_library_jobs.sql, ...) are the source of
// truth for the schema; this package is just the embed wrapper that
// lets main.go apply them at startup without a filesystem dependency.
// The migration SQL itself is idempotent (DO $$ ... EXCEPTION blocks
// + CREATE TABLE IF NOT EXISTS + ON CONFLICT DO NOTHING) so re-running
// across restarts is safe.
//
// Apply order is FK-driven and must be preserved:
//   1. LibraryJobsSQL          (Phase 03 — base)
//   2. LibraryEpisodesSQL      (Phase 04 — references library_jobs(id))
//   3. LibraryFilenamePatternsSQL (Phase 04 — independent)
//   4. AutocachePoolSQL        (Phase 07 — alters library_episodes; must follow 002)
//   5. AutocacheConfigSQL      (Phase 07 — singleton config table; independent)
//   6. AutocacheDemandSQL      (Phase 08 — demand intake table; independent)
package migrations

import _ "embed"

// LibraryJobsSQL is migrations/001_library_jobs.sql embedded as a
// string. main.go applies this via db.Exec(LibraryJobsSQL) on
// startup BEFORE the worker pool launches.
//
//go:embed 001_library_jobs.sql
var LibraryJobsSQL string

// LibraryEpisodesSQL is migrations/002_library_episodes.sql embedded
// as a string. Applied AFTER LibraryJobsSQL because the table has a
// FK to library_jobs(id).
//
//go:embed 002_library_episodes.sql
var LibraryEpisodesSQL string

// LibraryFilenamePatternsSQL is migrations/003_library_filename_patterns.sql
// embedded as a string. Applied AFTER LibraryEpisodesSQL by convention
// (no FK dependency). Seeds five uploader patterns idempotently via
// INSERT ... ON CONFLICT DO NOTHING.
//
//go:embed 003_library_filename_patterns.sql
var LibraryFilenamePatternsSQL string

// JackettSourceSQL is migrations/004_jackett_source.sql embedded as a
// string. Applied AFTER the three Phase-3/4 migrations; it only extends
// the job_source enum with 'jackett' (idempotent ADD VALUE IF NOT EXISTS).
//
//go:embed 004_jackett_source.sql
var JackettSourceSQL string

// AutocachePoolSQL is migrations/005_autocache_pool.sql embedded as a
// string. Applied AFTER the Phase-3/4 migrations; it creates the
// episode_source / episode_track enums and adds five accounting-ledger
// columns to library_episodes (Phase 07, POOL-01 + POOL-03). Idempotent
// via DO $$ ... EXCEPTION blocks + ADD COLUMN IF NOT EXISTS + a guarded
// WHERE downloaded_at IS NULL backfill. Must run after 002 (which created
// library_episodes).
//
//go:embed 005_autocache_pool.sql
var AutocachePoolSQL string

// AutocacheConfigSQL is migrations/006_autocache_config.sql embedded as
// a string. Creates the singleton autocache_config table (id fixed at 1
// via PK + CHECK constraint) holding the live-editable §3.5 tunables and
// the master `enabled` switch (Phase 07, POOL-04 + POOL-05), then seeds
// the one row idempotently via INSERT ... ON CONFLICT (id) DO NOTHING.
// Independent of the other tables — no FK ordering constraint.
//
//go:embed 006_autocache_config.sql
var AutocacheConfigSQL string

// AutocacheDemandSQL is migrations/007_autocache_demand.sql embedded as
// a string. Creates the autocache_demand_reason enum ('next_ep' reserved
// for Phase 09, only 'backfill' written in Phase 08) and the minimal
// autocache_demand intake table — one row per wanted (mal_id, episode),
// deduped by the composite PK. The Phase-08 ae serve MISS path records a
// backfill demand here so Phase 09's Planner can drain it across restarts.
// Idempotent via the DO $$ … EXCEPTION enum guard + CREATE TABLE IF NOT
// EXISTS. Independent of the other tables — no FK ordering constraint.
//
//go:embed 007_autocache_demand.sql
var AutocacheDemandSQL string
