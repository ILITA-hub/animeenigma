# Phase 02 — Context Index

Phase: **Catalog Internal Endpoint + Episode Detector + Cleanup** (workstream `notifications`, milestone `v1.0`).

Five-line index of the source artifacts a Phase-2 executor must read before touching code:

1. `/data/animeenigma/.planning/workstreams/notifications/PROJECT.md` — workstream scope, locked design decisions (Decision #7 single-DB, #8 catalog-over-HTTP, #9 Kodik included).
2. `/data/animeenigma/.planning/workstreams/notifications/REQUIREMENTS.md` — Phase 2 requirement IDs **NOTIF-DET-01..10** + non-functional **NOTIF-NF-01** (six Prometheus metrics) + **NOTIF-NF-02** (structured logging).
3. `/data/animeenigma/.planning/workstreams/notifications/ROADMAP.md` — Phase 2 section: goal, touch list, **seven success criteria** (SC1..SC7) that map directly into the verification matrix below.
4. `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md` — source design doc. §Detection Flow (Steps 1–6) is the canonical algorithm; §Bootstrap protection + §Failure modes are the correctness-critical subsections.
5. `/data/animeenigma/.planning/workstreams/notifications/phases/01-notifications-foundation/SUMMARY.md` — Phase 1 ship state: service on port **8090** (not 8087), shared `animeenigma` DB, gateway-non-routing security, `authz.UserIDFromContext` for auth, `repo.UpsertByDedupeKey` already wired with partial-index-aware `ON CONFLICT`.

This-plan: `02-PLAN.md` (sibling file).
