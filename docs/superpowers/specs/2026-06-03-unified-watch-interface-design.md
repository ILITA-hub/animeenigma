# Unified Watch Interface, Shared Episode Selector & Live Presence — Design

- **Date:** 2026-06-03
- **Status:** Approved (brainstorm) → ready for implementation plan
- **Author:** AI pair + 0neymik0
- **Workstream:** watch-together (extends v1.0)

## Metrics (per `.planning/CONVENTIONS.md`)

| Metric | Aggregate | Notes |
|---|---|---|
| **UXΔ** | **+3 (Better)** | One consistent selector + watched-state on every player + live co-presence in rooms. Removes a visible inconsistency the user reported and adds a new collaborative layer. |
| **CDI** | **0.06 * 55** | Wide spread (all 6 player SFCs, 2 views, WT service) but low per-site shift (delegation, not rewrite). Effort Fibonacci 55 total across 3 phases. |
| **MVQ** | **Kraken 80%/75%** | Sprawling, multi-tentacled change reaching frontend SFCs, composables, two views, and the Go WS service — but each tentacle is well-bounded. |

Per-phase effort (Fibonacci, not time): **Phase A = 8**, **Phase B = 21**, **Phase C = 21** (≈55 combined).

---

## 1. Background & Current State

The 5 (+1) video players — `KodikPlayer`, `AnimeLibPlayer`, `OurEnglishPlayer`, `HanimePlayer`, `RawPlayer` (+ `Anime18Player`) — each **inline their own episode-selector grid**. There is **no shared episode-selector component**. They share a base layout (`flex flex-wrap gap-2 … custom-scrollbar`) and selected style (`accent-bg text-white`), but diverge on watched-state:

- **Watched-episode highlighting exists only on Kodik + AnimeLib.** They fetch `userApi.getWatchlistEntry(animeId).episodes` (a watched **count**), then `isEpisodeWatched(n) = n <= count` → muted-accent button + checkmark badge.
  - Kodik: `KodikPlayer.vue:800-818`, badge `:111-118`.
  - AnimeLib: `AnimeLibPlayer.vue:854-863`, `:807-809`.
- **OurEnglish, Hanime, Raw fetch no watched-state at all** → episodes never highlight. This is the user's report: *"on kodik watched episodes are blue and on english they are not."*

The `accent-*` classes are **not global utilities** — each player redefines `.accent-bg/.accent-text/.accent-border/.accent-bg-muted` in its own scoped style from `--player-accent` + `--player-accent-rgb` (e.g. `HanimePlayer.vue:479-485`, `Anime18Player.vue:443-450`). CSS custom properties **inherit** into child components; scoped class definitions do **not**.

Solo (`Anime.vue:528-584`) and Watch Together (`WatchTogetherView.vue:477-510`) mount the **same** player SFCs. What differs:
- Solo shows language tabs + provider sub-tabs (`Anime.vue:355-484`); WT replaces them with `PlayerTabBar` (`WatchTogetherView.vue:452-457`).
- Solo passes `animeName`/`totalEpisodes`/`preferredCombo`; WT passes `room` + `initialEpisode` only.

The user wants: (a) **one shared episode selector** used everywhere, watched-state on every player; (b) **Watch Together to reuse the solo watch interface wholesale**; (c) a new **live-presence layer** (remote cursors + hover highlights, colored per user).

## 2. Goals / Non-Goals

**Goals**
- One `EpisodeSelector.vue` used by every player, in solo and WT, with watched-state highlighting on all players in **each player's own accent color**.
- One `useWatchedEpisodes(animeId)` composable as the single watched-count source.
- One room-aware `WatchSurface.vue` mounted by both `Anime.vue` (solo) and `WatchTogetherView.vue` (WT), replacing `PlayerTabBar`.
- A WT-only live-presence layer: remote cursors (resolution-independent) + hover highlights on controls, each colored by the median color of the user's avatar.
- An explicit, codified **sync boundary**.

**Non-Goals**
- No change to upstream stream resolution, provider failover, or the playback-sync drift engine (already shipped).
- No syncing of subtitles, watched-state, volume, quality, fullscreen, or PiP (these stay per-user — see §3).
- No persistence of cursor/hover presence to Redis room state (ephemeral broadcast only).
- No change to who can create rooms / room lifecycle.

## 3. Sync Boundary (codified)

| Category | Items | Behavior |
|---|---|---|
| **Synced** (room state, all members) | active player, episode, translation, playback (play/pause/seek + drift correction) | Authoritative room state; broadcast via `room:state_changed` / `playback:*`. Already implemented; Phase B routes the player/episode/translation switchers through the room when `room != null`. |
| **Individual** (per-user, never synced) | **subtitles** (track choice + rendering via `SubtitleOverlay`/`OtherSubsPanel`), watched-episode highlighting, volume, quality, fullscreen, PiP | Local component state only. Phase B must NOT add any of these to room sync. |
| **Presence** (ephemeral broadcast, not room state) | cursor position, hover target | Fan-out only, rate-limited, dropped on disconnect. Phase C. |

## 4. Architecture Overview

Three phases, each independently shippable:

- **Phase A — Shared episode selector** (`EpisodeSelector.vue` + `useWatchedEpisodes`). Resolves the reported complaint. Low risk.
- **Phase B — Shared `WatchSurface.vue`** (room-aware). The "WT reuses solo wholesale" piece. Touches the two most complex views.
- **Phase C — Live presence layer** (cursors + hover, avatar colors). New WT-only real-time feature; new WS message types.

Phase A is a prerequisite for clean Phase B; Phase C depends on Phase B (the WatchSurface container is the presence coordinate frame).

## 5. Phase A — `EpisodeSelector.vue` + `useWatchedEpisodes`

### 5.1 `EpisodeSelector.vue` (`src/components/player/EpisodeSelector.vue`)

A self-contained, presentational episode grid.

**Props**
```ts
interface EpisodeOption {
  key: string | number   // unique; used for v-for key + selection match + data-wt-id
  label: string | number // displayed text (episode number / ordinal)
  number: number         // ordinal used for watched comparison (n <= watchedUpTo)
  isFiller?: boolean      // optional dim styling
}
defineProps<{
  episodes: EpisodeOption[]
  selectedKey: string | number | null
  watchedUpTo?: number   // default 0
}>()
```
**Emits:** `(e: 'select', key: string | number)`.

**Styling.** The component **defines its own** `.accent-bg/.accent-text/.accent-border/.accent-bg-muted` reading the **inherited** `--player-accent` + `--player-accent-rgb` (provided by the host player). States:
- selected → `accent-bg text-white`
- watched & not selected → `accent-bg-muted accent-text border accent-border` + checkmark badge (top-right, mirrors `KodikPlayer.vue:111-118`)
- default → `bg-white/10 text-white hover:bg-white/20`

Layout reuses the existing `flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1`. Each button carries `:data-wt-id="\`episode:${key}\`"` (consumed by Phase C; inert otherwise). Design-system lint must stay green (brand accents are exempt; no off-palette literals).

Co-located `EpisodeSelector.spec.ts` (≥5 assertions: renders N buttons, selected class, watched class + badge ≤ watchedUpTo, no watched class > watchedUpTo, emits `select` with key).

### 5.2 `useWatchedEpisodes(animeId)` (`src/composables/useWatchedEpisodes.ts`)

```ts
function useWatchedEpisodes(animeId: MaybeRefOrGetter<string>): {
  watchedUpTo: Ref<number>      // 0 when unauthenticated or none
  refresh: () => Promise<void>
}
```
Fetches `userApi.getWatchlistEntry(animeId)` → `entry.episodes` when `authStore.isAuthenticated`; otherwise stays 0. Players call `refresh()` on mount, on episode change, and after auto/manual mark-watched. Replaces the duplicated Kodik/AnimeLib fetch and adds the capability to OurEnglish/Hanime/Raw. **Per-user; never synced.**

### 5.3 Per-player wiring

For each player (Kodik, AnimeLib, OurEnglish, Hanime, Raw, Anime18):
1. Build a normalized `EpisodeOption[]` from the player's native episode model:
   - Kodik: numeric range → `{key:n, label:n, number:n}`.
   - AnimeLib/OurEnglish/Raw: `{key: ep.id, label: ep.number, number: Number(ep.number)}`.
   - Hanime: `{key: ep.slug, label: idx+1, number: idx+1}`.
2. Replace the inline grid with `<EpisodeSelector :episodes :selected-key :watched-up-to @select>`.
3. Use `useWatchedEpisodes(animeId)` for `watchedUpTo`.
4. Ensure the player declares **both** `--player-accent` and `--player-accent-rgb` on its root (add `-rgb` where missing — OurEnglish `:574` and Raw `:447` currently hardcode cyan).

**Verification:** vitest specs; in-browser smoke at desktop + mobile per player confirming watched-blue appears on OurEnglish/Hanime/Raw and the existing Kodik/AnimeLib look is unchanged (DS-NF-06 standing rule — jsdom can't catch Tailwind v4 cascade bugs).

## 6. Phase B — `WatchSurface.vue` (room-aware)

### 6.1 Component (`src/components/watch/WatchSurface.vue`)

Extracts from `Anime.vue`: the language/provider tab UI (`:355-484`), the player `v-if` dispatch (`:528-584`), and the `ResumePill` header slot.

**Props**
```ts
defineProps<{
  anime: { id: string; title: string; totalEpisodes: number; isHentai: boolean; /* … */ }
  room?: WatchTogetherRoomHandle | null   // null/undefined ⇒ solo
  resumeStartEpisode?: number
  resumePillProps?: ResumePillProps
}>()
```
Owns `videoLanguage` / `videoProvider`, feature-flag gating, `handleAvailableTranslations`, `resolvedCombo` passthrough. Emits the events `Anime.vue` still needs (e.g. translation availability, list-status changes) so the surrounding page logic is unchanged.

**Room-aware behavior**
- `room == null` (solo): tabs are interactive; selecting a provider sets local `videoProvider`; episode/translation handled inside the active player locally. `Anime.vue` mounts `<WatchSurface :anime :resume… />`.
- `room != null` (WT): the active provider is `room.player` (the existing `livePlayer`); clicking a provider tab calls `room.emitChangePlayer(kind)` instead of mutating local state; `:room` is passed to the active player so its existing sync wiring drives episode/translation/playback. `WatchTogetherView.vue` mounts `<WatchSurface :anime :room="roomHandle" />` inside its existing sidebar/chat/reactions chrome, **replacing `PlayerTabBar`** (the language/provider tabs now serve that role; `hiddenPlayerKinds` logic for AniLib carries over).

The provider/language tab block becomes a small internal sub-unit (`ProviderTabs`) so the room-vs-local click handler is the only branch.

### 6.2 Migration

- `Anime.vue`: replace `:355-584` with `<WatchSurface>`, keep the click-to-load placeholder + not-released notice outside it (page concerns).
- `WatchTogetherView.vue`: replace the player dispatch (`:477-510`) + `PlayerTabBar` with `<WatchSurface :room>`.
- Keep `WatchTogetherView`'s WT-only chrome (RoomSidebar, ChatPanel, ReactionBurstOverlay, ConnectionStatusOverlay, guest banner) untouched.

**Verification:** in-browser smoke of solo (all providers switch, resume pill, episodes) and WT (provider/episode/translation switching still works post-ISS-024/025; sync across two members), desktop + mobile. Confirm subtitles remain local (not synced).

## 7. Phase C — Live Presence Layer (WT-only)

Layered on the `WatchSurface` container, which is the shared **coordinate frame** (cursors + hover normalized to its bounds — resolution-independent).

### 7.1 WS protocol (`services/watch-together/internal/domain/ws_message.go`)

New **ephemeral** message types (NOT persisted to Redis room state; fan-out only, sender excluded):

| Direction | Type | Payload |
|---|---|---|
| inbound | `presence:cursor` | `{ x: float32, y: float32 }` (0..1, normalized to WatchSurface) |
| inbound | `presence:hover` | `{ element_id: string }` (empty string ⇒ stopped hovering) |
| outbound | `presence:cursor` | `{ user_id, x, y }` |
| outbound | `presence:hover` | `{ user_id, element_id }` |

- High frequency → **in-process per-user rate limits** (reuse the `golang.org/x/time/rate` token-bucket pattern already in `internal/service/inbound.go`): cursor ≈ 20/s, hover ≈ 10/s. Excess silently dropped.
- Not written to `wt:` Redis keys; not part of `RoomSnapshot` (matches reactions' ephemeral model).
- On member disconnect/leave, the existing leave event drives client-side cursor removal; clients also stale-timeout a cursor after ~5 s of silence.
- `protocol_version` stays `"1.0"` (additive, forward-compatible — unknown types are already ignored by older clients).

### 7.2 Frontend

**`usePresenceLayer(room, surfaceRef)`** (`src/composables/`):
- Local: on `mousemove` over `surfaceRef`, compute `x=(clientX-rect.left)/rect.width`, `y=(clientY-rect.top)/rect.height`, clamp 0..1, throttle to ~20/s (rAF + last-sent diff), emit `presence:cursor`. On `mouseleave`, emit a final "gone" (empty) and stop.
- Hover: event-delegated `mouseover`/`mouseout` on elements with `data-wt-id` inside `surfaceRef` → emit `presence:hover` with the id (empty on out).
- Remote: subscribe to `presence:cursor`/`presence:hover`; maintain reactive `Map<userId, { x, y, elementId, lastSeen }>`.

**`PresenceOverlay.vue`** (`src/components/watch-together/`): absolutely-positioned layer filling `WatchSurface`. For each remote user: render a cursor at `x*width, y*height` with the user's color + short name label; and, for the hovered `elementId`, draw a colored ring around the element whose `data-wt-id` matches (position via that element's `getBoundingClientRect` relative to the surface). `pointer-events: none` so it never intercepts input.

`data-wt-id` is added to highlightable controls inside `WatchSurface`: episode buttons (`episode:<key>`, from Phase A), provider/language tabs (`player:<kind>`), translation buttons (`translation:<id>`), and primary playback controls where applicable.

**`useAvatarColor(member)`** (`src/composables/`): returns the user's presence color = **median color of their avatar**.
- Primary: load avatar with `crossOrigin='anonymous'`, draw to a 16×16 offscreen canvas, `getImageData`, take the per-channel median (robust to outliers/transparent edges) → `rgb()`.
- Fallback (CORS-tainted canvas, missing avatar, or load error): deterministic hash of `user_id` → HSL hue (stable, distinct per user).
- Memoized per `user_id`. Computed locally on every client → identical result everywhere without syncing color.
- **Dependency:** the room member payload must expose the avatar URL (and a display name). If absent, add it to the member projection; the hash fallback guarantees a color regardless. Guests (no avatar) always use the hash fallback.

### 7.3 Performance & abuse
- Cursor/hover are throttled client-side and rate-limited server-side; never persisted; capped per-user. A flooding client is bounded by the token bucket. Overlay updates are reactive-map driven and `pointer-events:none`.

**Verification:** two-browser manual test — cursors track with correct per-user colors and remain aligned across **different window sizes** (the normalization requirement); hovering an episode/tab button highlights it in the hoverer's color on the other screen; cursors disappear on leave. Confirm no main-thread jank at 20/s.

## 8. Testing Strategy

- **Phase A:** vitest (`EpisodeSelector.spec.ts`, `useWatchedEpisodes.spec.ts`); per-player in-browser smoke (watched-blue everywhere; Kodik/AnimeLib unchanged); design-system lint green.
- **Phase B:** in-browser smoke solo + WT; WT sync regression (episode/player/translation — guards ISS-024/025); subtitles confirmed local; `vue-tsc --noEmit` (import shared SFC types from the `@/components/ui` barrel per the known false-pass gotcha).
- **Phase C:** Go unit tests for the two new handlers (validation + rate-limit + fan-out-excludes-sender) using the `miniredis` + fake-hub pattern; frontend vitest for `usePresenceLayer` (normalization math) + `useAvatarColor` (median + hash fallback); two-browser manual cross-resolution test.
- Each phase deploys via `/animeenigma-after-update` (batched per phase, not per file).

## 9. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Phase B touches the app's most complex view (`Anime.vue`) + WT | Phase A ships value first; extract incrementally; in-browser smoke at each step; keep page-level concerns (placeholder, not-released) in `Anime.vue`. |
| Tailwind v4 unlayered-class cascade bug (memory) | `EpisodeSelector` owns its accent classes; verify in real browser, not jsdom. |
| Avatar canvas CORS taint | Deterministic hash fallback guarantees a color; primary path used only when canvas is readable. |
| Cursor spam / jank | Client throttle ~20/s + server token-bucket; ephemeral (no Redis); `pointer-events:none` overlay. |
| Accidentally syncing individual state (subtitles) | §3 sync boundary is explicit; Phase B review checks no subtitle/volume/quality emits are added to the room. |

## 10. Decisions Made

- Scope = **WT reuses solo wholesale** (shared `WatchSurface`).
- Watched-highlight color = **each player's own accent** (via inherited `--player-accent`).
- Presence coordinate frame = **whole WatchSurface**, normalized 0..1.
- User color = **median color of avatar**, computed client-side, hash fallback.
- Synced = playback/episode/player/translation; **subtitles + watched-state are individual**; cursor/hover are ephemeral presence (not room state).

## 11. Open Questions (resolve during planning, non-blocking)

- Does the current WT member payload already carry avatar URL + display name? If not, add to the member projection (Phase C dependency).
- Confirm `data-wt-id` coverage list for highlightable controls (episodes + tabs are in; decide whether to include the big play/pause + the report button).
