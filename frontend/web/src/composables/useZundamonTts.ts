import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'

const STATUS_ENDPOINT = '/api/zundamon/status'
const SYNTHESIS_ENDPOINT = '/api/zundamon/synthesis'

export interface VoicevoxStyle {
  id: number
  name: string
  type?: string
}

interface ZundamonStatusResponse {
  version: string
  styles: VoicevoxStyle[]
}

export type ZundamonTtsStatus =
  | 'disconnected'
  | 'connecting'
  | 'ready'
  | 'synthesizing'
  | 'playing'
  | 'done'
  | 'error'

export type ZundamonTtsError = 'unavailable' | 'speakerMissing' | 'degraded' | 'busy' | 'synthesis' | 'playback'

class ZundamonApiError extends Error {
  constructor(readonly status: number, readonly code: string) {
    super(`Zundamon API HTTP ${status}`)
  }
}

async function apiError(response: Response): Promise<ZundamonApiError> {
  let code = ''
  try {
    const body = await response.json() as { error?: { code?: string } }
    code = body.error?.code ?? ''
  } catch {
    // The status still provides a safe generic fallback for malformed errors.
  }
  return new ZundamonApiError(response.status, code)
}

/**
 * Talks to AnimeEnigma's narrow server-side facade over the official
 * VOICEVOX Zundamon model. The raw engine is never exposed to browsers.
 */
export function useZundamonTts() {
  const connected = ref(false)
  const status = ref<ZundamonTtsStatus>('disconnected')
  const error = ref<ZundamonTtsError | null>(null)
  const engineVersion = ref('')
  const styles = shallowRef<VoicevoxStyle[]>([])
  const selectedStyleId = ref(-1)
  let activeController: AbortController | null = null
  let activeAudio: HTMLAudioElement | null = null
  let activeAudioUrl = ''

  const engineReady = computed(() => connected.value && selectedStyleId.value >= 0)
  const busy = computed(
    () => status.value === 'connecting' || status.value === 'synthesizing' || status.value === 'playing',
  )

  function disposeAudio(): void {
    if (activeAudio) {
      activeAudio.pause()
      activeAudio.src = ''
      activeAudio = null
    }
    if (activeAudioUrl) {
      URL.revokeObjectURL(activeAudioUrl)
      activeAudioUrl = ''
    }
  }

  function stop(): void {
    activeController?.abort()
    activeController = null
    disposeAudio()
    status.value = connected.value ? 'ready' : 'disconnected'
  }

  async function connect(): Promise<boolean> {
    stop()
    connected.value = false
    engineVersion.value = ''
    styles.value = []
    selectedStyleId.value = -1
    error.value = null
    status.value = 'connecting'

    const controller = new AbortController()
    activeController = controller
    const timeout = window.setTimeout(() => controller.abort(), 5000)

    try {
      const response = await fetch(STATUS_ENDPOINT, { cache: 'no-store', signal: controller.signal })
      if (!response.ok) throw await apiError(response)
      const engine = await response.json() as ZundamonStatusResponse
      if (!engine.styles.length) {
        error.value = 'speakerMissing'
        status.value = 'error'
        return false
      }

      engineVersion.value = engine.version
      styles.value = engine.styles
      selectedStyleId.value =
        engine.styles.find((style) => style.name === 'ノーマル')?.id ?? engine.styles[0].id
      connected.value = true
      status.value = 'ready'
      return true
    } catch (caught) {
      if (controller.signal.aborted && activeController !== controller) return false
      error.value = caught instanceof ZundamonApiError && caught.code === 'degraded' ? 'degraded' : 'unavailable'
      status.value = 'error'
      return false
    } finally {
      window.clearTimeout(timeout)
      if (activeController === controller) activeController = null
    }
  }

  async function speak(text: string, speedScale: number, pitchScale: number): Promise<boolean> {
    const phrase = text.trim()
    if (!engineReady.value || !phrase) return false

    activeController?.abort()
    disposeAudio()
    error.value = null
    status.value = 'synthesizing'

    const controller = new AbortController()
    activeController = controller

    try {
      const synthesisResponse = await fetch(SYNTHESIS_ENDPOINT, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          text: phrase,
          styleId: selectedStyleId.value,
          speedScale,
          pitchScale,
        }),
        signal: controller.signal,
      })
      if (!synthesisResponse.ok) throw await apiError(synthesisResponse)
      const audioBlob = await synthesisResponse.blob()
      activeAudioUrl = URL.createObjectURL(audioBlob)
      const player = new Audio(activeAudioUrl)
      activeAudio = player
      player.addEventListener(
        'ended',
        () => {
          if (activeAudio !== player) return
          disposeAudio()
          status.value = 'done'
        },
        { once: true },
      )
      player.addEventListener(
        'error',
        () => {
          if (activeAudio !== player) return
          disposeAudio()
          error.value = 'playback'
          status.value = 'error'
        },
        { once: true },
      )

      await player.play()
      status.value = 'playing'
      return true
    } catch (caught) {
      if (controller.signal.aborted) return false
      disposeAudio()
      if (caught instanceof DOMException && caught.name === 'NotAllowedError') error.value = 'playback'
      else if (caught instanceof ZundamonApiError && caught.code === 'degraded') error.value = 'degraded'
      else if (caught instanceof ZundamonApiError && caught.code === 'busy') error.value = 'busy'
      else error.value = 'synthesis'
      status.value = 'error'
      return false
    } finally {
      if (activeController === controller) activeController = null
    }
  }

  onBeforeUnmount(stop)

  return {
    connected,
    engineReady,
    engineVersion,
    styles,
    selectedStyleId,
    busy,
    status,
    error,
    connect,
    speak,
    stop,
  }
}
