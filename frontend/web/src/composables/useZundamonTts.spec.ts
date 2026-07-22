import { createApp, type App } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useZundamonTts } from './useZundamonTts'

const zundamonStyles = [
  { id: 1, name: 'あまあま' },
  { id: 3, name: 'ノーマル' },
]

const speakers = [
  { name: '四国めたん', styles: [{ id: 2, name: 'ノーマル' }] },
  { name: 'ずんだもん', styles: zundamonStyles },
]

function jsonResponse(data: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: vi.fn().mockResolvedValue(data),
  } as unknown as Response
}

function audioResponse(): Response {
  return {
    ok: true,
    status: 200,
    blob: vi.fn().mockResolvedValue(new Blob(['voice'], { type: 'audio/wav' })),
  } as unknown as Response
}

const fetchMock = vi.fn()
const play = vi.fn().mockResolvedValue(undefined)
const pause = vi.fn()
const addEventListener = vi.fn()

class MockAudio {
  src: string
  play = play
  pause = pause
  addEventListener = addEventListener

  constructor(src: string) {
    this.src = src
  }
}

type MountedTts = ReturnType<typeof useZundamonTts> & { app: App }

function mountComposable(): MountedTts {
  let result!: ReturnType<typeof useZundamonTts>
  const app = createApp({
    setup() {
      result = useZundamonTts()
      return () => null
    },
  })
  app.mount(document.createElement('div'))
  return Object.assign(result, { app })
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.stubGlobal('fetch', fetchMock)
  vi.stubGlobal('Audio', MockAudio)
  vi.stubGlobal('URL', {
    createObjectURL: vi.fn(() => 'blob:zundamon'),
    revokeObjectURL: vi.fn(),
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useZundamonTts', () => {
  it('connects to the local engine and selects the real Zundamon normal style', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse('0.25.0'))
      .mockResolvedValueOnce(jsonResponse(speakers))
    const tts = mountComposable()

    await expect(tts.connect()).resolves.toBe(true)
    expect(fetchMock.mock.calls[0][0]).toBe('http://127.0.0.1:50021/version')
    expect(fetchMock.mock.calls[1][0]).toBe('http://127.0.0.1:50021/speakers')
    expect(tts.engineReady.value).toBe(true)
    expect(tts.engineVersion.value).toBe('0.25.0')
    expect(tts.styles.value).toEqual(zundamonStyles)
    expect(tts.selectedStyleId.value).toBe(3)

    tts.app.unmount()
  })

  it('synthesizes through VOICEVOX speaker 3 and plays the returned WAV', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse('0.25.0'))
      .mockResolvedValueOnce(jsonResponse(speakers))
      .mockResolvedValueOnce(jsonResponse({ speedScale: 1, pitchScale: 0 }))
      .mockResolvedValueOnce(audioResponse())
    const tts = mountComposable()
    await tts.connect()

    await expect(tts.speak('  ずんだもんなのだ！  ', 1.2, 0.03)).resolves.toBe(true)

    const queryCall = fetchMock.mock.calls[2]
    expect(queryCall[0]).toContain('/audio_query?')
    expect(queryCall[0]).toContain('speaker=3')
    expect(queryCall[0]).toContain(encodeURIComponent('ずんだもんなのだ！'))

    const synthesisCall = fetchMock.mock.calls[3]
    expect(synthesisCall[0]).toBe('http://127.0.0.1:50021/synthesis?speaker=3')
    expect(JSON.parse(synthesisCall[1].body)).toMatchObject({ speedScale: 1.2, pitchScale: 0.03 })
    expect(play).toHaveBeenCalledTimes(1)
    expect(tts.status.value).toBe('playing')

    tts.stop()
    expect(pause).toHaveBeenCalled()
    expect(URL.revokeObjectURL).toHaveBeenCalledWith('blob:zundamon')
    tts.app.unmount()
  })

  it('refuses to substitute another voice when the engine has no Zundamon speaker', async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse('0.25.0'))
      .mockResolvedValueOnce(jsonResponse([speakers[0]]))
    const tts = mountComposable()

    await expect(tts.connect()).resolves.toBe(false)
    expect(tts.engineReady.value).toBe(false)
    expect(tts.error.value).toBe('speakerMissing')
    expect(tts.styles.value).toEqual([])

    tts.app.unmount()
  })
})
