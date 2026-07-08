#!/usr/bin/env bash
# ae-land.sh — commit (with the standard co-authors), rebase onto origin/main,
# and push to main. Encodes the exact worktree→main dance so the caller doesn't
# retype co-authors or re-reason the fetch/rebase/push each time.
#
# Usage: printf '<subject>\n\n<body>' | bin/ae-land.sh [file ...]
#   Commit message is read from STDIN (co-authors are appended automatically).
#   Stages the given files, or `git add -A` if none are passed (prefer explicit
#   paths — the worktree index may hold unrelated hunks).
#
# On rebase conflict it STOPS and reports (never auto-resolves, never force-
# pushes): resolve the files, then
#   git add <files> && GIT_EDITOR=true git rebase --continue && git push origin HEAD:main
set -uo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

MSG="$(cat)"
[ -n "${MSG//[$'\n\t ']/}" ] || { echo "FAIL: empty commit message on stdin"; exit 1; }

# --- stage -------------------------------------------------------------------
if [ "$#" -gt 0 ]; then git add -- "$@"; else git add -A; fi
if git diff --cached --quiet; then echo "FAIL: nothing staged"; exit 1; fi
echo "== staged =="; git diff --cached --name-only | sed 's/^/  /'

# --- commit (co-authors appended) --------------------------------------------
git commit -F - <<EOF
$MSG

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
EOF

# --- rebase onto latest origin/main ------------------------------------------
echo "== rebase onto origin/main =="
git fetch origin -q || { echo "FAIL: fetch"; exit 1; }
if ! GIT_EDITOR=true git rebase origin/main; then
  echo ""
  echo "STOP: rebase conflict — resolve then continue. Conflicted files:"
  git diff --name-only --diff-filter=U | sed 's/^/  /'
  echo "  git add <files> && GIT_EDITOR=true git rebase --continue && git push origin HEAD:main"
  exit 2
fi

# --- push --------------------------------------------------------------------
echo "== push HEAD:main =="
git push origin HEAD:main
echo "LANDED: $(git log --oneline -1)"
