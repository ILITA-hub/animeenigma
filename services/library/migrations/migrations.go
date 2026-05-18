// Package migrations exposes the library service's SQL migration
// files as Go strings via go:embed.
//
// The companion files (001_library_jobs.sql, ...) are the source of
// truth for the schema; this package is just the embed wrapper that
// lets main.go apply them at startup without a filesystem dependency.
// The migration SQL itself is idempotent (DO $$ ... EXCEPTION blocks)
// so re-running across restarts is safe.
package migrations

import _ "embed"

// LibraryJobsSQL is migrations/001_library_jobs.sql embedded as a
// string. main.go applies this via db.Exec(LibraryJobsSQL) on
// startup BEFORE the worker pool launches.
//
//go:embed 001_library_jobs.sql
var LibraryJobsSQL string
