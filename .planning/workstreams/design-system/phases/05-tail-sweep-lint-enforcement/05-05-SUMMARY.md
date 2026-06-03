---
phase: 05-tail-sweep-lint-enforcement
plan: 05
subsystem: frontend-design-system
tags: [design-system, lint-gate, governance, DS-GOV-01, DS-GOV-02, allowlist, adjudication, makefile, ci-gate]
requires:
  - "05-01/05-02/05-03 tail-sweeps (off-palette + brand-alias clean src/**/*.vue — ZERO survivors)"
  - "05-04 --accent flip to var(--elevated) (DS-MIGRATE-05) — brand var(--accent) is now itself a violation"
  - "scripts/i18n-lint.sh (the custom-lint pattern mirrored)"
provides:
  - "Build-failing color/token lint gate (scripts/design-system-lint.sh) enforcing exactly 3 rules"
  - "Adjudicated path:hex:reason allowlist (scripts/design-system-allowlist.txt, 33 justified entries)"
  - "make lint-design wired into make lint-frontend (CI/all path) AND redeploy-web (deploy gate)"
  - "DESIGN-SYSTEM.md 'Lint gate (enforced)' docs matching the enforced rules exactly"
  - "--selftest provable fail-path (detects bg-red-500, passes on clean tree, leaves tree clean)"
  - "SC#5 full-suite green: vue-tsc exit 0 + vitest 831 pass / 1 documented pre-existing fail"
affects:
  - "frontend/web/scripts/design-system-lint.sh"
  - "frontend/web/scripts/design-system-allowlist.txt"
  - "Makefile"
  - "frontend/web/src/styles/DESIGN-SYSTEM.md"
  - "frontend/web/src/styles/__tests__/design-tokens.spec.ts (Rule-1 stale-test realignment)"
tech-stack:
  added: []
  patterns:
    - "Custom bash lint gate mirroring i18n-lint.sh (set -euo pipefail, colored ERRORS, per-rule sections, Summary block, exit 1 on errors) — NO new dependency"
    - "Per-(file,hex) allowlist: a hex is allowed only in the file that lists it; path:hex:reason format, # comments"
    - "Brand-exemption regex: cyan|pink|orange|rose|indigo|teal|lime deliberately ABSENT from the off-palette set"
    - "--selftest fail-path proof: inject scratch bg-red-500, assert DETECT, trap-guarded cleanup, assert clean-tree PASS"
    - "Adjudication-not-blanket-allowlist for out-of-scope hex (Auth.vue Telegram blue, Collections.vue gradient)"
key-files:
  created:
    - "frontend/web/scripts/design-system-lint.sh"
    - "frontend/web/scripts/design-system-allowlist.txt"
    - ".planning/workstreams/design-system/phases/05-tail-sweep-lint-enforcement/05-05-SUMMARY.md"
  modified:
    - "Makefile"
    - "frontend/web/src/styles/DESIGN-SYSTEM.md"
    - "frontend/web/src/styles/__tests__/design-tokens.spec.ts"
decisions:
  - "Brand-exemption regex uses the Phase-4 set (red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc); cyan/pink (brand primitives) + orange/rose/indigo/teal/lime (provider hues) excluded — including them would fail the clean tree"
  - "RULE 2 allows a hex per-(file,hex): the offending file path AND the hex must both appear on one non-comment allowlist line"
  - "Auth.vue Telegram blue #54a9eb/#4a96d2 ADJUDICATED -> justified provider-brand allowlist (Telegram official brand blue, third-party identity, analogous to per-provider player hues); confirmed it styles the 'Open in Telegram' button"
  - "Auth.vue QR-code colors #000000/#ffffff ADJUDICATED -> functional allowlist (qrcode.js canvas dark/light, not theme colors)"
  - "Collections.vue gradient #0e7490/#6b21a8 ADJUDICATED -> keep-and-allowlist (identical to CollectionsRow's kept novel gradient; #0e7490 is darker/desaturated teal != brand-cyan #00d4ff, #6b21a8 darker purple != brand-violet #a78bfa; a token gradient would shift the cover hue out of tolerance) — NOT migrated"
  - "var(--token,#fallback) canonical-fine fallbacks (#00d4ff/#11111c/#fff/#08080f/#00ff9d) allowlisted with reason since RULE 2's raw-hex grep flags them"
  - "LastUpdates #4c1d95 allowlisted as in-code-comment-only (the live value was migrated to var(--brand-violet) in 05-03); ActivityFeed #1a3a4a/#0e2030 teal gradient allowlisted (analytics-workstream file; color lines repointed in 05-03 Task 3)"
  - "[Rule 1 - Bug] design-tokens.spec.ts:27 stale assertion fixed (05-04 flipped --accent->var(--elevated) but missed the test); realigned to the committed intended behavior. Exposed by this plan's SC#5 verification"
metrics:
  duration: "~20 min"
  completed: "2026-06-03"
  tasks: 2
  files_modified: 5
---

# Phase 05 Plan 05: Build-Failing Design-System Lint Gate (DS-GOV-01 + DS-GOV-02) Summary

Built the build-failing color/token lint gate that locks the design-system migration shut: a custom bash
`design-system-lint.sh` (mirroring `i18n-lint.sh`) enforcing exactly 3 rules (off-palette classes,
non-allowlisted hex, deprecated `--ink/--accent/--pink` aliases), a seeded + ADJUDICATED `path:hex:reason`
allowlist, `make lint-design` wired into both `lint-frontend` (CI/`all`) and `redeploy-web` (deploy gate),
DESIGN-SYSTEM.md docs matching the enforced rules exactly, a `--selftest` provable fail-path, and the SC#5
full-suite green verification. DS-GOV-01 + DS-GOV-02 satisfied.

## What Was Built

**Task 1 — gate script + adjudicated allowlist** (commit `4701f8bc`):
- `scripts/design-system-lint.sh` (246 lines): `set -euo pipefail`, colored ERRORS/WARNINGS, per-rule
  `=== … ===` sections, a `=== Summary ===` block with per-rule counts, `exit 1` on errors / `exit 0` +
  PASS otherwise. Avoids the SIGPIPE-under-pipefail footgun (collects grep output into vars, `|| true`).
  - **RULE 1** (off-palette classes) — Phase-4 verbatim regex; brand/provider hues
    (cyan|pink|orange|rose|indigo|teal|lime) deliberately absent.
  - **RULE 2** (non-allowlisted hex) — collects every `file:line:hex`, skips a hit iff a non-comment
    allowlist line names BOTH the file path AND the hex (per-(file,hex)).
  - **RULE 3** (deprecated `var(--ink|--accent|--pink)`) — excludes literal survivors
    `--ink-2/--ink-4/--accent-soft/--accent-line/--accent-glow/--pink-soft`.
  - **`--selftest`** — injects a scratch `bg-red-500` `.vue`, asserts RULE 1 DETECTS it, trap-removes the
    scratch, asserts the clean tree PASSES; prints `SELFTEST PASS`.
- `scripts/design-system-allowlist.txt` (33 justified `path:hex:reason` entries) — seeded from the 05-01 +
  05-03 novel-hex inventories and Phase-4 player/chrome decisions, then EMPIRICALLY completed by running
  RULE 2 over the clean tree and justifying every remaining hex until the gate passes.

**Task 2 — make wiring + docs + selftest + full-suite green** (commit `e969c9ec`):
- `Makefile`: new `lint-design` target (`@cd frontend/web && bash scripts/design-system-lint.sh`);
  `lint-frontend: lint-design`; `redeploy-web: i18n-lint lint-design type-check`; `lint-design` added to
  the `.PHONY` block.
- `DESIGN-SYSTEM.md`: new "Lint gate (enforced)" subsection — the 3 enforced rules, the brand-exemption
  rationale (why cyan/pink/orange/rose are NOT off-palette), the allowlist file path + `path:hex:reason`
  format + how to add an exception, the adjudication rule for out-of-scope hex, and the `--selftest` proof
  command. Documented scope matches the enforced rules exactly (no documented-but-unenforced rule —
  the Phase-6 DS-GOV-03 precondition).
- `design-tokens.spec.ts:27` Rule-1 stale-test realignment (see Deviations).

## Final Allowlist (33 entries, path:hex:reason)

| path | hex | reason |
|------|-----|--------|
| KodikPlayer.vue | #06b6d4 | Kodik player-accent (cyan) provider-identity hue |
| AnimeLibPlayer.vue | #f97316 | AniLib player-accent (orange) provider-identity hue |
| HanimePlayer.vue | #ec4899 | Hanime player-accent (pink) provider-identity hue |
| browse/BrowseSidebar.vue | #f97316 | AniLib provider-identity hue (decorative accent) |
| player/SubtitleOverlay.vue | #ffffff | default subtitle text color (render default) |
| player/SubtitleOverlay.vue | #ffcccc | default subtitle outline/secondary color (render default) |
| layout/BrandMark.vue | #08080f | brand base fallback in var(--color-base,#08080f) |
| layout/Navbar.vue | #1a3a4a | avatar teal gradient START (novel chrome hue) |
| layout/Navbar.vue | #0e2030 | avatar teal gradient END (novel chrome hue) |
| layout/Navbar.vue | #00ff9d | online-dot success fallback in var(--color-success,#00ff9d) |
| layout/Navbar.vue | #08080f | online-dot base-ring fallback in var(--color-base,#08080f) |
| ActivityFeed.vue | #1a3a4a | avatar teal gradient START (same as Navbar) |
| ActivityFeed.vue | #0e2030 | avatar teal gradient END (same as Navbar) |
| spotlight/cards/FeaturedCard.vue | #001218 | .btn-primary-hero text-on-cyan near-base ink |
| spotlight/cards/PlatformStatsCard.vue | #0d2030 | ambient card-glow gradient START (near-base) |
| spotlight/cards/PlatformStatsCard.vue | #050a12 | ambient card-glow gradient END (near-base) |
| spotlight/cards/NowWatchingCard.vue | #0a0e1a | live-dot avatar-ring near-base |
| home/CollectionsRow.vue | #0e7490 | collection-cover fallback gradient START (cyan-tinted) |
| home/CollectionsRow.vue | #6b21a8 | collection-cover fallback gradient END (violet-tinted) |
| home/CollectionsRow.vue | #11111c | surface fallback in var(--color-surface,#11111c) |
| home/ContinueWatchingRow.vue | #001218 | .cw-play icon-on-cyan near-base ink |
| home/ContinueWatchingRow.vue | #00d4ff | brand-cyan fallback/raw glow (canonical-fine brand value) |
| home/ContinueWatchingRow.vue | #11111c | surface fallback in var(--color-surface,#11111c) |
| home/ContinueWatchingRow.vue | #fff | foreground fallback in var(--foreground,#fff) |
| LastUpdates.vue | #0e7490 | .update-thumb--empty cover gradient START (cyan-tinted) |
| LastUpdates.vue | #4c1d95 | in-code comment only (live value migrated to var(--brand-violet) in 05-03) |
| views/Auth.vue | #54a9eb | Telegram provider brand blue (official) — ADJUDICATED |
| views/Auth.vue | #4a96d2 | Telegram provider brand blue hover (official) — ADJUDICATED |
| views/Auth.vue | #000000 | QR-code module dark color (functional, qrcode.js) — ADJUDICATED |
| views/Auth.vue | #ffffff | QR-code module light color (functional, qrcode.js) — ADJUDICATED |
| views/Collections.vue | #0e7490 | hero fallback gradient START — no exact token pair; ADJUDICATED keep |
| views/Collections.vue | #6b21a8 | hero fallback gradient END — no exact token pair; ADJUDICATED keep |

## Adjudication Decisions (out-of-scope hex — files NOT touched by 05-01/02/03)

- **Auth.vue `#54a9eb`/`#4a96d2`** → **allowlisted-with-reason** (NOT migrated). Confirmed they style the
  "Open in Telegram" button (`bg-[#54a9eb] hover:bg-[#4a96d2]`, src/views/Auth.vue ~L58). Telegram's
  official brand blue is a third-party provider-identity color, directly analogous to the per-provider
  player accent hues already allowlisted. Reason recorded.
- **Auth.vue `#000000`/`#ffffff`** → **allowlisted-with-reason** (functional, NOT theme). They are the
  `qrcode.js` canvas module colors (`color: { dark: '#000000', light: '#ffffff' }`, ~L209) — a functional
  black/white QR rendering, not a design token.
- **Collections.vue `#0e7490`/`#6b21a8`** → **allowlisted-with-reason / adjudicated keep** (NOT migrated).
  The pair is the IDENTICAL cyan→purple fallback gradient kept verbatim for `CollectionsRow.vue` in 05-01.
  `#0e7490` is a darker/desaturated teal (not `--brand-cyan` `#00d4ff`) and `#6b21a8` a darker purple (not
  `--brand-violet` `#a78bfa`); a `var(--brand-cyan)`→`var(--brand-violet)` gradient would shift the rendered
  cover hue out of tolerance, so no faithful token gradient is achievable. Adjudicated keep — consistent
  with the CollectionsRow decision. (Collections.vue therefore NOT added to files_modified.)

## ActivityFeed.vue disposition

Its color lines were repointed in 05-03 Task 3 (via stash-isolation; the analytics-workstream changes stay
uncommitted). The kept teal-gradient hex (`#1a3a4a`/`#0e2030`, same novel chrome hue as Navbar) are
allowlisted with reason. The pre-existing uncommitted analytics changes to ActivityFeed.vue were NOT staged
into either of this plan's commits (verified).

## Acceptance Evidence

- `bash scripts/design-system-lint.sh` on the clean tree: **PASS, exit 0** (RULE1=0 RULE2=0 RULE3=0).
- RULE 2 genuinely enforces: with an emptied allowlist the gate reports **39 non-allowlisted hex / exit 1**;
  with the seeded allowlist it reports **0 / exit 0**.
- `bash scripts/design-system-lint.sh --selftest`: **SELFTEST PASS, exit 0** — detects the injected
  `bg-red-500` then passes on the reverted clean tree, leaving NO scratch file (`src/__ds_lint_selftest__.vue`
  absent; no new untracked src file).
- `make lint-design`: **exit 0**. `lint-frontend` depends on `lint-design`; `redeploy-web` prereqs are
  `i18n-lint lint-design type-check`; `lint-design` is in `.PHONY` (confirmed via awk over the multi-line
  `.PHONY` block).
- **SC#5** — `bunx vue-tsc --noEmit`: **exit 0**. `bunx vitest run` (FULL): **831 passed / 1 failed (63
  files)** — the sole failure is the documented pre-existing `AnimeContextMenu.spec.ts:227` (phase-04
  deferred-items.md), a Reka DropdownMenu `reference`-prop plumbing detail unrelated to color/token. Home +
  spotlight unit specs: **223/223 pass**.
- **e2e canary** — `bunx playwright test e2e/spotlight.spec.ts` requires a `webServer` on `localhost:5173`
  (config spins up the dev server when `BASE_URL` is unset); it cannot run reliably headless in this sandbox
  (2-min timeout was dev-server startup, not a test failure). Documented per plan ("if it needs a running
  server and can't run headless, document that and run what you can"). This plan adds only lint
  tooling + Makefile + docs + one stale-test fix — no rendered/route/component/spotlight-code change — so
  there is no plausible mechanism for an e2e regression; the 223 home/spotlight unit specs cover the
  carousel logic headlessly and all pass.
- `git status --porcelain`: this plan's 2 commits touch ONLY plan files (2 scripts; Makefile + DESIGN-SYSTEM.md
  + design-tokens.spec.ts). Pre-existing uncommitted changes (ActivityFeed analytics, scraper, changelog,
  maintenance, STATE.md, issues.json) were NOT swept in.

## Deviations from Plan

**1. [Rule 1 - Bug] Realigned stale `design-tokens.spec.ts:27` assertion**
- **Found during:** Task 2 SC#5 full-suite verification (`bunx vitest run`).
- **Issue:** `design-tokens.spec.ts:27` asserted `--accent: var(--brand-cyan)` ("stays brand-cyan for P1
  back-compat"). Plan **05-04** (`7127275f`, DS-MIGRATE-05) deliberately flipped `--accent` to the shadcn
  hover surface (`--accent: var(--elevated)`) and deleted the temp brand-cyan alias, but missed updating
  this co-located token test. The assertion was therefore stale (asserting the pre-flip value), failing
  against the committed intended behavior.
- **Fix:** Updated the assertion to `--accent: var(--elevated)` and renamed the test to reflect the flip,
  with an explanatory comment citing 05-04/DS-MIGRATE-05. No behavioral logic changed — only the asserted
  value, brought in line with the already-committed `main.css` flip.
- **Files modified:** `frontend/web/src/styles/__tests__/design-tokens.spec.ts`.
- **Commit:** `e969c9ec` (Task 2).
- **Scope note:** directly caused by a prior in-workstream commit and exposed by THIS plan's mandated SC#5
  full-suite gate; fixing it is required to leave the suite green (modulo the one documented
  AnimeContextMenu failure). Analogous to the 6 spec realignments in Plan 05-01.

## Known Stubs

None — pure governance/tooling. The gate adds no data sources, props, templates, or UI rendering paths.

## Threat Flags

None — no new network endpoints, auth paths, file access, or schema changes. A bash lint gate + allowlist +
Makefile wiring + docs + one stale-test fix.

## Self-Check: PASSED

- FOUND: frontend/web/scripts/design-system-lint.sh (commit 4701f8bc)
- FOUND: frontend/web/scripts/design-system-allowlist.txt (commit 4701f8bc)
- FOUND: Makefile lint-design wiring (commit e969c9ec)
- FOUND: frontend/web/src/styles/DESIGN-SYSTEM.md "Lint gate (enforced)" (commit e969c9ec)
- FOUND: frontend/web/src/styles/__tests__/design-tokens.spec.ts realignment (commit e969c9ec)
- FOUND commit 4701f8bc (Task 1)
- FOUND commit e969c9ec (Task 2)
