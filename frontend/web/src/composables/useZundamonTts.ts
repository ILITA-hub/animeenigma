import { computed, onBeforeUnmount, ref, shallowRef } from 'vue'

const VOICEVOX_ORIGIN = 'http://127.0.0.1:50021'
const ZUNDAMON_NAME = 'ずんだもん'

export interface VoicevoxStyle {
  id: number
  name: string
  type?: string
}

interface VoicevoxSpeaker {
  name: string
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

export type ZundamonTtsError = 'unavailable' | 'speakerMissing' | 'synthesis' | 'playback'

function ensureOk(response: Response): Response {
  if (!response.ok) throw new Error(`VOICEVOX HTTP ${response.status}`)
  return response
}

/**
 * Talks directly to the visitor's own VOICEVOX engine on loopback.
 * AnimeEnigma never receives the text or generated audio.
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
      const [versionResponse, speakersResponse] = await Promise.all([
        fetch(`${VOICEVOX_ORIGIN}/version`, { cache: 'no-store', signal: controller.signal }),
        fetch(`${VOICEVOX_ORIGIN}/speakers`, { cache: 'no-store', signal: controller.signal }),
      ])
      ensureOk(versionResponse)
      ensureOk(speakersResponse)

      const [version, speakers] = await Promise.all([
        versionResponse.json() as Promise<string>,
        speakersResponse.json() as Promise<VoicevoxSpeaker[]>,
      ])
      const zundamon = speakers.find((speaker) => speaker.name === ZUNDAMON_NAME)
      if (!zundamon || zundamon.styles.length === 0) {
        error.value = 'speakerMissing'
        status.value = 'error'
        return false
      }

      engineVersion.value = version
      styles.value = zundamon.styles
      selectedStyleId.value =
        zundamon.styles.find((style) => style.name === 'ノーマル')?.id ?? zundamon.styles[0].id
      connected.value = true
      status.value = 'ready'
      return true
    } catch (caught) {
      if (controller.signal.aborted && activeController !== controller) return false
      error.value = 'unavailable'
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
      const params = new URLSearchParams({
        text: phrase,
        speaker: String(selectedStyleId.value),
      })
      const queryResponse = ensureOk(
        await fetch(`${VOICEVOX_ORIGIN}/audio_query?${params}`, {
          method: 'POST',
          signal: controller.signal,
        }),
      )
      const query = (await queryResponse.json()) as Record<string, unknown>
      query.speedScale = speedScale
      query.pitchScale = pitchScale

      const synthesisResponse = ensureOk(
        await fetch(`${VOICEVOX_ORIGIN}/synthesis?speaker=${selectedStyleId.value}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(query),
          signal: controller.signal,
        }),
      )
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
      error.value = caught instanceof DOMException && caught.name === 'NotAllowedError'
        ? 'playback'
        : 'synthesis'
      status.value = 'error'
      return false
    } finally {
      if (activeController === controller) activeController = null
    }
  }

  onBeforeUnmount(stop)

  return {
    engineOrigin: VOICEVOX_ORIGIN,
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
