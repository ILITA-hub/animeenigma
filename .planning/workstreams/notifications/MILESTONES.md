# Milestones — `notifications` workstream

## v1.0 Notifications Engine (shipped)

**Status:** ✅ Shipped — 3/3 phases delivered, 30/30 requirements, audit PASSED
**Started:** 2026-05-20
**Shipped:** 2026-05-21
**Audit:** `v1.0-MILESTONE-AUDIT.md` — PASSED
**Summary:** `milestones/v1.0-SUMMARY.md`
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md`

**Scope:** A new `services/notifications/` Go microservice on port 8087 + two new tables (`user_notifications` + `parser_episode_snapshots`) + one new catalog internal endpoint (`/internal/anime/{shikimori_id}/episodes`) + hourly cron detector + daily cleanup + frontend bell / dropdown / toast / Pinia store / type-pluggable renderer registry / 3-locale i18n. First (and only v1.0) concrete notification type is `new_episode` — "a new episode of an ongoing show you are watching is available on the same player/translation". Engine is built type-pluggable so future types (`new_comment`, `friend_activity`, `system_announcement`) need a payload + a renderer component only, with zero engine changes.

**Phases (planned):**

1. Notifications Service Foundation (NOTIF-FOUND-01..08)
2. Catalog Internal Episode Endpoint + Episode Detector + Cleanup (NOTIF-DET-01..10)
3. Frontend — Bell + Dropdown + Toast + Pinia Store + Renderer Registry + i18n (NOTIF-UI-01..08)

See `REQUIREMENTS.md` for full requirement text and `ROADMAP.md` for the phase / requirement map + success criteria.

---

## v1.1 Type Expansion (planned — not yet scoped)

Conditional on v1.0 ship + 1-2 weeks of real usage data. Will add a second concrete notification type (renderer pattern proof) and a minimal preferences UI (mute-type toggle).

## v1.2 Real-Time Delivery (planned — conditional)

Triggered only if v1.0 usage shows polling cost is a problem or hourly detector latency is felt. SSE over WebSocket for the polling-replacement use case.
