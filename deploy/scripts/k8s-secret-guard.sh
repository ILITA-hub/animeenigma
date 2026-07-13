#!/bin/bash
# k8s-secret-guard.sh — fail if placeholder credentials are committed under deploy/kustomize/.
#
# WHY: The 2026-06-21 security audit found the k8s manifests shipping literal
# placeholder secrets ("change-this-in-production", "minioadmin", …). Anyone
# applying the tree as-is would run production with known credentials. This
# guard makes that a CI failure instead of a silent deploy: any git-TRACKED
# file under deploy/kustomize/ containing a known placeholder string blocks
# the build. Real secret material must come from git-ignored overlay env files
# (secretGenerator), never from committed manifests.
#
# Exemptions:
#   - files named *.example are skipped entirely (templates legitimately
#     document the placeholders they expect you to replace)
#   - a hit on a line carrying the marker `# guard-ok: <reason>` is skipped
#     (for the rare committed line that matches a pattern but is not a secret)
#
# Usage:   deploy/scripts/k8s-secret-guard.sh
# Exit:    0 = clean ("k8s-secret-guard: OK"), 1 = placeholders found
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

# Placeholder signatures (POSIX ERE, matched by `git grep -E`):
#   1-3: literal placeholder credential strings
#   4:   an htpasswd-style line (user:$scheme$…) containing EXAMPLE
PATTERNS=(
    'change-this-in-production'
    'CHANGE_ME'
    'minioadmin'
    '[A-Za-z0-9_.-]+:\$[A-Za-z0-9]+\$[^[:space:]]*EXAMPLE'
)

GREP_ARGS=()
for p in "${PATTERNS[@]}"; do
    GREP_ARGS+=(-e "$p")
done

# git grep exits 1 on "no match" — that is our success case, don't let set -e kill us.
hits="$(git grep -nIE "${GREP_ARGS[@]}" -- 'deploy/kustomize' || true)"

offenders=()
while IFS= read -r line; do
    [ -z "$line" ] && continue
    file="${line%%:*}"
    case "$file" in
        *.example) continue ;;                  # template files: placeholders are the point
    esac
    case "$line" in
        *'# guard-ok:'*) continue ;;            # explicitly exempted line
    esac
    offenders+=("$line")
done <<<"$hits"

if [ "${#offenders[@]}" -gt 0 ]; then
    echo "k8s-secret-guard: FAIL — placeholder credentials in git-tracked files under deploy/kustomize/:" >&2
    printf '  %s\n' "${offenders[@]}" >&2
    echo "" >&2
    echo "Fix: move real values into a git-ignored overlay secretGenerator env file;" >&2
    echo "commit only *.example templates. For a genuine false positive, append" >&2
    echo "'# guard-ok: <reason>' to the line." >&2
    exit 1
fi

echo "k8s-secret-guard: OK"
