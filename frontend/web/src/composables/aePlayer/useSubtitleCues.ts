import { shallowRef, watch, type Ref } from 'vue'
import { fetchAndParseCues, type SubtitleCue } from '@/utils/subtitle-parser'

export function useSubtitleCues(
  url: Ref<string | null>,
  format: Ref<'ass' | 'srt' | 'vtt' | null>,
) {
  const cues = shallowRef<SubtitleCue[]>([])
  let abort: AbortController | null = null

  watch(url, async (u) => {
    abort?.abort()
    if (!u) { cues.value = []; return }
    abort = new AbortController()
    try { cues.value = await fetchAndParseCues(u, format.value || 'auto', abort.signal) }
    catch { cues.value = [] }   // abort / network / parse → no-op for auto-sync
  }, { immediate: true })

  return { cues }
}
