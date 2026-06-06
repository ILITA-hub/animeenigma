import { ref, onUnmounted, type Ref } from 'vue'
import type { StreamResult } from '@/types/unifiedPlayer'

export type LoadStrategy = 'native' | 'hlsjs'

export function chooseLoadStrategy(stream: StreamResult, nativeHls: boolean): LoadStrategy {
  if (stream.type === 'mp4') return 'native'
  return nativeHls ? 'native' : 'hlsjs'
}

export function useVideoEngine(videoEl: Ref<HTMLVideoElement | null>) {
  const fatal = ref<string | null>(null)
  let hls: any = null

  function nativeHlsSupported(v: HTMLVideoElement): boolean {
    return v.canPlayType('application/vnd.apple.mpegurl') !== ''
  }

  async function load(stream: StreamResult) {
    const v = videoEl.value
    if (!v) return
    fatal.value = null
    destroy()
    const strategy = chooseLoadStrategy(stream, nativeHlsSupported(v))
    if (strategy === 'native') {
      v.src = stream.url
      return
    }
    const Hls = (await import('hls.js')).default
    if (!Hls.isSupported()) {
      v.src = stream.url
      return
    }
    hls = new Hls({ enableWorker: true })
    hls.loadSource(stream.url)
    hls.attachMedia(v)
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
