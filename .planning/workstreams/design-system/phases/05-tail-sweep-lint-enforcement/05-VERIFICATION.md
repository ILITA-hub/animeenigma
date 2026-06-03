---
phase: 05-tail-sweep-lint-enforcement
verified: 2026-06-03T04:45:00Z
status: human_needed
score: 5/5 code-verifiable must-haves verified (2 human-smoke items open)
overrides_applied: 0
human_verification:
  - test: "In-browser smoke of the --accent flip across the 5 standing surfaces (Home spotlight + rails, Browse/catalog, Anime detail, a player surface, 404)."
    expected: "Elements that previously read bg-accent / var(--accent) as a HOVER surface now show the neutral elevated hover (#1c1c2c), NOT the old brand-cyan. No brand-cyan element (which now reads --brand-cyan directly) accidentally went neutral. Hover interactive bg-accent elements to confirm the corrected surface."
    why_human: "Tailwind v4 cascade footgun (reference_tailwind_v4_css_cascade): unlayered custom classes beat utilities and a real-browser cascade regression cannot be reproduced in jsdom. The vite build does not render the cascade. The --accent flip is the milestone's ONLY intentional rendered change (SC#2) and must be confirmed against a live deploy. Auto-APPROVED in autonomous mode and persisted in 05-04-HUMAN-UAT.md."
  - test: "Run the spotlight e2e canary (frontend/web/e2e/spotlight.spec.ts) against a live dev server."
    expected: "Carousel + token-clean surfaces render green; no visual regression from the tail-sweep migration."
    why_human: "Playwright is installed (v1.58.0) but the canary needs a live dev server / real Chromium and could not run headless in the sandbox (documented limitation). This is the e2e portion of SC#5; vitest + vue-tsc (the headless portion) are green."
---

# Phase 5: Tail Sweep + Lint Enforcement Verification Report

**Phase Goal:** Migrate the remaining components, complete the alias retirement, flip `--accent`, then lock the door with a build-failing lint gate.
**Verified:** 2026-06-03T04:45:00Z
**Status:** human_needed
**Re-verification:** No — initial verification

## Goal Achievement

### Observable Truths

| # | Truth (ROADMAP Success Criteria) | Status | Evidence |
| --- | --- | --- | --- |
| 1 | Repo-wide grep: zero off-palette color classes, zero hardcoded hex (outside allowlist), zero `var(--ink\|--accent\|--pink)` brand usages | ✓ VERIFIED | Independent greps over `src/**/*.vue` (excl spec/__tests__): RULE1 off-palette = **0**, RULE3 alias usages (minus literal survivors) = **0**, 36 allowlisted survivor refs remain (allowed). RULE2: all 39 hex occurrences across 32 (file,hex) pairs are allowlisted (lint RULE2 = 0). Brand hues (cyan/pink/orange/rose/indigo/teal/lime) are BRAND-EXEMPT per DS-GOV-02 and correctly NOT in the regex set. |
| 2 | `--accent` resolves to the shadcn hover surface; temp brand-cyan alias deleted; no visual regression (in-browser smoke) | ⚠️ CODE-VERIFIED / human smoke pending | `main.css:405` → `--accent: var(--elevated)` (`--elevated: #1c1c2c`, line 398) with comment "flipped from brand-cyan P5". Zero `var(--accent)` brand usages in `src/`. Temp brand-cyan alias annotation/NOTE block deleted (no `--accent ... brand-cyan` definition remains; only the documenting comment). `--brand-cyan`, `--accent-soft/-line/-glow` intact. In-browser visual smoke → human item #1 (Tailwind v4 cascade). |
| 3 | Lint rule FAILS the build on a deliberate `bg-red-500`, passes on clean tree; wired into deploy gate | ✓ VERIFIED | `bash scripts/design-system-lint.sh --selftest` → DETECTED injected bg-red-500, then CLEAN TREE PASSES, exit 0, scratch file removed (tree clean). `make lint-design` → PASS, exit 0. Wired: Makefile `lint-frontend: lint-design` (L149) AND `redeploy-web: i18n-lint lint-design type-check` (L270). |
| 4 | Allowlist/escape-hatch documented | ✓ VERIFIED | `scripts/design-system-allowlist.txt` — 33 `path:hex:reason` entries, all live (none stale — every entry's hex still present in its file). Documented in `DESIGN-SYSTEM.md` §"Lint gate (enforced) — DS-GOV-01 / DS-GOV-02" incl. per-(file,hex) rule, adjudication policy, brand-exemption rationale, and `--selftest` instructions. |
| 5 | Full vitest + tsc + e2e green | ⚠️ tsc+vitest VERIFIED / e2e human | `bunx vue-tsc --noEmit` → exit 0. `bunx vitest run` → **831 pass / 1 fail (832 total)**. The SOLE failure is `AnimeContextMenu.spec.ts:227` — confirmed PRE-EXISTING (last touched by Phase-03/04 commits 6322d955/63b00179/7b11666a, NOT phase 5; logged in `04.../deferred-items.md` L19-28 with git-stash provenance), NOT a phase-5 regression. e2e canary → human item #2 (needs live server). |

**Score:** 5/5 code-verifiable must-haves verified. SC#2 in-browser smoke + SC#5 e2e are the only open items → human verification.

### Required Artifacts

| Artifact | Expected | Status | Details |
| --- | --- | --- | --- |
| `frontend/web/scripts/design-system-lint.sh` | 3-rule build-failing gate + `--selftest` | ✓ VERIFIED | 247 lines; RULE1 off-palette, RULE2 hex-vs-allowlist, RULE3 alias; `set -euo pipefail`; exit 1 on ERRORS; selftest passes. |
| `frontend/web/scripts/design-system-allowlist.txt` | `path:hex:reason` allowlist | ✓ VERIFIED | 33 adjudicated entries, all live; comment header documents format + policy. |
| `frontend/web/src/styles/main.css` (`--accent` flip) | `--accent` → shadcn hover surface, temp alias deleted | ✓ VERIFIED | L405 `var(--elevated)`; brand-cyan alias removed. |
| `frontend/web/src/styles/DESIGN-SYSTEM.md` | Lint gate + allowlist docs | ✓ VERIFIED | §"Lint gate (enforced)" L54+ documents all 3 rules, allowlist, brand-exemption, selftest. |
| `Makefile` (`lint-design` wiring) | Wired into lint-frontend + redeploy-web | ✓ VERIFIED | L149 `lint-frontend: lint-design`; L270 `redeploy-web: i18n-lint lint-design type-check`. |
| `frontend/web/src/components/ActivityFeed.vue` | Stash-isolated color repoint | ✓ VERIFIED | +63 lines; only hex are allowlisted teal gradient (`#1a3a4a`/`#0e2030`); 0 off-palette/alias. |

### Key Link Verification

| From | To | Via | Status | Details |
| --- | --- | --- | --- | --- |
| `make lint-frontend` | `design-system-lint.sh` | `lint-design` sub-target | ✓ WIRED | Makefile L149 + L152-153. |
| `make redeploy-web` | `design-system-lint.sh` | `lint-design` prereq | ✓ WIRED | Makefile L270 (deploy gate). |
| `design-system-lint.sh` RULE2 | `design-system-allowlist.txt` | reads `$ALLOWLIST` per-(file,hex) | ✓ WIRED | L28, L99-130; mktemp parse + awk match. |
| `--accent` token | `--elevated` (#1c1c2c) | `var(--elevated)` | ✓ WIRED | main.css L405 → L398. |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
| --- | --- | --- | --- |
| Lint gate passes clean tree | `make lint-design` | exit 0, all 3 rules OK | ✓ PASS |
| Lint gate fail-path (SC#3) | `design-system-lint.sh --selftest` | DETECTED bg-red-500 → clean PASS, exit 0, tree clean | ✓ PASS |
| Off-palette grep (SC#1) | RULE1 regex over src/*.vue | 0 hits | ✓ PASS |
| Alias-usage grep (SC#1) | RULE3 regex minus survivors | 0 hits | ✓ PASS |
| Type-check (SC#5) | `bunx vue-tsc --noEmit` | exit 0 | ✓ PASS |
| Unit tests (SC#5) | `bunx vitest run` | 831 pass / 1 pre-existing fail | ✓ PASS (regression-free) |
| Spotlight e2e (SC#5) | `bunx playwright test spotlight` | needs live dev server | ? SKIP → human |

### Requirements Coverage

| Requirement | Source Plan | Description | Status | Evidence |
| --- | --- | --- | --- | --- |
| DS-MIGRATE-02 | 05-01/02/03 | Off-palette Tailwind → semantic tokens | ✓ SATISFIED | RULE1 grep = 0 across full tree; lint passes. (REQUIREMENTS.md still marks ⏳ — stale checkbox; codebase proves done.) |
| DS-MIGRATE-03 | 05-01/03 | Hardcoded hex → tokens (or allowlisted) | ✓ SATISFIED | RULE2 = 0; all surviving hex adjudicated into allowlist. |
| DS-MIGRATE-04 | 05-01/03 | Deprecated alias usages repointed | ✓ SATISFIED | RULE3 = 0 (minus literal survivors). |
| DS-MIGRATE-05 | 05-04 | Flip `--accent`, drop temp brand-cyan alias | ✓ SATISFIED (visual smoke → human) | main.css L405; 0 brand var(--accent) usages; 05-04-HUMAN-UAT.md. |
| DS-GOV-01 | 05-05 | Build-failing lint gate wired into redeploy-web | ✓ SATISFIED | selftest + Makefile L270 wiring. |
| DS-GOV-02 | 05-05 | Allowlist/escape-hatch documented | ✓ SATISFIED | allowlist.txt + DESIGN-SYSTEM.md docs. |

Note: REQUIREMENTS.md still renders DS-MIGRATE-02/03/04/05 as ⏳ (unchecked). This is a stale tracking checkbox, NOT a goal gap — the codebase (zero off-palette/hex/alias, lint green) is authoritative evidence the migration is complete. Recommend the orchestrator flip these to ✅. Not a blocker.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
| --- | --- | --- | --- | --- |
| (none) | — | No TBD/FIXME/XXX in lint script, allowlist, ActivityFeed, or main.css | ℹ️ Info | Clean — no unreferenced debt markers. |

### Human Verification Required

1. **`--accent` flip in-browser smoke (SC#2)** — Load Home (spotlight + rails), Browse/catalog, Anime detail, a player surface, and 404 on the live deploy. Confirm `bg-accent`/`var(--accent)` HOVER surfaces now render neutral elevated (#1c1c2c) not brand-cyan, and that brand-cyan elements (read from `--brand-cyan` directly) did NOT go neutral. Hover interactive `bg-accent` elements. *Why human:* Tailwind v4 unlayered-cascade footgun is unreproducible in jsdom; vite build doesn't render the cascade.

2. **Spotlight e2e canary (SC#5)** — `bunx playwright test spotlight` against a live dev server. *Why human:* needs a real browser + dev server; could not run headless in the sandbox.

### Gaps Summary

No code-verifiable gaps. All five ROADMAP success criteria are satisfied at the code level: the tail sweep is complete (zero off-palette classes, zero non-allowlisted hex, zero deprecated brand-alias usages across `src/**/*.vue`), `--accent` is flipped to the shadcn `--elevated` hover surface with the temp brand-cyan alias deleted, the build-failing lint gate proves its fail-path via `--selftest` and is wired into both `make lint-frontend` and `make redeploy-web`, the allowlist/escape-hatch is documented, and vue-tsc + vitest are green (the single vitest failure is the documented Phase-3/4 `AnimeContextMenu.spec.ts:227` pre-existing failure, not a phase-5 regression).

The only open items are the two inherently human/non-headless checks the phase itself flagged: the in-browser visual smoke of the `--accent` flip (the milestone's sole intentional rendered change, auto-approved in autonomous mode and persisted in 05-04-HUMAN-UAT.md) and the spotlight e2e canary requiring a live dev server. Status is therefore `human_needed`, not `passed`.

---

_Verified: 2026-06-03T04:45:00Z_
_Verifier: Claude (gsd-verifier)_
