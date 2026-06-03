# Phase 6: Governance into Memory - Context

**Gathered:** 2026-06-03
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure/docs phase — discuss skipped)

<domain>
## Phase Boundary

Write the design-system rules where every future session (human or AI) will see them, so the
now-enforced reality (Phases 1–5) doesn't silently erode. Pure documentation/governance — no code,
no rendered change. Two surfaces:
1. **Project memory** (`/root/.claude/projects/-data-animeenigma/memory/` + the `MEMORY.md` index
   pointer) — a durable design-system governance entry the assistant loads each session.
2. **`CLAUDE.md`** (repo root) — a "Design System" subsection pointing at the canonical
   `frontend/web/src/styles/DESIGN-SYSTEM.md` + the enforced lint gate.

Critical consistency rule (SC#3): the governance text MUST match the ENFORCED lint rule exactly —
do not document a rule that isn't enforced, and don't omit one that is. The enforced gate
(`frontend/web/scripts/design-system-lint.sh`, wired into `make lint-frontend` + `redeploy-web`)
enforces ONLY the 3 color/token rules:
1. No off-palette Tailwind color classes (brand cyan/pink/orange/rose/indigo/teal/lime are EXEMPT).
2. No hardcoded hex outside `frontend/web/scripts/design-system-allowlist.txt`.
3. No deprecated `var(--ink | --accent | --pink)` brand-alias usages.
Structural rules (reuse `ui/` primitives before building new; only `font-medium`/`font-semibold`
weights; padding scales; `cva` variants) are GOVERNANCE-ONLY (human/AI-followed, NOT build-enforced)
— the governance text must label them as such so the "matches the enforced rule" criterion holds.

</domain>

<decisions>
## Implementation Decisions

### Governance content (the rules to write — same on both surfaces, phrased for the audience)
- **Use semantic tokens, never hardcode** — bind to the canonical tokens in
  `frontend/web/src/styles/DESIGN-SYSTEM.md`; never raw Tailwind off-palette color classes
  (`text-red-500`, `bg-emerald-400`, …) and never hardcoded hex. Brand/identity hues
  (cyan/pink/orange/rose/indigo/teal/lime — Neon-Tokyo + per-provider player accents) are the
  documented exception and are lint-exempt; genuinely-novel hex goes in
  `scripts/design-system-allowlist.txt` with a `path:hex:reason` entry, not inline.
- **Reuse `@/components/ui` primitives before building new** — Button/Card/Badge/Input/Select/
  Dialog/Tabs/DropdownMenu/Tooltip/Popover/Switch/Checkbox are token-driven shadcn-vue primitives;
  reach for them before hand-rolling. (Governance-only — not lint-enforced.)
- **Verify visual changes in a real browser** (DS-NF-06) — jsdom/vitest cannot catch Tailwind-v4
  cascade bugs (unlayered custom classes beat utilities). Any rendered change gets an in-browser
  smoke at desktop + mobile, not just a passing unit test.
- **The gate is real** — `make lint-frontend` (and therefore `make redeploy-web`) fails the build on
  the 3 enforced rules; `bash frontend/web/scripts/design-system-lint.sh --selftest` proves the
  fail-path. Don't disable it; add an allowlist entry with a reason instead.
- **`--accent` is the shadcn hover surface** (since 05-04), not brand-cyan — use `--brand-cyan`
  for brand cyan.

### Surface specifics
- **Memory entry:** one focused file in `/root/.claude/projects/-data-animeenigma/memory/`
  (kebab-name, frontmatter `type: project` or `reference`, description for recall), body =
  the rules above + a `[[...]]` link to related memories where natural, plus a pointer to
  `DESIGN-SYSTEM.md`. Add a one-line pointer to `MEMORY.md`. (This is the assistant's own memory
  store — orchestrator may write it directly given the system-prompt memory format.)
- **CLAUDE.md:** a new "## Design System" subsection (concise) — the rules above in brief +
  the canonical `DESIGN-SYSTEM.md` path + the lint-gate command. Place it near the existing
  frontend/conventions guidance. Do NOT duplicate the full DESIGN-SYSTEM.md; point at it.

### Claude's Discretion
Exact wording/placement is at Claude's discretion provided SC#1–3 hold (memory entry exists with the
4 rule themes + DESIGN-SYSTEM pointer; CLAUDE.md has a Design System subsection; governance text
matches the enforced lint rules — enforced labeled enforced, governance-only labeled governance-only).

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `frontend/web/src/styles/DESIGN-SYSTEM.md` — the canonical token reference + the "Lint gate
  (enforced)" section 05-05 added. Both governance surfaces POINT at this; don't duplicate it.
- `frontend/web/scripts/design-system-lint.sh` + `design-system-allowlist.txt` — the enforced gate.
- `CLAUDE.md` — already documents the 5-player architecture, conventions, after-update skill; the
  Design System subsection slots into the conventions area.
- Project memory format is specified in the assistant's system prompt (frontmatter name/description/
  metadata.type; `MEMORY.md` one-line pointer; `[[name]]` cross-links).

### Established Patterns
- Memory entries are one-fact files with a `MEMORY.md` index pointer.
- CLAUDE.md uses `##`/`###` sections with terse, imperative project rules.

### Integration Points
- `/root/.claude/projects/-data-animeenigma/memory/` (+ `MEMORY.md`).
- `CLAUDE.md` (repo root).

</code_context>

<specifics>
## Specific Ideas

- SC#3 is the load-bearing one: enumerate the 3 enforced rules vs the governance-only rules and make
  the docs say which is which, so a reader never thinks "reuse primitives" is build-enforced (it
  isn't) or that off-palette classes are merely advisory (they're a hard build failure).
- DS-NF-05 (every phase independently shippable) and DS-NF-06 (verify-in-browser) are process/standing
  rules — DS-NF-06 becomes a governance bullet; DS-NF-05 is satisfied by the milestone's per-phase
  green builds and needs only a one-line acknowledgement in the milestone record, not new machinery.

</specifics>

<deferred>
## Deferred Ideas

- An AST-based lint that enforces the structural rules (reuse-primitives, weights, padding) — out of
  scope; those stay governance-only by deliberate decision (Phase 5).

</deferred>
