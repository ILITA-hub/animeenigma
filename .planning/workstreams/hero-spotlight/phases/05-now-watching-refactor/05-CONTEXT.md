# Phase 05: NowWatchingCard refactor - Context

**Gathered:** 2026-05-22
**Mode:** Auto-generated from approved REFACTOR-PROPOSAL.md

<domain>
Make NowWatchingCard feel alive:
- Bigger poster thumbs (56×84, 3.5× current).
- Deterministic hashed avatar circles per user.
- `<SpotlightBackdrop variant="gradient-mesh" accent="green">` (animated cyan→green mesh).
- Pulsing LIVE micro-element next to avatar (not text on right).

In scope: `cards/NowWatchingCard.vue` + spec.
</domain>

<decisions>
- Avatar palette: 8 colors. Deterministic hash → palette index.
- Mesh animation: 20s drift (existing CSS pattern in Phase 01's gradient-mesh variants — extend if needed).
- LIVE indicator: pulsing green dot (`bg-green-400 animate-pulse`) at bottom-right of avatar circle.
- Hover row: lift effect + accent border (`hover:bg-white/10`).
</decisions>

<code_context>
- `SpotlightBackdrop variant="gradient-mesh" accent="green"` exists from Phase 01.
- `useI18n()` already imported.
- NowWatchingData.sessions: array of `{public_id, username, anime_id, anime_name, anime_name_ru, poster_url, episode_number}`.
- `<router-link>` used for each row.
</code_context>

<specifics>
- Avatar circle: `w-10 h-10 rounded-full flex items-center justify-center text-sm font-semibold text-white {bgClass}` showing `username[0].toUpperCase()`.
- Hash: deterministic 31-mult algorithm (`hash = hash * 31 + ch.charCodeAt(0)`); `abs(hash) % 8` → palette.
</specifics>

<deferred>
- WebSocket-driven now_watching updates (still 10s Redis polling — deferred to v1.2).
- Avatar URL support if users get profile pictures in future.
</deferred>
