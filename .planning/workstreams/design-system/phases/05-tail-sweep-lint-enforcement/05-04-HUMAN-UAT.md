# 05-04 HUMAN-UAT — `--accent` flip in-browser smoke

**Plan:** 05-04 (design-system / Phase 05 tail-sweep-lint-enforcement)
**Requirement:** DS-MIGRATE-05
**Date:** 2026-06-03
**Mode:** Autonomous (`auto_advance: true`) — checkpoint auto-approved
**Commit under test:** `7127275f` — `refactor(05): flip --accent to shadcn hover surface, drop temp brand-cyan alias [DS-MIGRATE-05]`

## What changed (the ONLY intentional rendered change in the milestone)

`main.css :root --accent` flipped from `var(--brand-cyan)` (cyan `#00d4ff`) to
`var(--elevated)` (`#1c1c2c`), the neutral shadcn-vue hover surface — consistent with the
existing `--popover` / `--secondary` elevated tokens. The temporary brand-cyan alias
annotation and the "deferred to P2 / stays brand-cyan for back-compat" NOTE block were
deleted. `--brand-cyan`, `--accent-soft`, `--accent-line`, `--accent-glow` are untouched.

## Automated gate (green)

| Check | Result |
|---|---|
| Hard precondition — brand `var(--accent)` usages in `src/**/*.vue` (minus literal `-soft/-line/-glow`) | **0 hits** |
| `--accent` no longer `var(--brand-cyan)` | confirmed |
| `--brand-cyan` + literal `--accent-*` intact | confirmed |
| `bunx vue-tsc --noEmit` | exit 0 |
| `bunx vite build` | exit 0 |

## Visual smoke — disposition

**Auto-APPROVED** in autonomous mode. Per DS-NF-06 / reference_tailwind_v4_css_cascade, a
real-browser cascade regression cannot be caught by jsdom/unit tests and the automated build
does not render the cascade. The standing 5-surface in-browser smoke is therefore recorded
here as a **deferred HUMAN-UAT** to be confirmed against the live deploy:

1. Home — spotlight carousel (RandomTail purple cta-hero), Онгоинги / Топ rails.
2. Browse / catalog — filter sidebar, star badges, cards, pagination.
3. Anime detail — cyan `.btn-primary` ("Смотреть"), status badge, schedule pill, language pills.
4. A watch/player surface — one player loads with styled controls.
5. 404 — muted styling + "На главную" button.

**Specifically confirm:** elements that previously read `bg-accent` / `var(--accent)` as a
HOVER surface now show the intended neutral elevated hover (not the old cyan), and NO
brand-cyan element accidentally went neutral (brand-cyan elements read from `--brand-cyan`
directly, repointed in Wave-1, including ActivityFeed's hover/title links repointed in
05-03 Task 3). Hover interactive `bg-accent` elements to confirm the corrected surface.

**Status:** AUTO-APPROVED (autonomous). Persisted for human confirmation on next live smoke.
No code regression expected — the flip is a single token redefinition, structurally verified.
