#!/usr/bin/env bash
# i18n lint — checks for missing translations, hardcoded text, and unused keys.
# Exit code 1 on errors (missing keys), 0 on warnings-only (hardcoded text, unused keys).
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SRC_DIR="$SCRIPT_DIR/../src"
LOCALES_DIR="$SRC_DIR/locales"
ERRORS=0
WARNINGS=0

RED='\033[0;31m'
YELLOW='\033[0;33m'
GREEN='\033[0;32m'
NC='\033[0m'

# ============================================================================
# 1. Missing translation keys across locale files
# ============================================================================
echo "=== Checking for missing translation keys ==="

# Extract all keys (flattened dot-notation) from a JSON file
extract_keys() {
  python3 -c "
import json, sys
def flatten(obj, prefix=''):
    for k, v in obj.items():
        key = f'{prefix}.{k}' if prefix else k
        if isinstance(v, dict):
            yield from flatten(v, key)
        else:
            yield key
with open(sys.argv[1]) as f:
    for k in sorted(flatten(json.load(f))):
        print(k)
" "$1"
}

RU_KEYS=$(extract_keys "$LOCALES_DIR/ru.json")
EN_KEYS=$(extract_keys "$LOCALES_DIR/en.json")
JA_KEYS=$(extract_keys "$LOCALES_DIR/ja.json")

# Check each locale against the others
check_missing() {
  local source_name="$1"
  local source_keys="$2"
  local target_name="$3"
  local target_keys="$4"

  while IFS= read -r key; do
    if ! echo "$target_keys" | grep -qxF "$key"; then
      echo -e "  ${RED}MISSING${NC} key '$key' in $target_name (exists in $source_name)"
      ERRORS=$((ERRORS + 1))
    fi
  done <<< "$source_keys"
}

check_missing "en.json" "$EN_KEYS" "ru.json" "$RU_KEYS"
check_missing "en.json" "$EN_KEYS" "ja.json" "$JA_KEYS"
check_missing "ru.json" "$RU_KEYS" "en.json" "$EN_KEYS"
check_missing "ja.json" "$JA_KEYS" "en.json" "$EN_KEYS"

if [ "$ERRORS" -eq 0 ]; then
  echo -e "  ${GREEN}OK${NC} All locale files have matching keys"
fi

MISSING_ERRORS=$ERRORS

# ============================================================================
# 2. Hardcoded Cyrillic text in Vue templates (warning only)
# ============================================================================
echo ""
echo "=== Checking for hardcoded Cyrillic text in Vue/TS files ==="

# Use find to recursively discover all Vue and TS files
HARDCODED_FILES=$(find "$SRC_DIR" -type f \( -name '*.vue' -o -name '*.ts' \) -exec grep -lP '[а-яА-ЯёЁ]' {} + 2>/dev/null || true)

HARDCODED_COUNT=0
if [ -n "$HARDCODED_FILES" ]; then
  while IFS= read -r file; do
    # Skip locale JSON files
    [[ "$file" == *"/locales/"* ]] && continue

    MATCHES=$(grep -nP '[а-яА-ЯёЁ]' "$file" 2>/dev/null \
      | grep -v '^\s*//' \
      | grep -v '<!--' \
      | grep -v 'console\.' \
      | grep -v '^\s*\*' \
      | grep -v 'import ' \
      | grep -v "\.includes(" \
      | grep -v "name: 'Русский'" \
      || true)
    if [ -n "$MATCHES" ]; then
      REL_PATH="${file#$SRC_DIR/}"
      while IFS= read -r line; do
        LINENUM=$(echo "$line" | cut -d: -f1)
        CONTENT=$(echo "$line" | cut -d: -f2-)
        # Skip lines that use $t() or t() — likely already i18n'd
        if echo "$CONTENT" | grep -qP "\\\$t\(|[^a-zA-Z]t\(" 2>/dev/null; then
          continue
        fi
        echo -e "  ${YELLOW}HARDCODED${NC} $REL_PATH:$LINENUM: $(echo "$CONTENT" | sed 's/^[[:space:]]*//' | head -c 100)"
        HARDCODED_COUNT=$((HARDCODED_COUNT + 1))
      done <<< "$MATCHES"
    fi
  done <<< "$HARDCODED_FILES"
fi

if [ "$HARDCODED_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}OK${NC} No hardcoded Cyrillic text found"
else
  echo -e "  Found ${YELLOW}$HARDCODED_COUNT${NC} lines with hardcoded Cyrillic text (warning)"
  WARNINGS=$((WARNINGS + HARDCODED_COUNT))
fi

# ============================================================================
# 3. Unused i18n keys (warning only, does not fail build)
# ============================================================================
echo ""
echo "=== Checking for unused i18n keys (warnings) ==="

# Collect all source into a temp file (avoids SIGPIPE with pipefail)
ALL_SRC_FILE=$(mktemp)
trap 'rm -f "$ALL_SRC_FILE"' EXIT
find "$SRC_DIR" -type f \( -name '*.vue' -o -name '*.ts' \) ! -path '*/locales/*' -exec cat {} + > "$ALL_SRC_FILE" 2>/dev/null || true

UNUSED_COUNT=0
while IFS= read -r key; do
  # Skip keys that are likely used dynamically (status.*, days.*, gameType.*)
  if echo "$key" | grep -qP '\.(status|days|gameType)\.' 2>/dev/null; then
    continue
  fi

  PARENT="${key%.*}"

  # Search for the full key in single or double quotes
  if ! grep -qF "'$key'" "$ALL_SRC_FILE" 2>/dev/null; then
    if ! grep -qF "\"$key\"" "$ALL_SRC_FILE" 2>/dev/null; then
      # Check for dynamic key construction like `prefix.${var}`
      if ! grep -qP "\`${PARENT//./\\.}\.\\\$" "$ALL_SRC_FILE" 2>/dev/null; then
        echo -e "  ${YELLOW}UNUSED${NC} $key"
        UNUSED_COUNT=$((UNUSED_COUNT + 1))
        WARNINGS=$((WARNINGS + 1))
      fi
    fi
  fi
done <<< "$EN_KEYS"

if [ "$UNUSED_COUNT" -eq 0 ]; then
  echo -e "  ${GREEN}OK${NC} All keys are referenced in source"
else
  echo -e "  Found ${YELLOW}$UNUSED_COUNT${NC} potentially unused keys (warnings only)"
fi

# ============================================================================
# Summary
# ============================================================================
echo ""
echo "=== Summary ==="
echo -e "  Missing keys:    ${MISSING_ERRORS:-0}"
echo -e "  Hardcoded text:  $HARDCODED_COUNT (warning)"
echo -e "  Unused keys:     $UNUSED_COUNT (warning)"

if [ "$MISSING_ERRORS" -gt 0 ]; then
  echo ""
  echo -e "${RED}FAIL${NC}: Missing translation keys found. Add them to all locale files."
  exit 1
fi

echo ""
echo -e "${GREEN}PASS${NC}: No blocking i18n issues."
exit 0
