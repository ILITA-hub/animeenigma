import { ref } from 'vue'
import type { AudioKind, TrackLang, Combo } from '@/types/unifiedPlayer'

export function usePlayerState() {
  const playing = ref(false)
  const progress = ref(0)        // 0..100
  const volume = ref(80)
  const muted = ref(false)
  const quality = ref<string>('Auto')
  const speed = ref(1)
  const autoNext = ref(false)
  const autoSkip = ref(false)

  const combo = ref<Combo>({ audio: 'sub', lang: 'en', provider: '', server: '', team: null })

  const setAudio = (a: AudioKind) => { combo.value = { ...combo.value, audio: a, team: null } }
  const setLang = (l: TrackLang) => { combo.value = { ...combo.value, lang: l, team: null } }
  const setProvider = (id: string, server: string) => { combo.value = { ...combo.value, provider: id, server } }
  const setServer = (server: string) => { combo.value = { ...combo.value, server } }
  const setTeam = (team: string | null) => { combo.value = { ...combo.value, team } }

  // subtitle prefs
  const subLang = ref<'off' | TrackLang>('en')
  const subSize = ref(26)
  const subBg = ref(45)
  const subOffset = ref(0)

  return {
    playing, progress, volume, muted, quality, speed, autoNext, autoSkip, combo,
    subLang, subSize, subBg, subOffset,
    setAudio, setLang, setProvider, setServer, setTeam,
  }
}
export type PlayerState = ReturnType<typeof usePlayerState>
