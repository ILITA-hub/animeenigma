import { classifyFrame, type SpeechTap } from './subtitleAlign'

const TICK_HZ = 20   // VAD segments are >> 50ms; 20Hz is ample and ~3x cheaper than display rate

function ctxCtor(): typeof AudioContext | null {
  const w = window as unknown as { AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext }
  return w.AudioContext ?? w.webkitAudioContext ?? null
}

type Capturable = HTMLVideoElement & {
  captureStream?: () => MediaStream
  mozCaptureStream?: () => MediaStream
}

/**
 * createAudioTap — a NON-INTERRUPTIVE audio tap for the subtitle auto-sync VAD.
 *
 * The VAD only needs to LISTEN to the audio; it must never affect what the
 * viewer hears. The old implementation used `ctx.createMediaElementSource(el)`,
 * which permanently reroutes the element's ONE audio output through a Web Audio
 * graph — so any hiccup in that graph silenced playback: a suspended
 * AudioContext (this is created off a watcher, not a user gesture → the context
 * often starts suspended → "sound randomly gone when enabling subs"), or a
 * CORS-tainted cross-origin source (audio zeroed for security → "sound gone
 * after switching provider"). Both were real bug reports.
 *
 * Instead we analyse a `captureStream()` fork — a SEPARATE MediaStream that
 * mirrors the element's audio while the element keeps playing to the speakers on
 * its own, untouched path. The analyser is a passive leaf (never connected to
 * `ctx.destination`), so this graph produces no output and cannot double, echo,
 * or silence playback no matter what state the context is in.
 *
 * FAIL SILENTLY: if `captureStream` is unavailable (Safari) we throw so the
 * caller marks auto-sync `unsupported` and simply skips it; if the fork has no
 * audio track yet (called before playback) or the source is tainted, we idle and
 * retry each tick. In every failure mode the element's own audio is never
 * touched — playback is unaffected, auto-sync just doesn't lock.
 */
export function createAudioTap(el: HTMLVideoElement): SpeechTap {
  const Ctor = ctxCtor()
  if (!Ctor) throw new Error('Web Audio unavailable')

  const cap = el as Capturable
  const capFn = cap.captureStream ?? cap.mozCaptureStream
  if (typeof capFn !== 'function') {
    // No way to fork the audio without hijacking the element's output — refuse.
    // Auto-sync stays unsupported; playback is completely unaffected.
    throw new Error('captureStream unavailable')
  }

  const ctx = new Ctor()
  const analyser = ctx.createAnalyser()
  analyser.fftSize = 2048

  let stream: MediaStream | null = null
  let src: MediaStreamAudioSourceNode | null = null

  // Lazily wire the fork once the element actually has an audio track:
  // captureStream() before metadata yields a track-less stream, and a
  // MediaStreamAudioSourceNode bound to it would stay silent forever. captureStream
  // returns the SAME live stream on repeat calls, so we grab it once and poll its
  // audio tracks. A failure here NEVER touches the element's own output.
  function ensureWired() {
    if (src) return
    if (!stream) {
      try { stream = capFn!.call(cap) } catch { stream = null; return }
    }
    if (!stream || stream.getAudioTracks().length === 0) return
    try {
      src = ctx.createMediaStreamSource(stream)
      src.connect(analyser)   // passive leaf — deliberately NOT connected to ctx.destination
    } catch {
      src = null   // tainted / transient — retry next tick
    }
  }

  // Created off a watcher (not a user gesture) → the context may start suspended.
  // Resume best-effort so the analyser runs; if it stays suspended the VAD simply
  // idles — the element plays on its own path, so nothing is silenced.
  void Promise.resolve(ctx.resume?.()).catch(() => {})

  const freq = new Uint8Array(analyser.frequencyBinCount)
  const minGap = 1000 / TICK_HZ
  let cb: ((t: number, s: boolean) => void) | null = null
  let raf: number | null = null
  let lastTick = -Infinity

  function tick(now: number) {
    raf = requestAnimationFrame(tick)
    if (now - lastTick < minGap) return
    lastTick = now
    if (el.paused || el.seeking) return
    ensureWired()
    if (!src) return   // audio not capturable yet — idle; playback unaffected
    analyser.getByteFrequencyData(freq)
    cb?.(el.currentTime, classifyFrame(freq, ctx.sampleRate, analyser.fftSize))
  }
  raf = requestAnimationFrame(tick)

  return {
    onFrame(fn) { cb = fn },
    dispose() {
      if (raf !== null) cancelAnimationFrame(raf)
      raf = null
      try { src?.disconnect(); analyser.disconnect() } catch { /* already gone */ }
      // Release the fork so the element can drop the capture.
      try { stream?.getTracks().forEach((t) => t.stop()) } catch { /* ignore */ }
      void ctx.close().catch(() => {})
    },
  }
}
