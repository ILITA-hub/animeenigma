# Requirements: AnimeEnigma `design-system` workstream — v1.0 Design System Consolidation

**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-06-02-design-system-consolidation-design.md`
**Convention:** No days/hours. Metrics in UXΔ / CDI / MVQ (`.planning/CONVENTIONS.md`).

Requirement IDs are workstream-local. Status: ✅ done · ⏳ planned.

## DS-FOUND — Token foundation + reference (Phase 1, shipped 2026-06-02)

| ID | Requirement | Status |
|----|-------------|--------|
| DS-FOUND-01 | Tier 2 shadcn-vue semantic slots (`--background`, `--foreground`, `--card(-foreground)`, `--popover(-foreground)`, `--primary(-foreground)`, `--secondary(-foreground)`, `--muted(-foreground)`, `--border`, `--input`, `--ring`) declared in `:root`, each referencing a Tier-1 primitive. | ✅ |
| DS-FOUND-02 | Tier 3 brand-extension tokens (`--brand-cyan`, `--brand-pink(-foreground)`, `--brand-violet`, `--success/-warning/-info/-destructive` + `-foreground`, all `*-soft` variants) declared. | ✅ |
| DS-FOUND-03 | Canonical tokens exposed to Tailwind v4 via `@theme inline` so utilities (`bg-primary`, `bg-destructive`, `border-border`, `bg-success-soft`, …) generate. | ✅ |
| DS-FOUND-04 | Duplicate legacy tokens (`--ink`, `--ink-3`, `--accent`, `--pink`, `--pink-soft`) converted to value-preserving aliases of canonical tokens; `--accent-soft`/`--ink-2`/`--ink-4` left literal (no exact equivalent). | ✅ |
| DS-FOUND-05 | Hand-rolled `.btn-primary`/`.btn-secondary`/`.btn:focus-visible` re-pointed to canonical tokens (value-preserving). | ✅ |
| DS-FOUND-06 | Canonical `frontend/web/src/styles/DESIGN-SYSTEM.md` reference (token tiers, usage rules, type/spacing/radius/elevation, deprecated-alias map, component inventory). | ✅ |
| DS-FOUND-07 | Vitest guard tests: canonical tokens declared, aliases wired, `.btn-*` re-points pinned, and the new utilities emitted in the **built** CSS (robust to multi-chunk output). | ✅ |
| DS-NF-01 | Zero rendered change: every alias / re-point resolves to an identical computed color; verified by var-chain trace + in-browser smoke on ≥5 surfaces. | ✅ |
| DS-NF-02 | Load-bearing cascade behavior preserved: `.cta-*` stays in `@layer components`; `.spotlight-frame`/`.shuffle-deck`/`.glass-card` stay unlayered. | ✅ |

## DS-LIB — shadcn-vue library adoption (Phases 2–3)

| ID | Requirement | Status |
|----|-------------|--------|
| DS-LIB-01 | Install Reka UI + `class-variance-authority` + `clsx` + `tailwind-merge`; add `cn()` helper at `src/lib/utils.ts`. | ⏳ |
| DS-LIB-02 | Configure shadcn-vue (CLI `components.json` or equivalent) to emit into `components/ui/`, wired to the canonical token names. | ⏳ |
| DS-LIB-03 | Convert `Button` to a token-driven `cva` variant API (`default`/`brand`(pink)/`ghost`/`outline`/`destructive`; sizes `sm/md/lg/icon`) — variant map per the design doc. | ⏳ |
| DS-LIB-04 | Convert `Card` (+ `CardHeader/Content/Footer`) to shadcn-vue, token-driven (`bg-card`). | ⏳ |
| DS-LIB-05 | Bring the **shadcn-vue-equivalent** `components/ui/` primitives onto shadcn-vue **behind the same import paths**: Badge, Input, Select, `Modal→Dialog`, Tabs. (ContextMenu is handled separately — see DS-LIB-08.) | ⏳ |
| DS-LIB-06 | Add the primitives currently hand-rolled: Tooltip, Popover, Switch, Checkbox. | ⏳ |
| DS-LIB-07 | The 6 **app-specific composites** in `components/ui/` (`ButtonGroup`, `GenreFilterPopup`, `PaginationBar`, `SearchAutocomplete`, `Skeleton`, `Toaster`) are NOT shadcn-vue primitives — they stay as-is in Phase 3, but must be re-pointed to canonical tokens during the migration phases (4–5) like any other component. Explicitly out of scope for the primitive *swap*. | ⏳ |
| DS-LIB-08 | **Right-click → DropdownMenu (user decision 2026-06-02).** Reka's `DropdownMenu` is trigger-anchored, not cursor-positioned, so `ContextMenu`→`DropdownMenu` is NOT a 1:1 swap. Instead: (a) **remove the custom right-click interception** so native browser right-click works again — drop the cursor `open(event)`/`preventDefault`/`clientX-Y` path from `useContextMenu` + any `@contextmenu` binding, and retire the bespoke `components/ui/ContextMenu.vue`; (b) **add a Reka-based `DropdownMenu`** to the `@/components/ui` barrel; (c) **wire it to explicit triggers where relevant** — the anime-card kebab (`AnimeKebab` → list-management actions: set status / remove / mark-next, preserving the action set + auth-gating from `AnimeContextMenu.vue`) and the subtitle chooser (`OtherSubsPanel`) — keeping the trigger-anchored `openAtElement`/kebab UX. This is an intentional UX change (native right-click restored, actions behind discoverable triggers), NOT zero-diff. See [[feedback_native_rightclick_dropdown_triggers]]. | ⏳ |
| DS-NF-03 | No npm dependency added beyond the shadcn-vue toolchain (Reka UI, cva, clsx, tailwind-merge); all MIT/BSD/Apache-compatible. | ⏳ |
| DS-NF-04 | Each primitive keeps its existing import path + a back-compatible prop surface (or a documented codemod) so consumers don't break on swap. | ⏳ |

## DS-MIGRATE — Component migration to tokens-only (Phases 4–5)

| ID | Requirement | Status |
|----|-------------|--------|
| DS-MIGRATE-01 | High-traffic surfaces (Home, Browse, Watch/player, nav, anime detail) use ONLY tokens + `ui/` primitives — no off-palette colors, no hardcoded hex. | ⏳ |
| DS-MIGRATE-02 | Off-palette Tailwind color usages (241 occurrences / 44 files) migrated to semantic tokens (`red→destructive`, `amber/yellow→warning`, `emerald/green→success`, `blue/sky→info`, `purple/violet→brand-violet`, `gray/slate/zinc→muted/card/border`), each with a human/agent semantic judgment. | ⏳ |
| DS-MIGRATE-03 | Hardcoded hex in `.vue` (17 files) replaced with tokens (or a new token added if a value is legitimately novel). | ⏳ |
| DS-MIGRATE-04 | Deprecated alias usages (`var(--ink)`, `var(--accent)`, `var(--pink)`; ~19 files) repointed to canonical names. | ⏳ |
| DS-MIGRATE-05 | After DS-MIGRATE-04, flip `--accent` to its shadcn hover-surface meaning and drop the temporary brand-cyan alias. | ⏳ |
| DS-MIGRATE-06 | Hand-rolled buttons/cards/badges replaced with `ui/` primitives where they exist. | ⏳ |

## DS-GOV — Enforcement + governance (Phases 5–6)

| ID | Requirement | Status |
|----|-------------|--------|
| DS-GOV-01 | Lint rule (stylelint or custom check) that FAILS the build on hardcoded hex / off-palette Tailwind colors in `.vue`; wired into the `redeploy-web` deploy gate. | ⏳ |
| DS-GOV-02 | Allowlist/escape-hatch documented for legitimate exceptions (e.g. third-party embed colors) so the lint rule is livable. | ⏳ |
| DS-GOV-03 | Governance rules ("use tokens, never hardcode, reuse `ui/` primitives before building new") written into project memory + `CLAUDE.md`. | ⏳ |
| DS-NF-05 | Every phase independently shippable with zero breakage between phases; the app builds, tests pass, and renders correctly after each. | ⏳ |
| DS-NF-06 | Visual changes verified in a real browser (jsdom can't catch cascade bugs), per the project's standing rule. | ⏳ |
