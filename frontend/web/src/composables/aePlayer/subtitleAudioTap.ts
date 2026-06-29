import { classifyFrame, type SpeechTap } from './subtitleAlign'

const TICK_HZ = 20   // VAD segments are >> 50ms; 20Hz is ample and ~3x cheaper than display rate

function ctxCtor(): typeof AudioContext | null {
  const w = window as unknown as { AudioContext?: typeof AudioContext; webkitAudioContext?: typeof AudioContext }
  return w.AudioContext ?? w.webkitAudioContext ?? null
}

export function createAudioTap(el: HTMLVideoElement): SpeechTap {
  const Ctor = ctxCtor()
  if (!Ctor) throw new Error('Web Audio unavailable')

  const ctx = new Ctor()
  let src: MediaElementAudioSourceNode
  try {
    src = ctx.createMediaElementSource(el)   // binds the element ONCE for its lifetime
  } catch (e) {
    void ctx.close().catch(() => {})         // don't leak the context if binding fails
    throw e
  }
  const gain = ctx.createGain()
  const analyser = ctx.createAnalyser()
  analyser.fftSize = 2048

  // Preserve player volume/mute: route audio through a mirrored gain to output.
  src.connect(gain); gain.connect(ctx.destination)
  // Analyser is an intentional passive leaf branch (ubiquitous VAD-tap pattern; browsers process it).
  src.connect(analyser)

  const syncGain = () => { gain.gain.value = el.muted ? 0 : el.volume }
  syncGain()
  el.addEventListener('volumechange', syncGain)
  // Created from a watcher (not a user gesture) → the context may start suspended,
  // routing the element's audio into a non-running graph. Resume it (best-effort).
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
    analyser.getByteFrequencyData(freq)
    cb?.(el.currentTime, classifyFrame(freq, ctx.sampleRate, analyser.fftSize))
  }
  raf = requestAnimationFrame(tick)

  return {
    onFrame(fn) { cb = fn },
    dispose() {
      if (raf !== null) cancelAnimationFrame(raf)
      raf = null
      el.removeEventListener('volumechange', syncGain)
      try { src.disconnect(); gain.disconnect(); analyser.disconnect() } catch { /* already gone */ }
      void ctx.close().catch(() => {})
    },
  }
}
