import { computed, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Anime } from '@/composables/useAnime'
import { useUserTimezone } from '@/composables/useUserTimezone'
import { wallClockDate, formatUtcOffset } from '@/composables/schedule/timezone'
import { parseDescription } from '@/utils/description-parser'
import {
  formatReviewStats as formatReviewStatsPure,
  isReviewFlagged as isReviewFlaggedPure,
  formatEpisodeCount as formatEpisodeCountPure,
  formatCount as formatCountPure,
  secondaryTitleForms as secondaryTitleFormsPure,
} from '@/composables/anime/animeFormatters'
import type { Review } from './types'

/**
 * Locale-bound display formatters + derived metadata computeds for the anime
 * page (extracted from Anime.vue). Pure stateless helpers live in
 * composables/anime/animeFormatters.ts; this composable binds them to the
 * active i18n locale / user timezone and to the page's anime ref.
 */
export function useAnimeDisplay(anime: Ref<Anime | null>) {
  const { t, locale } = useI18n()
  const { timezone: userTimezone } = useUserTimezone()

  const statusVariant = computed(() => {
    const status = anime.value?.status?.toLowerCase()
    if (status === 'completed' || status === 'released') return 'success'
    if (status === 'upcoming' || status === 'announced') return 'warning'
    return 'primary' // ongoing
  })

  const parsedDescription = computed(() =>
    anime.value?.description ? parseDescription(anime.value.description) : ''
  )

  const isHentai = computed(() =>
    anime.value?.rawGenres?.some(g => g.name === 'Hentai') ?? false
  )

  // Other-language title forms rendered under the h1 (AUTO-656). The h1 shows
  // the locale-picked form, so that one is excluded to avoid a duplicate.
  const secondaryTitles = computed(() =>
    anime.value ? secondaryTitleFormsPure(anime.value, anime.value.title) : []
  )

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr)
    // REVIEW.md WR-05: respect the i18n locale instead of hardcoding ru-RU.
    // English / Japanese users previously saw review and comment dates
    // formatted as Russian (e.g. "13 мая 2026 г."). Map the active vue-i18n
    // locale to a BCP-47 tag accepted by Intl. The pattern mirrors the
    // formatNextEpisode helper below.
    const loc = locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US'
    return date.toLocaleDateString(loc, {
      day: 'numeric',
      month: 'long',
      year: 'numeric',
    })
  }

  // An announced/upcoming title with no resolved sources hasn't aired yet —
  // so "no available videos" is misleading. Show a premiere notice instead.
  // hasVideo guards the rare case of an announced title with an early leak.
  const notReleasedYet = computed(() => {
    const s = anime.value?.status?.toLowerCase()
    return (s === 'announced' || s === 'upcoming') && !anime.value?.hasVideo
  })
  const premiereDate = computed(() =>
    anime.value?.airedOn ? formatDate(anime.value.airedOn) : ''
  )

  const formatReviewStats = (review: Review): string => formatReviewStatsPure(review, t)

  const isReviewFlagged = (review: Review): boolean => isReviewFlaggedPure(review)

  const formatNextEpisode = (dateStr: string) => {
    const date = new Date(dateStr)
    const now = new Date()
    const diffMs = date.getTime() - now.getTime()
    const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))
    const tz = userTimezone.value

    const timeStr = date.toLocaleTimeString('ru-RU', {
      hour: '2-digit',
      minute: '2-digit',
      timeZone: tz
    })

    if (diffDays === 0) return t('anime.todayAt', { time: timeStr })
    if (diffDays === 1) return t('anime.tomorrowAt', { time: timeStr })
    if (diffDays > 1 && diffDays < 7) {
      const dayKeys = ['sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday']
      return t('anime.dayAtTime', { day: t(`schedule.daysWhen.${dayKeys[wallClockDate(date, tz).getDay()]}`), time: timeStr })
    }
    return t('anime.timeAt', { time: date.toLocaleDateString(locale.value === 'ru' ? 'ru-RU' : locale.value === 'ja' ? 'ja-JP' : 'en-US', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit', timeZone: tz }), tz: formatUtcOffset(tz) })
  }

  const formatEpisodeCount = (a: { episodesAired?: number; totalEpisodes?: number; status?: string }) =>
    formatEpisodeCountPure(a, t)

  // Phase 14 / UX-28 — render compact watchers count ("1.2K", "12K") via the
  // platform's Intl.NumberFormat. Locale-aware: passes the active i18n locale
  // so the grouping/notation matches the user's language settings.
  const formatCount = (n: number): string => formatCountPure(n, locale.value)

  return {
    statusVariant,
    parsedDescription,
    isHentai,
    secondaryTitles,
    notReleasedYet,
    premiereDate,
    formatDate,
    formatNextEpisode,
    formatEpisodeCount,
    formatCount,
    formatReviewStats,
    isReviewFlagged,
  }
}
