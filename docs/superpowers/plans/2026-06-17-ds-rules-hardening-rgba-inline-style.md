# DS Lint Rules 6 & 7 + rgba/hsl Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add two build-enforced DS lint rules (Rule 6: no raw rgba/hsl in `.vue`; Rule 7: no static color in inline `style`) and migrate the ~277 existing rgba literals to a curated token set so the tree passes.

**Architecture:** Three-phase: (1) extend `design-system-lint.sh` with Rules 6/7 + selftest cases; (2) add ~13 alpha tokens to `main.css`; (3) run a one-shot deterministic codemod that maps each rgba value → token (whitespace-insensitive, value-based snapping), then triage the residue into allowlist (identity/decorative) or manual fixes (off-palette grays, Anime.vue). The codemod is value-based so it is context-independent and safe to run globally.

**Tech Stack:** Bash (lint), CSS custom properties (Tailwind v4 `@theme`/`:root`), a throwaway Bun/ESM codemod, Vitest + vue-tsc for verification.

**Source spec:** `docs/superpowers/specs/2026-06-17-ds-rules-hardening-rgba-inline-style-design.md`

**Shared-tree discipline (from project memory):** This repo is a shared working tree with concurrent agents. Use path-scoped commits (`git commit <pathspec> -m …`), run `git show --stat HEAD` after every commit, push after every commit (realtime backup). NEVER `git add -A`, `git stash`, or `git commit --amend` in this tree. If a push is rejected, land via a `git worktree` on `origin/main`, then `git reset --mixed origin/main` (never `--hard`).

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `frontend/web/scripts/design-system-lint.sh` | The gate | Add `run_rule6`, `run_rule7`, counters, Summary lines, selftest cases, allowlist-matches-rgba helper | 
| `frontend/web/scripts/design-system-allowlist.txt` | Justified exceptions | Add identity/decorative `path:value:reason` lines |
| `frontend/web/src/styles/main.css` | Token source of truth | Add ~13 alpha tokens in `:root` |
| `frontend/web/scripts/ds-rgba-codemod.mjs` | One-shot migration | Create, run, then DELETE (never committed) |
| ~40 `.vue` files | Consumers | Literal → `var(--token)` (done by codemod + manual triage) |
| `CLAUDE.md`, `frontend/web/src/styles/DESIGN-SYSTEM.md` | Docs | 5 rules → 7; list new tokens |

---

## Task 1: Add the alpha tokens to `main.css`

**Files:**
- Modify: `frontend/web/src/styles/main.css` (insert after the `--line-strong:` definition, currently line 375)

- [ ] **Step 1: Insert the token block**

Find the line `  --line-strong: rgba(255, 255, 255, 0.12);` and insert immediately after it:

```css
  /* Alpha overlay scales — DS Rules 6/7 (curated; see specs/2026-06-17-ds-rules-hardening). */
  --white-a4: rgba(255, 255, 255, 0.04);
  --white-a8: rgba(255, 255, 255, 0.08);
  --white-a20: rgba(255, 255, 255, 0.20);
  --white-a30: rgba(255, 255, 255, 0.30);
  --cyan-a08: rgba(0, 212, 255, 0.08);
  --cyan-a20: rgba(0, 212, 255, 0.20);
  --cyan-a40: rgba(0, 212, 255, 0.40);
  --cyan-a60: rgba(0, 212, 255, 0.60);
  --black-a40: rgba(0, 0, 0, 0.40);
  --black-a60: rgba(0, 0, 0, 0.60);
  --black-a80: rgba(0, 0, 0, 0.80);
  --scrim-bg-soft: rgba(8, 8, 15, 0.40);
  --scrim-bg-strong: rgba(8, 8, 15, 0.85);
```

- [ ] **Step 2: Verify the tokens parse (no build, just grep)**

Run: `grep -nE '\-\-(white|cyan|black|scrim-bg)-a?' frontend/web/src/styles/main.css | head`
Expected: the 13 new lines appear.

- [ ] **Step 3: Commit**

```bash
git commit frontend/web/src/styles/main.css -m "feat(ds): add curated alpha overlay tokens (white/cyan/black/scrim)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -6
git push
```

> Note: these tokens reference `rgba(...)` literals inside `main.css`, NOT a `.vue` file — Rule 6 scopes to `*.vue` only, so the token definitions themselves are never flagged.

---

## Task 2: Add Rule 6 (no raw rgba/hsl in `.vue`) to the lint script

**Files:**
- Modify: `frontend/web/scripts/design-system-lint.sh`

- [ ] **Step 1: Add the Rule 6 regex + counter near the other regex defs**

After the `FONT_RE='...'` line (~line 70), add:

```bash
# Raw rgba()/rgb()/hsla()/hsl() color literals (comma OR modern space/slash form).
# var()-based forms (e.g. rgba(var(--player-accent-rgb), .3)) are token-based and
# exempt — filtered out in run_rule6 by a grep -v 'var('.
RGBA_RE='(rgba?|hsla?)\([0-9 .,/%]+\)'
```

And in the per-rule counter block (after `RULE5_ERRORS=0`, ~line 52) add:

```bash
RULE6_ERRORS=0
RULE7_ERRORS=0
```

- [ ] **Step 2: Add a shared allowlist-match helper (handles hex AND rgba, whitespace-insensitive)**

After the `relpath()` function (~line 83) add:

```bash
# allow_hit <relpath> <normalized-literal> <allow_file> — returns 0 (allowed) iff a
# non-comment allowlist line names BOTH this path AND this literal. The literal is
# compared whitespace-stripped so `rgba(0, 0, 0, .5)` and `rgba(0,0,0,.5)` match the
# same allowlist entry. Allowlist format stays `path:value:reason`.
allow_hit() {
  local rel="$1" lit="$2" allow_file="$3"
  awk -F: -v p="$rel" -v h="$lit" '
    /^[[:space:]]*(#|$)/ { next }
    {
      line=$0; gsub(/[[:space:]]/,"",line)
      if (index($0,p) && index(line,h)) { found=1 }
    }
    END { exit !found }
  ' "$allow_file"
}
```

- [ ] **Step 3: Add the `run_rule6` function**

After `run_rule5()` (~line 242) add:

```bash
# ============================================================================
# RULE 6 — raw rgba()/rgb()/hsl() color literals in .vue (use a token)
# ============================================================================
run_rule6() {
  echo ""
  echo "=== RULE 6: raw rgba()/hsl() literals (use a token; var() forms exempt) ==="

  local allow_file
  allow_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$allow_file'" RETURN
  [ -f "$ALLOWLIST" ] && cp "$ALLOWLIST" "$allow_file"

  local hits
  hits=$(grep -rnoE "$RGBA_RE" "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' | grep -v 'var(' || true)

  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No raw rgba()/hsl() literals"
    return 0
  fi

  local any=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file lineno lit rel norm
    file="${line%%:*}"
    local tail="${line#*:}"
    lineno="${tail%%:*}"
    lit="${tail#*:}"
    rel="$(relpath "$file")"
    norm="$(echo "$lit" | tr -d '[:space:]')"
    if allow_hit "$rel" "$norm" "$allow_file"; then continue; fi
    echo -e "  ${RED}ERROR${NC} ${rel}:${lineno}: ${lit} (not in allowlist)"
    RULE6_ERRORS=$((RULE6_ERRORS + 1)); ERRORS=$((ERRORS + 1)); any=1
  done <<< "$hits"

  [ "$any" -eq 0 ] && echo -e "  ${GREEN}OK${NC} All rgba()/hsl() literals are allowlisted"
}
```

- [ ] **Step 4: Wire `run_rule6` into the entry point and Summary**

In the entry-point block (after `run_rule5` call, ~line 306) add `run_rule6`.
In the Summary block (after the RULE 5 echo, ~line 314) add:

```bash
echo -e "  Raw rgba/hsl lits   (RULE 6): ${RULE6_ERRORS}"
```

- [ ] **Step 5: Run the lint to confirm Rule 6 now reports the existing literals (expected to FAIL pre-migration)**

Run: `bash frontend/web/scripts/design-system-lint.sh 2>&1 | grep -E 'RULE 6|Raw rgba' ; echo "exit=$?"`
Expected: Rule 6 prints many ERROR lines and a non-zero `RULE6_ERRORS` count (this is correct — migration happens in Task 5). Do NOT commit a red tree yet; continue to Task 3.

---

## Task 3: Add Rule 7 (no static color in inline `style`) to the lint script

**Files:**
- Modify: `frontend/web/scripts/design-system-lint.sh`

- [ ] **Step 1: Add the `run_rule7` function**

After `run_rule6()` add:

```bash
# ============================================================================
# RULE 7 — static color literal inside an inline style="…" / :style="'…'" attr
# ============================================================================
# Flags ONLY hardcoded color (#hex | rgb( | hsl() inside an inline style attribute.
# Dynamic object/array bindings (:style="{ width: pct }") and px/%/transform/layout
# values are NOT the DS concern and are not flagged. var() forms are exempt.
run_rule7() {
  echo ""
  echo "=== RULE 7: static color in inline style=/:style attr (use a class/token) ==="

  local allow_file
  allow_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$allow_file'" RETURN
  [ -f "$ALLOWLIST" ] && cp "$ALLOWLIST" "$allow_file"

  # Lines containing a style attribute whose value carries a hardcoded color.
  local hits
  hits=$(grep -rnE ':?style=("|'"'"')[^"'"'"']*(#[0-9a-fA-F]{3,8}|rgba?\(|hsla?\()' "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' | grep -v 'var(' || true)

  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No static color in inline style attributes"
    return 0
  fi

  local any=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file lineno rel norm
    file="${line%%:*}"
    local tail="${line#*:}"
    lineno="${tail%%:*}"
    rel="$(relpath "$file")"
    norm="$(echo "$line" | tr -d '[:space:]')"
    # Allow if the offending literal on this line is allowlisted for this path.
    if grep -qF "$rel" "$allow_file" 2>/dev/null && allow_hit "$rel" "$norm" "$allow_file"; then continue; fi
    echo -e "  ${RED}ERROR${NC} ${rel}:${lineno}: inline style color (use a class/token)"
    RULE7_ERRORS=$((RULE7_ERRORS + 1)); ERRORS=$((ERRORS + 1)); any=1
  done <<< "$hits"

  [ "$any" -eq 0 ] && echo -e "  ${GREEN}OK${NC} All inline style colors are allowlisted"
}
```

- [ ] **Step 2: Wire `run_rule7` into entry point and Summary**

Add `run_rule7` after `run_rule6` in the entry-point block, and in Summary add:

```bash
echo -e "  Inline style colors (RULE 7): ${RULE7_ERRORS}"
```

- [ ] **Step 3: Extend `--selftest` to prove Rules 6 & 7 fail-paths**

In `run_selftest()`, reset the new counters in the clean-tree assertion (the `ERRORS=0; RULE1_ERRORS=0; …` line, ~line 280) by appending `RULE6_ERRORS=0; RULE7_ERRORS=0` and adding `run_rule6 >/dev/null` and `run_rule7 >/dev/null` after `run_rule5 >/dev/null`.

Then, just before the clean-tree section (after the bg-red-500 detection block, ~line 274), add a second injection proving Rule 6 + Rule 7 catch color:

```bash
  # 1b) Injected rgba literal + inline-style color must be detected by Rule 6 / Rule 7.
  cat > "$scratch" <<'EOF'
<template>
  <div class="text-[rgba(1,2,3,0.5)]" style="background: rgba(4, 5, 6, 0.5); color:#abc">x</div>
</template>
EOF
  local r6=0 r7=0
  grep -qE "$RGBA_RE" "$scratch" && r6=1
  grep -qE ':?style=("|'"'"')[^"'"'"']*(#[0-9a-fA-F]{3,8}|rgba?\()' "$scratch" && r7=1
  if [ "$r6" -ne 1 ] || [ "$r7" -ne 1 ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — Rule 6/7 did NOT detect injected color (r6=$r6 r7=$r7)"
    rm -f "$scratch"; trap - EXIT; exit 1
  fi
  echo -e "  ${GREEN}DETECTED${NC} injected rgba literal (R6) + inline-style color (R7)"
```

- [ ] **Step 4: Run the selftest — fail-path must pass even though the real tree is still red**

Run: `bash frontend/web/scripts/design-system-lint.sh --selftest 2>&1 | tail -8 ; echo "exit=$?"`
Expected: `DETECTED injected bg-red-500`, `DETECTED injected rgba literal … (R7)`, then `SELFTEST FAIL — clean tree did NOT pass` is EXPECTED right now because the real literals remain — that confirms detection works. (After Task 5 the selftest will fully pass.)

> Do not commit the script yet — commit happens in Task 4 once the allowlist exists, so the gate has something coherent to read.

---

## Task 4: Seed the allowlist with identity/decorative exceptions

**Files:**
- Modify: `frontend/web/scripts/design-system-allowlist.txt`

- [ ] **Step 1: Append the identity/decorative allowlist block**

Append these lines (format `path:value:reason`; values stored whitespace-stripped to match the normalized comparison):

```
# --- DS Rule 6: identity/decorative rgba literals (not tokenized) ---
# Gacha rarity-tier identity palette (data-driven gradient stops; like providerRegistry.ts hex).
src/components/gacha/CardViewer3D.vue:rgb(45,212,191):gacha rarity identity (teal)
src/components/gacha/CardViewer3D.vue:rgb(251,146,60):gacha rarity identity (orange)
src/components/gacha/CardViewer3D.vue:rgb(129,140,248):gacha rarity identity (indigo)
src/components/gacha/CardViewer3D.vue:rgb(167,139,250):gacha rarity identity (violet)
src/components/gacha/CardViewer3D.vue:rgba(40,40,60,0.5):gacha bespoke glow ramp
src/components/gacha/CardViewer3D.vue:rgba(120,120,140,0.28):gacha bespoke glow ramp
src/components/gacha/CardViewer3D.vue:rgba(2,2,8,0.96):gacha bespoke glow ramp endpoint
src/components/gacha/DropsModal.vue:rgb(45,212,191):gacha rarity identity (teal)
src/components/gacha/DropsModal.vue:rgb(251,146,60):gacha rarity identity (orange)
src/components/gacha/DropsModal.vue:rgb(129,140,248):gacha rarity identity (indigo)
src/components/gacha/GemCeremony.vue:rgb(45,212,191):gacha rarity identity (teal)
src/components/gacha/GemCeremony.vue:rgb(251,146,60):gacha rarity identity (orange)
src/components/gacha/GemCeremony.vue:rgb(129,140,248):gacha rarity identity (indigo)
src/components/gacha/PullSummary.vue:rgb(45,212,191):gacha rarity identity (teal)
src/components/gacha/PullSummary.vue:rgb(251,146,60):gacha rarity identity (orange)
src/components/gacha/PullSummary.vue:rgb(129,140,248):gacha rarity identity (indigo)
# Decorative opaque dark gradient ramps (spotlight/gacha backdrops; not part of the surface ladder).
src/components/home/spotlight/SpotlightBackdrop.vue:rgb(11,11,24):decorative backdrop ramp
src/components/home/spotlight/SpotlightBackdrop.vue:rgb(16,16,28):decorative backdrop ramp
src/components/home/spotlight/cards/RandomTailCard.vue:rgb(13,13,28):decorative ramp
```

> The exact set of identity/decorative lines is finalized in Task 5 Step 4 from the live lint output — this block seeds the obvious ones; add any residual identity values the codemod intentionally skips.

- [ ] **Step 2: Commit the lint script + allowlist together**

```bash
git commit frontend/web/scripts/design-system-lint.sh frontend/web/scripts/design-system-allowlist.txt -m "feat(ds): lint Rules 6 (raw rgba/hsl) + 7 (inline-style color) + identity allowlist

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -6
git push
```

---

## Task 5: Run the codemod migration

**Files:**
- Create (then DELETE — never commit): `frontend/web/scripts/ds-rgba-codemod.mjs`
- Modify: ~40 `.vue` files (by the codemod)

- [ ] **Step 1: Write the codemod**

Create `frontend/web/scripts/ds-rgba-codemod.mjs`:

```js
// One-shot, value-based rgba→token codemod. Run once, then delete (do NOT commit).
// Maps are context-independent: a given color value always maps to the same token,
// so a global whitespace-insensitive replace is safe. Unmapped values are reported
// and left untouched (they become allowlist entries or manual fixes).
import { readFileSync, writeFileSync } from 'node:fs';
import { execSync } from 'node:child_process';

// canonical key = comma-joined numeric components, alpha via parseFloat (".10"→"0.1").
const MAP = {
  // white overlays
  '255,255,255,0.01': '--white-a4', '255,255,255,0.025': '--white-a4',
  '255,255,255,0.03': '--white-a4', '255,255,255,0.04': '--white-a4', '255,255,255,0.05': '--white-a4',
  '255,255,255,0.06': '--line',
  '255,255,255,0.07': '--white-a8', '255,255,255,0.08': '--white-a8',
  '255,255,255,0.1': '--border',
  '255,255,255,0.11': '--line-strong', '255,255,255,0.12': '--line-strong', '255,255,255,0.14': '--line-strong',
  '255,255,255,0.16': '--white-a20', '255,255,255,0.18': '--white-a20', '255,255,255,0.2': '--white-a20',
  '255,255,255,0.22': '--white-a20', '255,255,255,0.25': '--white-a20',
  '255,255,255,0.28': '--white-a30', '255,255,255,0.3': '--white-a30', '255,255,255,0.35': '--white-a30',
  '255,255,255,0.36': '--ink-4', '255,255,255,0.4': '--ink-4',
  '255,255,255,0.45': '--muted-foreground', '255,255,255,0.55': '--muted-foreground',
  '255,255,255,0.56': '--muted-foreground', '255,255,255,0.6': '--muted-foreground',
  '255,255,255,0.65': '--ink-2', '255,255,255,0.7': '--ink-2', '255,255,255,0.78': '--ink-2', '255,255,255,0.85': '--ink-2',
  '255,255,255,0': 'transparent', '255,255,255': '--foreground',
  // brand cyan
  '0,212,255,0.05': '--cyan-a08', '0,212,255,0.08': '--cyan-a08', '0,212,255,0.1': '--cyan-a08',
  '0,212,255,0.12': '--accent-soft', '0,212,255,0.14': '--accent-soft', '0,212,255,0.16': '--accent-soft',
  '0,212,255,0.18': '--cyan-a20', '0,212,255,0.2': '--cyan-a20', '0,212,255,0.22': '--cyan-a20',
  '0,212,255,0.25': '--accent-line', '0,212,255,0.28': '--accent-line', '0,212,255,0.3': '--accent-line',
  '0,212,255,0.35': '--cyan-a40', '0,212,255,0.4': '--cyan-a40', '0,212,255,0.45': '--cyan-a40', '0,212,255,0.5': '--cyan-a40',
  '0,212,255,0.6': '--cyan-a60',
  '0,212,255,0.8': '--brand-cyan', '0,212,255,0.9': '--brand-cyan', '0,212,255,0.95': '--brand-cyan',
  // anime.vue shiki cyan-400 (brand-align to brand cyan)
  '34,211,238': '--brand-cyan', '34,211,238,0.4': '--cyan-a40',
  // black scrims
  '0,0,0,0.25': '--black-a40', '0,0,0,0.35': '--black-a40', '0,0,0,0.4': '--black-a40', '0,0,0,0.45': '--black-a40',
  '0,0,0,0.5': '--black-a60', '0,0,0,0.55': '--black-a60', '0,0,0,0.6': '--black-a60',
  '0,0,0,0.65': '--black-a60', '0,0,0,0.66': '--black-a60',
  '0,0,0,0.7': '--black-a80', '0,0,0,0.72': '--black-a80', '0,0,0,0.78': '--black-a80',
  '0,0,0,0.8': '--black-a80', '0,0,0,0.82': '--black-a80',
  // dark-bg scrims
  '8,8,15,0': 'transparent',
  '8,8,15,0.1': '--scrim-bg-soft', '8,8,15,0.25': '--scrim-bg-soft', '8,8,15,0.3': '--scrim-bg-soft', '8,8,15,0.4': '--scrim-bg-soft',
  '8,8,15,0.65': '--scrim-bg-strong', '8,8,15,0.72': '--scrim-bg-strong', '8,8,15,0.75': '--scrim-bg-strong',
  '8,8,15,0.85': '--scrim-bg-strong', '8,8,15,0.88': '--scrim-bg-strong', '8,8,15,0.92': '--scrim-bg-strong', '8,8,15,0.96': '--scrim-bg-strong',
  '2,2,8,0.93': '--scrim-bg-strong',
  // semantic/status colors → *-soft tokens
  '0,184,230,0.14': '--primary-soft',
  '0,255,157,0.12': '--success-soft', '0,255,157,0.14': '--success-soft', '0,255,157,0.2': '--success-soft', '16,185,129,0.2': '--success-soft',
  '255,214,0,0.12': '--warning-soft', '255,214,0,0.14': '--warning-soft', '255,214,0,0.2': '--warning-soft', '245,158,11,0.2': '--warning-soft',
  '14,165,233,0.2': '--info-soft',
  '255,45,124,0.14': '--brand-pink-soft', '255,45,124,0.18': '--brand-pink-soft',
  // off-palette gray stray (badge bg) → subtle neutral fill
  '107,114,128,0.2': '--white-a20',
};

const files = execSync(
  "git ls-files 'frontend/web/src/**/*.vue' | grep -v -E '\\.spec\\.|/__tests__/'",
  { encoding: 'utf8' }
).trim().split('\n').filter(Boolean);

const unmapped = new Map();
let edited = 0;

for (const f of files) {
  const src = readFileSync(f, 'utf8');
  const out = src.replace(/\b(rgba?|hsla?)\(([^)]*)\)/gi, (m, _fn, body) => {
    if (body.includes('var(')) return m;                 // token-based → skip
    const parts = body.split(/[\s,/]+/).filter(Boolean);
    if (parts.length < 3 || parts.some((p) => !/^[0-9.]+%?$/.test(p))) return m; // hsl% or odd → skip
    const key = parts.map((p) => String(parseFloat(p))).join(',');
    const tok = MAP[key];
    if (!tok) { unmapped.set(key, (unmapped.get(key) || 0) + 1); return m; }
    return tok === 'transparent' ? 'transparent' : `var(${tok})`;
  });
  if (out !== src) { writeFileSync(f, out); edited++; }
}

console.log(`edited ${edited} files`);
console.log('UNMAPPED (left untouched — allowlist or manual fix):');
[...unmapped.entries()].sort((a, b) => b[1] - a[1]).forEach(([k, n]) => console.log(`  ${n}x  ${k}`));
```

- [ ] **Step 2: Run the codemod**

Run: `cd frontend/web && bun scripts/ds-rgba-codemod.mjs`
Expected: prints `edited N files` and an `UNMAPPED` list. The unmapped list should be only identity/decorative values (teal/orange/indigo/violet/pink rgb, opaque dark gradient stops, solid `0,0,0`, sparse pink high-alphas).

- [ ] **Step 3: Delete the codemod (one-shot, never committed)**

Run: `rm frontend/web/scripts/ds-rgba-codemod.mjs`

- [ ] **Step 4: Run the lint and triage residue into the allowlist**

Run: `bash frontend/web/scripts/design-system-lint.sh 2>&1 | grep -A100 'RULE 6' | grep ERROR`
For each remaining ERROR: if it is an identity/decorative value (matches the UNMAPPED list), add a `path:value:reason` line to `design-system-allowlist.txt` (extend the Task 4 block). Re-run until Rule 6 + Rule 7 report 0.

- [ ] **Step 5: Sanity-check the 3 brand-cyan ≥.8 → solid sites and Anime.vue brand-align**

Run: `git diff -- frontend/web/src/views/Anime.vue | grep -E 'brand-cyan|cyan-a40|ink-4'`
Confirm the shiki `:deep` link colors became `var(--brand-cyan)` / `var(--cyan-a40)` / `var(--ink-4)`. These are the documented intended hue/alpha shifts. If any ≥.8 cyan glow visibly needs translucency, replace its `var(--brand-cyan)` with `var(--cyan-a60)` instead.

- [ ] **Step 6: Verify the full gate passes**

Run: `bash frontend/web/scripts/design-system-lint.sh 2>&1 | tail -12 ; echo "exit=$?"`
Expected: `PASS: No design-system color/token violations.` and `exit=0`.

- [ ] **Step 7: Verify the selftest now fully passes**

Run: `bash frontend/web/scripts/design-system-lint.sh --selftest 2>&1 | tail -6 ; echo "exit=$?"`
Expected: `CLEAN TREE PASSES` + `SELFTEST PASS` + `exit=0`.

- [ ] **Step 8: Type-check and unit tests stay green**

Run: `cd frontend/web && bunx tsc --noEmit && bunx vitest run 2>&1 | tail -15`
Expected: tsc clean; vitest all pass. (No spec asserts on literal rgba strings; if any snapshot did, update it.)

- [ ] **Step 9: Commit the migration (path-scoped to the changed .vue files + allowlist)**

```bash
git add -- $(git diff --name-only -- 'frontend/web/src/**/*.vue') frontend/web/scripts/design-system-allowlist.txt
git commit -- $(git diff --cached --name-only) -m "refactor(ds): migrate rgba/hsl literals to curated alpha tokens (Rules 6/7 green)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -20
git push
```

> If `git diff --name-only` is noisy from other agents' parallel work, list only the files the codemod touched (from its `edited N files` run) and pass those exact paths to `git commit -- <paths>`.

---

## Task 6: Update the docs

**Files:**
- Modify: `CLAUDE.md` (Design System "Lint gate" section)
- Modify: `frontend/web/src/styles/DESIGN-SYSTEM.md` (token tiers)

- [ ] **Step 1: Update CLAUDE.md rule count + add Rules 6/7**

In the "Lint gate (build-ENFORCED)" paragraph, change "enforces 5 color/token/typography/primitive rules" to "enforces 7 …", and after the Rule 5 bullet add:

```markdown
6. **No raw `rgba()`/`hsl()` color literals in `.vue`** — use an alpha token (`--white-a8`, `--cyan-a20`, `--black-a60`, `--scrim-bg-*`, or a semantic `*-soft`). Matches both `rgba(0,0,0,.5)` and modern `rgb(0 0 0 / .5)`. `rgb*(var(--…))` forms are exempt; `.vue` only (`.ts` color data stays intentional). Identity/decorative literals go in `design-system-allowlist.txt` (`path:value:reason`).
7. **No static color inside inline `style`** — `style="…"`/`:style="'…'"` carrying `#hex`/`rgb()`/`hsl()` is forbidden; use a class or token. Dynamic bindings and `px`/layout values are NOT flagged.
```

- [ ] **Step 2: Add the alpha-token tier to DESIGN-SYSTEM.md**

Add a subsection listing the new tokens and the curated-snap rationale (one paragraph + the token list from Task 1). Reference the spec.

- [ ] **Step 3: Commit the docs**

```bash
git commit CLAUDE.md frontend/web/src/styles/DESIGN-SYSTEM.md -m "docs(ds): document lint Rules 6/7 + alpha overlay tokens

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
git show --stat HEAD | head -6
git push
```

---

## Task 7: Ship + verify

- [ ] **Step 1: Run the after-update skill**

Invoke `/animeenigma-after-update`. `make redeploy-web` runs `design-system-lint.sh` as a hard deploy prereq — it must exit 0 (Task 5 Step 6 guarantees this). The skill also adds the Russian-Trump-mode changelog entry, health-checks, and commits/pushes.

- [ ] **Step 2: Offer an opt-in Chrome smoke (per feedback_chrome_smoke_opt_in)**

This pass touches `<style>` blocks + `main.css` tokens (cascade-sensitive), and jsdom/vitest cannot catch Tailwind-v4 cascade bugs. ASK the owner whether they want a Chrome checkup focused on: player overlays (control bar, source/subs panels, scrub bar), home rows (Continue/Recs/Collections scrollbars + cards), modals (BrowseSubs), and gacha (admin-gated). Run only if they say yes.

---

## Self-Review

**Spec coverage:** Rule 6 → Task 2; Rule 7 → Task 3; modern space/slash syntax → Task 2 (`RGBA_RE`) + codemod parser; var() exemption → Tasks 2/3 + codemod skip; ~13 tokens → Task 1; curated snap tables → codemod `MAP`; gacha included (white/cyan/black migrated, rarity allowlisted) → Tasks 4/5; allowlist mechanism extended to rgba → Task 2 `allow_hit`; selftest fail-path → Task 3; verification (lint/selftest/tsc/vitest) → Task 5; docs → Task 6; deploy gate + opt-in Chrome smoke → Task 7. No gaps.

**Placeholder scan:** No "TBD/TODO/handle edge cases". Task 4 Step 1 notes the allowlist is finalized from live output in Task 5 Step 4 — that is a deterministic procedure (triage the printed UNMAPPED list), not a placeholder; the codemod's `MAP` is complete for every value found in the 2026-06-17 audit.

**Type/name consistency:** Token names identical across Task 1, codemod `MAP`, and docs (`--white-a4/a8/a20/a30`, `--cyan-a08/a20/a40/a60`, `--black-a40/a60/a80`, `--scrim-bg-soft/strong`). Function names `run_rule6`/`run_rule7`/`allow_hit` consistent. Counter names `RULE6_ERRORS`/`RULE7_ERRORS` consistent.

**Known acceptable shifts (documented):** `.8/.9/.95` cyan → solid `--brand-cyan`; `.45–.6` white → `--muted-foreground`; `.65–.85` white → `--ink-2`; Anime.vue cyan-400 → brand cyan; gray-500 badge bg → `--white-a20`. All sparse; Task 5 Step 5 reviews the cyan-solid sites.
