import { createApp, nextTick, type App } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useZundamonTts, type ZundamonTtsStatus } from './useZundamonTts'

class MockUtterance {
  text: string
  voice: SpeechSynthesisVoice | null = null
  lang = ''
  rate = 1
  pitch = 1
  onstart: (() => void) | null = null
  onend: (() => void) | null = null
  onerror: ((event: { error: string }) => void) | null = null

  constructor(text: string) {
    this.text = text
  }
}

const englishVoice = {
  voiceURI: 'english',
  name: 'English Voice',
  lang: 'en-US',
  default: true,
  localService: true,
} as SpeechSynthesisVoice

const japaneseVoice = {
  voiceURI: 'japanese',
  name: 'Japanese Voice',
  lang: 'ja-JP',
  default: false,
  localService: true,
} as SpeechSynthesisVoice

const speak = vi.fn()
const cancel = vi.fn()
const addEventListener = vi.fn()
const removeEventListener = vi.fn()
const getVoices = vi.fn(() => [englishVoice, japaneseVoice])

const synthesis = {
  speak,
  cancel,
  addEventListener,
  removeEventListener,
  getVoices,
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
  vi.stubGlobal('SpeechSynthesisUtterance', MockUtterance)
  Object.defineProperty(window, 'speechSynthesis', {
    configurable: true,
    value: synthesis,
  })
})

afterEach(() => {
  vi.unstubAllGlobals()
  Reflect.deleteProperty(window, 'speechSynthesis')
})

describe('useZundamonTts', () => {
  it('prioritizes Japanese voices and selects one by default', () => {
    const tts = mountComposable()

    expect(tts.supported.value).toBe(true)
    expect(tts.voices.value.map((voice) => voice.voiceURI)).toEqual(['japanese', 'english'])
    expect(tts.selectedVoiceUri.value).toBe('japanese')
    expect(addEventListener).toHaveBeenCalledWith('voiceschanged', tts.loadVoices)

    tts.app.unmount()
  })

  it('speaks trimmed text with the selected browser voice and requested tuning', async () => {
    const tts = mountComposable()

    expect(tts.speak('  ずんだもんなのだ！  ', 1.1, 1.25)).toBe(true)
    const utterance = speak.mock.calls[0][0] as unknown as MockUtterance
    expect(utterance.text).toBe('ずんだもんなのだ！')
    expect(utterance.voice).toBe(japaneseVoice)
    expect(utterance.lang).toBe('ja-JP')
    expect(utterance.rate).toBe(1.1)
    expect(utterance.pitch).toBe(1.25)

    utterance.onstart?.()
    expect(tts.isSpeaking.value).toBe(true)
    expect(tts.status.value).toBe<ZundamonTtsStatus>('speaking')

    utterance.onend?.()
    await nextTick()
    expect(tts.isSpeaking.value).toBe(false)
    expect(tts.status.value).toBe<ZundamonTtsStatus>('done')

    tts.app.unmount()
  })

  it('cancels active speech from the stop control and on page teardown', () => {
    const tts = mountComposable()
    tts.speak('test', 1, 1)
    cancel.mockClear()

    tts.stop()
    expect(cancel).toHaveBeenCalledTimes(1)
    expect(tts.status.value).toBe<ZundamonTtsStatus>('idle')

    tts.app.unmount()
    expect(removeEventListener).toHaveBeenCalledWith('voiceschanged', tts.loadVoices)
    expect(cancel).toHaveBeenCalledTimes(2)
  })
})
