import { nextTick, ref, shallowRef } from 'vue'

// ── Hi10P wasm compat engine (AUTO-629) ─────────────────────────────────────
// Browsers whose native stacks cannot decode H.264 High 10 (Firefox on every
// OS — profile support is deliberately absent, Bugzilla 1711812; Safari via
// VideoToolbox) get a lazy-loaded software pipeline instead: libmedia's
// AVPlayer (vendored, unmodified, LGPL — see public/libmedia/README.md)
// demuxes the same signed HLS-TS URL and decodes with an ffmpeg-derived
// h264 wasm module, rendering into its own canvas.
//
// This is deliberately NOT a drop-in replacement for the <video> element:
// AePlayer swaps in a dedicated CompatSurface with its own control strip and
// keeps element-coupled extras (VAD auto-sync, scrub shadow engine, offline
// downloads, WT sync) inert while active. SubtitleOverlay and the watch
// clock keep working through `clockElement` — a minimal duck-type of the
// few properties they actually read.

/** Runtime module URL — outside the Vite graph on purpose: the webpack-built
 *  player spawns workers from sibling chunk files, and LGPL compliance wants
 *  separate unmodified files. */
export const LIBMEDIA_PLAYER_URL = '/libmedia/player/avplayer.js'
/** `${base}/decode/h264-simd.wasm` etc. — vendored in public/libmedia. */
export const LIBMEDIA_WASM_BASE = '/libmedia'

/** The slice of @libmedia/avplayer's API this engine drives. Times are int64
 *  milliseconds (BigInt) — seek(Number) fails with a demuxer error, which is
 *  why toMs()/toSec() below exist. */
export interface AVPlayerLike {
  load(url: string): Promise<void>
  play(): Promise<void>
  pause(): Promise<void>
  seek(ms: bigint): Promise<void>
  destroy(): Promise<void>
  setVolume(v: number): void
  setPlaybackRate(r: number): void
  /** Forces the internal render viewport (and canvas raster size) to match
   *  the given CSS pixel dimensions — the library's own remedy for a host
   *  container whose size wasn't known at construction time. */
  resize(width: number, height: number): void
  getDuration(): bigint
  getStats(): { videoDecodeFramerate?: number; videoRenderFramerate?: number; width?: number; height?: number }
  readonly currentTime: bigint
  on(event: string, cb: (...args: unknown[]) => void): void
}

type AVPlayerCtor = new (opts: Record<string, unknown>) => AVPlayerLike

let loader: (() => Promise<{ default: AVPlayerCtor }>) | null = null
/** Test seam — the real loader imports a runtime URL jsdom cannot resolve. */
export function __setCompatLoaderForTest(l: typeof loader): void {
  loader = l
}

async function loadAVPlayer(): Promise<AVPlayerCtor> {
  if (loader) return (await loader()).default
  const mod = await import(/* @vite-ignore */ LIBMEDIA_PLAYER_URL)
  return (mod.default ?? mod.AVPlayer) as AVPlayerCtor
}

export function toMs(seconds: number): bigint {
  return BigInt(Math.max(0, Math.round(seconds * 1000)))
}

export function toSec(ms: bigint | number): number {
  return Number(ms) / 1000
}

export interface CompatActivateOptions {
  container: HTMLDivElement
  url: string
  /** Resume position in seconds (e.g. the playhead salvaged from the failed
   *  native attempt). Sub-second values are skipped — not worth a seek. */
  startAt?: number
  /** 0-100, the player-state scale. */
  volume: number
  muted: boolean
  rate: number
}

export function useCompatEngine() {
  const active = ref(false)
  const loading = ref(false)
  const error = ref<string | null>(null)
  const currentTime = ref(0)
  const duration = ref(0)
  const paused = ref(true)
  const ended = ref(false)
  const stats = ref<{ decodeFps: number; renderFps: number; width: number; height: number } | null>(null)
  const player = shallowRef<AVPlayerLike | null>(null)

  let raf = 0
  let gen = 0
  let lastVolume = 100
  let muted = false
  let lastStatsAt = 0

  function applyVolume(): void {
    // AVPlayer volume is a GainNode factor (1 = 100%); no mute API, so mute
    // is volume 0 with the previous value remembered.
    player.value?.setVolume(muted ? 0 : lastVolume / 100)
  }

  function tick(now = performance.now()): void {
    const p = player.value
    if (p) {
      // 4 Hz snap, same rationale as usePlaybackClock's PROGRESS_SYNC_HZ.
      const t = Math.floor(toSec(p.currentTime) * 4) / 4
      if (t !== currentTime.value) currentTime.value = t
      // Stats at the same 4 Hz cadence: they only feed the debug HUD and the
      // clockElement dimensions, and a per-frame reactive write would rebuild
      // the HUD line at 60 fps on a path where every CPU cycle belongs to the
      // software decoder.
      if (now - lastStatsAt >= 250) {
        lastStatsAt = now
        try {
          const s = p.getStats()
          stats.value = {
            decodeFps: Math.round(s.videoDecodeFramerate ?? 0),
            renderFps: Math.round(s.videoRenderFramerate ?? 0),
            width: s.width ?? 0,
            height: s.height ?? 0,
          }
        } catch {
          // stats are advisory
        }
      }
    }
    raf = requestAnimationFrame(tick)
  }

  /** Minimal duck-type of the <video> properties SubtitleOverlay and the
   *  progress heartbeat actually read. Cast at the binding site. */
  const clockElement = {
    get currentTime(): number {
      return currentTime.value
    },
    get duration(): number {
      return duration.value
    },
    get paused(): boolean {
      return paused.value
    },
    get videoWidth(): number {
      return stats.value?.width || 1920
    },
    get videoHeight(): number {
      return stats.value?.height || 1080
    },
    get clientHeight(): number {
      return stats.value?.height || 400
    },
  }

  async function activate(opts: CompatActivateOptions): Promise<boolean> {
    const myGen = ++gen
    loading.value = true
    error.value = null
    ended.value = false
    try {
      const AVPlayer = await loadAVPlayer()
      if (myGen !== gen) return false
      const p = new AVPlayer({
        container: opts.container,
        wasmBaseUrl: LIBMEDIA_WASM_BASE,
        // Never route back into the broken native pipeline: the whole point
        // is that MSE/hardware "supports" the codec string but cannot decode
        // the frames (Firefox never validates the profile byte).
        enableHardware: false,
        enableWebCodecs: false,
        enableWebGPU: false,
        checkUseMSE: () => false,
        // Workers work WITHOUT cross-origin isolation (io/demux + decode
        // split off the UI thread) — we do not ship COOP/COEP.
        enableWorker: true,
        preLoadTime: 10,
      })
      player.value = p
      p.on('error', (e: unknown) => {
        error.value = String((e as { message?: string })?.message ?? e ?? 'compat engine error')
      })
      p.on('ended', () => {
        ended.value = true
        paused.value = true
      })
      p.on('playing', () => {
        paused.value = false
      })
      p.on('paused', () => {
        paused.value = true
      })
      await p.load(opts.url)
      if (myGen !== gen) return false
      duration.value = toSec(p.getDuration())
      lastVolume = opts.volume
      muted = opts.muted
      applyVolume()
      if (opts.rate !== 1) p.setPlaybackRate(opts.rate)
      await p.play()
      if (myGen !== gen) return false
      const startAt = opts.startAt ?? 0
      if (startAt >= 1) await p.seek(toMs(startAt))
      active.value = true
      paused.value = false
      // `opts.container` is `v-show`-hidden until `active` flips (see
      // AePlayer.vue's compat-host div), so the canvas libmedia created
      // inside `new AVPlayer()` above read a 0×0 offsetWidth/offsetHeight
      // and locked in a ~1×1 raster — decode/playback (audio included) is
      // unaffected, but no frame is ever visible. Wait for Vue to paint the
      // now-visible container, then force the real size via the library's
      // own resize() so it re-measures instead of staying pinned at 1×1.
      await nextTick()
      if (myGen !== gen) return false
      p.resize(opts.container.offsetWidth, opts.container.offsetHeight)
      cancelAnimationFrame(raf)
      lastStatsAt = 0 // sample stats on the first tick
      raf = requestAnimationFrame(tick)
      return true
    } catch (e) {
      error.value = String((e as Error)?.message ?? e)
      await destroy()
      return false
    } finally {
      if (myGen === gen) loading.value = false
    }
  }

  function play(): void {
    void player.value?.play().then(() => {
      paused.value = false
    })
  }

  function pause(): void {
    void player.value?.pause().then(() => {
      paused.value = true
    })
  }

  function toggle(): void {
    if (paused.value) play()
    else pause()
  }

  function seekTo(seconds: number): void {
    const p = player.value
    if (!p) return
    const clamped = Math.min(Math.max(0, seconds), duration.value || seconds)
    currentTime.value = clamped
    void p.seek(toMs(clamped))
  }

  function setVolume(v: number): void {
    lastVolume = v
    applyVolume()
  }

  function setMuted(m: boolean): void {
    muted = m
    applyVolume()
  }

  function setRate(r: number): void {
    player.value?.setPlaybackRate(r)
  }

  async function destroy(): Promise<void> {
    gen++
    cancelAnimationFrame(raf)
    raf = 0
    const p = player.value
    player.value = null
    active.value = false
    loading.value = false
    paused.value = true
    stats.value = null
    if (p) {
      try {
        await p.destroy()
      } catch {
        // teardown is best-effort
      }
    }
  }

  return {
    active,
    loading,
    error,
    currentTime,
    duration,
    paused,
    ended,
    stats,
    clockElement,
    activate,
    play,
    pause,
    toggle,
    seekTo,
    setVolume,
    setMuted,
    setRate,
    destroy,
  }
}
