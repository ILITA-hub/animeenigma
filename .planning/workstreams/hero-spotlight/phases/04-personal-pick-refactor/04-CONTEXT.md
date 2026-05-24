# Phase 04: PersonalPickCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Replace the 3-equal-posters grid with featured (60%) + 2 secondary (40% stacked) layout.
Fix the truncated-title bug. Username personalization. Per-item reason chip.
Mobile "+ N more" becomes a full-width footer button.

In scope: `cards/PersonalPickCard.vue` + spec, i18n updates for `titleWithName` key.
</domain>

<decisions>
- Desktop: `grid-cols-[3fr_2fr]`; featured uses backdrop = its own poster.
- Mobile: featured fills width, secondary picks hidden, full-width "+ N more" button.
- Username read from `useAuthStore()` (already wired in profile/header).
- Reason chip uses `<SpotlightIcon name="sparkles" />` + i18n key from each item.
</decisions>

<code_context>
- `useAuthStore()` at `frontend/web/src/stores/auth.ts` — has `user.username`.
- `useMediaQuery('(min-width: 768px)')` already used in current PersonalPickCard.
- PersonalPickData type: `data.items: PersonalPickItem[]` + `data.source: 'trending' | 'personal'`.
</code_context>

<specifics>
- New i18n key: `spotlight.personalPick.titleWithName` with `{name}` interpolation, all 3 locales.
- Title computed: anon → `titleAnon`, personal with username → `titleWithName`, personal without username → `title`.
</specifics>

<deferred>
- Reason chip color variants per source (could go in tokens.ts if v1.2 adds more sources).
</deferred>
