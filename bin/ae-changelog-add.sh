#!/usr/bin/env bash
# ae-changelog-add.sh — prepend user-facing changelog entries to TODAY's group,
# then regenerate the served file. Avoids hand-editing JSON (no "modified since
# read" retries) and keeps entries appended to the top group so rebase conflicts
# stay trivial.
#
# Usage: bin/ae-changelog-add.sh <type> "<message>" [<type> "<message>" ...]
#   <type> ∈ fix | feature | perf. Message is Russian Trump-mode prose (see
#   .claude/commands/animeenigma-after-update.md step 5 for the style spec).
#
# Edits frontend/web/changelog.full.json (source of truth) and regenerates
# frontend/web/public/changelog.json via changelog-trim.mjs. Both must be
# committed. A web redeploy is required for the served file to go live.
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
WEB="$ROOT/frontend/web"

if [ "$#" -lt 2 ] || [ $(( $# % 2 )) -ne 0 ]; then
  echo "usage: $(basename "$0") <fix|feature|perf> \"<message>\" [<type> \"<message>\" ...]" >&2
  exit 1
fi

cd "$WEB"
node -e '
const fs = require("fs");
const file = "changelog.full.json";
const args = process.argv.slice(1);
const valid = new Set(["fix", "feature", "perf"]);
const today = new Date().toISOString().slice(0, 10);
const arr = JSON.parse(fs.readFileSync(file, "utf8"));
let grp = (arr[0] && arr[0].date === today) ? arr[0] : null;
if (!grp) { grp = { date: today, entries: [] }; arr.unshift(grp); }
for (let i = 0; i < args.length; i += 2) {
  const type = args[i], message = args[i + 1];
  if (!valid.has(type)) { console.error("bad type: " + type + " (want fix|feature|perf)"); process.exit(1); }
  if (!message || !message.trim()) { console.error("empty message for entry " + (i / 2 + 1)); process.exit(1); }
  grp.entries.push({ type, message });
}
fs.writeFileSync(file, JSON.stringify(arr, null, 2) + "\n");
console.log("changelog: +" + (args.length / 2) + " entry(ies) in " + today + " group (now " + grp.entries.length + " total)");
' "$@"

node scripts/changelog-trim.mjs
