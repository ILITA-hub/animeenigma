import { ref, watch } from 'vue'
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

  // subtitle prefs. Default OFF: there is no soft subtitle track until the
  // user picks one — EN provider streams carry subs burned into the video
  // (hardsub), which is not a selectable track. `string` (not TrackLang):
  // browsed tracks (Jimaku/OpenSubtitles) can be any language.
  const subLang = ref<string>('off')
  const subSize = ref(26)
  const subBg = ref(45)
  const subOffset = ref(0)

  // Hacker mode (debug HUD) — persisted, default off.
  const HACKER_KEY = 'pl_hacker_mode'
  const hackerMode = ref(
    typeof localStorage !== 'undefined' && localStorage.getItem(HACKER_KEY) === '1',
  )
  watch(hackerMode, (on) => {
    if (typeof localStorage === 'undefined') return
    if (on) localStorage.setItem(HACKER_KEY, '1')
    else localStorage.removeItem(HACKER_KEY)
  })

  // HUD pin — keep the debug HUD on screen during playback (persisted).
  const PIN_KEY = 'pl_hud_pin'
  const hudPinned = ref(
    typeof localStorage !== 'undefined' && localStorage.getItem(PIN_KEY) === '1',
  )
  watch(hudPinned, (on) => {
    if (typeof localStorage === 'undefined') return
    if (on) localStorage.setItem(PIN_KEY, '1')
    else localStorage.removeItem(PIN_KEY)
  })

  return {
    playing, progress, volume, muted, quality, speed, autoNext, autoSkip, combo,
    subLang, subSize, subBg, subOffset, hackerMode, hudPinned,
    setAudio, setLang, setProvider, setServer, setTeam,
  }
}
export type PlayerState = ReturnType<typeof usePlayerState>
