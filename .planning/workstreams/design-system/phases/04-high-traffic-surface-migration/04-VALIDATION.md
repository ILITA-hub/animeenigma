---
phase: 4
slug: high-traffic-surface-migration
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-02
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | vitest (jsdom) + vue-tsc; no `test` npm script — invoke `bunx` directly |
| **Config file** | `frontend/web/vitest.config.ts` |
| **Quick run command** | `cd frontend/web && bunx vue-tsc --noEmit` |
| **Full suite command** | `cd frontend/web && bunx vitest run && bunx vue-tsc --noEmit && bun run build` |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** Run the acceptance grep on the just-migrated file(s) (off-palette regex + hex regex must return zero hits) + `bunx vue-tsc --noEmit`.
- **After every plan wave:** Run `bunx vitest run` + `bunx vue-tsc --noEmit`.
- **Before `/gsd-verify-work`:** Full suite green + standing 5-surface in-browser smoke (desktop + mobile) clean — jsdom cannot catch cascade regressions (DS-NF-06).
- **Max feedback latency:** ~90 seconds (automated) / manual for in-browser smoke.

---

## Per-Task Verification Map

> Planner fills one row per task. Acceptance grep is the primary automated gate; in-browser smoke is the manual "zero rendered change" gate.

| Task ID | Plan | Wave | Requirement | Threat Ref | Secure Behavior | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|------------|-----------------|-----------|-------------------|-------------|--------|
| 4-01-01 | 01 | 1 | DS-MIGRATE-01/02/03 | — | N/A | grep + tsc | acceptance grep (zero off-palette/hex) on migrated files; `bunx vue-tsc --noEmit` | ✅ | ⬜ pending |

---

## Wave 0 Requirements

*Existing infrastructure covers all phase requirements — vitest + vue-tsc + build already in place from Phases 2–3. No new framework install.*

The single new automated gate is the **acceptance grep** (already specified in 04-UI-SPEC / 04-RESEARCH):
- off-palette regex: `(bg|text|border|ring|from|to|via|fill|stroke|decoration|outline|shadow|accent|caret|divide|ring-offset)-(red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|gray|slate|zinc|neutral|stone)-[0-9]`
- hex regex: `#[0-9a-fA-F]{3,8}\b` (outside the documented novel-hex allowlist)

Both must return **zero** hits in the migrated files.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Zero rendered change on the 5 standing surfaces | DS-MIGRATE-01, DS-NF-06 | jsdom cannot render the Tailwind cascade / layered vs unlayered CSS; only a real browser reveals cascade shifts | Smoke Home (spotlight + rails), Browse/catalog, Anime detail (cyan «Смотреть», badges, language pills), one player surface, NotFound (404) at desktop + mobile widths; confirm pixel-equivalent to pre-migration |
| Novel-hex per-case judgment did not shift a brand hue | DS-MIGRATE-03 | Some player `--player-accent` hues / SubtitleOverlay defaults are intentionally novel; snapping to a token would change rendering | For each allowlisted hex, confirm it was either kept verbatim (allowlist) or replaced with a token that resolves to the identical computed color |

---

## Validation Sign-Off

- [ ] All tasks have an automated verify (acceptance grep + tsc) or are in the Manual-Only table
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references (none — existing infra)
- [ ] No watch-mode flags (`bunx vitest run`, not `vitest`)
- [ ] Feedback latency < 90s (automated)
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
