#!/usr/bin/env bash
# design-system-lint — build-failing color/token-discipline gate (DS-GOV-01).
#
# Enforces 5 color/token/typography/primitive rules over frontend/web/src/**/*.vue
# (excluding *.spec.* and __tests__), mirroring scripts/i18n-lint.sh:
#
#   RULE 1 — Zero off-palette Tailwind color classes (ERROR). Scans *.vue AND
#            *.ts (component class-strings leak via cva variant files etc.),
#            excluding *-variants.ts (the canonical semantic-variant defs).
#   RULE 2 — Zero hardcoded hex outside scripts/design-system-allowlist.txt (ERROR).
#            *.vue only — *.ts hex is intentional brand/provider color data
#            (e.g. providerRegistry.ts) not subject to the class palette.
#   RULE 3 — Zero deprecated-alias var(--ink|--accent|--pink|--violet|--f-display|
#            --f-ui|--f-mono|--f-jp) usages (ERROR).
#   RULE 4 — Zero off-scale font weights font-(bold|extrabold|black|light|thin)
#            (ERROR; DS allows only font-medium/font-semibold). Scans *.vue + *.ts.
#   RULE 5 — Zero bare native form controls: <select>, <input type="date">,
#            <input type="checkbox">, <input type="radio"> (ERROR; use the
#            Select / DatePicker / Checkbox / Switch / RadioGroup primitives).
#            Exempts components/player/ (reka portals break in fullscreen) and
#            type="datetime-local". Per-site escape hatch: a `bespoke-keep`
#            comment within 6 lines above the control (justify inline).
#
# Brand-exemption (DS-GOV-02): cyan|pink|orange|rose|indigo|teal|lime are NOT
# treated as off-palette — cyan/pink are the Neon-Tokyo brand primitives and
# orange/rose are per-provider identity hues (Kodik cyan, AniLib orange, Hanime
# pink, Raw rose). Including them would (correctly) fail the clean tree. See
# DESIGN-SYSTEM.md "Lint gate (enforced)".
#
# Exit 1 if ERRORS>0, else 0. `--selftest` proves the fail-path: it injects a
# scratch bg-red-500 file, asserts the gate DETECTS it, removes it (trap-guarded),
# and asserts the clean tree PASSES — leaving the tree exactly as it was.
#
# Manual fail-path fallback (if --selftest is unavailable): add `bg-red-500` to any
# real src/**/*.vue, run `bash scripts/design-system-lint.sh` (must exit 1), then
# `git checkout -- <file>` to revert.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$SCRIPT_DIR/../src"
ALLOWLIST="$SCRIPT_DIR/design-system-allowlist.txt"
SPACING_ALLOWLIST="$SCRIPT_DIR/design-system-spacing-allowlist.txt"
ERRORS=0
WARNINGS=0

RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
NC='\033[0m'

# Per-rule counters (printed in the Summary block).
RULE1_ERRORS=0
RULE2_ERRORS=0
RULE3_ERRORS=0
RULE4_ERRORS=0
RULE5_ERRORS=0
RULE6_ERRORS=0
RULE7_ERRORS=0
RULE8_ERRORS=0

# Off-palette palette set (Phase-4 verbatim). Brand/provider hues
# (cyan|pink|orange|rose|indigo|teal|lime) are deliberately ABSENT.
OFF_PALETTE_RE='(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50|100|200|300|400|500|600|700|800|900|925|950|975)'

# Deprecated-alias var() usages, EXCLUDING the literal-alias survivors
# (--ink-2, --ink-4, --accent-soft, --accent-line, --accent-glow, --pink-soft).
# The font aliases (--f-display/ui/mono/jp → --font-*) and --violet (→ --brand-violet)
# were migrated to canonical tokens (slice #2) and are now fully forbidden — no survivors.
# The regex anchors on the alias name + \b, so the canonical forms (--font-display,
# --brand-violet) are NOT matched.
ALIAS_RE='var\(--(ink|accent|pink|violet|f-display|f-ui|f-mono|f-jp)\b'
ALIAS_SURVIVORS='ink-2|ink-4|accent-soft|accent-line|accent-glow|pink-soft'

# Off-scale Tailwind font-weight utilities. DS allows only font-medium /
# font-semibold; bold/extrabold/black (too heavy) and light/thin (too light)
# are forbidden. \b end-anchor avoids matching e.g. a hypothetical font-boldish.
FONT_RE='\bfont-(bold|extrabold|black|light|thin)\b'

# Arbitrary spacing values on props that HAVE a 4px token scale (padding / margin
# / gap / space). `p-[10px]` etc. dodge the scale; bind to a token (px-2.5) instead.
# Matches numeric-unit brackets only (px/rem/em) — calc()/var() arbitrary values are
# intentional computed spacing and start with a letter, so they don't match. Sizing
# props (w/h/min-*/max-*/size) are DELIBERATELY EXCLUDED: no token scale exists for
# arbitrary pixel dimensions, so flagging `w-[380px]` would be a false positive.
SPACING_RE='\b(p|px|py|pt|pr|pb|pl|m|mx|my|mt|mr|mb|ml|gap|gap-x|gap-y|space-x|space-y)-\[[0-9][0-9.]*(px|rem|em)\]'

# Raw rgba()/rgb()/hsla()/hsl() color literals (comma OR modern space/slash form).
# var()-based forms (e.g. rgba(var(--player-accent-rgb), .3)) are token-based and
# exempt — filtered out in run_rule7 by a grep -v 'var('.
RGBA_RE='(rgba?|hsla?)\([0-9 .,/%]+\)'

# List the in-scope .vue files (exclude *.spec.* and __tests__).
list_vue_files() {
  find "$SRC_DIR" -type f -name '*.vue' \
    ! -name '*.spec.*' \
    ! -path '*/__tests__/*' 2>/dev/null || true
}

# relpath <abs> -> path relative to frontend/web (e.g. src/views/Auth.vue),
# matching the allowlist path convention.
relpath() {
  echo "${1#"$SCRIPT_DIR"/../}"
}

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

# ============================================================================
# RULE 1 — off-palette Tailwind color classes
# ============================================================================
run_rule1() {
  echo "=== RULE 1: off-palette Tailwind color classes ==="
  local hits
  hits=$(grep -rnE "$OFF_PALETTE_RE" "$SRC_DIR" --include='*.vue' --include='*.ts' \
    | grep -v '\.spec\.' | grep -v '__tests__' | grep -v -- '-variants.ts' || true)
  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No off-palette Tailwind color classes"
    return 0
  fi
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file rest
    file="${line%%:*}"
    rest="${line#*:}"
    echo -e "  ${RED}ERROR${NC} $(relpath "$file"):${rest}"
    RULE1_ERRORS=$((RULE1_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
  done <<< "$hits"
}

# ============================================================================
# RULE 2 — hardcoded hex outside the allowlist
# ============================================================================
run_rule2() {
  echo ""
  echo "=== RULE 2: hardcoded hex outside the allowlist ==="

  # Collect the non-comment allowlist lines once.
  local allow_file
  allow_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$allow_file'" RETURN
  if [ -f "$ALLOWLIST" ]; then
    grep -vE '^\s*(#|$)' "$ALLOWLIST" > "$allow_file" 2>/dev/null || true
  fi

  # Every file:line:hex across the in-scope tree. Exclude rgba()/rgb() — those
  # are not hex literals. \b avoids matching e.g. an 8-char id substring.
  local hits
  hits=$(grep -rnoE '#[0-9a-fA-F]{3,8}\b' "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' || true)

  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No hardcoded hex found"
    return 0
  fi

  local any=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    # line = <abs-file>:<lineno>:<hex>
    local file lineno hex rel
    file="${line%%:*}"
    local tail="${line#*:}"
    lineno="${tail%%:*}"
    hex="${tail#*:}"
    rel="$(relpath "$file")"

    # Allowed iff a non-comment allowlist line names BOTH this path AND this hex.
    # The allowlist format is `path:hex:reason`, so match `rel` and `hex` literally.
    if grep -qF "$rel" "$allow_file" 2>/dev/null \
       && awk -F: -v p="$rel" -v h="$hex" \
            'index($0,p) && (($2==h) || index($0,h)) {found=1} END{exit !found}' \
            "$allow_file"; then
      continue
    fi

    echo -e "  ${RED}ERROR${NC} ${rel}:${lineno}: ${hex} (not in allowlist)"
    RULE2_ERRORS=$((RULE2_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
    any=1
  done <<< "$hits"

  if [ "$any" -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} All hardcoded hex are allowlisted (path:hex:reason)"
  fi
}

# ============================================================================
# RULE 3 — deprecated brand-alias var() usages
# ============================================================================
run_rule3() {
  echo ""
  echo "=== RULE 3: deprecated-alias var(--ink|--accent|--pink|--violet|--f-*) ==="
  local hits
  hits=$(grep -rnE "$ALIAS_RE" "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' \
    | grep -vE "$ALIAS_SURVIVORS" || true)
  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No deprecated brand-alias var() usages"
    return 0
  fi
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file rest
    file="${line%%:*}"
    rest="${line#*:}"
    echo -e "  ${RED}ERROR${NC} $(relpath "$file"):${rest}"
    RULE3_ERRORS=$((RULE3_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
  done <<< "$hits"
}

# ============================================================================
# RULE 4 — off-scale font weights (font-bold|extrabold|black|light|thin)
# ============================================================================
run_rule4() {
  echo ""
  echo "=== RULE 4: off-scale font weights (only font-medium/font-semibold allowed) ==="
  local hits
  hits=$(grep -rnE "$FONT_RE" "$SRC_DIR" --include='*.vue' --include='*.ts' \
    | grep -v '\.spec\.' | grep -v '__tests__' || true)
  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No off-scale font weights"
    return 0
  fi
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file rest
    file="${line%%:*}"
    rest="${line#*:}"
    echo -e "  ${RED}ERROR${NC} $(relpath "$file"):${rest}"
    RULE4_ERRORS=$((RULE4_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
  done <<< "$hits"
}

# ============================================================================
# RULE 5 — bare native form controls outside ui primitives + players
# ============================================================================
# Match <select>, <input type="date|checkbox|radio">. Use the Select /
# DatePicker / Checkbox / Switch / RadioGroup primitives instead. Players
# (components/player/) are EXEMPT: reka Select/Popover portal to <body> and
# break inside the fullscreen element, so player pickers stay native. NOTE:
# type="datetime-local" does NOT match `type="date"` (closing quote anchors it),
# so AdminGacha's banner schedule inputs are intentionally allowed.
#
# Per-site escape hatch: a `bespoke-keep` comment within 6 lines above the
# matched control exempts it (e.g. a per-provider-accent checkbox, an sr-only
# segmented-toggle radio, a rich card-style radio the flat RadioGroup can't
# model). The justification lives at the code site — no central allowlist.
run_rule5() {
  echo ""
  echo "=== RULE 5: bare native form controls outside ui+players (use Select/DatePicker/Checkbox/Switch/RadioGroup) ==="
  local hits
  hits=$(grep -rnE '<select(\s|>)|type="date"|type="checkbox"|type="radio"' "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' | grep -v 'components/player/' || true)
  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No bare native form controls (primitives used)"
    return 0
  fi
  local reported=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    local file rest lineno start
    file="${line%%:*}"
    rest="${line#*:}"
    lineno="${rest%%:*}"
    # bespoke-keep escape hatch: a justifying comment within 6 lines above.
    start=$(( lineno > 6 ? lineno - 6 : 1 ))
    if sed -n "${start},${lineno}p" "$file" 2>/dev/null | grep -q 'bespoke-keep'; then
      continue
    fi
    echo -e "  ${RED}ERROR${NC} $(relpath "$file"):${rest}"
    RULE5_ERRORS=$((RULE5_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
    reported=1
  done <<< "$hits"
  if [ "$reported" -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} No bare native form controls (primitives used; exceptions carry bespoke-keep)"
  fi
}

# ============================================================================
# RULE 6 — arbitrary spacing values outside the spacing allowlist
# ============================================================================
# Spacing props (padding/margin/gap/space) have a 4px token scale, so an arbitrary
# `p-[10px]` should be `px-2.5`. On-grid values were migrated to tokens; only off-grid
# sub-pixel tuning (odd px on the dense player menus + Stepper) is allowlisted, per
# (file,class), in design-system-spacing-allowlist.txt. Sizing props are out of scope.
run_rule6() {
  echo ""
  echo "=== RULE 6: arbitrary spacing values (use the 4px token scale) ==="

  # Collect the non-comment spacing-allowlist lines once.
  local allow_file
  allow_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$allow_file'" RETURN
  if [ -f "$SPACING_ALLOWLIST" ]; then
    grep -vE '^\s*(#|$)' "$SPACING_ALLOWLIST" > "$allow_file" 2>/dev/null || true
  fi

  # Every file:line:class across the in-scope .vue tree.
  local hits
  hits=$(grep -rnoE "$SPACING_RE" "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' || true)

  if [ -z "$hits" ]; then
    echo -e "  ${GREEN}OK${NC} No arbitrary spacing values"
    return 0
  fi

  local any=0
  while IFS= read -r line; do
    [ -z "$line" ] && continue
    # line = <abs-file>:<lineno>:<class>
    local file lineno cls rel
    file="${line%%:*}"
    local tail="${line#*:}"
    lineno="${tail%%:*}"
    cls="${tail#*:}"
    rel="$(relpath "$file")"

    # Allowed iff a non-comment allowlist line names BOTH this path AND this class.
    # The allowlist format is `path:class:reason`; match `rel` and `cls` literally.
    if awk -F: -v p="$rel" -v c="$cls" \
         'index($0,p) && index($0,c) {found=1} END{exit !found}' \
         "$allow_file"; then
      continue
    fi

    echo -e "  ${RED}ERROR${NC} ${rel}:${lineno}: ${cls} (use the 4px token scale, or allowlist)"
    RULE6_ERRORS=$((RULE6_ERRORS + 1))
    ERRORS=$((ERRORS + 1))
    any=1
  done <<< "$hits"

  if [ "$any" -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} All arbitrary spacing values are allowlisted (path:class:reason)"
  fi
}

# ============================================================================
# RULE 7 — raw rgba()/rgb()/hsl() color literals in .vue (use a token)
# ============================================================================
run_rule7() {
  echo ""
  echo "=== RULE 7: raw rgba()/hsl() literals (use a token; var() forms exempt) ==="

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
    RULE7_ERRORS=$((RULE7_ERRORS + 1)); ERRORS=$((ERRORS + 1)); any=1
  done <<< "$hits"

  if [ "$any" -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} All rgba()/hsl() literals are allowlisted"
  fi
}

# ============================================================================
# RULE 8 — static color literal inside an inline style="…" / :style="'…'" attr
# ============================================================================
# Flags ONLY hardcoded color (#hex | rgb( | hsl() inside an inline style attribute.
# Dynamic object/array bindings (:style="{ width: pct }") and px/%/transform/layout
# values are NOT the DS concern and are not flagged. var() forms are exempt.
run_rule8() {
  echo ""
  echo "=== RULE 8: static color in inline style=/:style attr (use a class/token) ==="

  local allow_file
  allow_file=$(mktemp)
  # shellcheck disable=SC2064
  trap "rm -f '$allow_file'" RETURN
  [ -f "$ALLOWLIST" ] && cp "$ALLOWLIST" "$allow_file"

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
    if grep -qF "$rel" "$allow_file" 2>/dev/null && allow_hit "$rel" "$norm" "$allow_file"; then continue; fi
    echo -e "  ${RED}ERROR${NC} ${rel}:${lineno}: inline style color (use a class/token)"
    RULE8_ERRORS=$((RULE8_ERRORS + 1)); ERRORS=$((ERRORS + 1)); any=1
  done <<< "$hits"

  if [ "$any" -eq 0 ]; then
    echo -e "  ${GREEN}OK${NC} All inline style colors are allowlisted"
  fi
}

# ============================================================================
# --selftest — provable fail-path (SC#3)
# ============================================================================
run_selftest() {
  echo "=== DESIGN-SYSTEM LINT SELFTEST ==="
  local scratch="$SRC_DIR/__ds_lint_selftest__.vue"
  # Guarantee cleanup even on failure / interrupt — never leave the tree dirty.
  # shellcheck disable=SC2064
  trap "rm -f '$scratch'" EXIT

  # 1) Inject a deliberate off-palette violation (bg-red-500) AND an arbitrary
  #    spacing violation (p-[7px], off-grid + not allowlisted for this scratch file).
  cat > "$scratch" <<'EOF'
<template>
  <!-- design-system-lint selftest marker; auto-removed -->
  <div class="bg-red-500 p-[7px]">selftest</div>
</template>
EOF

  # 2) Assert the gate DETECTS the off-palette class (exit 1).
  local scratch_hits
  scratch_hits=$(grep -nE "$OFF_PALETTE_RE" "$scratch" 2>/dev/null || true)
  if [ -z "$scratch_hits" ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — gate did NOT detect the injected bg-red-500"
    rm -f "$scratch"; trap - EXIT
    exit 1
  fi
  echo -e "  ${GREEN}DETECTED${NC} injected bg-red-500 (gate would exit 1)"

  # 2b) Assert RULE 6 DETECTS the injected arbitrary spacing (counts > 0).
  ( ERRORS=0 RULE6_ERRORS=0; run_rule6 >/dev/null 2>&1
    [ "$RULE6_ERRORS" -gt 0 ] ) || {
    echo -e "  ${RED}SELFTEST FAIL${NC} — RULE 6 did NOT detect the injected p-[7px]"
    rm -f "$scratch"; trap - EXIT
    exit 1
  }
  echo -e "  ${GREEN}DETECTED${NC} injected p-[7px] arbitrary spacing (RULE 6 would exit 1)"

  # 1c) Injected rgba literal + inline-style color must be detected by Rule 7 / Rule 8.
  cat > "$scratch" <<'EOF'
<template>
  <div class="text-[rgba(1,2,3,0.5)]" style="background: rgba(4, 5, 6, 0.5); color:#abc">x</div>
</template>
EOF
  local r7=0 r8=0
  grep -qE "$RGBA_RE" "$scratch" && r7=1
  grep -qE ':?style=("|'"'"')[^"'"'"']*(#[0-9a-fA-F]{3,8}|rgba?\()' "$scratch" && r8=1
  if [ "$r7" -ne 1 ] || [ "$r8" -ne 1 ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — Rule 7/8 did NOT detect injected color (r7=$r7 r8=$r8)"
    rm -f "$scratch"; trap - EXIT; exit 1
  fi
  echo -e "  ${GREEN}DETECTED${NC} injected rgba literal (R7) + inline-style color (R8)"

  # 1d) Rule 5 — a raw checkbox must be detected; a bespoke-keep'd radio exempted.
  cat > "$scratch" <<'EOF'
<template>
  <label><input type="checkbox" :value="x" /> flag-me</label>
  <!-- bespoke-keep: selftest segmented toggle -->
  <label><input type="radio" :value="y" /> keep-me</label>
</template>
EOF
  local r5flag=0 r5keep=1 r5matches
  r5matches=$(grep -nE 'type="(checkbox|radio)"' "$scratch")
  while IFS= read -r r5ln; do
    [ -z "$r5ln" ] && continue
    local r5no r5st
    r5no="${r5ln%%:*}"
    r5st=$(( r5no > 6 ? r5no - 6 : 1 ))
    if sed -n "${r5st},${r5no}p" "$scratch" | grep -q 'bespoke-keep'; then
      grep -qE 'type="radio"' <<< "$r5ln" || r5keep=0   # only the radio should be exempt
    else
      grep -qE 'type="checkbox"' <<< "$r5ln" && r5flag=1
    fi
  done <<< "$r5matches"
  if [ "$r5flag" -ne 1 ] || [ "$r5keep" -ne 1 ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — Rule 5 checkbox detect / bespoke-keep wrong (flag=$r5flag keep=$r5keep)"
    rm -f "$scratch"; trap - EXIT; exit 1
  fi
  echo -e "  ${GREEN}DETECTED${NC} raw checkbox (R5) + honored bespoke-keep radio exemption"

  # 3) Remove the scratch file and assert the clean tree PASSES.
  rm -f "$scratch"
  trap - EXIT

  ERRORS=0; RULE1_ERRORS=0; RULE2_ERRORS=0; RULE3_ERRORS=0; RULE4_ERRORS=0; RULE5_ERRORS=0; RULE6_ERRORS=0; RULE7_ERRORS=0; RULE8_ERRORS=0
  run_rule1 >/dev/null
  run_rule2 >/dev/null
  run_rule3 >/dev/null
  run_rule4 >/dev/null
  run_rule5 >/dev/null
  run_rule6 >/dev/null
  run_rule7 >/dev/null
  run_rule8 >/dev/null
  if [ "$ERRORS" -gt 0 ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — clean tree did NOT pass ($ERRORS errors)"
    exit 1
  fi
  echo -e "  ${GREEN}CLEAN TREE PASSES${NC} after scratch removal"
  echo -e "${GREEN}SELFTEST PASS${NC}: gate detects bg-red-500 then passes on the clean tree."
  exit 0
}

# ============================================================================
# Entry point
# ============================================================================
if [ "${1:-}" = "--selftest" ]; then
  run_selftest
fi

run_rule1
run_rule2
run_rule3
run_rule4
run_rule5
run_rule6
run_rule7
run_rule8

echo ""
echo "=== Summary ==="
echo -e "  Off-palette classes (RULE 1): ${RULE1_ERRORS}"
echo -e "  Non-allowlisted hex (RULE 2): ${RULE2_ERRORS}"
echo -e "  Deprecated aliases  (RULE 3): ${RULE3_ERRORS}"
echo -e "  Off-scale font wts  (RULE 4): ${RULE4_ERRORS}"
echo -e "  Bare form controls  (RULE 5): ${RULE5_ERRORS}"
echo -e "  Arbitrary spacing   (RULE 6): ${RULE6_ERRORS}"
echo -e "  Raw rgba/hsl lits   (RULE 7): ${RULE7_ERRORS}"
echo -e "  Inline style colors (RULE 8): ${RULE8_ERRORS}"

if [ "$ERRORS" -gt 0 ]; then
  echo ""
  echo -e "${RED}FAIL${NC}: $ERRORS design-system color/token violation(s)."
  echo -e "  Fix: migrate to a canonical token, or add a justified path:hex:reason"
  echo -e "  line to scripts/design-system-allowlist.txt (see DESIGN-SYSTEM.md)."
  exit 1
fi

echo ""
echo -e "${GREEN}PASS${NC}: No design-system color/token violations."
exit 0
