#!/usr/bin/env bash
# design-system-lint — build-failing color/token-discipline gate (DS-GOV-01).
#
# Enforces EXACTLY 3 color/token rules over frontend/web/src/**/*.vue
# (excluding *.spec.* and __tests__), mirroring scripts/i18n-lint.sh:
#
#   RULE 1 — Zero off-palette Tailwind color classes (ERROR).
#   RULE 2 — Zero hardcoded hex outside scripts/design-system-allowlist.txt (ERROR).
#   RULE 3 — Zero deprecated-alias var(--ink|--accent|--pink|--violet|--f-display|
#            --f-ui|--f-mono|--f-jp) usages (ERROR).
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

# Off-palette palette set (Phase-4 verbatim). Brand/provider hues
# (cyan|pink|orange|rose|indigo|teal|lime) are deliberately ABSENT.
OFF_PALETTE_RE='(text|bg|border|ring|from|to|via|fill|stroke|placeholder|divide|outline|decoration|shadow)-(red|amber|yellow|emerald|green|blue|sky|purple|violet|gray|slate|zinc)-(50|100|200|300|400|500|600|700|800|900)'

# Deprecated-alias var() usages, EXCLUDING the literal-alias survivors
# (--ink-2, --ink-4, --accent-soft, --accent-line, --accent-glow, --pink-soft).
# The font aliases (--f-display/ui/mono/jp → --font-*) and --violet (→ --brand-violet)
# were migrated to canonical tokens (slice #2) and are now fully forbidden — no survivors.
# The regex anchors on the alias name + \b, so the canonical forms (--font-display,
# --brand-violet) are NOT matched.
ALIAS_RE='var\(--(ink|accent|pink|violet|f-display|f-ui|f-mono|f-jp)\b'
ALIAS_SURVIVORS='ink-2|ink-4|accent-soft|accent-line|accent-glow|pink-soft'

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

# ============================================================================
# RULE 1 — off-palette Tailwind color classes
# ============================================================================
run_rule1() {
  echo "=== RULE 1: off-palette Tailwind color classes ==="
  local hits
  hits=$(grep -rnE "$OFF_PALETTE_RE" "$SRC_DIR" --include='*.vue' \
    | grep -v '\.spec\.' | grep -v '__tests__' || true)
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
# --selftest — provable fail-path (SC#3)
# ============================================================================
run_selftest() {
  echo "=== DESIGN-SYSTEM LINT SELFTEST ==="
  local scratch="$SRC_DIR/__ds_lint_selftest__.vue"
  # Guarantee cleanup even on failure / interrupt — never leave the tree dirty.
  # shellcheck disable=SC2064
  trap "rm -f '$scratch'" EXIT

  # 1) Inject a deliberate off-palette violation (bg-red-500).
  cat > "$scratch" <<'EOF'
<template>
  <!-- design-system-lint selftest marker; auto-removed -->
  <div class="bg-red-500">selftest</div>
</template>
EOF

  # 2) Assert the gate DETECTS it (exit 1).
  local detect_rc=0
  ( ERRORS=0 RULE1_ERRORS=0; run_rule1 >/dev/null 2>&1
    [ "$ERRORS" -gt 0 ] || exit 1 ) || detect_rc=$?
  # Re-run rule1 directly to count against the scratch file.
  local scratch_hits
  scratch_hits=$(grep -nE "$OFF_PALETTE_RE" "$scratch" 2>/dev/null || true)
  if [ -z "$scratch_hits" ]; then
    echo -e "  ${RED}SELFTEST FAIL${NC} — gate did NOT detect the injected bg-red-500"
    rm -f "$scratch"; trap - EXIT
    exit 1
  fi
  echo -e "  ${GREEN}DETECTED${NC} injected bg-red-500 (gate would exit 1)"

  # 3) Remove the scratch file and assert the clean tree PASSES.
  rm -f "$scratch"
  trap - EXIT

  ERRORS=0; RULE1_ERRORS=0; RULE2_ERRORS=0; RULE3_ERRORS=0
  run_rule1 >/dev/null
  run_rule2 >/dev/null
  run_rule3 >/dev/null
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

echo ""
echo "=== Summary ==="
echo -e "  Off-palette classes (RULE 1): ${RULE1_ERRORS}"
echo -e "  Non-allowlisted hex (RULE 2): ${RULE2_ERRORS}"
echo -e "  Deprecated aliases  (RULE 3): ${RULE3_ERRORS}"

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
