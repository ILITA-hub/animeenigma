#!/usr/bin/env bash
# ds-lint-postedit — PostToolUse(Write|Edit) early-warning for the Design-System gate.
#
# WHAT: when an FE source file (frontend/web/src/**/*.{vue,ts}) is edited, run
# design-system-lint.sh and, if it now fails, surface the violation back to the
# agent IMMEDIATELY (at edit time) instead of only at `make lint` / `make redeploy-web`.
#
# WHY whole-tree scan is safe: design-system-lint.sh scans the entire src tree (it has
# no single-file mode) and exits non-zero ONLY on non-allowlisted ERRORS. `main` is
# build-gated clean (the same script is a prerequisite of `make lint-frontend` AND the
# deploy gate), so against a clean baseline any error this hook reports was introduced
# in THIS session. Pre-existing / allowlisted lines never trip it.
#
# NON-BLOCKING by design: emits systemMessage (user) + additionalContext (model) and
# exits 0. The hard stop stays the build/deploy gate; this is just faster feedback.
# Silently no-ops (exit 0) when: not an FE src file, jq is missing, repo root can't be
# resolved, or the lint script is absent (e.g. a checkout without the frontend toolchain).
#
# Companion to /frontend-verify (the on-demand FE/DS pre-flight) and git-guard.py.
set -uo pipefail

# jq parses the hook stdin payload; degrade to a silent no-op if it's unavailable.
command -v jq >/dev/null 2>&1 || exit 0

INPUT="$(cat)"
FILE="$(printf '%s' "$INPUT" | jq -r '.tool_input.file_path // empty' 2>/dev/null)"
[ -n "$FILE" ] || exit 0

# Scope strictly to frontend/web/src/**/*.{vue,ts} (matches the lint gate's surface).
case "$FILE" in
  */frontend/web/src/*.vue|*/frontend/web/src/*.ts|frontend/web/src/*.vue|frontend/web/src/*.ts) ;;
  *) exit 0 ;;
esac

# Resolve repo root: prefer the harness-provided var, else ask git from the file's dir.
ROOT="${CLAUDE_PROJECT_DIR:-}"
[ -n "$ROOT" ] || ROOT="$(git -C "$(dirname "$FILE")" rev-parse --show-toplevel 2>/dev/null || true)"
[ -n "$ROOT" ] || exit 0

SCRIPT="$ROOT/frontend/web/scripts/design-system-lint.sh"
[ -f "$SCRIPT" ] || exit 0

# Run the gate from frontend/web (mirrors the Makefile invocation). Capture rc explicitly
# (no `set -e`) so a lint failure is handled, not propagated as a hook crash.
OUT="$(cd "$ROOT/frontend/web" && bash scripts/design-system-lint.sh 2>&1)"
RC=$?
[ "$RC" -eq 0 ] && exit 0

# Keep the model-facing context tight: just the rule/error/summary lines.
SUMMARY="$(printf '%s\n' "$OUT" | grep -iE 'ERROR|RULE|FAIL|✗|Summary' | head -30)"
[ -n "$SUMMARY" ] || SUMMARY="$(printf '%s\n' "$OUT" | tail -30)"

jq -nc \
  --arg ctx "design-system-lint.sh FAILED after this edit — the DS gate is build-enforced (it blocks make lint-frontend AND make redeploy-web), so fix it before building:

${SUMMARY}

Fix: migrate to a semantic token (text-destructive / bg-warning / --white-a20 / etc.), or — ONLY if no token reproduces the value — add a justified allowlist line (design-system-allowlist.txt or design-system-spacing-allowlist.txt). Never disable the gate. Full report: \`bash frontend/web/scripts/design-system-lint.sh\`. Rules + escape-hatch: frontend/web/src/styles/DESIGN-SYSTEM.md and /frontend-verify." \
  '{hookSpecificOutput:{hookEventName:"PostToolUse",additionalContext:$ctx},systemMessage:"⚠ design-system-lint found a new violation in frontend/web/src — see context"}'
exit 0
