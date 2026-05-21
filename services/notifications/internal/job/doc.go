// Package job reserves the cron-job scaffold for Phase 2 of the v1.0
// Notifications Engine workstream.
//
// Phase 1 ships this as an empty package on purpose — Phase 1 is "infra spine
// only" per the ROADMAP. Phase 2 will add:
//
//   - detector.go   — per-user per-combo "what episode have we seen?" scan
//                     that diffs against parser_episode_snapshots and calls
//                     POST /internal/notifications on every fresh new episode.
//   - hotcombos.go  — pre-compute the "hot combos" (popular
//                     anime+player+language+watch_type+translation
//                     groupings) so the detector doesn't scan every combo.
//   - cleanup.go    — soft-delete UserNotification rows older than the
//                     retention window (design doc §Retention).
//   - scheduler.go  — wires a robfig/cron/v3 instance with the three jobs
//                     above and starts it from cmd/notifications-api/main.go.
//
// Reference:
//   - .planning/workstreams/notifications/ROADMAP.md (Phase 2 section)
//   - docs/superpowers/specs/2026-05-11-notifications-engine-design.md
//     (§Detector + §Cron sections)
package job
