// Package job hosts the v1.0 Notifications Engine background workers.
//
// Phase 2 lands the four file types this package is built around:
//
//   - hotcombos.go   — HotCombosCollector: one DISTINCT-join SQL that finds
//     every (anime, player, language, watch_type, translation_id) combo
//     where any user with status='watching' on an 'ongoing' anime has
//     a watch_history row. The detector starts each run by collecting
//     this set.
//
//   - detector.go    — NewEpisodeDetectorJob orchestrates the six steps of
//     the design-doc §Detection Flow: collect hot combos → bulk-load
//     prior snapshots → fan-out parser lookups (errgroup cap 5,
//     per-call 10s timeout) → diff against prior snapshot (with
//     bootstrap protection: first-ever snapshot for a combo never
//     fires a notification, per NOTIF-DET-06) → bulk-UPSERT
//     snapshots BEFORE notifications (so a mid-run crash replays
//     idempotently) → per-affected-combo iterate users, compute
//     first_unwatched = max_watched + 1, UPSERT notification via the
//     in-process NotificationService (D-DET-01 — no HTTP self-loopback).
//
//   - cleanup.go     — DismissedRetentionCleanupJob: single DELETE statement,
//     removes notifications dismissed > 30 days ago (NOTIF-DET-09).
//
//   - scheduler.go   — Scheduler wraps robfig/cron, registers the detector
//     ("0 * * * *" + ±5m boot-time jitter) and cleanup ("30 3 * * *")
//     plus a background goroutine that polls active-unread count into
//     notifications_active_unread_gauge every 5m.
//
//   - metrics.go     — Six promauto-registered series matching NOTIF-NF-01
//     names + labels exactly. Grafana dashboards in v1.1 will alert
//     off these.
//
// Cross-references:
//   - Design doc:    docs/superpowers/specs/2026-05-11-notifications-engine-design.md
//   - Plan:          .planning/workstreams/notifications/phases/02-detector-and-catalog-endpoint/02-PLAN.md
//   - Requirements:  .planning/workstreams/notifications/REQUIREMENTS.md (NOTIF-DET-01..10, NOTIF-NF-01..02)
package job
