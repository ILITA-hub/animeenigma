import { ref, onUnmounted, type Ref } from 'vue'
import type { StreamResult } from '@/types/aePlayer'
import { ladder } from '@/utils/protocolLadder'

export type LoadStrategy = 'native' | 'hlsjs'

/**
 * Decide how to attach a stream to the <video> element.
 *
 * - mp4 → always native (<video src>).
 * - hls → hls.js whenever it is supported (Chrome/Firefox/Edge via MSE); fall
 *   back to native HLS only when hls.js is unsupported (Safari/iOS, which plays
 *   m3u8 natively).
 *
 * `hlsJsSupported` MUST come from `Hls.isSupported()`. Do NOT use
 * `video.canPlayType('application/vnd.apple.mpegurl')` to gate this: Chrome
 * returns 'maybe' for that probe yet cannot actually play HLS natively, so
 * trusting it routes every HLS stream down the native path and stalls at 0:00.
 */
export function chooseLoadStrategy(stream: StreamResult, hlsJsSupported: boolean): LoadStrategy {
  if (stream.type === 'mp4') return 'native'
  return hlsJsSupported ? 'hlsjs' : 'native'
}

export interface QualityLevel {
  label: string
  index: number
}

export interface FragStat {
  /** media start position of the fragment (sec) */
  start: number
  /** fragment duration (sec) */
  duration: number
  /** payload size in bytes */
  size: number
  /** wall-clock load time in ms */
  loadMs: number
}

/**
 * Build the quality menu from hls.js levels: label by height (`720p`),
 * dedupe by label keeping the FIRST hls index for it, sort high→low.
 * Height-less levels fall back to a bitrate label (`1500k`); unlabelable
 * levels are dropped (no fake entries — design rule D-05).
 */
export function buildLevelLabels(
  levels: { height?: number; bitrate?: number }[],
): QualityLevel[] {
  const byLabel = new Map<string, number>()
  levels.forEach((l, index) => {
    const label = l.height
      ? `${l.height}p`
      : l.bitrate
        ? `${Math.round(l.bitrate / 1000)}k`
        : ''
    if (!label || byLabel.has(label)) return
    byLabel.set(label, index)
  })
  return [...byLabel.entries()]
    .map(([label, index]) => ({ label, index }))
    .sort((a, b) => parseInt(b.label) - parseInt(a.label))
}

/**
 * Decide whether a fatal hls.js NETWORK_ERROR should give up (→ player switches
 * source) rather than retry via startLoad(). A dead PLAYLIST load (manifest or
 * level — e.g. a CDN host that 403/502s our IP) is unrecoverable for this
 * source, so bail immediately; transient fragment errors get `maxRetries`
 * startLoad() attempts first. Pure + exported for unit testing.
 */
export function shouldFatalOnNetworkError(
  playlistDead: boolean,
  netRetries: number,
  maxRetries = 2,
): boolean {
  return playlistDead || netRetries >= maxRetries
}

export interface PlaybackSnapshot {
  time: number
  wasPlaying: boolean
}

/**
 * Snapshot position + play state from a <video> element. MUST be read BEFORE
 * hls.js's destroy() runs on a fatal error: destroy() → detachMedia() makes
 * hls.js strip the element's src and call media.load(), which per the HTML
 * media-load algorithm resets currentTime to 0 synchronously — before the
 * player's own fatal-error watcher or a retry click ever run. Capturing here
 * is the only point that still sees the real position.
 */
export function snapshotPlayback(media: { currentTime: number; paused: boolean }): PlaybackSnapshot {
  return { time: media.currentTime, wasPlaying: !media.paused }
}

export function useVideoEngine(
  videoEl: Ref<HTMLVideoElement | null>,
  // When provided and false, skip building the per-fragment hacker-mode stats
  // (rolling fragStats window + bandwidthEstimate) — pure allocation churn that
  // only the debug HUD / scrub heatmap consume. The always-on fragLoadedCount
  // below keeps the stall watchdog working regardless.
  collectStats?: Ref<boolean>,
) {
  const fatal = ref<string | null>(null)
  // Position salvaged the instant a fatal error is declared — see snapshotPlayback.
  // Reset per-load (below) so a stale snapshot from a prior failure can never
  // leak into an unrelated later capture.
  const lastKnownPlayback = ref<PlaybackSnapshot | null>(null)
  const levels = ref<QualityLevel[]>([])
  const currentLevelLabel = ref('')
  const fragStats = ref<FragStat[]>([])
  const bandwidthEstimate = ref(0)
  // Which Kodik solodcdn edge actually served the last fragment, and the compact
  // attempt trail behind that choice — read from the streaming proxy's
  // X-AE-Edge-Served / X-AE-Edge-Trail response headers. Empty for non-Kodik
  // sources (no header). Surfaced in the hacker-mode HUD (metrics + logic, not
  // just the decision). See docs/superpowers/specs/2026-07-10-kodik-edge-failover-design.md.
  const servedEdge = ref('')
  const edgeTrail = ref('')
  // Cheap always-on signal: how many fragments have loaded for the current
  // stream. The silent-stall watchdog needs "are fragments flowing?" even when
  // the detailed fragStats array isn't being built (hacker mode off).
  const fragLoadedCount = ref(0)
  // URL of the most recently loaded fragment — a sample segment URL for the
  // protocol-ladder probe (Task 5) to test the tier above the current one.
  const lastFragUrl = ref('')
  let hls: any = null
  // Monotonic load generation. `load()` awaits a dynamic import of hls.js, so two
  // calls in quick succession (e.g. a provider change immediately followed by an
  // audio/lang re-resolve) can interleave. Without this guard each would create
  // its own hls.js instance and attachMedia() to the SAME <video>, leaving
  // conflicting MediaSources and a player frozen at readyState 0. Only the latest
  // generation is allowed to attach.
  let loadGen = 0

  async function load(stream: StreamResult) {
    const v = videoEl.value
    if (!v) return
    const gen = ++loadGen
    fatal.value = null
    lastKnownPlayback.value = null
    levels.value = []
    currentLevelLabel.value = ''
    fragStats.value = []
    bandwidthEstimate.value = 0
    servedEdge.value = ''
    edgeTrail.value = ''
    fragLoadedCount.value = 0
    lastFragUrl.value = ''
    // C1: a stale inflight record from the PREVIOUS source (a different XHR
    // URL entirely) must never leak into this source's watchdog checks.
    ladder.clearInflight()
    destroy()

    // Progressive MP4 — native playback. The backend proxy injects Referer and
    // serves byte ranges, so the element can seek directly.
    // INVARIANT: stream.url MUST be an HLS-proxy URL (ACAO: *). The <video>
    // element carries crossorigin="anonymous" (for the subtitle-VAD captureStream
    // fork), so a native load of a non-CORS host here would FAIL to play. Every
    // adapter wraps its URL via buildProxyUrl → never hand a raw CDN url to this.
    if (stream.type === 'mp4') {
      v.src = stream.url
      return
    }

    // HLS — prefer hls.js (works on Chrome/Firefox/Edge); native is the Safari
    // fallback. Importing here keeps hls.js out of the mp4 path's critical chunk.
    const Hls = (await import('hls.js')).default
    // A newer load() superseded us during the async import — abort so we don't
    // attach a second hls.js instance / MediaSource over the winning one.
    if (gen !== loadGen) return
    const strategy = chooseLoadStrategy(stream, Hls.isSupported())
    if (strategy === 'native') {
      v.src = stream.url
      return
    }

    // Match the proven legacy player config exactly. enableWorker:true is required
    // here: on CODECS-less HLS (e.g. Kodik's solodcdn streams) the main-thread
    // transmux path stalls at "bufferCodec event(s) expected" and never requests
    // fragment 0, leaving the player frozen at readyState 0 with no error.
    hls = new Hls({
      enableWorker: true,
      // Retain only ~30s of already-played media behind the playhead. The "10s
      // behind" seek requirement needs far less than the old 90s; the surplus
      // was pure memory held for content the viewer already watched (notably on
      // mobile during long episodes). Forward buffering is left at the proven
      // values — that's the rebuffering-protection knob, not a memory/egress one.
      backBufferLength: 30,
      // Seek-ahead window (spec 2026-06-10): keep ~1 min buffered ahead so
      // ±5s arrow-key seeks land inside the buffer and resolve instantly.
      maxBufferLength: 60,
      maxMaxBufferLength: 120,
      // Feed the protocol ladder (Task 2) so it can track per-fragment XHR
      // throughput and react to a stalled/slow-starting fragment before
      // FRAG_LOADED (or a timeout) ever fires.
      xhrSetup: (xhr: XMLHttpRequest, url: string) => {
        ladder.onXhrOpen(url)
        xhr.addEventListener('progress', (e: ProgressEvent) => ladder.onXhrProgress(url, e.loaded, e.total))
        // C1: 'loadend' fires on load/abort/error/timeout alike — every hls.js
        // XHR (including playlist/level loads that never reach FRAG_LOADED)
        // clears its own inflight slot here once it's done, so a completed or
        // aborted request can't leave a stale bytes>0 record poisoning
        // shouldDeferStallToLadder forever. Harmless if reportFragment's own
        // clear already ran first (double-clear is a no-op).
        xhr.addEventListener('loadend', () => ladder.onXhrLoadEnd(url))
      },
    })
    hls.loadSource(stream.url)
    hls.attachMedia(v)
    // Explicitly kick fragment loading once the manifest parses. On CODECS-less
    // HLS (Kodik's solodcdn streams) hls.js can otherwise sit after LEVEL_LOADED
    // at "bufferCodec event(s) expected" without ever requesting fragment 0 — it
    // needs the first fragment to detect the codec. startLoad(-1) forces the load
    // from the natural start position without auto-playing (preserves click-to-play).
    hls.on(Hls.Events.MANIFEST_PARSED, (_e: unknown, data: any) => {
      levels.value = buildLevelLabels(data?.levels ?? [])
      hls?.startLoad(-1)
    })
    hls.on(Hls.Events.LEVEL_SWITCHED, (_e: unknown, data: any) => {
      const lvl = levels.value.find((l) => l.index === data?.level)
      if (lvl) currentLevelLabel.value = lvl.label
    })
    hls.on(Hls.Events.FRAG_LOADED, (_e: unknown, data: any) => {
      const f = data?.frag
      const st = f?.stats
      if (!f || !st) return
      fragLoadedCount.value++ // cheap always-on signal for the stall watchdog
      lastFragUrl.value = f.url ?? ''
      // loadMs is hoisted above the collectStats gate below: the protocol-ladder
      // report (always-on, feeds the QoE tier state machine) needs it whether
      // or not the hacker-mode HUD is collecting the detailed fragStats window.
      const loadMs = Math.max(0, (st.loading?.end ?? 0) - (st.loading?.start ?? 0))
      const xhrNd = data?.networkDetails
      let rt: PerformanceResourceTiming | undefined
      try {
        rt = xhrNd?.responseURL
          ? (performance?.getEntriesByName?.(xhrNd.responseURL)?.pop() as PerformanceResourceTiming | undefined)
          : undefined
      } catch {
        // getEntriesByName unsupported/throwing in this environment -> protocol stays unknown
        rt = undefined
      }
      ladder.reportFragment({
        bytes: st.total ?? 0,
        ms: loadMs,
        mediaDurationS: f.duration ?? 0,
        protocol: rt?.nextHopProtocol,
      })
      // The detailed rolling window + bandwidth read are consumed ONLY by the
      // hacker-mode HUD / scrub heatmap. Skip the per-fragment array
      // reallocation (and the reactivity it triggers) when nobody's looking.
      if (collectStats && !collectStats.value) return
      // Rolling window of the last 30 fragments — enough for the hacker-mode
      // HUD + scrub-bar heatmap without unbounded growth on long episodes.
      fragStats.value = [
        ...fragStats.value.slice(-29),
        { start: f.start ?? 0, duration: f.duration ?? 0, size: st.total ?? 0, loadMs },
      ]
      bandwidthEstimate.value = hls?.bandwidthEstimate ?? 0
      // Edge telemetry from the streaming proxy (Kodik/solodcdn only). The XHR
      // response headers ride on data.networkDetails; getResponseHeader returns
      // null for a header that isn't present (non-Kodik source), leaving these
      // empty. Exposed cross-origin via Access-Control-Expose-Headers.
      const xhr = data?.networkDetails
      if (xhr?.getResponseHeader) {
        servedEdge.value = xhr.getResponseHeader('X-AE-Edge-Served') || ''
        edgeTrail.value = xhr.getResponseHeader('X-AE-Edge-Trail') || ''
      }
    })
    // Bounded network-error retries. A failed PLAYLIST load (manifest/level —
    // e.g. a megaplay CDN host that 403/502s our IP) means this source is dead:
    // looping startLoad() forever just freezes the player at a silent error.
    // Signal `fatal='network'` so the player can switch to the next candidate
    // source (the dynamic-BEST path). Transient fragment errors still get a few
    // startLoad() retries before giving up.
    let netRetries = 0
    // Snapshot the playhead BEFORE destroy() (see snapshotPlayback) and declare
    // the fatal state. Shared by both destroy()-ing branches below so the
    // salvage-then-destroy sequence can't drift out of sync between them.
    const declareFatal = (kind: string) => {
      lastKnownPlayback.value = snapshotPlayback(v)
      fatal.value = kind
      destroy()
    }
    hls.on(Hls.Events.ERROR, (_e: unknown, data: any) => {
      // Ladder timeout signal is always-on — report it even for a non-fatal
      // fragLoadTimeOut (hls.js usually just retries), before the fatal gate.
      if (data?.details === Hls.ErrorDetails.FRAG_LOAD_TIMEOUT) ladder.reportTimeout()
      if (!data?.fatal) return
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        const d = data.details
        const playlistDead =
          d === Hls.ErrorDetails.MANIFEST_LOAD_ERROR ||
          d === Hls.ErrorDetails.MANIFEST_LOAD_TIMEOUT ||
          d === Hls.ErrorDetails.MANIFEST_PARSING_ERROR ||
          d === Hls.ErrorDetails.LEVEL_LOAD_ERROR ||
          d === Hls.ErrorDetails.LEVEL_LOAD_TIMEOUT
        if (shouldFatalOnNetworkError(playlistDead, netRetries)) {
          declareFatal('network')
          return
        }
        netRetries++
        hls.startLoad()
      } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError()
      } else {
        declareFatal('unrecoverable')
      }
    })
  }

  function setLevel(label: string) {
    if (!hls) return
    if (label === 'Auto') {
      hls.currentLevel = -1
      return
    }
    const lvl = levels.value.find((l) => l.label === label)
    if (lvl) hls.currentLevel = lvl.index
  }

  function destroy() {
    if (hls) {
      hls.destroy()
      hls = null
    }
  }

  onUnmounted(destroy)

  return { fatal, lastKnownPlayback, load, destroy, levels, currentLevelLabel, setLevel, fragStats, bandwidthEstimate, servedEdge, edgeTrail, fragLoadedCount, lastFragUrl }
}
