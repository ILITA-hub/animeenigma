# Phase 5: Tail Sweep + Lint Enforcement - Context

**Gathered:** 2026-06-03
**Status:** Ready for planning
**Mode:** Smart discuss (autonomous) — grey areas proposed in batch, user-accepted

<domain>
## Phase Boundary

Finish the design-system migration and lock it shut:
1. **Tail sweep** — migrate the ~62 remaining `frontend/web/src/**/*.vue` components that still carry
   off-palette Tailwind color classes (and any non-allowlisted hex) onto canonical semantic tokens.
   Same value-preserving color/token-only discipline proven in Phase 4 (zero behavioral change).
2. **Alias retirement** — delete the retired brand-alias vocabulary from `main.css` once no component
   references it.
3. **`--accent` flip** — repoint `--accent` from its temporary brand-cyan alias to the shadcn hover
   surface, and delete the temporary alias.
4. **Lint enforcement** — a build-failing custom lint gate that prevents regression of the color/token
   discipline, wired into the same path the project's existing lint gate runs.

OUT OF SCOPE: structural/semantic DS conventions ("use `ui/` primitives", weight/padding rules,
`cva` variants) are NOT mechanically enforced here — they are Phase 6 governance documentation.
The `--accent` flip is the only intentional rendered change in the whole milestone (a deliberate,
documented hover-surface correction), and must be smoke-checked.

</domain>

<decisions>
## Implementation Decisions

### Lint Gate — Tooling & Scope
- **Tooling:** a custom bash script `frontend/web/scripts/design-system-lint.sh`, mirroring the
  existing `frontend/web/scripts/i18n-lint.sh` pattern (`set -euo pipefail`, colored output,
  ERRORS/WARNINGS counters, exit 1 on errors). NO new dependency (no stylelint, no eslint plugin).
  Reuses the exact off-palette/hex/alias greps Phase 4's executors already validated.
- **Scope (color/token discipline only — the roadmap SC#1/#3 rules):**
  1. Zero off-palette Tailwind color classes (`text-/bg-/border-/ring-/from-/to-/via-` +
     red|amber|yellow|green|emerald|blue|purple|pink|orange|cyan|indigo|teal|violet|rose|lime|sky + `-<n>`).
  2. Zero hardcoded hex outside the documented allowlist (both class arbitrary values `*-[#...]` and
     raw hex in `<style>`/inline).
  3. Zero deprecated brand-alias `var(--ink | --accent | --pink)` usages (after the flip, `--accent`
     brand usage is itself a violation).
- **Explicitly NOT in lint scope** (deferred to Phase 6 governance docs, not build-enforced):
  use-the-primitives, font-weight scale, padding scale, `cva` variant usage. A grep cannot reliably
  distinguish a hand-rolled button from a primitive without AST analysis.

### Escape-Hatch / Allowlist
- **Central allowlist file** that the lint script reads — `frontend/web/scripts/design-system-allowlist.txt`,
  one `path:hex:reason` (or `path # reason`) entry per documented novel-hex. Seed it from the items
  Phase 4 already documented: per-player `--player-accent` hues (`#06b6d4`, `#f97316`, `#ec4899`),
  SubtitleOverlay render defaults (`#ffffff`, `#ffcccc`), Navbar avatar teal gradient
  (`#1a3a4a`→`#0e2030`), BrandMark `--accent-glow`, and the canonical-fine `var(--token,#fallback)`
  fallbacks. Allowlist is documented in DESIGN-SYSTEM.md.

### `--accent` Flip + Temp-Alias Deletion
- **Atomic dedicated step.** First grep-confirm there are ZERO remaining brand `var(--accent)` usages
  across `src/` (Phase 4 already repointed brand `--accent` → `--brand-cyan`). Then, in one commit,
  flip the `--accent` definition in `main.css` to the shadcn hover surface and delete the temporary
  brand-cyan alias. This is the milestone's only intentional rendered change — must be visually
  smoke-checked (human-verify; auto-approved in autonomous mode, persisted as HUMAN-UAT).

### Enforcement Wiring
- Wire the new gate into **`make lint-frontend`** (a new `lint-design` sub-target invoked from
  `lint-frontend`, alongside `bun lint`), so it runs on the SAME path `redeploy-web` / CI already use.
  A deliberately-introduced `bg-red-500` must fail the gate (SC#3 test case); the clean tree must pass.
- NO standalone GitHub Actions workflow this phase (user chose the make-chain path).

### Tail-Sweep Execution (Claude's discretion)
- Batch the ~62 files by area across multiple plans/waves (e.g. settings/profile, watch-together,
  rooms/game, modals/dialogs, misc surfaces) to keep each plan reviewable. Files are disjoint per plan
  for safety. Same atomic-per-task, explicit-path staging discipline as Phase 4 (never `git add -A`;
  pre-existing unrelated uncommitted changes must not be swept in).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `frontend/web/scripts/i18n-lint.sh` — the canonical custom-lint pattern to mirror (bash,
  `set -euo pipefail`, colored ERRORS/WARNINGS, exit 1). Wired via `make lint-frontend` → `bun lint`.
- `frontend/web/src/styles/DESIGN-SYSTEM.md` — token reference; source of truth for off-palette →
  semantic token mapping. Add the allowlist + lint-gate documentation here.
- Phase 4 SUMMARYs + `04-.../deferred-items.md` — the proven grep patterns and the documented
  novel-hex inventory to seed the allowlist.
- `main.css` (Tailwind v4) — holds the token + alias definitions. Note the v4 cascade footgun
  (unlayered custom classes beat utilities): verify the `--accent` flip in-browser, not just jsdom
  (per project memory reference_tailwind_v4_css_cascade).

### Established Patterns
- Frontend lint runs through `make lint-frontend` (`cd frontend/web && bun lint`), itself part of the
  top-level `make lint` / `all` chain. `make redeploy-web` is the deploy path.
- Color migration = value-preserving class/var swaps verified by: acceptance grep (zero off-palette),
  `bunx vue-tsc --noEmit`, `bunx vite build`, `bunx vitest run`.

### Integration Points
- `Makefile` `lint-frontend` target (add `lint-design`).
- `main.css` `--accent` definition + retired aliases.
- `frontend/web/scripts/` (new `design-system-lint.sh` + `design-system-allowlist.txt`).

</code_context>

<specifics>
## Specific Ideas

- Reuse Phase 4's exact off-palette/hex/alias greps verbatim in the lint script — they are already
  field-proven across 24 migrated files.
- The `bg-red-500` SC#3 self-test should be runnable (e.g. the script supports a `--selftest` mode, or
  a documented one-liner that injects + reverts the marker) so the gate's fail-path is provable.

</specifics>

<deferred>
## Deferred Ideas

- Extra greppable rules (hex in `<style>` blocks beyond allowlist, arbitrary `text-[#...]` values,
  off-scale font weights `font-bold`/`font-light`) — user chose the narrower roadmap scope; these can
  be added to the lint script later if desired.
- A dedicated GitHub Actions CI workflow for the DS gate — deferred; the make-chain wiring is enough
  for this phase.
- Structural/semantic DS conventions (use-the-primitives, weight/padding scales, `cva` variants) —
  Phase 6 governance documentation, not build-enforced.

</deferred>
