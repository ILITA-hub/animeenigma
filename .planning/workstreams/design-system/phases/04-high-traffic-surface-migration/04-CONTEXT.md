# Phase 4: High-Traffic Surface Migration - Context

**Gathered:** 2026-06-02
**Status:** Ready for planning
**Mode:** Auto-generated (infrastructure/migration phase — discuss skipped)

<domain>
## Phase Boundary

Migrate the heaviest-used surfaces to tokens-only + `ui/` primitives — Home, Browse,
Watch/player (all 5 players), nav/layout, anime detail + children. End-state: those
surfaces contain **zero off-palette color classes and zero hardcoded `#hex`**; their
buttons/cards/badges use the `@/components/ui` primitives delivered in Phases 2–3.

This is a **value-preserving migration** — the milestone premise is "zero rendered
change." Every touched surface must render pixel-identically to today at desktop +
mobile widths (the standing 5-surface smoke set). No new UX, no layout change.

**In scope (Touches):** `views/Home.vue`, `views/Browse.vue`, `views/Watch*.vue`,
the 5 player components, `components/layout/*` nav, anime-detail components + children.

**Out of scope:** the tail components (Phase 5), the `--accent` semantic flip
(DS-MIGRATE-05, deferred to Phase 5 — do NOT flip here), the lint gate (Phase 5),
the 6 app-specific composites already excluded in Phase 3 (DS-LIB-07).

</domain>

<decisions>
## Implementation Decisions

### Color → Token Mapping (locked by DS-MIGRATE-02)
The off-palette → semantic-token mapping is fixed by requirements, applied per-occurrence
with a semantic judgment:
- `red` → `destructive`
- `amber`/`yellow` → `warning`
- `emerald`/`green` → `success`
- `blue`/`sky` → `info`
- `purple`/`violet` → `brand-violet`
- `gray`/`slate`/`zinc` → `muted`/`card`/`border` (judgment per role)

### Hardcoded hex (DS-MIGRATE-03)
Replace `#hex` in `.vue` with the nearest canonical token. If a value is legitimately
novel (no token within tolerance), add a new token to `main.css` rather than inlining.

### Deprecated aliases (DS-MIGRATE-04, partial here)
Repoint `var(--ink)`/`var(--accent)`/`var(--pink)` brand usages found in the
high-traffic files to canonical token names. The `--accent` *semantic flip* itself
stays in Phase 5 — Phase 4 only repoints usages.

### Primitive adoption (DS-MIGRATE-06, partial here)
Replace hand-rolled buttons/cards/badges on these surfaces with the `ui/` primitives
where an equivalent exists; leave bespoke structures that have no primitive analog.

### Claude's Discretion
- Per-occurrence semantic judgment within the fixed mapping (e.g. is this gray a
  `muted` background, a `card` surface, or a `border`?).
- Order of surface migration; how to batch into plans/waves.
- When a hex is "novel enough" to warrant a new token vs. snap to an existing one.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `@/components/ui` barrel — Button, Card, Badge, Input, Select, Dialog, Tabs,
  DropdownMenu, Tooltip, Popover, Switch, Checkbox (all token-driven, shadcn-vue, Phases 2–3).
- `main.css` — single layered token source of truth (Phase 1): Tier-2/3 tokens,
  `--success/--warning/--info/--destructive`, `--brand-violet`, value-preserving aliases.
- `DESIGN-SYSTEM.md` — canonical token reference.

### Surfaces carrying off-palette color classes (scout, 2026-06-02)
Home.vue, Browse.vue; anime/{AnimeCardNew, AnimeContextMenu, AnimeKebab, AnimeQuickNav,
EpisodeCard, GenreChip}.vue; layout/{FeedbackButton, Navbar}.vue;
player/{AnimeLibPlayer, HanimePlayer, KodikPlayer, OtherSubsPanel, OurEnglishPlayer,
RawPlayer, ResumePill, SubtitleSettingsMenu}.vue.

### Surfaces carrying hardcoded hex (scout, 2026-06-02)
anime/AnimeCard.vue; layout/{BrandMark, Navbar}.vue;
player/{AnimeLibPlayer, HanimePlayer, KodikPlayer, SubtitleOverlay}.vue.
(BrandMark/SubtitleOverlay hex may be legitimately novel brand/render values — judge per case.)

### Established Patterns
- Tailwind utility-only styling bound to `@theme inline` tokens.
- jsdom CANNOT catch cascade/render regressions (DS-NF-06) → in-browser smoke is mandatory.
- `main.css` custom classes are UNLAYERED and beat utilities — verify cascade in-browser.

</code_context>

<specifics>
## Specific Ideas

Standing visual-regression smoke set (MUST re-smoke at desktop + mobile after migration):
1. **Home** — spotlight carousel (stats joke card + RandomTail purple `cta-hero`), rails
2. **Browse / catalog** — filter sidebar, star badges, cards, pagination
3. **Anime detail** — cyan `.btn-primary` ("Смотреть"), status badge, schedule pill, language pills + green OurEnglish button
4. **A watch/player surface** — one of the 5 players loads + styled controls
5. **404** — muted styling + "На главную" button

</specifics>

<deferred>
## Deferred Ideas

- `--accent` semantic flip + brand-cyan alias deletion → Phase 5 (DS-MIGRATE-05).
- Tail component sweep + lint enforcement gate → Phase 5.
- Governance into memory/CLAUDE.md → Phase 6.

</deferred>
