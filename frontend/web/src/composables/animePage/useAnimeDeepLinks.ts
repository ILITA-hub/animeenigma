import { computed, type Ref } from 'vue'
import { useRoute } from 'vue-router'
import type { Anime } from '@/composables/useAnime'

/**
 * Deep-link query-param readers for the anime watch page (extracted from
 * Anime.vue). All values are HINTS consumed one-shot at player mount; the
 * two-way URL sync write path lives in usePlayerSurface (onUrlSync).
 */
export function useAnimeDeepLinks(anime: Ref<Anime | null>) {
  const route = useRoute()

  // Phase 8 (CR-01) — explicit deep-link hint from the Continue-Watching row.
  // `/anime/{id}?episode=N` is the contract used by ContinueWatchingRow.vue;
  // without this read, the link silently falls back to the resume-state-machine
  // startEpisode (which usually matches, but diverges whenever the server-side
  // resume state has advanced past the row's episode_number — e.g. when the
  // user just finished E5 but is being shown an older "in-progress" E4 row).
  //
  // The query is a HINT, not a hard override: the manual "Rewatch from ep. 1"
  // button (resumeOverrideEpisode) still wins above this. The episode is
  // clamped to [1, totalEpisodes] when totalEpisodes is known; otherwise the
  // raw value is accepted (the player components defend their own bounds).
  const queryEpisode = computed<number | undefined>(() => {
    const v = route.query.episode
    const s = Array.isArray(v) ? v[0] : v
    if (typeof s !== 'string' || s === '') return undefined
    const n = parseInt(s, 10)
    if (!Number.isFinite(n) || n <= 0) return undefined
    const total = anime.value?.totalEpisodes ?? 0
    if (total > 0 && n > total) return total
    return n
  })

  // Notification deep-link — `?provider=` is an aePlayer source id, `?team=` is a
  // team TITLE. Both are HINTS preselected on aePlayer (see AePlayer initialProvider).
  // Single non-empty string query param (first value if repeated), else undefined.
  function queryString(key: string): string | undefined {
    const v = route.query[key]
    const s = Array.isArray(v) ? v[0] : v
    return typeof s === 'string' && s !== '' ? s : undefined
  }
  const queryProvider = computed(() => queryString('provider'))
  const queryTeam = computed(() => queryString('team'))
  const queryAudio = computed(() => queryString('audio'))
  const queryLang = computed(() => queryString('lang'))

  // Shared-link playback position (`?t=` seconds) — the Share button in the player
  // encodes the exact combo + episode + timestamp. Non-negative integer only.
  const queryTimestamp = computed<number | undefined>(() => {
    const s = queryString('t')
    if (s === undefined) return undefined
    const n = parseInt(s, 10)
    return Number.isFinite(n) && n > 0 ? n : undefined
  })

  return { queryEpisode, queryProvider, queryTeam, queryAudio, queryLang, queryTimestamp }
}
