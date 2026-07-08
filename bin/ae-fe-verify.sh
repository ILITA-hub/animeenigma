#!/usr/bin/env bash
# ae-fe-verify.sh — terse frontend gate runner (DS-lint + eslint + build + vitest).
#
# Replaces the manual /frontend-verify + after-update lint/build dance: runs all
# gates and prints ONE status line each. On failure, dumps only the failing
# gate's output and exits 1 — so the caller reads ~5 lines on success instead of
# ~120 lines of build/lint/test logs.
#
# Usage: bin/ae-fe-verify.sh [touched-file ...]
#   With no args, derives touched files from `git diff --name-only HEAD` under
#   frontend/web. Pass explicit files to scope eslint + vitest precisely.
#
# NOTE: i18n parity (en/ru/ja) is NOT run here — run it only when locale JSON
# changed: `bash frontend/web/scripts/i18n-lint.sh` + the locale vitest specs.
set -uo pipefail

ROOT="$(git rev-parse --show-toplevel)"
WEB="$ROOT/frontend/web"
cd "$WEB" || { echo "FAIL: no $WEB"; exit 1; }

# --- collect touched files (repo-relative) -----------------------------------
declare -a FILES=()
if [ "$#" -gt 0 ]; then
  FILES=("$@")
else
  while IFS= read -r line; do [ -n "$line" ] && FILES+=("$line"); done \
    < <(git -C "$ROOT" diff --name-only HEAD -- frontend/web/src frontend/web/scripts 2>/dev/null)
fi

# normalize to paths relative to frontend/web
declare -a SRCREL=()
for f in "${FILES[@]:-}"; do
  [ -z "$f" ] && continue
  case "$f" in
    frontend/web/*) SRCREL+=("${f#frontend/web/}") ;;
    src/*|scripts/*) SRCREL+=("$f") ;;
  esac
done

pass()    { printf '  \033[0;32m%-9s PASS\033[0m %s\n' "$1" "${2:-}"; }
failout() { printf '  \033[0;31m%-9s FAIL\033[0m\n' "$1"; echo "----- $1 output -----"; cat "$2"; exit 1; }

TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT

# 1) DS-lint (build-enforced; always run — it scans the whole tree)
if bash scripts/design-system-lint.sh >"$TMP/ds" 2>&1; then pass "DS-lint"; else failout "DS-lint" "$TMP/ds"; fi

# 2) eslint on touched vue/ts
declare -a LINTABLE=()
for f in "${SRCREL[@]:-}"; do case "$f" in *.vue|*.ts) [ -f "$f" ] && LINTABLE+=("$f");; esac; done
if [ "${#LINTABLE[@]}" -gt 0 ]; then
  if bunx eslint "${LINTABLE[@]}" >"$TMP/lint" 2>&1; then pass "eslint" "(${#LINTABLE[@]} files)"; else failout "eslint" "$TMP/lint"; fi
else
  pass "eslint" "(no touched vue/ts)"
fi

# 3) build (type truth — vue-tsc + vite; catches TS2614 + stale-cache false-pass)
if bun run build >"$TMP/build" 2>&1; then pass "build"; else failout "build" "$TMP/build"; fi

# 4) vitest on specs derived from touched files
declare -a SPECS=()
for f in "${SRCREL[@]:-}"; do
  case "$f" in
    *.spec.ts) [ -f "$f" ] && SPECS+=("$f") ;;
    *.vue|*.ts)
      base="${f%.*}"; dir="$(dirname "$f")"; name="$(basename "$base")"
      [ -f "$base.spec.ts" ] && SPECS+=("$base.spec.ts")
      [ -f "$dir/__tests__/$name.spec.ts" ] && SPECS+=("$dir/__tests__/$name.spec.ts")
      ;;
  esac
done
if [ "${#SPECS[@]}" -gt 0 ]; then
  declare -a USPECS=()
  while IFS= read -r s; do [ -n "$s" ] && USPECS+=("$s"); done < <(printf '%s\n' "${SPECS[@]}" | sort -u)
  if bunx vitest run "${USPECS[@]}" --reporter=dot >"$TMP/test" 2>&1; then
    pass "tests" "($(grep -oE '[0-9]+ passed' "$TMP/test" | tail -1))"
  else
    failout "tests" "$TMP/test"
  fi
else
  pass "tests" "(no co-located specs found — verify manually if needed)"
fi

echo "  ALL FRONTEND GATES PASS"
