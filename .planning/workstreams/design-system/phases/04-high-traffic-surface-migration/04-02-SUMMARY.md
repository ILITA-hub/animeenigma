---
phase: 04-high-traffic-surface-migration
plan: 02
subsystem: frontend-design-system
tags: [migration, tokens, css, nav, layout, 404, value-preserving]
requires:
  - "Phase 1 canonical token system (main.css aliases --accent/--ink/--pink → canonical)"
  - "DESIGN-SYSTEM.md off-palette → semantic mapping (DS-MIGRATE-02)"
provides:
  - "Token-clean nav chrome (Navbar, BrandMark, FeedbackButton) on every surface"
  - "Token-clean 404 smoke surface (NotFound)"
  - "Documented novel-hex allowlist: Navbar teal avatar gradient + BrandMark --accent-glow"
affects:
  - "Every page (Navbar/BrandMark render globally)"
  - "404 / nonexistent routes (NotFound)"
tech-stack:
  added: []
  patterns:
    - "var(--alias, #fallback) → var(--canonical) repoint (zero-diff: alias === canonical value)"
    - "off-palette Tailwind utility → semantic token utility (green→success, purple→brand-violet)"
    - "novel-hex kept verbatim + allowlisted (no main.css token this phase; promotion → Phase 5)"
key-files:
  created:
    - ".planning/workstreams/design-system/phases/04-high-traffic-surface-migration/04-02-SUMMARY.md"
  modified:
    - "frontend/web/src/components/layout/FeedbackButton.vue"
    - "frontend/web/src/views/NotFound.vue"
    - "frontend/web/src/components/layout/Navbar.vue"
    - "frontend/web/src/components/layout/BrandMark.vue"
decisions:
  - "NotFound .btn:hover collapsed from #ff5252 → var(--destructive) per locked hex_classification (both reds map to --destructive); micro hover-darken removed by design — zero hex bar satisfied"
  - "Navbar avatar teal gradient #1a3a4a→#0e2030 kept verbatim + allowlisted (no token within tolerance, would shift hue); token promotion deferred to Phase 5 to avoid same-wave main.css conflict with 04-04"
  - "BrandMark --accent-glow shadow kept as the var (literal, no canonical alias)"
  - "--accent definition NOT flipped (Phase 5); main.css untouched so all 4 Phase-4 plans run parallel in Wave 1"
metrics:
  duration: "~4 min"
  completed: "2026-06-02"
  tasks: 2 auto + 1 checkpoint (auto-approved)
  files: 4
---

# Phase 4 Plan 02: Nav/Layout + 404 Token Migration Summary

Value-preserving migration of the global nav chrome (Navbar, BrandMark, FeedbackButton) and the
404 smoke surface (NotFound) off off-palette Tailwind classes and hardcoded hex onto canonical
tokens — repointing `var(--alias, #fallback)` brand fallbacks to canonical names and keeping the
novel teal gradient + brand glow verbatim under a documented allowlist.

## What Was Built

- **FeedbackButton.vue** — `text-green-400`→`text-success`; `bg-purple-500/20`+`border-purple-400/50`→`bg-brand-violet/20`+`border-brand-violet/50` (category chip); footer submit `bg-purple-500/20`/`/30`+`text-purple-300`→`bg-brand-violet/20`/`/30`+`text-brand-violet`.
- **NotFound.vue** — `.btn` + headings: `#ff6b6b`/`#ff5252`→`var(--destructive)`; `#fff`→`var(--foreground)`; `#999`→`var(--muted-foreground)`. `.btn` structure kept (token-only repoint, no Button-primitive swap — default zero-diff per primitive_note).
- **BrandMark.vue** — `var(--accent,#00d4ff)`→`var(--brand-cyan)` (gradient stop + "AE" letters), `var(--pink,#ff2d7c)`→`var(--brand-pink)`. Kept `var(--accent-glow)` (literal) and `var(--color-base,#08080f)` (canonical-fine).
- **Navbar.vue** — repointed alias usages: `var(--accent,#fallback)`→`var(--brand-cyan)` (brand B1, active underline, avatar initials), `var(--ink,#fallback)`→`var(--foreground)` (brand B2, nav-link hover/active, icon-btn/lang-pill hover), `var(--ink-3,#fallback)`→`var(--muted-foreground)` (nav-link/icon-btn rest). Left literal `--ink-2`/`--accent-line` (no canonical equivalent) and canonical-fine `var(--color-success,#00ff9d)`/`var(--color-base,#08080f)` fallbacks as-is.

## Zero-Diff Verification

Every repoint resolves to the identical computed value (verified against `main.css`):
- `--brand-cyan` = `--color-cyan-400` = `#00d4ff` (old `--accent` fallback)
- `--brand-pink` = `--color-pink-500` = `#ff2d7c` (old `--pink` fallback)
- `--foreground` = `#ffffff` (old `--ink` fallback)
- `--muted-foreground` = `rgba(255,255,255,0.56)` (old `--ink-3` fallback)
- `--accent` still aliases `--brand-cyan`, `--pink` still aliases `--brand-pink` — definitions untouched.

## Novel-Hex Allowlist (documented, kept verbatim — token promotion → Phase 5)

| Hex | File / line | Reason kept |
|-----|-------------|-------------|
| `#1a3a4a` → `#0e2030` | `Navbar.vue` `.avatar-nt` linear-gradient (~650) | Novel teal; no token within tolerance — snapping would shift the gradient hue. Phase 5 promotes to a token (deferred to avoid same-wave main.css conflict with 04-04). |
| `var(--accent-glow)` | `BrandMark.vue` box-shadow (14) | Literal in main.css, no canonical alias — kept as the var (no raw hex inlined). |
| `var(--color-base,#08080f)` | BrandMark 25, Navbar 682 | Canonical-fine fallback (matches `--color-base`). |
| `var(--color-success,#00ff9d)` | Navbar 681 | Canonical-fine fallback (matches `--color-success`). |

## Verification Results

- **Off-palette grep** (FeedbackButton + NotFound): ZERO hits.
- **Hex grep** (NotFound): ZERO hits.
- **Alias grep** (`var(--ink|--accent|--pink)` modulo literals) on Navbar + BrandMark: ZERO repointable hits.
- **Hex grep** (Navbar + BrandMark): ONLY the documented allowlist entries (teal gradient + canonical-fine fallbacks).
- **vitest**: 830 passed; 1 pre-existing out-of-scope failure (`AnimeContextMenu.spec.ts` — see Deferred Issues).
- **vue-tsc --noEmit** (after `rm -f *.tsbuildinfo`): exit 0, zero errors.
- **vite build** (production bundle): exit 0, clean. (Used `bunx vite build` directly; the `bun run build` `vue-tsc` step hits a known pre-existing unrelated analytics-spec TS2307.)

## Checkpoint (Task 3, human-verify)

Auto mode ACTIVE → ⚡ Auto-approved. Automated checks ran green (vitest modulo out-of-scope spec, vue-tsc clean, vite build clean). In-browser visual smoke (nav + 404 at desktop ~1440px + mobile ~390px) auto-approved per objective — no live browser required. Zero-diff is structurally guaranteed since every repoint resolves to the identical computed value and the novel hex is kept verbatim.

## Deviations from Plan

None functional. One locked-mapping note:
- **[Note — locked mapping] NotFound `.btn:hover`**: the original darkened `#ff6b6b`→`#ff5252` on hover; the plan's hex_classification locks BOTH reds to `var(--destructive)`, so the hover now resolves to the same `--destructive` (`#ff4d4d`). The hover micro-darken is intentionally removed by the locked mapping (satisfies the "zero hex on NotFound" acceptance bar). The `transition: background` rule is preserved.

## Deferred Issues (out of scope — SCOPE BOUNDARY)

- **`AnimeContextMenu.spec.ts` 1/9 failing** (`forwards anchorEl to the DropdownMenu reference prop`). Confirmed PRE-EXISTING: reproduces on the committed tree with all 04-02 working-tree changes stashed. `AnimeContextMenu.vue`/`.spec.ts` are NOT in this plan's `files_modified` (they belong to 04-01 / the Phase-3 DropdownMenu kebab rebuild). Already logged in `deferred-items.md`. Not fixed here.

## Threat Flags

None — pure presentational token/class migration; no new endpoints, auth paths, file access, or schema changes (matches T-04-02 `accept` disposition).

## Self-Check: PASSED

All 4 modified SFCs + SUMMARY.md present on disk; both task commits (`2b7ef083`, `c943c877`) present in git log.
