#!/usr/bin/env bash
# audit-hls-allowlist.sh — quarterly review tool for the HLS proxy allow-list.
#
# Prints every entry in libs/videoutils/proxy.go's HLSProxyAllowedDomainsWithProvenance
# slice with its owner / date-added / reason. The actual reading is done by a
# small Go program (libs/videoutils/cmd/audit-allowlist) that imports the
# package and iterates the struct slice directly — so the audit is robust
# against reformatting / reordering / wildcard syntax changes that would
# break a naive textual `git diff` audit.
#
# Usage:
#   scripts/audit-hls-allowlist.sh                         # human-readable table
#   scripts/audit-hls-allowlist.sh -format=tsv             # TAB-separated, scriptable
#   scripts/audit-hls-allowlist.sh -format=csv             # CSV
#   scripts/audit-hls-allowlist.sh -legacy-only            # only @legacy-owned entries (backfill candidates)
#
# See docs/security/hls-proxy-allowlist.md for the review process.
set -euo pipefail

# Resolve repository root from script location so the script works regardless
# of the caller's CWD.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "$SCRIPT_DIR/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
  echo "error: 'go' is not in PATH — install Go to run the audit script" >&2
  exit 127
fi

cd "$REPO_ROOT"
exec go run ./libs/videoutils/cmd/audit-allowlist "$@"
