# DS Allowlist Rename-Safety + i18n Parity (shift-left) — Design

**Date:** 2026-06-18
**Status:** Approved (approaches chosen via brainstorming)
**Author:** Claude (with owner)

## Goal

Close two frontend-tooling gaps surfaced during the DS/aePlayer work:

1. **Allowlist rename-safety** — the two design-system allowlists key exceptions
   by file path; a `git mv` silently desyncs them. Make the desync loud and
   located instead of silent.
2. **i18n parity, shifted left** — full en/ru/ja key parity is only enforced at
   `make redeploy-web` (via `i18n-lint.sh`). Move an equivalent check into the
   `vitest` loop that agents and CI actually run, and extend it to ICU
   placeholder parity.

Out of scope (tracked separately): narrowing the player surface to aePlayer —
owned by `project_retire_all_players_except_aeplayer`. See "Point 3" at the end
for the one concrete FE next-step this work enables.

---

## Point 1 — Stale-path detection in `design-system-lint.sh`

### Problem

`frontend/web/scripts/design-system-lint.sh` reads two allowlists:
- `design-system-allowlist.txt` (`path:hex|rgba|inline-color:reason`, Rules 2/7/8)
- `design-system-spacing-allowlist.txt` (`path:class:reason`, Rule 6)

Both key entries by file path relative to `frontend/web`. When a `.vue` is
renamed/moved (e.g. `UnifiedPlayer.vue` → `AePlayer.vue`), two things break:

- **(a)** the renamed file's real exceptions stop matching → false-positive lint
  errors on legitimately-allowlisted literals; and
- **(b)** the old line points at a vanished file → **silent rot**: nothing today
  ever checks that an allowlisted path still exists, so the dead line lingers
  forever and the next rename's confusion compounds.

This bit the AePlayer rename and is recorded as a hazard in project memory.

### Approach (chosen): stale-path detection

Add a path-integrity pre-pass that fails the build (ERROR tier — this is a build
gate; a WARN would just rot) when an allowlist line references a path that no
longer exists. This converts (b) from silent rot into a **loud, located,
build-failing** signal at the moment of the rename. DS lint already runs in
`make lint`/CI (not only at deploy), so the failure surfaces early, and the fix
is a one-line path edit in the same commit.

Rejected alternative: migrating all ~160 entries to inline `bespoke-keep`-style
comments (rename-immune). Philosophically nicer and the pattern already exists
for Rule 5, but the migration cost isn't justified by how rarely renames bite,
and some entries (cva `.ts` class-strings, gradient `<stop>`s) annotate awkwardly
inline.

### Design

New helper, called once per allowlist file:

```bash
# check_allowlist_paths <allowlist_file> — ERROR on any non-comment line whose
# leading path field (everything before the first ':') does not resolve to an
# existing file under frontend/web. Catches stale lines left by renames/deletes.
check_allowlist_paths() {
  local allow_file="$1" label="$2"
  [ -f "$allow_file" ] || return 0
  local any=0
  while IFS= read -r line; do
    case "$line" in ''|\#*) continue ;; esac   # skip blank + comment lines
    local path="${line%%:*}"
    [ -z "$path" ] && continue
    if [ ! -f "$SCRIPT_DIR/../$path" ]; then
      echo -e "  ${RED}ERROR${NC} ${label}: '${path}' no longer exists (renamed/deleted? update or remove the line)"
      ALLOWLIST_PATH_ERRORS=$((ALLOWLIST_PATH_ERRORS + 1)); ERRORS=$((ERRORS + 1)); any=1
    fi
  done < "$allow_file"
  [ "$any" -eq 0 ] && echo -e "  ${GREEN}OK${NC} ${label}: all allowlisted paths exist"
}
```

Notes on the path-extraction:
- Allowlist format is `path:value:reason`. The path is always the segment before
  the **first** `:`. Values like `rgb(45,212,191)` contain commas but no colon
  before `:reason`, so `${line%%:*}` cleanly yields the path for every existing
  entry. (Confirmed against both current allowlist files.)
- `case` skips blank and `#`-comment lines (same convention as the rest of the
  script).

Wiring:
- New section header `=== ALLOWLIST: path integrity ===` run before Rule 1.
- New counter `ALLOWLIST_PATH_ERRORS` initialized with the other per-rule
  counters and printed in the Summary block.
- Call `check_allowlist_paths "$ALLOWLIST" "design-system-allowlist.txt"` and
  `check_allowlist_paths "$SPACING_ALLOWLIST" "design-system-spacing-allowlist.txt"`.

`--selftest` additions (prove the fail-path, leave the tree clean):
- Write a temp allowlist file containing one bogus path line
  (`src/__does_not_exist__.vue:#abc:bogus`) → assert `check_allowlist_paths`
  increments the error count.
- Write a temp allowlist file containing one real path line (e.g.
  `src/App.vue:...`) → assert it passes. (Use a path known to exist; the helper
  only checks existence, not the value.)

Drive-by fix while in the file: the header doc-comment still says "Enforces 5
… rules" (it runs 8) — update it to "8 rules + allowlist path integrity".

### Acceptance

- Renaming/deleting a `.vue` that has an allowlist entry, without updating the
  allowlist, makes `bash scripts/design-system-lint.sh` exit 1 with a message
  naming the stale path and file.
- A clean tree (all allowlist paths exist) still passes all rules.
- `bash scripts/design-system-lint.sh --selftest` exits 0 and proves both the
  stale-detect and the clean-pass cases for the new check.
- No change to the behavior of Rules 1–8 on the current clean tree.

---

## Point 2 — `locale-parity.spec.ts` (vitest, full-tree)

### Problem

`i18n-lint.sh` already does full en/ru/ja key parity, but it only runs as a
prerequisite of `make redeploy-web`. The loops agents and CI actually run —
`bunx vitest run`, `bunx tsc --noEmit` — stay green while `ja.json` lags, so a
missing/added key is only caught at deploy. There is a per-namespace parity spec
(`spotlight-keys.spec.ts`, plus gacha/wt namespaces) but no general one.

### Approach (chosen): one full-tree vitest spec, key + placeholder parity

Add `frontend/web/src/locales/__tests__/locale-parity.spec.ts` (~40 lines). It
generalizes the existing `spotlight-keys.spec.ts` pattern to **all** namespaces
and adds ICU placeholder-set parity. This lands the check in the vitest loop so a
lagging locale fails immediately.

The existing per-namespace specs are now subsumed but are left in place (harmless;
deleting them is optional cleanup, not part of this change).

### Design

```ts
import { describe, it, expect } from 'vitest'
import en from '../en.json'
import ru from '../ru.json'
import ja from '../ja.json'

type Json = Record<string, unknown>

function flatten(obj: Json, prefix = ''): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${k}` : k
    if (v && typeof v === 'object' && !Array.isArray(v)) {
      Object.assign(out, flatten(v as Json, key))
    } else {
      out[key] = String(v)
    }
  }
  return out
}

// ICU named/list placeholders: {name}, {count}, {0}. (Not vue-i18n linked
// messages @:foo or escaped literals {'@'} — those are not interpolation vars.)
function placeholders(s: string): Set<string> {
  const set = new Set<string>()
  for (const m of s.matchAll(/\{\s*([a-zA-Z0-9_]+)\s*\}/g)) set.add(m[1])
  return set
}

const locales = { en: flatten(en as Json), ru: flatten(ru as Json), ja: flatten(ja as Json) }
const names = Object.keys(locales) as Array<keyof typeof locales>

describe('locale key parity', () => {
  const allKeys = new Set<string>()
  for (const n of names) Object.keys(locales[n]).forEach((k) => allKeys.add(k))

  for (const n of names) {
    it(`${n}.json has no missing/extra keys vs the union`, () => {
      const have = new Set(Object.keys(locales[n]))
      const missing = [...allKeys].filter((k) => !have.has(k)).sort()
      // every key must appear in every locale → union === each locale's set
      expect({ locale: n, missing }).toEqual({ locale: n, missing: [] })
    })
  }
})

describe('locale ICU placeholder parity', () => {
  // Only keys present in all locales (key parity is asserted above; this keeps
  // the failure messages from this block focused on placeholder drift).
  const common = Object.keys(locales.en).filter(
    (k) => k in locales.ru && k in locales.ja,
  )
  for (const key of common) {
    it(`"${key}" has matching placeholders across locales`, () => {
      const en_ = [...placeholders(locales.en[key])].sort()
      const ru_ = [...placeholders(locales.ru[key])].sort()
      const ja_ = [...placeholders(locales.ja[key])].sort()
      expect({ key, ru: ru_, ja: ja_ }).toEqual({ key, ru: en_, ja: en_ })
    })
  }
})
```

Notes:
- Reads JSON via `import` (matches how the app and existing specs consume
  locales; `resolveJsonModule` is already on).
- Key-parity assertion is symmetric: every key in the union must be in every
  locale, so a key in only en (extra) or only ru (missing elsewhere) both fail.
- Placeholder parity compares the **set** of `{var}` names per key against en as
  the reference; order/duplication don't matter.
- Plural `|`-form-count parity is intentionally *not* checked (more nuanced;
  out of scope for this pass).

### Acceptance

- `bunx vitest run src/locales/__tests__/locale-parity.spec.ts` passes on the
  current tree (or, if it fails, it has found a real pre-existing lag — surface
  it and fix the locales, don't weaken the test).
- Removing a key from `ja.json` makes the spec fail with the key named.
- Changing a `{count}` to `{n}` in only `ja.json` makes the placeholder block
  fail with the key named.
- `bunx tsc --noEmit` stays clean.

---

## Point 3 (scoped separately) — FE player-surface next step

Direction is locked by `project_retire_all_players_except_aeplayer` (retire
Kodik/AniLib/Hanime/Raw; aePlayer survives; retire via `status`, not delete; the
`scraper_providers → stream_providers` roster + playback-health dashboard
refactor is the backend half). The one concrete FE step this tooling work
*unblocks*: when the legacy player `.vue` files are eventually deleted, Point 1's
stale-path check will immediately flag their orphaned allowlist lines (the
per-player accent-hue entries in `design-system-allowlist.txt`, lines ~18–24),
turning "silently forgotten cleanup" into a build error that names each line.
That makes the deletion step self-checking. No FE player code changes here.

---

## Files

- Modify: `frontend/web/scripts/design-system-lint.sh` (new helper + wiring +
  selftest + header doc-comment fix).
- Create: `frontend/web/src/locales/__tests__/locale-parity.spec.ts`.

No production code changes; both items are build/test tooling.
