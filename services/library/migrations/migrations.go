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
//  1. LibraryJobsSQL          (Phase 03 — base)
//  2. LibraryEpisodesSQL      (Phase 04 — references library_jobs(id))
//  3. LibraryFilenamePatternsSQL (Phase 04 — independent)
//  4. AutocachePoolSQL        (Phase 07 — alters library_episodes; must follow 002)
//  5. AutocacheConfigSQL      (Phase 07 — singleton config table; independent)
//  6. AutocacheDemandSQL      (Phase 08 — demand intake table; independent)
//  7. AutocacheJobSourceSQL   (Phase 09 — extends the job_source enum; independent)
//  8. LibraryJobsEpisodeSQL   (Phase 09 — alters library_jobs; must follow 001)
//  9. AutocacheDemandOngoingSQL (Phase 09 — extends autocache_demand_reason enum; independent)
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

// AutocacheJobSourceSQL is migrations/008_autocache_job_source.sql embedded as
// a string. Extends the job_source enum with 'autocache' so the Phase-09 Planner
// can enqueue library_jobs tagged source='autocache' (OBS-04 trigger attribution
// + admin-UI provenance). Idempotent ADD VALUE IF NOT EXISTS — independent of
// the other tables, no FK ordering constraint. main.go apply wiring is Plan
// 09-02's responsibility (alongside the Planner DI).
//
//go:embed 008_autocache_job_source.sql
var AutocacheJobSourceSQL string

// LibraryJobsEpisodeSQL is migrations/009_library_jobs_episode.sql embedded as a
// string. Adds the nullable `episode INT` column to library_jobs — the INTENDED
// episode persisted at enqueue (before filename detection) so the Phase-09
// single-flight dedup on (shikimori_id, episode) + the per-trigger download
// metric work. Idempotent ADD COLUMN IF NOT EXISTS; must run after 001 (which
// created library_jobs). Applied by main.go in Plan 09-02.
//
//go:embed 009_library_jobs_episode.sql
var LibraryJobsEpisodeSQL string

// AutocacheDemandOngoingSQL is migrations/010_autocache_demand_ongoing.sql
// embedded as a string. Extends the autocache_demand_reason enum with 'ongoing'
// (Logic A — scheduler ongoing-push), distinct from 'next_ep' (Logic B) and
// 'backfill', so OBS-04 can attribute downloads by trigger (CONTEXT decision 7).
// Idempotent ADD VALUE IF NOT EXISTS — independent, no FK ordering constraint.
// Applied by main.go in Plan 09-02.
//
//go:embed 010_autocache_demand_ongoing.sql
var AutocacheDemandOngoingSQL string

// AutocacheDemandTitlesSQL is migrations/011_autocache_demand_titles.sql embedded
// as a string. Adds the newline-delimited `titles TEXT` column to autocache_demand
// so the Planner can search trackers by anime TITLE (name_jp → romaji → name_en)
// instead of the useless "<mal_id> <episode>" query. Idempotent ADD COLUMN IF NOT
// EXISTS; must run after 007 (which created autocache_demand). Applied by main.go.
//
//go:embed 011_autocache_demand_titles.sql
var AutocacheDemandTitlesSQL string

// AutocacheTriggerLogSQL is migrations/012_autocache_trigger_log.sql embedded as a
// string. Creates the append-only autocache_trigger_log (the cause→effect log:
// who/when/what-watched per user-driven trigger + the target episode the autocache
// fetched). Independent table (no FK ordering); applied by main.go.
//
//go:embed 012_autocache_trigger_log.sql
var AutocacheTriggerLogSQL string

// LibraryJobsEpisodeIndexSQL is migrations/013_library_jobs_episode_index.sql
// embedded as a string. Adds two partial indexes on library_jobs that serve the
// Phase-09 hot queries (audit finding L578): idx_library_jobs_shikimori_episode
// on (shikimori_id, episode) for HasActiveForEpisode, and
// idx_library_jobs_autocache_inflight on (source) for SumInflightJobBytes (which
// runs on every EnsureRoom). Both are partial on the non-terminal set so they
// stay small as the unbounded durable library_jobs audit table grows. Idempotent
// CREATE INDEX IF NOT EXISTS; must follow 001 (created library_jobs) and 009
// (added the episode column). Applied by main.go.
//
//go:embed 013_library_jobs_episode_index.sql
var LibraryJobsEpisodeIndexSQL string

// LibraryJobsTranscodingSQL is migrations/014_library_jobs_transcoding.sql
// embedded as a string. Adds the 'transcoding' value to the job_status enum so
// the encoder has a dedicated, NON-claimable "encode in progress" state
// distinct from 'encoding' (the download→encode handoff). Before this the row
// sat in claimable 'encoding' for the whole ffmpeg run, letting a second idle
// encoder worker re-claim it and double-encode. Idempotent ADD VALUE IF NOT
// EXISTS (Postgres >= 12) — independent, no FK ordering constraint; must follow
// 001 (which created the job_status enum). Applied by main.go.
//
//go:embed 014_library_jobs_transcoding.sql
var LibraryJobsTranscodingSQL string
