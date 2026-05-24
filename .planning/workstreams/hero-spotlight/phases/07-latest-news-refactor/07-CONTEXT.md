# Phase 07: LatestNewsCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Visual hierarchy for changelog entries:
- Type-icons per entry (feat → sparkles, fix → wrench, perf → lightning).
- Type pill ("Новая фича" / "Исправление" / "Улучшение") with type-coded accent.
- Relative dates via `Intl.RelativeTimeFormat`.
- Drop fragile title-split regex; 60-char fallback truncation.

In scope: `cards/LatestNewsCard.vue` + spec, i18n updates for typeFeat/typeFix/typePerf keys.
</domain>

<decisions>
- Extend tokens.ts: `cardTokens.latest_news.iconByType` + `cardTokens.latest_news.accentByType`.
- Backend `latest_news` resolver already emits `{date, type, message}` — no backend changes this phase. Optional body/title split is deferred to v1.2.
- Drop the sentence-splitter regex entirely. Title = first 60 chars + ellipsis if longer; body removed from card.
</decisions>

<code_context>
- `SpotlightIcon names`: sparkles, wrench, lightning exist from Phase 01.
- Backend `LatestNewsData.entries[]` already has `type` field (feat/fix/perf/docs).
- i18n: add `spotlight.latestNews.{typeFeat, typeFix, typePerf}` keys to all 3 locales.
</code_context>

<specifics>
- `formatEntryDate` uses Intl.RelativeTimeFormat with locale; falls back to absolute date for >30 days.
- Title computed via `entryTitle(msg)` = `msg.length > 60 ? msg.slice(0, 60).trimEnd() + '…' : msg`.
- Body element removed entirely.
</specifics>

<deferred>
- Backend split of `message` → `title` + `body` (v1.2; resolver schema change).
</deferred>
