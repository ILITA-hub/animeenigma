---
status: human_needed
phase: 07-structural-primitive-swap
verified: 2026-06-03
score: 4/4
verifier: orchestrator (gates re-run independently)
human_verification:
  - affected-surface in-browser smoke (SubtitleSettingsMenu timing menu + the 9 kept controls render unchanged at desktop + mobile) — deferred per DS-NF-06 standing rule; auto-approved in autonomous mode → HUMAN-UAT
---

# Phase 07: Structural Primitive Swap — Verification

**Goal:** Close DS-MIGRATE-06 + DS-MIGRATE-01 (primitive half) — swap remaining hand-rolled `<button>` onto `<Button>` where the API fits; keep + document the bespoke ones.
**Result:** PASSED (4/4 code-verifiable must-haves) — user accepted the "1 swap + 9 justified keeps" closure at the milestone audit (2026-06-03).

## Success Criteria

### SC#1 — Each raw `<button>` adjudicated (swap-where-fits OR keep+document) ✓
All 10 audit-enumerated sites resolved across 07-01 (Themes/Browse/AnimeCard — 8 keeps) + 07-02 (SubtitleSettingsMenu ×5 + CarouselDots — 1 swap, 6 keeps):
- **1 SWAP:** SubtitleSettingsMenu gear toggle → `<Button variant="ghost" size="sm">` (diff-confirmed `484de820`: `<button type="button">…</button>` → `<Button>`; data-test/:disabled/:title/aria-*/aria-haspopup/@click preserved; byte-identical render via cn() class overrides).
- **9 KEEPS** (each with an inline one-line reason): AnimeCard 60px round play overlay; Themes admin-sync soft pill, segmented type-filter (ButtonGroup), bare Retry text-link; Browse clear-recent + refresh-Shikimori text-links, `rounded-full` recent-search chips, `p-1` drawer-close icon, mobile-toggle (useFocusTrap `returnFocusTo` DOM-node target); SubtitleSettingsMenu 4 sub-scale nudge steppers + bare reset text-link; CarouselDots 4px scoped-CSS dot pills. Validated against the actual `Button` variant source (no `link`/text variant; `ghost` carries base bg+border; `icon`≈40×40; all variants opinionated) — the keeps are technically justified, not work-avoidance.

### SC#2 — No behavioral/visual regression ✓ (in-browser smoke = the human item)
`bunx vue-tsc --noEmit` exit 0; `bunx vite build` clean; `bunx vitest run` 831 pass / 1 fail (sole failure = pre-existing AnimeContextMenu.spec.ts:227, NOT this phase); co-located specs (SubtitleSettingsMenu 6 + CarouselDots 9) green.

### SC#3 — Lint gate still passes ✓
`make lint-design` exit 0 (RULE 1/2/3 = 0) — no off-palette/hex/alias reintroduced by the swap.

### SC#4 — DS-MIGRATE-06 + DS-MIGRATE-01 closed ✓
Both marked ✅ in REQUIREMENTS.md on the "where they fit" reading (user-accepted). 22 total `<Button>` consumers now cover every fitting case; the 9 bespoke controls are governance-only documented keeps.

## Why human_needed (not passed)
Only the standing 5-surface / affected-surface in-browser visual smoke remains (DS-NF-06 — jsdom can't catch cascade/render diffs). Auto-approved in autonomous mode; persisted as a HUMAN-UAT item for the end-of-milestone `make redeploy-web` deploy.

## Note
This phase was a deliberate post-audit addition. Its honest finding — that the remaining hand-rolled buttons are overwhelmingly bespoke controls the opinionated `Button` primitive can't model without a visible diff — is itself the value: it converts "unmet requirement" into "documented, justified, governance-only residual," with the swap path proven on the one case that fit.
