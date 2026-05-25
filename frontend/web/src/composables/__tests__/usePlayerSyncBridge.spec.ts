/**
 * Workstream watch-together — Phase 3 (player-sync) Plan 03.1.
 *
 * Vitest spec for `usePlayerSyncBridge` — the generic HTML5
 * <video>-to-room bridge composable. Mirrors the locked behavior contract
 * documented in 03.1-PLAN.md §<behavior> and 03-CONTEXT.md §"Locked
 * Decisions / Generic HTML5 bridge".
 *
 * Strategy
 * --------
 * - `makeFakeVideo()` builds an EventTarget-like stub with `play()` / `pause()`
 *   spies, an addEventListener map so tests can fire `play|pause|seeked` at
 *   will, and mutable `currentTime`/`playbackRate` setters.
 * - `makeFakeRoom()` builds a partial `WatchTogetherRoomHandle` — emit
 *   methods are `vi.fn`, subscribe methods capture the handler in a closure
 *   so the test can simulate inbound playback / correction frames.
 * - The composable is mounted inside a tiny `defineComponent` via
 *   `@vue/test-utils` `mount()` so Vue's setup + onBeforeUnmount run.
 * - `requestAnimationFrame` is spied to call its callback via a `setTimeout`
 *   so `vi.useFakeTimers()` + `advanceTimersByTime` drives the 1Hz tick loop
 *   deterministically.
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { defineComponent, h, ref, type Ref } from 'vue'
import { mount } from '@vue/test-utils'

import { usePlayerSyncBridge } from '../usePlayerSyncBridge'
import type { WatchTogetherRoomHandle } from '../useWatchTogetherRoom'
import type {
  PlaybackEventData,
  PlaybackCorrectionData,
} from '@/types/watch-together'

// ──────────────────────────────────────────────────────────────────────────
//  Fake HTMLVideoElement.
// ──────────────────────────────────────────────────────────────────────────

interface FakeVideo {
  paused: boolean
  ended: boolean
  seeking: boolean
  currentTime: number
  playbackRate: number
  play: ReturnType<typeof vi.fn>
  pause: ReturnType<typeof vi.fn>
  addEventListener: (name: string, handler: (e?: Event) => void) => void
  removeEventListener: (name: string, handler: (e?: Event) => void) => void
  dispatchEvent: (name: string) => void
  _listeners: Map<string, Set<(e?: Event) => void>>
}

function makeFakeVideo(): FakeVideo {
  const listeners = new Map<string, Set<(e?: Event) => void>>()
  const v: FakeVideo = {
    paused: true,
    ended: false,
    seeking: false,
    currentTime: 0,
    playbackRate: 1.0,
    play: vi.fn(function (this: FakeVideo) {
      this.paused = false
      return Promise.resolve()
    }),
    pause: vi.fn(function (this: FakeVideo) {
      this.paused = true
    }),
    addEventListener(name, handler) {
      let set = listeners.get(name)
      if (!set) {
        set = new Set()
        listeners.set(name, set)
      }
      set.add(handler)
    },
    removeEventListener(name, handler) {
      listeners.get(name)?.delete(handler)
    },
    dispatchEvent(name) {
      const set = listeners.get(name)
      if (!set) return
      // Snapshot the set so handlers that mutate listeners don't affect this dispatch.
      for (const h of Array.from(set)) {
        h()
      }
    },
    _listeners: listeners,
  }
  // Bind the play/pause spies' `this` to the fake.
  v.play = vi.fn(() => {
    v.paused = false
    return Promise.resolve()
  }) as unknown as ReturnType<typeof vi.fn>
  v.pause = vi.fn(() => {
    v.paused = true
  })
  return v
}

// ──────────────────────────────────────────────────────────────────────────
//  Fake WatchTogetherRoomHandle.
// ──────────────────────────────────────────────────────────────────────────

interface FakeRoom {
  handle: WatchTogetherRoomHandle
  emitPlay: ReturnType<typeof vi.fn>
  emitPause: ReturnType<typeof vi.fn>
  emitSeek: ReturnType<typeof vi.fn>
  emitTimeTick: ReturnType<typeof vi.fn>
  /** Trigger an inbound `playback:event`. */
  firePlaybackEvent: (e: PlaybackEventData) => void
  /** Trigger an inbound `playback:correction`. */
  fireCorrection: (c: PlaybackCorrectionData) => void
  /** Spy counts of unsubscribe-calls so unmount can be verified. */
  playbackEventUnsub: ReturnType<typeof vi.fn>
  correctionUnsub: ReturnType<typeof vi.fn>
}

function makeFakeRoom(): FakeRoom {
  let playbackHandler: ((e: PlaybackEventData) => void) | null = null
  let correctionHandler: ((c: PlaybackCorrectionData) => void) | null = null
  const playbackEventUnsub = vi.fn(() => {
    playbackHandler = null
  })
  const correctionUnsub = vi.fn(() => {
    correctionHandler = null
  })

  const emitPlay = vi.fn()
  const emitPause = vi.fn()
  const emitSeek = vi.fn()
  const emitTimeTick = vi.fn()

  // Build a Handle stub. Unused methods are `vi.fn()` no-ops to satisfy the
  // full surface; the bridge only ever touches the four emit + two subscribe
  // methods enumerated below.
  const handle: WatchTogetherRoomHandle = {
    room: ref(null),
    members: ref([]),
    messages: ref([]),
    reactions: ref([]),
    connectionStatus: ref('open'),
    lastError: ref(null),
    emitPlay,
    emitPause,
    emitSeek,
    emitTimeTick,
    emitChangeEpisode: vi.fn(),
    emitChangePlayer: vi.fn(),
    emitChangeTranslation: vi.fn(),
    sendChat: vi.fn(),
    sendReaction: vi.fn(),
    heartbeat: vi.fn(),
    onPlaybackEvent: vi.fn((handler: (e: PlaybackEventData) => void) => {
      playbackHandler = handler
      return playbackEventUnsub
    }),
    onStateChanged: vi.fn(() => () => {}),
    onChatMessage: vi.fn(() => () => {}),
    onReaction: vi.fn(() => () => {}),
    onMemberJoined: vi.fn(() => () => {}),
    onMemberLeft: vi.fn(() => () => {}),
    onCorrection: vi.fn((handler: (c: PlaybackCorrectionData) => void) => {
      correctionHandler = handler
      return correctionUnsub
    }),
    onError: vi.fn(() => () => {}),
    onRoomClosed: vi.fn(() => () => {}),
    connect: vi.fn(async () => {}),
    disconnect: vi.fn(),
  }

  return {
    handle,
    emitPlay,
    emitPause,
    emitSeek,
    emitTimeTick,
    firePlaybackEvent: (e) => {
      if (playbackHandler) playbackHandler(e)
    },
    fireCorrection: (c) => {
      if (correctionHandler) correctionHandler(c)
    },
    playbackEventUnsub,
    correctionUnsub,
  }
}

// ──────────────────────────────────────────────────────────────────────────
//  Test harness.
// ──────────────────────────────────────────────────────────────────────────

interface Harness {
  video: FakeVideo
  videoRef: Ref<HTMLVideoElement | null>
  room: FakeRoom
  unmount: () => void
}

/**
 * Mount the bridge inside a real Vue component so lifecycle hooks fire.
 * `videoRef` starts as `null` then is set to the fake video AFTER mount so we
 * exercise the watcher path the real players hit (template ref populates
 * asynchronously).
 */
function harness(opts: { startWithVideo?: boolean; nullVideo?: boolean } = {}): Harness {
  const video = makeFakeVideo()
  const room = makeFakeRoom()
  const videoRef: Ref<HTMLVideoElement | null> = ref(null)

  if (opts.startWithVideo) {
    videoRef.value = video as unknown as HTMLVideoElement
  }

  const Comp = defineComponent({
    setup() {
      // The bridge is invoked once per consumer; we treat the videoRef as the
      // template ref pattern from the HTML5 players.
      usePlayerSyncBridge(videoRef, room.handle)
      return () => h('div')
    },
  })

  const wrapper = mount(Comp)

  if (!opts.startWithVideo && !opts.nullVideo) {
    // Standard test path — populate the ref after mount.
    videoRef.value = video as unknown as HTMLVideoElement
  }

  return {
    video,
    videoRef,
    room,
    unmount: () => wrapper.unmount(),
  }
}

// `requestAnimationFrame` mock — schedules its callback via setTimeout so
// fake-timers can advance it deterministically.
let rafCallbacks: Array<{ id: number; cb: FrameRequestCallback }> = []
let nextRafId = 1

beforeEach(() => {
  vi.useFakeTimers()
  rafCallbacks = []
  nextRafId = 1
  vi.stubGlobal('requestAnimationFrame', (cb: FrameRequestCallback) => {
    const id = nextRafId++
    // Drive RAF at ~60Hz so the 1Hz tick has well-defined latched behavior.
    setTimeout(() => {
      rafCallbacks = rafCallbacks.filter((r) => r.id !== id)
      cb(performance.now())
    }, 16)
    rafCallbacks.push({ id, cb })
    return id
  })
  vi.stubGlobal('cancelAnimationFrame', (id: number) => {
    rafCallbacks = rafCallbacks.filter((r) => r.id !== id)
  })
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests — emit on local events.
// ──────────────────────────────────────────────────────────────────────────

describe('usePlayerSyncBridge — local emit', () => {
  it('Test 1: emits playback:play on local @play event with currentTime', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0) // let watcher attach listeners
    h.video.currentTime = 42
    h.video.dispatchEvent('play')
    expect(h.room.emitPlay).toHaveBeenCalledTimes(1)
    expect(h.room.emitPlay).toHaveBeenCalledWith(42)
    h.unmount()
  })

  it('Test 2: emits playback:pause on local @pause event with currentTime', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = false // simulate a "was playing" precondition
    h.video.currentTime = 17.5
    h.video.dispatchEvent('pause')
    expect(h.room.emitPause).toHaveBeenCalledTimes(1)
    expect(h.room.emitPause).toHaveBeenCalledWith(17.5)
    h.unmount()
  })

  it('Test 3: emits playback:seek on local @seeked event with currentTime', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 120
    h.video.dispatchEvent('seeked')
    expect(h.room.emitSeek).toHaveBeenCalledTimes(1)
    expect(h.room.emitSeek).toHaveBeenCalledWith(120)
    h.unmount()
  })
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests — apply remote events to the video.
// ──────────────────────────────────────────────────────────────────────────

describe('usePlayerSyncBridge — apply remote', () => {
  it('Test 4: does NOT re-emit when applying a remote play (echo guard)', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 0
    h.video.paused = true
    h.room.firePlaybackEvent({
      kind: 'play',
      time: 30,
      by_user_id: 'remote-user',
      server_ts: Date.now(),
    })
    // Simulate the native `play` event the browser fires from `video.play()`.
    h.video.dispatchEvent('play')
    expect(h.room.emitPlay).not.toHaveBeenCalled()
    h.unmount()
  })

  it('Test 5: applies remote play — sets currentTime when drift > 1s and calls play()', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 5
    h.video.paused = true
    h.room.firePlaybackEvent({
      kind: 'play',
      time: 30,
      by_user_id: 'other',
      server_ts: Date.now(),
    })
    expect(h.video.currentTime).toBe(30)
    expect(h.video.play).toHaveBeenCalledTimes(1)
    h.unmount()
  })

  it('Test 6: applies remote play — does NOT seek when drift <= 1s', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 10.2
    h.video.paused = true
    h.room.firePlaybackEvent({
      kind: 'play',
      time: 10,
      by_user_id: 'other',
      server_ts: Date.now(),
    })
    expect(h.video.currentTime).toBe(10.2)
    expect(h.video.play).toHaveBeenCalledTimes(1)
    h.unmount()
  })

  it('Test 7: applies remote pause — pauses and sets currentTime', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = false
    h.video.currentTime = 0
    h.room.firePlaybackEvent({
      kind: 'pause',
      time: 25,
      by_user_id: 'other',
      server_ts: Date.now(),
    })
    expect(h.video.pause).toHaveBeenCalledTimes(1)
    expect(h.video.currentTime).toBe(25)
    // Simulate the native pause echo — should NOT re-emit.
    h.video.dispatchEvent('pause')
    expect(h.room.emitPause).not.toHaveBeenCalled()
    h.unmount()
  })

  it('Test 8: applies remote seek — sets currentTime, no re-emit on native seeked echo', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 0
    h.room.firePlaybackEvent({
      kind: 'seek',
      time: 88,
      by_user_id: 'other',
      server_ts: Date.now(),
    })
    expect(h.video.currentTime).toBe(88)
    h.video.dispatchEvent('seeked')
    expect(h.room.emitSeek).not.toHaveBeenCalled()
    h.unmount()
  })
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests — time-tick heartbeat.
// ──────────────────────────────────────────────────────────────────────────

describe('usePlayerSyncBridge — time_tick heartbeat', () => {
  it('Test 9: emits time_tick ~1Hz while playing', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = false
    h.video.currentTime = 0
    // Start the tick loop by dispatching `play` (the bridge wires startTickLoop here).
    h.video.dispatchEvent('play')

    // Advance 3.5 seconds — expect 3 ticks (at ~1s, ~2s, ~3s; jitter-tolerant
    // because the 1s gate is checked against Date.now() inside the RAF loop).
    await vi.advanceTimersByTimeAsync(3500)
    const ticks = h.room.emitTimeTick.mock.calls.length
    expect(ticks).toBeGreaterThanOrEqual(2)
    expect(ticks).toBeLessThanOrEqual(4)
    h.unmount()
  })

  it('Test 10: does NOT emit time_tick while paused', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = true
    h.video.currentTime = 0
    // No play dispatch — tick loop should not be running.
    await vi.advanceTimersByTimeAsync(3000)
    expect(h.room.emitTimeTick).not.toHaveBeenCalled()
    h.unmount()
  })

  it('Test 11: stops emitting time_tick after pause', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = false
    h.video.currentTime = 0
    h.video.dispatchEvent('play')
    await vi.advanceTimersByTimeAsync(2200) // 2 ticks expected
    const ticksWhilePlaying = h.room.emitTimeTick.mock.calls.length
    expect(ticksWhilePlaying).toBeGreaterThanOrEqual(1)

    // Pause and advance — tick count must NOT grow further.
    h.video.paused = true
    h.video.dispatchEvent('pause')
    await vi.advanceTimersByTimeAsync(3000)
    expect(h.room.emitTimeTick.mock.calls.length).toBe(ticksWhilePlaying)
    h.unmount()
  })
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests — drift correction.
// ──────────────────────────────────────────────────────────────────────────

describe('usePlayerSyncBridge — correction', () => {
  it('Test 12: soft correction adjusts playbackRate then restores after 5s', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 10
    // We are AHEAD of target — soft slow-down.
    h.room.fireCorrection({
      time: 9.7,
      server_ts: Date.now(),
    })
    // Drift = |10 - (9.7 + ~0)| ≈ 0.3 < 1.0 → soft correction.
    // We are ahead → playbackRate < 1.0.
    expect(h.video.playbackRate).toBeCloseTo(0.97, 5)

    await vi.advanceTimersByTimeAsync(5100)
    expect(h.video.playbackRate).toBe(1.0)
    h.unmount()
  })

  it('Test 13: soft correction adjusts playbackRate UP when behind', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 10
    // We are BEHIND target — soft speed-up.
    h.room.fireCorrection({
      time: 10.3,
      server_ts: Date.now(),
    })
    expect(h.video.playbackRate).toBeCloseTo(1.03, 5)
    await vi.advanceTimersByTimeAsync(5100)
    expect(h.video.playbackRate).toBe(1.0)
    h.unmount()
  })

  it('Test 14: hard correction sets currentTime directly', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 10
    h.room.fireCorrection({
      time: 25,
      server_ts: Date.now(),
    })
    // Drift = |10 - ~25| = 15 >> 1.0 → hard seek.
    expect(h.video.currentTime).toBe(25)
    // playbackRate should not be touched for hard corrections.
    expect(h.video.playbackRate).toBe(1.0)
    h.unmount()
  })
})

// ──────────────────────────────────────────────────────────────────────────
//  Tests — lifecycle / cleanup / null tolerance.
// ──────────────────────────────────────────────────────────────────────────

describe('usePlayerSyncBridge — lifecycle', () => {
  it('Test 15: unmount unsubscribes room handlers and restores playbackRate', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.currentTime = 10
    h.room.fireCorrection({
      time: 10.3,
      server_ts: Date.now(),
    })
    expect(h.video.playbackRate).toBeCloseTo(1.03, 5)

    h.unmount()
    // Unsubscribe closures must fire.
    expect(h.room.playbackEventUnsub).toHaveBeenCalledTimes(1)
    expect(h.room.correctionUnsub).toHaveBeenCalledTimes(1)
    // playbackRate must be restored.
    expect(h.video.playbackRate).toBe(1.0)
  })

  it('Test 16: null videoRef is a no-op — inbound events do not throw', async () => {
    const room = makeFakeRoom()
    const videoRef: Ref<HTMLVideoElement | null> = ref(null)
    const Comp = defineComponent({
      setup() {
        usePlayerSyncBridge(videoRef, room.handle)
        return () => h('div')
      },
    })
    const wrapper = mount(Comp)
    await vi.advanceTimersByTimeAsync(0)

    // Fire remote events while ref is still null — must not throw, must not emit.
    expect(() =>
      room.firePlaybackEvent({
        kind: 'play',
        time: 5,
        by_user_id: 'r',
        server_ts: Date.now(),
      }),
    ).not.toThrow()
    expect(() =>
      room.fireCorrection({ time: 10, server_ts: Date.now() }),
    ).not.toThrow()
    expect(room.emitPlay).not.toHaveBeenCalled()
    expect(room.emitTimeTick).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('Test 17: ended event stops the tick loop', async () => {
    const h = harness()
    await vi.advanceTimersByTimeAsync(0)
    h.video.paused = false
    h.video.currentTime = 0
    h.video.dispatchEvent('play')
    await vi.advanceTimersByTimeAsync(2200)
    const ticksBeforeEnd = h.room.emitTimeTick.mock.calls.length
    expect(ticksBeforeEnd).toBeGreaterThanOrEqual(1)

    // Video ends — tick should freeze even though `paused` may still be false
    // because the bridge gates on `ended` too.
    h.video.ended = true
    h.video.dispatchEvent('ended')
    await vi.advanceTimersByTimeAsync(3000)
    expect(h.room.emitTimeTick.mock.calls.length).toBe(ticksBeforeEnd)
    h.unmount()
  })
})
