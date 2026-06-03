---
phase: 06-governance-into-memory
plan: 01
subsystem: docs
tags: [design-system, governance, lint-gate, claude-md, project-memory, shadcn-vue]

requires:
  - phase: 05-design-system-lint-gate
    provides: "design-system-lint.sh (3 enforced color/token rules) + DESIGN-SYSTEM.md 'Lint gate (enforced)' section"
provides:
  - "CLAUDE.md '### Design System (Neon Tokyo, shadcn-vue)' subsection (enforced vs governance-only labeled)"
  - "project-memory governance entry (project_design_system_governance.md) loaded each session"
  - "MEMORY.md one-line index pointer to the governance entry"
  - "SC#3 consistency verified: 3 enforced rules labeled enforced, structural rules labeled governance-only, exempt hues named"
affects: [any future frontend styling/color/Tailwind/shadcn work]

tech-stack:
  added: []
  patterns:
    - "Governance dual-surface: durable project-memory entry + CLAUDE.md subsection, both POINTING at the canonical DESIGN-SYSTEM.md (no duplication)"
    - "Enforced-vs-governance-only labeling so docs mirror the lint gate exactly (SC#3)"

key-files:
  created:
    - "/root/.claude/projects/-data-animeenigma/memory/project_design_system_governance.md (OUTSIDE git repo — assistant's persistent memory store)"
  modified:
    - "CLAUDE.md (added '### Design System' subsection under Code Conventions)"
    - "/root/.claude/projects/-data-animeenigma/memory/MEMORY.md (one-line pointer under existing Design System section — OUTSIDE git repo)"

key-decisions:
  - "Placed the subsection as ### under Code Conventions (after Logging) — natural slot near frontend/conventions guidance; grep gate (## Design System) matches the ### heading via substring"
  - "Memory files written with Write/Edit, NOT committed — they live outside the git repo by design; acceptance is file-existence + content"
  - "3 enforced rule names taken verbatim from design-system-lint.sh (off-palette classes / non-allowlisted hex / deprecated brand-alias vars); all 7 exempt hues (cyan/pink/orange/rose/indigo/teal/lime) named under Rule 1"

patterns-established:
  - "Governance text mirrors the enforced gate exactly — no documented-but-unenforced rule, no enforced rule omitted (SC#3 invariant)"

metrics:
  duration: ~9 min
  completed: 2026-06-03
---

# Phase 6 Plan 01: Governance into Memory Summary

Wrote the design-system governance rules onto two durable surfaces — repo-root `CLAUDE.md` and the assistant's project-memory store — so the Phases 1–5 consolidation doesn't silently erode. The load-bearing constraint (SC#3) holds: the 3 lint-ENFORCED color/token rules are labeled enforced (with the 7 brand/provider exempt hues named) and the structural rules are labeled GOVERNANCE-ONLY, matching `design-system-lint.sh` exactly. Both surfaces point at the canonical `DESIGN-SYSTEM.md` rather than duplicating it.

## What Was Built

**Task 1 — CLAUDE.md `### Design System` subsection** (committed `f39f9a0d`, CLAUDE.md only by explicit path):
- Canonical-reference pointer (`frontend/web/src/styles/DESIGN-SYSTEM.md`) + lint-gate command (`make lint-frontend` / `make redeploy-web` via `frontend/web/scripts/design-system-lint.sh`; `--selftest` fail-path; `design-system-allowlist.txt` escape-hatch).
- 3 rules labeled **build-ENFORCED** verbatim from the gate: (1) off-palette Tailwind color classes — with EXEMPT hues `cyan/pink/orange/rose/indigo/teal/lime` named; (2) non-allowlisted hardcoded hex; (3) deprecated `var(--ink|--accent|--pink)` brand-alias usages. Stated as "FAIL THE BUILD."
- Structural rules labeled **GOVERNANCE-ONLY** (not build-enforced): reuse `@/components/ui` primitives, `font-medium`/`font-semibold` only, padding scale, `cva` variants.
- DS-NF-06 in-browser-verify standing rule (desktop + mobile; jsdom can't catch Tailwind-v4 cascade bugs).
- `--accent` is the shadcn hover surface since 05-04; use `--brand-cyan` for brand cyan.

**Task 2 — project-memory governance entry** (`project_design_system_governance.md`, OUTSIDE git repo — written, NOT committed):
- Frontmatter (`name`, recall-friendly `description`, `metadata.type: project`).
- All 4 rule themes (tokens-not-hardcode / reuse-primitives / verify-in-browser / the-gate-is-real + allowlist escape-hatch), the explicit ENFORCED-vs-GOVERNANCE-ONLY distinction, a `DESIGN-SYSTEM.md` pointer, and `[[reference_tailwind_v4_css_cascade]]` + `[[feedback_native_rightclick_dropdown_triggers]]` cross-links.
- One-line pointer bullet added to `MEMORY.md` under the EXISTING "## Design System (Neon Tokyo, shadcn-vue)" section (no duplicate section).

**Task 3 — SC#3 consistency verification + DS-NF-05 ack**: grep-confirmed the 3 enforced rule families from `design-system-lint.sh` each appear in CLAUDE.md labeled enforced, the structural rules appear labeled governance-only, and the exempt hues are named. No enforced rule undocumented; no governance-only rule mislabeled.

## DS-NF-05 Acknowledgement

DS-NF-05 (every phase independently shippable) was satisfied by each phase's green build (vue-tsc + vitest + 5-surface in-browser smoke) — no new machinery needed for it in this phase. This plan is pure docs/governance and ships independently with zero rendered change.

## Deviations from Plan

None — plan executed exactly as written. (Note: the CLAUDE.md heading is `###` rather than a top-level `##` because it nests under the existing "## Code Conventions" section, which is the slot the plan specified; the verify grep `## Design System` matches via substring.)

## Known Stubs

None.

## Self-Check: PASSED

- `CLAUDE.md` Design System subsection — FOUND (committed `f39f9a0d`).
- `/root/.claude/projects/-data-animeenigma/memory/project_design_system_governance.md` — FOUND (file-existence + content verified; outside git repo by design, not committed).
- `MEMORY.md` pointer (`project_design_system_governance`) — FOUND.
- Commit `f39f9a0d` — FOUND in `git log`.
