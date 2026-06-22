#!/bin/bash
# Regression test for with-deploy-lock.sh — proves concurrent invocations
# serialize (the OOM guard from 2026-06-22: parallel builds must not pile up).
#
# Run: bash deploy/scripts/with-deploy-lock.test.sh
set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
HELPER="$SCRIPT_DIR/with-deploy-lock.sh"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

export ANIMEENIGMA_DEPLOY_LOCK="$TMP/test.lock"
LOG="$TMP/critical.log"

fail() { echo "FAIL: $1" >&2; exit 1; }

[ -x "$HELPER" ] || fail "helper not found / not executable at $HELPER"

# Each worker enters the critical section, holds it briefly, then exits.
# If the lock works, the two workers cannot overlap.
worker() {
    "$HELPER" bash -c "echo ENTER-$1 >> '$LOG'; sleep 0.4; echo EXIT-$1 >> '$LOG'"
}

# --- Test 1: two concurrent workers must not interleave -----------------------
worker A &
P1=$!
sleep 0.05          # ensure A grabs the lock first
worker B &
P2=$!
wait "$P1" "$P2"

mapfile -t LINES < "$LOG"
[ "${#LINES[@]}" -eq 4 ] || fail "expected 4 log lines, got ${#LINES[@]}: ${LINES[*]}"

# Critical-section integrity: every ENTER must be immediately followed by its
# own EXIT. Two consecutive ENTERs => overlap => lock failed.
prev=""
for line in "${LINES[@]}"; do
    case "$line" in
        ENTER-*)
            [ -z "$prev" ] || fail "overlap detected — '$line' before previous section closed (log: ${LINES[*]})"
            prev="${line#ENTER-}"
            ;;
        EXIT-*)
            [ "$prev" = "${line#EXIT-}" ] || fail "mismatched exit '$line' (expected EXIT-$prev; log: ${LINES[*]})"
            prev=""
            ;;
        *) fail "unexpected log line: '$line'" ;;
    esac
done
[ -z "$prev" ] || fail "section left open: $prev"
echo "PASS: concurrent invocations serialized (${LINES[*]})"

# --- Test 2: nested call (ANIMEENIGMA_DEPLOY_LOCK_HELD set) runs without re-locking
OUT="$(ANIMEENIGMA_DEPLOY_LOCK_HELD=1 "$HELPER" echo nested-ok)"
[ "$OUT" = "nested-ok" ] || fail "nested invocation did not pass through (got '$OUT')"
echo "PASS: nested invocation passes through without deadlock"

# --- Test 3: command exit status propagates ----------------------------------
"$HELPER" bash -c 'exit 7'; rc=$?
[ "$rc" -eq 7 ] || fail "exit status not propagated (got $rc, expected 7)"
echo "PASS: command exit status propagates"

echo "ALL TESTS PASSED"
