import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'

export type ZundamonTtsStatus = 'idle' | 'speaking' | 'done' | 'error'

function speechAvailable(): boolean {
  return (
    typeof window !== 'undefined' &&
    'speechSynthesis' in window &&
    'SpeechSynthesisUtterance' in window
  )
}

/**
 * Browser-only speech synthesis for the hidden Zundamon voice lab.
 *
 * No text or audio is sent to AnimeEnigma: the browser/OS owns voice discovery
 * and rendering through the Web Speech API.
 */
export function useZundamonTts() {
  const supported = ref(speechAvailable())
  // Browser voice objects are host objects. Keep them raw so the Web Speech
  // implementation receives the original identity, never a Vue proxy.
  const voices = shallowRef<SpeechSynthesisVoice[]>([])
  const selectedVoiceUri = ref('')
  const isSpeaking = ref(false)
  const status = ref<ZundamonTtsStatus>('idle')
  let activeUtterance: SpeechSynthesisUtterance | null = null

  const selectedVoice = computed(
    () => voices.value.find((voice) => voice.voiceURI === selectedVoiceUri.value) ?? null,
  )

  function loadVoices(): void {
    if (!supported.value) return

    voices.value = window.speechSynthesis
      .getVoices()
      .slice()
      .sort((a, b) => {
        const aJapanese = a.lang.toLowerCase().startsWith('ja')
        const bJapanese = b.lang.toLowerCase().startsWith('ja')
        if (aJapanese !== bJapanese) return aJapanese ? -1 : 1
        if (a.default !== b.default) return a.default ? -1 : 1
        return a.name.localeCompare(b.name)
      })

    if (!voices.value.some((voice) => voice.voiceURI === selectedVoiceUri.value)) {
      selectedVoiceUri.value = voices.value[0]?.voiceURI ?? ''
    }
  }

  function stop(): void {
    if (!supported.value) return
    window.speechSynthesis.cancel()
    activeUtterance = null
    isSpeaking.value = false
    status.value = 'idle'
  }

  function speak(text: string, rate: number, pitch: number): boolean {
    const phrase = text.trim()
    if (!supported.value || !phrase) return false

    window.speechSynthesis.cancel()
    const utterance = new window.SpeechSynthesisUtterance(phrase)
    const voice = selectedVoice.value
    utterance.voice = voice
    utterance.lang = voice?.lang || 'ja-JP'
    utterance.rate = rate
    utterance.pitch = pitch
    utterance.onstart = () => {
      isSpeaking.value = true
      status.value = 'speaking'
    }
    utterance.onend = () => {
      if (activeUtterance !== utterance) return
      activeUtterance = null
      isSpeaking.value = false
      status.value = 'done'
    }
    utterance.onerror = (event) => {
      if (activeUtterance !== utterance) return
      activeUtterance = null
      isSpeaking.value = false
      status.value = event.error === 'canceled' || event.error === 'interrupted' ? 'idle' : 'error'
    }

    activeUtterance = utterance
    status.value = 'idle'
    window.speechSynthesis.speak(utterance)
    return true
  }

  onMounted(() => {
    supported.value = speechAvailable()
    if (!supported.value) return
    loadVoices()
    window.speechSynthesis.addEventListener('voiceschanged', loadVoices)
  })

  onBeforeUnmount(() => {
    if (!supported.value) return
    window.speechSynthesis.removeEventListener('voiceschanged', loadVoices)
    window.speechSynthesis.cancel()
  })

  return {
    supported,
    voices,
    selectedVoiceUri,
    selectedVoice,
    isSpeaking,
    status,
    loadVoices,
    speak,
    stop,
  }
}
