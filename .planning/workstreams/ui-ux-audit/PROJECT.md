# Project: AnimeEnigma — `ui-ux-audit` workstream

**Parent project:** AnimeEnigma (see `/data/animeenigma/.planning/PROJECT.md`)
**Workstream:** ui-ux-audit
**Created:** 2026-05-13
**Lifecycle:** Independent of v3.0 Universal Anime Scraper. Runs in parallel — touches frontend + small infra config; no overlap with `services/scraper/` or `services/catalog/internal/parser/`.

## Scope of this workstream

Ship the actionable findings from the 2026-05-12 UX reassessment (and its three predecessor audits). The source of truth is:

- `docs/issues/ui-audit-2026-05-12.md` — master report
- `docs/issues/ui-audit-2026-05-12/ranked-findings-with-metrics.md` — tier-A through tier-E priority order with UXΔ / CDI / MVQ metrics
- `docs/issues/ui-audit-2026-05-12/competitive-benchmark.md` — Tier E strategic recommendations vs Crunchyroll / animejoy / animevost / yummyanime / 9anime / Netflix / FOD

Each phase below maps to one batch from the ranked-findings document. Phase numbering follows execution order, not severity — Phase 1 is the security/a11y catastrophic batch, Phase 2 is the quick-wins batch, and so on through Tier E strategic initiatives.

## Out of scope for this workstream

- Anything inside `services/scraper/`, `services/catalog/internal/parser/`, or the EnglishPlayer surface — that's v3.0 Universal Anime Scraper milestone in root `.planning/`.
- Phase 16 single-player abstraction (E11 in the ranked file) — already in flight in the root milestone; this workstream **coordinates with** it for Phase 18 (Skip-Intro), but doesn't take over.
- Anything in `services/player/` not related to surfacing existing data to the UI (watch_progress, anime_list aggregates). Backend rec-engine and notifications-engine work belongs in their own phases of the root milestone.
- New OAuth providers, new player providers, new scraper providers.

## Active milestone

**v0.1 UX Reassessment Remediation** — see `ROADMAP.md` in this directory. Bundles Tier A through Tier E phases. Tier F is empty by audit.

## Autonomous mode

The phase list is designed to run cleanly under `/gsd-autonomous --ws ui-ux-audit`:
- Each phase has a single goal, distinct file radius, and falsifiable acceptance criteria.
- Dependencies are explicit in ROADMAP.md (`Depends on:` line per phase). The autonomous runner respects them.
- Phase 1 (Tier A security/a11y) is independent and must ship first. Phases 2–8 can run in any order after that. Tier D (Phase 20) intentionally runs last.

## Notes

- All commands in this workstream use `--ws ui-ux-audit` explicitly. Do not rely on `.planning/active-workstream` (it's gitignored and per-session — see project memory `feedback_workstream_parallelism.md`).
- The audit's UA-NNN finding IDs are preserved verbatim in `Requirements:` lines per phase. The `UX-NN` codes in REQUIREMENTS.md are workstream-internal and aggregate one or more UA-NNN.
- "No fixes applied in this pass" applied to the audit itself; **this workstream applies them**. Each phase ships its own fix and updates `frontend/web/public/changelog.json` via the standard `animeenigma-after-update` hook.

---

*Workstream root: `.planning/workstreams/ui-ux-audit/`*
*Switch via `--ws ui-ux-audit` on every GSD command — do not switch the local marker (see `feedback_workstream_parallelism.md`).*
