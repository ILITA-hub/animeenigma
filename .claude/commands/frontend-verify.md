---
allowed-tools: Bash(*), Read(*), Glob(*), Grep(*), AskUserQuestion(*)
description: FE/DS pre-flight — design-system + i18n + real-build gates for any frontend/web change, before commit / after-update
---

# /frontend-verify — Frontend & Design-System pre-flight

Run this after any change under `frontend/web/` and BEFORE `/animeenigma-after-update` (or any commit). It runs the repo's frontend gates in dependency order and encodes the traps that have bitten us before. Stop at the first hard failure, fix it, re-run from that gate.

**Auto-context:** a `PostToolUse` hook (`.claude/hooks/ds-lint-postedit.sh`) already runs the DS gate on every `frontend/web/src/**/*.{vue,ts}` edit, so RULE 1–8 violations surface live as you work. This command is the explicit full sweep (DS + i18n + lint + types + tests) before you ship.

## What changed?

```bash
git -C "$CLAUDE_PROJECT_DIR" diff --name-only HEAD -- frontend/web | sort -u
```

If nothing under `frontend/web/` changed, there is nothing to verify — say so and stop.

## Gates (run in order; fix-then-rerun on failure)

**Fast path:** gates 1 + 3 + 4 + 5 (DS-lint · eslint · build · vitest) are bundled in **`bin/ae-fe-verify.sh <touched-file …>`**, which prints ~5 status lines instead of ~120 and dumps only the failing gate. Run that; add gate 2 (i18n) only when locale JSON changed, and the cascade Chrome smoke only if opted-in (below). The per-gate commands below are the manual fallback / reference.

### 1. Design-System lint (build-enforced)
```bash
cd "$CLAUDE_PROJECT_DIR/frontend/web" && bash scripts/design-system-lint.sh
```
- `ERRORS>0 ⇒ exit 1`, and it FAILS THE BUILD (prerequisite of `make lint-frontend` AND `make redeploy-web`).
- **Brand/provider hues are EXEMPT** (`cyan pink orange rose indigo teal lime`) — Neon-Tokyo brand + per-provider accents. Do NOT "fix" those.
- On failure: migrate to a semantic token (`text-destructive`, `bg-warning`, `text-muted-foreground`, `--white-a20`, …). ONLY if no token reproduces the value, add a justified line to `scripts/design-system-allowlist.txt` (hex/rgba/inline-style) or `scripts/design-system-spacing-allowlist.txt` (spacing) — **never disable the gate**. Prove a fail-path with `bash scripts/design-system-lint.sh --selftest`.

### 2. i18n — all three locales (en / ru / ja)
```bash
bash "$CLAUDE_PROJECT_DIR/frontend/web/scripts/i18n-lint.sh"            # flaky → retry ONCE before trusting a failure
cd "$CLAUDE_PROJECT_DIR/frontend/web" && bunx vitest run src/locales/__tests__
```
- Every new key MUST exist in `en.json`, `ru.json`, AND `ja.json`. A key in one file but not the others fails the parity specs (`locale-parity.spec.ts` + per-feature key specs) and blocks redeploy.
- ICU placeholders (`{count}`, `{name}`) must match across all three locales.
- After adding keys, **smoke-verify the actual `t()` paths** you call resolve (typo'd keys lint clean but render the raw key).

### 3. Lint (eslint)
```bash
cd "$CLAUDE_PROJECT_DIR/frontend/web" && bun lint
```

### 4. Types — use the REAL build, not `--noEmit`
```bash
cd "$CLAUDE_PROJECT_DIR/frontend/web" && bun run build
```
- **`bunx vue-tsc --noEmit` (a.k.a. `bun run type-check`) can FALSE-PASS from a stale cache** — for anything touching types, trust only a real `bun run build` (`vue-tsc && vite build`).
- Import component **types from the `@/components/ui` barrel**; a named type import from a deep path can throw **TS2614** that `.vue` SFCs don't surface until a real build.

### 5. Unit tests for touched components
```bash
cd "$CLAUDE_PROJECT_DIR/frontend/web" && bunx vitest run <paths of touched components / co-located *.spec.ts>
```

## Cascade-sensitive change? (Tailwind v4)

jsdom/vitest **cannot** catch Tailwind-v4 cascade bugs — unlayered custom classes beat utilities, so a utility class can silently lose. If your change touches cascade-sensitive styling (custom CSS without `@layer`, specificity-dependent utility overrides, fullscreen/teleport surfaces like the player or `SubtitleOverlay`):

- Per **DS-NF-06**, an in-browser Chrome smoke is **OPT-IN, not mandatory**. For a small fix, skip it silently.
- For a non-small visual change, **ASK the owner** (AskUserQuestion) whether they want a Chrome checkup — and say explicitly that the change is cascade-sensitive, so jsdom/vitest won't have caught it.
- Do NOT run the Chrome smoke automatically (it costs tokens).

## Quick gotcha table

| Trap | Rule |
|---|---|
| Off-palette color (`text-red-500`) | Use a semantic token; brand hues (cyan/pink/orange/rose/indigo/teal/lime) are exempt |
| `vue-tsc --noEmit` passes but build breaks | Cache false-pass — run `bun run build` for type truth |
| Named type import → TS2614 | Import types from the `@/components/ui` barrel |
| lucide icons | **Named** imports only: `import { Play } from 'lucide-vue-next'` (never default/namespace) |
| New i18n key | Add to en + ru + ja with matching ICU placeholders; smoke the `t()` path |
| DS violation with no matching token | Justified allowlist line — never disable the gate |
| Tailwind-v4 style "lost" / overridden | Cascade bug — only a Chrome smoke catches it |
| Bare `<select>`/`<input type=checkbox>` | Use the Select/Checkbox/Switch/RadioGroup primitives (player/ is exempt) |

## When green

All gates pass → proceed to `/animeenigma-after-update` (lint/build, redeploy `web`, changelog, commit, push). This pre-flight does NOT deploy or commit — it only verifies. Canonical DS reference: `frontend/web/src/styles/DESIGN-SYSTEM.md`.
