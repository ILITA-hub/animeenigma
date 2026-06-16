#!/usr/bin/env node
// changelog-trim.mjs — generate the SERVED changelog from the full history.
//
// Why: `public/changelog.json` is fetched whole on every page load (FE
// LastUpdates.vue Changelog tab + the backend spotlight "Latest News" card).
// The full history grew to hundreds of KB while consumers only ever render
// the newest handful of entries. We keep the complete log in
// `changelog.full.json` (source of truth, NOT under public/ so it is never
// served) and emit only the latest N entries to the served file.
//
// Flow (driven by the /animeenigma-after-update skill, step 4):
//   1. Prepend new entries to changelog.full.json
//   2. node scripts/changelog-trim.mjs   (regenerates public/changelog.json)
//   3. Commit both files
//
// "Latest N entries" counts individual entries across date groups (newest
// first), slicing the boundary group — matching LastUpdates.vue's
// limitedChangelog logic so the served file is exactly what the UI can show.

import { readFileSync, writeFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, join } from 'node:path'

const MAX_ENTRIES = 30

const here = dirname(fileURLToPath(import.meta.url))
const FULL = join(here, '..', 'changelog.full.json')
const SERVED = join(here, '..', 'public', 'changelog.json')

const groups = JSON.parse(readFileSync(FULL, 'utf8'))
if (!Array.isArray(groups)) {
  console.error('changelog-trim: changelog.full.json is not an array')
  process.exit(1)
}

const out = []
let count = 0
for (const group of groups) {
  if (count >= MAX_ENTRIES) break
  const entries = Array.isArray(group.entries) ? group.entries : []
  const sliced = entries.slice(0, MAX_ENTRIES - count)
  out.push({ date: group.date, entries: sliced })
  count += sliced.length
}

writeFileSync(SERVED, JSON.stringify(out, null, 2) + '\n')
console.log(
  `changelog-trim: wrote ${count} entries across ${out.length} date group(s) ` +
  `to public/changelog.json (full history: ${groups.length} groups retained in changelog.full.json)`,
)
