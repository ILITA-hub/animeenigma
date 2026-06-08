import { ref, onUnmounted, type Ref } from 'vue'
import type { StreamResult } from '@/types/unifiedPlayer'

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

export function useVideoEngine(videoEl: Ref<HTMLVideoElement | null>) {
  const fatal = ref<string | null>(null)
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
    destroy()

    // Progressive MP4 — native playback. The backend proxy injects Referer and
    // serves byte ranges, so the element can seek directly.
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
    hls = new Hls({ enableWorker: true, backBufferLength: 90 })
    hls.loadSource(stream.url)
    hls.attachMedia(v)
    // Explicitly kick fragment loading once the manifest parses. On CODECS-less
    // HLS (Kodik's solodcdn streams) hls.js can otherwise sit after LEVEL_LOADED
    // at "bufferCodec event(s) expected" without ever requesting fragment 0 — it
    // needs the first fragment to detect the codec. startLoad(-1) forces the load
    // from the natural start position without auto-playing (preserves click-to-play).
    hls.on(Hls.Events.MANIFEST_PARSED, () => {
      hls?.startLoad(-1)
    })
    hls.on(Hls.Events.ERROR, (_e: unknown, data: any) => {
      if (!data?.fatal) return
      if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
        hls.startLoad()
      } else if (data.type === Hls.ErrorTypes.MEDIA_ERROR) {
        hls.recoverMediaError()
      } else {
        fatal.value = 'unrecoverable'
        destroy()
      }
    })
  }

  function destroy() {
    if (hls) {
      hls.destroy()
      hls = null
    }
  }

  onUnmounted(destroy)

  return { fatal, load, destroy }
}
