---
phase: 04-high-traffic-surface-migration
plan: 04
subsystem: frontend-design-system
tags: [migration, tokens, players, value-preserving, neon-tokyo]
requires:
  - "Phase 1 token foundation (--success/--warning/--info/--destructive, --popover)"
  - "Phase 3 primitives (no Button swap on player chrome — DS-MIGRATE-06: no analog)"
provides:
  - "9 player SFCs token-clean of off-palette color classes"
  - "Documented novel-hex allowlist (per-player --player-accent ×3 + subtitle render ×2)"
affects:
  - "Watch surface (all 5 players + chrome helpers)"
tech-stack:
  added: []
  patterns:
    - "Off-palette → semantic token (red→destructive, amber/yellow→warning, emerald/green→success, blue→info)"
    - "Opacity modifier kept on base token when no -soft matches the alpha (e.g. text-warning/70)"
    - "Novel per-player/render hex kept verbatim + allowlisted (no main.css token — deferred to Phase 5)"
key-files:
  created: []
  modified:
    - frontend/web/src/components/player/KodikPlayer.vue
    - frontend/web/src/components/player/OtherSubsPanel.vue
    - frontend/web/src/components/player/ResumePill.vue
    - frontend/web/src/components/player/SubtitleSettingsMenu.vue
    - frontend/web/src/components/player/OurEnglishPlayer.vue
    - frontend/web/src/components/player/AnimeLibPlayer.vue
  verified-clean-no-edit:
    - frontend/web/src/components/player/HanimePlayer.vue
    - frontend/web/src/components/player/RawPlayer.vue
    - frontend/web/src/components/player/SubtitleOverlay.vue
decisions:
  - "Kept #06b6d4 (Kodik cyan), #f97316 (AnimeLib orange), #ec4899 (Hanime pink) player-accents verbatim — NOVEL, near-but-not-equal brand tokens; snapping would shift hue (Pitfall 2)."
  - "Kept SubtitleOverlay #ffffff (default text) + #ffcccc (rt render) verbatim — user-overridable render defaults, legitimately novel."
  - "No main.css token promotion — deferred to Phase 5 to avoid same-wave conflict with 04-02."
  - "bg-zinc-900/95 → bg-popover/95 (elevated menu surface, not bg-card)."
  - "Opacity modifiers (/20, /30, /50, /70, /80, /90) kept on base semantic tokens — no -soft token matches those alphas."
metrics:
  duration: "~10 min"
  completed: 2026-06-02
  tasks: 3
  files-modified: 6
---

# Phase 4 Plan 04: Player + Chrome Helper Token Migration Summary

Value-preserving color migration of all 5 video players (Kodik, AnimeLib, Hanime, OurEnglish, Raw) plus their chrome helpers (OtherSubsPanel, ResumePill, SubtitleSettingsMenu, SubtitleOverlay) off raw off-palette Tailwind classes onto the canonical Neon Tokyo semantic tokens, while keeping each player's deliberate `--player-accent` identity hue and SubtitleOverlay's user-overridable render defaults verbatim in a documented novel-hex allowlist.

## What Was Done

### Task 1 — KodikPlayer + 4 chrome helpers (commit `1b32c4ff`)
- **KodikPlayer.vue** — all 19 off-palette occurrences mapped:
  - line 37 `bg-yellow-500/90` → `bg-warning/90` (kept `text-black`)
  - line 132 `bg-green-500/20 text-green-400 border-green-500/50` → `bg-success/20 text-success border-success/50`
  - line 145 `bg-blue-500/20 text-blue-400 border-blue-500/50` → `bg-info/20 text-info border-info/50`
  - line 169 voice/sub group → `bg-success/20 border-success/50` / `bg-info/20 border-info/50`
  - line 171 `ring-amber-500/30` → `ring-warning/30`
  - line 180 `bg-amber-500/20 text-amber-400` → `bg-warning/20 text-warning`
  - line 194 solid `bg-green-500`/`bg-blue-500` → `bg-success`/`bg-info`
  - line 208 `bg-amber-500/20 text-amber-400 hover:bg-amber-500/30` → `bg-warning/20 text-warning hover:bg-warning/30`
- **OtherSubsPanel.vue** — `text-red-400` → `text-destructive`; `text-amber-400/80` → `text-warning/80`
- **ResumePill.vue** — `text-emerald-400/70` → `text-success/70`; `text-amber-400/70` → `text-warning/70`
- **SubtitleSettingsMenu.vue** — `bg-zinc-900/95` → `bg-popover/95` (elevated menu surface)
- **OurEnglishPlayer.vue** — `text-amber-400/80` → `text-warning/80`

### Task 2 — AnimeLib/Hanime/Raw + SubtitleOverlay (commit `f7f8d466`)
- **AnimeLibPlayer.vue** — `text-red-400/70` → `text-destructive/70` (only off-palette hit)
- **RawPlayer.vue** — re-grepped clean of both off-palette and hex; no edit needed
- **HanimePlayer.vue / SubtitleOverlay.vue** — only contained allowlisted novel hex; kept verbatim, no edit

### Task 3 — Checkpoint (auto-approved, auto mode ACTIVE)
- Automated checks run pre-gate, all green: full vitest (830 pass), `vue-tsc --noEmit` exit 0, `vite build` exit 0.
- In-browser visual smoke auto-approved per objective (no live browser required in auto mode).

## Novel-Hex Allowlist (kept verbatim, NO main.css token — deferred to Phase 5)

| Hex | File:line | Role | Why kept verbatim |
|-----|-----------|------|-------------------|
| `#06b6d4` | KodikPlayer.vue:1077 | `--player-accent` cyan | Near-but-not-equal brand-cyan; snapping shifts hue |
| `#f97316` | AnimeLibPlayer.vue:869 | `--player-accent` orange | No token within tolerance |
| `#ec4899` | HanimePlayer.vue:479 | `--player-accent` pink | Near-but-not-equal brand-pink; snapping shifts hue |
| `#ffffff` | SubtitleOverlay.vue:202 | default JP-subtitle text | User-overridable render default |
| `#ffcccc` | SubtitleOverlay.vue:336 | `rt` (furigana) render color | User-overridable render default |

## Verification

- Off-palette acceptance grep across all 9 player files: ZERO hits.
- Hex grep across the 4 hex-bearing files: ONLY the 5 allowlisted entries (player-accent ×3 + subtitle render ×2); zero undocumented hex.
- `bunx vitest run src/components/player`: 16/16 pass (Task 1); SubtitleOverlay 2/2 (Task 2).
- Full `bunx vitest run`: 830 pass, 1 pre-existing failure (`AnimeContextMenu.spec.ts:227`, Reka DropdownMenu anchored-mode — out of scope, in deferred-items.md).
- `rm -f *.tsbuildinfo && bunx vue-tsc --noEmit`: exit 0.
- `bunx vite build`: exit 0 (used per known-pre-existing-issue guidance — the full `bun run build` vue-tsc step fails on the untracked analytics spec, unrelated to this plan).

## Deviations from Plan

None — plan executed exactly as written. RawPlayer.vue was re-grepped per inventory and confirmed clean (no off-palette, no hex), so no edit was applied — as the plan anticipated ("map only if a hit appears").

## Out-of-Scope / Pre-existing (NOT fixed)

- `AnimeContextMenu.spec.ts:227` — 1 pre-existing failing test (Reka DropdownMenu anchored-mode, Phase 3); already logged in `deferred-items.md`.
- `src/analytics/__tests__/index.spec.ts` TS2307 missing barrel (untracked analytics-workstream file) — causes `bun run build`'s vue-tsc step to fail; validated bundle with `bunx vite build` instead.

## Notes

- `main.css` deliberately NOT touched — novel `--player-accent` hex kept in the SFC; token promotion + `--accent` semantic flip remain Phase 5 (DS-MIGRATE-05).
- Player chrome controls kept structurally intact — no Button primitive swap (DS-MIGRATE-06: no analog).
- This was the final Wave-1 plan of Phase 4.

## Self-Check: PASSED

All 7 modified/created files exist on disk; both task commits (`1b32c4ff`, `f7f8d466`) present in git log.
