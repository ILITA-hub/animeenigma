# Phase 4: State Switching - Context

**Gathered:** 2026-05-25
**Status:** Ready for planning
**Mode:** Auto-generated (discuss skipped via workflow.skip_discuss)
**Workstream:** watch-together (v1.0)

<domain>
## Phase Boundary

Episode/player/translation switching propagates to all room members. After Phase 4: clicking next-episode in browser A causes browser B's player to switch to the new episode paused at 0. Same for player swap (Kodik → AniLib) and translation swap (Crunchyroll → AniRise).

**Requirements covered:** WT-STATE-01..05.

</domain>

<decisions>
## Implementation Decisions (Locked)

### Backend — `services/watch-together/`

#### Inbound message handlers (extend existing `internal/service/inbound.go` from 01.6)
Phase 1 shipped pass-through stubs for `state:change_episode|change_player|change_translation`. Phase 4 swaps them for validated handlers:

1. `state:change_episode {episode_id}`:
   - Look up the current room state (anime_id, player, translation_id)
   - Call catalog: `GET /internal/anime/{anime_id}/episodes?player={player}&translation={translation_id}` — verify episode_id exists in the returned list
   - If valid: update Redis `room.episode_id`, `playback_state="paused"`, `playback_time=0`, `playback_time_updated_at=now`; broadcast `room:state_changed` to ALL members
   - If invalid: send `error: {code: 'EPISODE_UNAVAILABLE'}` to sender only

2. `state:change_player {player}`:
   - Validate that player is one of the 5 valid players (kodik, animelib, ourenglish, hanime, raw)
   - Call catalog: verify the anime has at least one episode on the requested player (`GET /internal/anime/{id}/episodes?player={new_player}` returns non-empty)
   - If valid: update Redis (player, episode_id reset to first episode, translation_id reset to "" since translation is player-specific, playback_state/time reset), broadcast `room:state_changed`
   - If invalid: send `error: {code: 'PLAYER_UNAVAILABLE'}`

3. `state:change_translation {translation_id}`:
   - Look up current state (anime_id, player, episode_id)
   - Call catalog: `GET /internal/anime/{anime_id}/episodes?player={player}&translation={translation_id}` returns the specific episode
   - If valid: update Redis (translation_id, playback_state/time reset), broadcast `room:state_changed`
   - If invalid: send `error: {code: 'TRANSLATION_UNAVAILABLE'}`

#### Catalog HTTP client — `services/watch-together/internal/service/state.go` (NEW)
- HTTP client with retry + timeout (3s)
- Reads `CATALOG_URL` from config (default `http://catalog:8081`)
- Returns typed episode list response

#### Catalog endpoint
Verify `services/catalog/internal/handler/internal_episodes.go` (or equivalent) exists with the right query params. If a new endpoint or param is needed, extend it in this phase (one new param max).

### Frontend — `frontend/web/`

#### `useWatchTogetherRoom.ts` extension
Phase 2's composable already exposes `emitChangeEpisode/emitChangePlayer/emitChangeTranslation` + `onStateChanged`. Phase 4 uses these:
- Subscribe to `onStateChanged` in `WatchTogetherView.vue`
- When the event fires: swap the active player or episode in the view's reactive state

#### `WatchTogetherView.vue` integration
- Subscribe to `room.onStateChanged` on mount
- Handler:
  - If `room.player` changed: re-mount the player (use a `:key` binding that includes player)
  - If `room.episode_id` changed: pass updated prop to the active player
  - If `room.translation_id` changed: pass updated prop to the active player
- The existing 5-way player dispatch from Phase 2 still applies; this phase just wires the room handle to drive prop values

#### Per-player switcher routing
Each player component has internal switchers (episode dropdown, translation dropdown — for the players that have multi-track support). When the user clicks "Next episode" or selects a translation:
- If in a room context (`props.room` is set): instead of mutating local state, call `room.emitChangeEpisode(episodeId)` etc. — the backend validates + broadcasts → the view's `onStateChanged` handler updates the prop
- If NOT in a room: existing behavior (local state mutation)

The simplest path is a single conditional in each switcher: `if (props.room) room.emitChange...; else localState = ...;`

#### Error handling UX
When user receives `EPISODE_UNAVAILABLE` / `PLAYER_UNAVAILABLE` / `TRANSLATION_UNAVAILABLE` error (sender-only):
- Show toast with `t('watch_together.episode_unavailable')` etc.
- Don't change local state
- Other members see nothing

### Claude's Discretion
- Whether to add a new catalog endpoint or extend an existing one — make the smallest change
- Validation cache: catalog calls can be cached briefly (e.g., 5s) per (anime, player, translation) tuple to avoid hammering on rapid switches
- Drift correction interaction: after a state change, the playback time is reset to 0 — drift correction should not fire spurious corrections during the brief transition window

</decisions>

<canonical_refs>
## Canonical References

### Source design + requirements
- `docs/superpowers/specs/2026-05-25-watch-together-design.md` (state change protocol section)
- `.planning/workstreams/watch-together/REQUIREMENTS.md` (WT-STATE-01..05)

### Prior phase outputs
- `.planning/workstreams/watch-together/phases/01-backend-foundation/01-PHASE-SUMMARY.md` (backend protocol)
- `.planning/workstreams/watch-together/phases/02-frontend-shell/02-PHASE-SUMMARY.md` (composable + view)
- `.planning/workstreams/watch-together/phases/03-player-sync/03-PHASE-SUMMARY.md` (sync UX layer)

### Backend code anchors
- `services/watch-together/internal/service/inbound.go` (from 01.6 — has stub handlers for state:change_*)
- `services/watch-together/internal/repo/redis_repo.go` (UpdateRoomState exists)
- `services/watch-together/internal/hub/hub.go` (Broadcast)
- `services/catalog/internal/handler/` (find existing `/internal/anime/{id}/episodes` endpoint)
- `services/catalog/internal/service/` (the underlying service for episode lookup)

### Frontend code anchors
- `frontend/web/src/views/WatchTogetherView.vue` (mount onStateChanged here)
- `frontend/web/src/composables/useWatchTogetherRoom.ts` (emitChange* and onStateChanged already exist)
- `frontend/web/src/components/player/{Kodik,AnimeLib,OurEnglish,Hanime,Raw}Player.vue` (each has internal switchers — route through room.emitChange* when in a room)

</canonical_refs>

<specifics>
## Specific Ideas

### Catalog endpoint contract (target shape)
```
GET /internal/anime/{anime_id}/episodes?player={player}&translation={translation_id}
Returns: [{episode_id: "1", title: "...", available: true}, ...]
```

If the existing endpoint doesn't accept `?translation=`, extend it.

### Optimistic vs validated update
For Phase 4: **validated** only. The frontend doesn't show the change until the backend confirms via room:state_changed. This avoids inconsistent state across members.

### Test scenarios
1. Host clicks next-episode → both browsers swap to new episode paused at 0
2. Host switches player → both browsers swap player, resume paused at 0
3. Host switches translation → both reload the source
4. Host tries to switch to non-existent episode → sender sees EPISODE_UNAVAILABLE; others unaffected
5. Drift correction re-stabilizes within 5s of both starting playback

</specifics>

<deferred>
## Deferred Ideas

- Optimistic local update + rollback on error (Phase 5 polish?)
- Polish animations on player swap (Phase 5)
- Mobile UX for switching (Phase 5)

</deferred>

---

*Phase: 04-state-switching*
*Context auto-generated: 2026-05-25 via workflow.skip_discuss*
