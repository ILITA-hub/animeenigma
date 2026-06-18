# aePlayer in WatchTogether — Implementation Plan (Plan A)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make aePlayer a fully synced WatchTogether player so co-watch rooms can run on aePlayer (its time/play/seek AND its source combo converge across members), unblocking the legacy-player deletions in Plan B.

**Architecture:** aePlayer is HTML5 (`<video>` + hls.js via `useVideoEngine`), so playback time/play/pause/seek sync reuses the existing `usePlayerSyncBridge(videoRef, room)` one-liner that the four legacy HTML5 players already use — nearly free. The genuinely new work is aePlayer's **source combo** (`provider/audio/lang/team/server`): in a room, all members must resolve the **same** stream, because mismatched providers have different encodes/intros and make drift-correction meaningless. We carry the combo in the room's existing opaque `translation_id` field (no new message types or schema), pin aePlayer to the room combo (disabling its auto smart-default reselection while in a room), and let any member change it (broadcast), mirroring WT's "any member drives playback" model. Plan A is **add-only**: it adds `aeplayer` as a 6th WT player alongside the legacy ones; removing the legacy WT branches is Plan B.

**Tech Stack:** Go (services/watch-together, Redis-only), Vue 3 + TS (UnifiedPlayer.vue, WatchTogetherView.vue, composables), vitest, vue-tsc.

---

## Design decisions (technical, fold-in — strategic decisions live in the scope doc `2026-06-18-player-surface-retirement-scope-design.md`)

- **DD1 — Combo carrier = `translation_id` (opaque).** When `room.player == "aeplayer"`, `translation_id` holds the JSON-serialized combo `{provider,audio,lang,team,server}`. `translation_id` is already opaque + permissively validated, so zero new message types / snapshot schema. A room is either aeplayer OR a legacy player, so there's no collision with legacy translation ids.
- **DD2 — Pinned mode.** When `props.room` is set, aePlayer suppresses its auto smart-default (Stage 1a) and saved-combo (Stage 1b) reselection — the room combo is authoritative. A local user may still change the source; that change broadcasts to the room.
- **DD3 — Reuse the HTML5 bridge** for time/play/pause/seek: `if (props.room) usePlayerSyncBridge(videoRef, props.room)`.
- **DD4 — Reuse existing emits.** Combo → `room.emitChangeTranslation(token)`; episode → `room.emitChangeEpisode(String(epNumber))`; player kind → `room.emitChangePlayer('aeplayer')`. All three already exist on the room handle (`useWatchTogetherRoom.ts:120-122`).
- **DD5 — Add-only / non-breaking.** Legacy WT branches stay; Plan A just adds the `aeplayer` branch. Independently shippable.

## File structure

- `services/watch-together/internal/domain/ws_message.go` — add `PlayerAePlayer` constant.
- `services/watch-together/internal/service/rooms.go` — add `aeplayer` to `allowedPlayers`.
- `services/watch-together/internal/service/inbound.go` — add `aeplayer` to `validPlayers`; permissive episode validation for `aeplayer`.
- `frontend/web/src/types/watch-together.ts` — add `'aeplayer'` to `PlayerKind`.
- `frontend/web/src/composables/unifiedPlayer/comboMapping.ts` — add `comboToToken` / `tokenToCombo`.
- `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — `room` prop, bridge wiring, pinned mode, combo + episode broadcast/apply.
- `frontend/web/src/views/WatchTogetherView.vue` — add `aeplayer` branch.
- `frontend/web/src/locales/{en,ru,ja}.json` — `player.unified.tab` already exists; add any new room-label key in all three.
- Tests co-located per existing convention.

---

### Task 1: Backend — register `aeplayer` as a known player

**Files:**
- Modify: `services/watch-together/internal/domain/ws_message.go:80-86`
- Modify: `services/watch-together/internal/service/rooms.go:50-55`
- Modify: `services/watch-together/internal/service/inbound.go` (the `validPlayers` set near :458)
- Test: `services/watch-together/internal/service/rooms_test.go`, `services/watch-together/internal/service/inbound_test.go`

- [ ] **Step 1: Write the failing test (room creation accepts aeplayer)**

In `rooms_test.go`, add to the existing validation test table a case asserting `ValidateCreate`/`CreateRoom` accepts `Player: "aeplayer"`:

```go
func TestCreateRoom_AcceptsAePlayer(t *testing.T) {
	in := CreateRoomInput{AnimeID: "123", EpisodeID: "1", Player: "aeplayer", TranslationID: ""}
	if err := validateCreate(in); err != nil { // use whatever the existing validator entrypoint is named
		t.Fatalf("aeplayer must be an allowed player, got: %v", err)
	}
}
```

- [ ] **Step 2: Run it to confirm it fails**

Run: `cd services/watch-together && go test ./internal/service/ -run TestCreateRoom_AcceptsAePlayer`
Expected: FAIL (`unknown player "aeplayer"`).

- [ ] **Step 3: Add the constant + allow it**

In `ws_message.go`, extend the player union:

```go
const (
	PlayerKodik      = "kodik"
	PlayerAnimeLib   = "animelib"
	PlayerOurEnglish = "ourenglish"
	PlayerHanime     = "hanime"
	PlayerRaw        = "raw"
	PlayerAePlayer   = "aeplayer" // first-party AnimeEnigma unified player (multi-source)
)
```

In `rooms.go`, add to `allowedPlayers`:

```go
	domain.PlayerAePlayer: {},
```

In `inbound.go`, add `domain.PlayerAePlayer` to the `validPlayers` set used by `handleChangePlayer`. Update the `(allowed: kodik|animelib|ourenglish|hanime|raw)` error string in `rooms.go:81` to include `aeplayer`.

- [ ] **Step 4: Run the test to confirm it passes**

Run: `cd services/watch-together && go test ./internal/service/ -run TestCreateRoom_AcceptsAePlayer`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit services/watch-together/internal/domain/ws_message.go \
  services/watch-together/internal/service/rooms.go \
  services/watch-together/internal/service/inbound.go \
  services/watch-together/internal/service/rooms_test.go \
  -m "feat(wt): register aeplayer as an allowed WatchTogether player"
```

---

### Task 2: Backend — permissive episode validation for aeplayer

**Files:**
- Modify: `services/watch-together/internal/service/inbound.go` (the `ValidateEpisode` call path, ~:436-460)
- Modify: `services/watch-together/internal/service/catalog_client.go` (if the permissive list lives there)
- Test: `services/watch-together/internal/service/inbound_test.go`

**Context:** The WT reference says validation is "permissive for ourenglish/hanime/raw" — those skip/loosen the catalog `ValidateEpisode` round-trip. aePlayer's episode set spans multiple providers, so a strict per-provider validate would be wrong. Treat `aeplayer` the same as the permissive set: accept the episode/combo from the driving member without a catalog round-trip (v1; tighten later via aePlayer's own `/capabilities` if desired).

- [ ] **Step 1: Write the failing test**

```go
func TestValidateEpisode_AePlayerIsPermissive(t *testing.T) {
	// With player=aeplayer, validation must NOT reject on an unknown catalog
	// episode id (no round-trip / always-permit), mirroring ourenglish.
	r := newInboundTestRig(t) // use the existing test rig/fake catalog
	res, err := r.svc.validateForChange(ctx, "anime1", "aeplayer", "9999", "<combo-json>", "sub")
	if err != nil || !res.OK {
		t.Fatalf("aeplayer episode validation must be permissive, got ok=%v err=%v", res.OK, err)
	}
}
```

(Adapt to the actual permissive-check entrypoint — find where `ourenglish` is special-cased and add `aeplayer` to that branch.)

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd services/watch-together && go test ./internal/service/ -run TestValidateEpisode_AePlayerIsPermissive`

- [ ] **Step 3: Add aeplayer to the permissive branch**

Find the special-case (grep `PlayerOurEnglish` in `inbound.go`/`catalog_client.go`) and include `domain.PlayerAePlayer` wherever `ourenglish`/`hanime`/`raw` skip or soften the catalog `ValidateEpisode` round-trip.

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit services/watch-together/internal/service/inbound.go \
  services/watch-together/internal/service/catalog_client.go \
  services/watch-together/internal/service/inbound_test.go \
  -m "feat(wt): permissive episode validation for aeplayer (multi-source)"
```

---

### Task 3: Frontend types — add `aeplayer` to `PlayerKind`

**Files:**
- Modify: `frontend/web/src/types/watch-together.ts`
- Test: type-check only (`bunx vue-tsc --noEmit`)

- [ ] **Step 1: Add the union member**

In `watch-together.ts`, find `export type PlayerKind = 'kodik' | 'animelib' | 'ourenglish' | 'hanime' | 'raw'` and add `| 'aeplayer'`.

- [ ] **Step 2: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: clean (no consumer break — it's a union widening).

- [ ] **Step 3: Commit**

```bash
git commit frontend/web/src/types/watch-together.ts \
  -m "feat(wt): add 'aeplayer' to PlayerKind union"
```

---

### Task 4: comboMapping — serialize/deserialize the combo token

**Files:**
- Modify: `frontend/web/src/composables/unifiedPlayer/comboMapping.ts`
- Test: `frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts`

**Context:** `state.combo.value` is `{ audio: AudioKind; lang: TrackLang; team: string | null; provider: string; server: string }`. The token is the JSON of exactly these 5 fields — compact, robust to delimiters in `team`, and opaque on the wire (DD1).

- [ ] **Step 1: Write the failing round-trip test**

```ts
import { comboToToken, tokenToCombo } from './comboMapping'

describe('combo <-> WT token', () => {
  it('round-trips a full combo', () => {
    const combo = { audio: 'sub', lang: 'en', team: 'SubsPlease', provider: 'allanime', server: 'wixmp' } as const
    expect(tokenToCombo(comboToToken(combo))).toEqual(combo)
  })
  it('round-trips a null team', () => {
    const combo = { audio: 'dub', lang: 'ru', team: null, provider: 'kodik', server: '' } as const
    expect(tokenToCombo(comboToToken(combo))).toEqual(combo)
  })
  it('tokenToCombo returns null on garbage', () => {
    expect(tokenToCombo('not-json')).toBeNull()
  })
})
```

- [ ] **Step 2: Run it — expect FAIL** (`comboToToken is not a function`)

Run: `cd frontend/web && bunx vitest run src/composables/unifiedPlayer/comboMapping.spec.ts`

- [ ] **Step 3: Implement**

```ts
// WT source-combo token: opaque JSON carried in room.translation_id (DD1).
// Exactly the 5 combo fields so all room members resolve the SAME stream.
export interface WtComboFields {
  audio: AudioKind
  lang: TrackLang
  team: string | null
  provider: string
  server: string
}

export function comboToToken(c: WtComboFields): string {
  return JSON.stringify({ provider: c.provider, audio: c.audio, lang: c.lang, team: c.team ?? null, server: c.server })
}

export function tokenToCombo(token: string): WtComboFields | null {
  try {
    const o = JSON.parse(token)
    if (!o || typeof o !== 'object' || typeof o.provider !== 'string') return null
    return {
      provider: o.provider,
      audio: o.audio,
      lang: o.lang,
      team: o.team ?? null,
      server: typeof o.server === 'string' ? o.server : '',
    }
  } catch {
    return null
  }
}
```

(Import `AudioKind`/`TrackLang` from wherever the `Combo` type is declared — same module the resolver uses.)

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/composables/unifiedPlayer/comboMapping.ts \
  frontend/web/src/composables/unifiedPlayer/comboMapping.spec.ts \
  -m "feat(player): WT combo token serialize/deserialize (comboToToken/tokenToCombo)"
```

---

### Task 5: aePlayer — accept a `room` prop and bridge playback time/play/seek

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (props ~:369, setup body)
- Test: `frontend/web/src/components/player/unified/__tests__/UnifiedPlayer.room.spec.ts` (new)

- [ ] **Step 1: Write the failing test (bridge wired only when room present)**

Mount UnifiedPlayer with a fake `room` handle (stub of `WatchTogetherRoomHandle` with vi.fn emits + on* subscribers) and assert that a native `play` on the exposed `videoRef` calls `room.emitPlay`. Mount WITHOUT a room and assert no emit. Use the same mocking style as `OurEnglishPlayer.spec.ts`.

```ts
it('bridges playback to the room when room prop is set', async () => {
  const room = makeFakeRoom()
  const wrapper = mountUnified({ room })
  const video = wrapper.find('video').element as HTMLVideoElement
  video.dispatchEvent(new Event('play'))
  await nextTick()
  expect(room.emitPlay).toHaveBeenCalled()
})
```

- [ ] **Step 2: Run it — expect FAIL**

Run: `cd frontend/web && bunx vitest run src/components/player/unified/__tests__/UnifiedPlayer.room.spec.ts`

- [ ] **Step 3: Add the prop + bridge**

In UnifiedPlayer.vue `defineProps`, add:

```ts
  /** Watch-together room handle. When set, aePlayer runs in synced "room" mode. */
  room?: import('@/composables/useWatchTogetherRoom').WatchTogetherRoomHandle | null
```

In the setup body, after `const videoRef = ref(...)` and `const engine = useVideoEngine(videoRef)`:

```ts
import { usePlayerSyncBridge } from '@/composables/usePlayerSyncBridge'
// ...
if (props.room) usePlayerSyncBridge(videoRef, props.room) // DD3 — same HTML5 path as legacy players
const roomPinned = computed(() => !!props.room) // DD2 gate
```

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/player/unified/UnifiedPlayer.vue \
  frontend/web/src/components/player/unified/__tests__/UnifiedPlayer.room.spec.ts \
  -m "feat(player): aePlayer accepts a WT room prop, bridges HTML5 playback sync"
```

---

### Task 6: aePlayer — pin to room combo (suppress auto-reselect), apply remote combo

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue` (Stage-1a/1b auto-select watchers; combo apply)
- Test: extend `UnifiedPlayer.room.spec.ts`

**Context:** Stage 1a smart-default (~UnifiedPlayer.vue:521-530) and Stage 1b saved-combo (~:460-483) auto-pick a provider on mount. In room mode they must NOT run (the room combo wins, DD2).

- [ ] **Step 1: Write the failing test (remote combo is applied; auto-select suppressed)**

```ts
it('applies the room combo and does not auto-reselect', async () => {
  const room = makeFakeRoom({ player: 'aeplayer', translation_id: JSON.stringify({ provider: 'allanime', audio: 'sub', lang: 'en', team: null, server: 'wixmp' }) })
  const wrapper = mountUnified({ room })
  await flushPromises()
  // the player's active combo provider must equal the room combo provider
  expect(wrapper.vm.activeProviderId ?? /* expose for test */ readCombo(wrapper).provider).toBe('allanime')
})
```

(If the combo isn't readable from the instance, add a minimal `defineExpose({ __combo: state.combo })` guarded for tests, or assert via the rendered SourcePanel active chip.)

- [ ] **Step 2: Run it — expect FAIL**

- [ ] **Step 3: Implement pin + apply**

Guard the two auto-select watchers with `if (roomPinned.value) return` at the top of their callbacks. Add an apply function + a watcher on the room's combo field:

```ts
import { tokenToCombo, comboToToken } from '@/composables/unifiedPlayer/comboMapping'
// ...
let applyingRoomCombo = false
function applyRoomCombo(token: string | undefined | null) {
  if (!token) return
  const c = tokenToCombo(token)
  if (!c) return
  applyingRoomCombo = true
  state.combo.value = { audio: c.audio, lang: c.lang, team: c.team, provider: c.provider, server: c.server }
  // resolver watcher reacts to combo change → re-resolves the stream
  nextTick(() => { applyingRoomCombo = false })
}

if (props.room) {
  // initial + reactive: room translation_id carries the combo
  watch(
    () => props.room?.room.value?.translation_id,
    (tid) => applyRoomCombo(tid),
    { immediate: true },
  )
}
```

(Confirm the room handle exposes the reactive room state as `room.value` — see `useWatchTogetherRoom.ts:365` which mutates `room.value.player`. Use the matching accessor.)

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/player/unified/UnifiedPlayer.vue \
  frontend/web/src/components/player/unified/__tests__/UnifiedPlayer.room.spec.ts \
  -m "feat(player): aePlayer pins to room combo, suppresses auto-reselect in WT"
```

---

### Task 7: aePlayer — broadcast local combo + episode changes to the room

**Files:**
- Modify: `frontend/web/src/components/player/unified/UnifiedPlayer.vue`
- Test: extend `UnifiedPlayer.room.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
it('broadcasts a local combo change to the room (not echoing remote ones)', async () => {
  const room = makeFakeRoom({ player: 'aeplayer' })
  const wrapper = mountUnified({ room })
  await flushPromises()
  // simulate a user picking a provider in SourcePanel:
  setCombo(wrapper, { provider: 'miruro', audio: 'sub', lang: 'en', team: null, server: 'kiwi' })
  await nextTick()
  expect(room.emitChangeTranslation).toHaveBeenCalledWith(
    JSON.stringify({ provider: 'miruro', audio: 'sub', lang: 'en', team: null, server: 'kiwi' }),
  )
})

it('broadcasts an episode change to the room', async () => {
  const room = makeFakeRoom({ player: 'aeplayer' })
  const wrapper = mountUnified({ room })
  await flushPromises()
  changeEpisode(wrapper, 4)
  await nextTick()
  expect(room.emitChangeEpisode).toHaveBeenCalledWith('4')
})
```

- [ ] **Step 2: Run it — expect FAIL**

- [ ] **Step 3: Implement broadcasts (echo-guarded)**

```ts
if (props.room) {
  // local combo change → broadcast, unless we're applying a remote one
  watch(
    () => state.combo.value,
    (c) => {
      if (applyingRoomCombo) return
      if (!c.provider) return // not yet resolved
      props.room!.emitChangeTranslation(comboToToken(c))
    },
    { deep: true },
  )
}
```

For episode: locate aePlayer's episode-change handler (the function that sets the current episode when the user picks one in EpisodeSelector) and, mirroring `OurEnglishPlayer.vue:550-551`, add — gated against room-originated changes (`fromRoomSync`):

```ts
if (props.room && !fromRoomSync) props.room.emitChangeEpisode(String(epNumber))
```

Also add a watcher on `props.room.room.value.episode_id` that switches the local episode when it changes remotely (set a `fromRoomSync` flag around it to prevent the echo), mirroring the OurEnglish pattern (`OurEnglishPlayer.vue:520`, `:648`).

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/components/player/unified/UnifiedPlayer.vue \
  frontend/web/src/components/player/unified/__tests__/UnifiedPlayer.room.spec.ts \
  -m "feat(player): aePlayer broadcasts combo + episode changes to the WT room"
```

---

### Task 8: WatchTogetherView — mount aePlayer for `aeplayer` rooms

**Files:**
- Modify: `frontend/web/src/views/WatchTogetherView.vue` (imports ~:86-91; template branch ~:482-517)
- Test: `frontend/web/src/views/__tests__/WatchTogetherView.aeplayer.spec.ts` (new), or extend an existing WT view test

- [ ] **Step 1: Write the failing test**

Assert that when `livePlayer === 'aeplayer'`, the view renders UnifiedPlayer (stubbed) with `:room` bound, and does NOT render a legacy player.

- [ ] **Step 2: Run it — expect FAIL**

- [ ] **Step 3: Add the import + branch**

Import alongside the others:

```ts
const UnifiedPlayer = defineAsyncComponent(() => import('@/components/player/unified/UnifiedPlayer.vue'))
```

Add a branch in the `livePlayer` chain (before the forward-compat empty-state `div`), passing the same props aePlayer needs in `Anime.vue` plus the room handle:

```vue
<UnifiedPlayer
  v-else-if="livePlayer === 'aeplayer'"
  :key="`player-${livePlayer}`"
  :anime-id="animeId"
  :anime="aeAnimeMeta"
  :theater="false"
  :is-hentai="isHentai"
  :initial-episode="initialEpisode"
  :mal-id="malId"
  :room="roomHandle"
/>
```

Provide `aeAnimeMeta` (the `{ title, ep, eps, still }` shape aePlayer requires) from the view's existing room/anime data; if `malId`/`isHentai` aren't already in the view, thread them from the room snapshot / anime fetch.

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/views/WatchTogetherView.vue \
  frontend/web/src/views/__tests__/WatchTogetherView.aeplayer.spec.ts \
  -m "feat(wt): mount aePlayer for aeplayer rooms in WatchTogetherView"
```

---

### Task 9: Room creation + in-room switch to aePlayer

**Files:**
- Modify: `frontend/web/src/views/Anime.vue` (the "watch together" create-room CTA — pass `player: 'aeplayer'` when aePlayer is the active surface)
- Modify: the in-room player-switch control (if WatchTogetherView exposes one) to offer aePlayer
- Test: unit test on the create-room payload builder

**Context:** A room gets `player` at creation. With aePlayer becoming the default surface (Plan B), creating a WT room from the anime page while aePlayer is active should create an `aeplayer` room seeded with the current combo as `translation_id`.

- [ ] **Step 1: Write the failing test**

Test the create-room payload builder: when the active surface is aePlayer with a resolved combo, the payload has `player: 'aeplayer'` and `translation_id: comboToToken(combo)` and `episode_id: String(currentEp)`.

- [ ] **Step 2: Run it — expect FAIL**

- [ ] **Step 3: Implement**

In the create-room flow, when aePlayer is the active surface, set `player='aeplayer'`, `translation_id=comboToToken(currentCombo)`, `episode_id=String(currentEp)`. For the in-room switch control, add an `aeplayer` option that emits `room.emitChangePlayer('aeplayer')` followed by an `emitChangeTranslation(comboToToken(currentCombo))` so joiners get a combo immediately.

- [ ] **Step 4: Run the test — expect PASS**

- [ ] **Step 5: Commit**

```bash
git commit frontend/web/src/views/Anime.vue frontend/web/src/views/WatchTogetherView.vue \
  <test-file> \
  -m "feat(wt): create + switch to aeplayer rooms seeded with the current combo"
```

---

### Task 10: i18n + final verification

**Files:**
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (any new room-switch label, e.g. `watchTogether.player.aeplayer`) — add to ALL THREE (locale-parity spec + i18n-lint enforce this).
- No code change beyond locales.

- [ ] **Step 1: Add any new i18n keys symmetrically to en/ru/ja**

- [ ] **Step 2: Run the full gate suite**

```bash
cd services/watch-together && go test ./... -count=1 -race
cd ../../frontend/web && bunx vitest run src/composables/unifiedPlayer/comboMapping.spec.ts \
  src/components/player/unified/__tests__/ src/views/__tests__/ src/locales/__tests__/locale-parity.spec.ts
bunx vue-tsc --noEmit
bash scripts/i18n-lint.sh && bash scripts/design-system-lint.sh
```

Expected: all green. (locale-parity + i18n-lint catch any asymmetric locale edit; DS-lint unaffected — no new player files.)

- [ ] **Step 3: Manual / e2e smoke (document results, do not skip)**

- Two browsers, one room, `player=aeplayer`: play/pause/seek converge; both resolve the same provider; episode switch propagates; a combo change by one member re-pins the other.
- Confirm the daily Kodik canary path is untouched (Plan A is add-only; Kodik branch unchanged).

- [ ] **Step 4: Commit**

```bash
git commit frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json \
  -m "i18n(wt): aeplayer room-switch labels (en/ru/ja)"
```

---

## Final review

After all tasks: dispatch a whole-branch code review (or run `/gsd:code-review`). Then deploy per house practice — `make redeploy-watch-together` + `make redeploy-web` from a CLEAN origin/main worktree (the deploy gates run there). Plan A is non-breaking; once shipped and soak-verified, **Plan B** (retire the legacy player surfaces) is unblocked.

## Open risks carried into execution

- **Reactive room accessor shape:** confirm whether the room handle exposes state as `room.room.value` or a flat reactive — adjust the watchers in Tasks 6-7 accordingly (grep `room.value.player` in `useWatchTogetherRoom.ts:365`).
- **Combo readiness on join:** a joiner must not broadcast its own auto-combo before the room combo arrives — the `roomPinned` guard (Task 6) + `immediate: true` watcher ordering must apply the remote combo before the local-combo broadcast watcher can fire. Verify ordering in the test.
- **anime metadata in WatchTogetherView:** aePlayer needs `{ title, ep, eps, still }` + `malId` + `isHentai`; ensure the view has them (room snapshot carries `anime_id` + `episode_id` only — may need a catalog fetch the legacy players didn't require).
