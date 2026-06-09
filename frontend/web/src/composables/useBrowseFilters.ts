import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

// Phase 15 (UX-31) — multi-axis browse filter state. The composable owns
// both directions of the URL <-> state sync: ?route.query is the source of
// truth on mount + on browser back/forward; in-place changes (sidebar
// click) call writeUrl() to mutate ?route.query and the watcher re-reads.
//
// Important: the composable does NOT call the network. It owns state +
// URL; the consumer (Browse.vue) decides when to reload — typically a
// watch(filters.apiParams, () => loadAnime()) drives re-fetch. This keeps
// the composable trivially test-mockable and the existing useAnime
// composable untouched.

export type Kind = '' | 'tv' | 'movie' | 'ova' | 'ona' | 'special'
export type Provider = 'kodik' | 'animelib' | 'english'
export type Sort = 'popularity' | 'rating' | 'year' | 'updated' | 'title'

const KIND_VALUES: Kind[] = ['', 'tv', 'movie', 'ova', 'ona', 'special']
const PROVIDER_VALUES: Provider[] = ['kodik', 'animelib', 'english']
const SORT_VALUES: Sort[] = ['popularity', 'rating', 'year', 'updated', 'title']
const STATUS_VALUES = ['', 'ongoing', 'released', 'announced'] as const
type Status = (typeof STATUS_VALUES)[number]

export function useBrowseFilters() {
  const route = useRoute()
  const router = useRouter()

  const q = ref('')
  const genres = ref<string[]>([])
  const kind = ref<Kind>('')
  const status = ref<Status>('')
  const yearFrom = ref<number | null>(null)
  const yearTo = ref<number | null>(null)
  const providers = ref<Provider[]>([])
  const sort = ref<Sort>('popularity')
  const scoreMin = ref<number | null>(null)

  // Internal lock so writeUrl()'s router.replace doesn't trigger our own
  // watcher to re-read and stomp the in-progress write.
  let suppressNextWatch = false

  function readUrl() {
    const qry = route.query
    q.value = (typeof qry.q === 'string' ? qry.q : '') || ''
    // Guard array assignments: only replace the ref when content actually
    // changes. router.replace({page: N}) hits this watcher but leaves filter
    // params untouched — assigning a new [] here would dirty apiParams and
    // fire the Browse.vue deep watcher, resetting currentPage to 1.
    const newGenres = ((qry.genre as string) || '')
      .split(',')
      .map(s => s.trim())
      .filter(Boolean)
    if (newGenres.join(',') !== genres.value.join(',')) {
      genres.value = newGenres
    }
    const rawKind = typeof qry.kind === 'string' ? (qry.kind as Kind) : ''
    kind.value = KIND_VALUES.includes(rawKind) ? rawKind : ''
    const rawStatus = (typeof qry.status === 'string' ? qry.status : '') as Status
    status.value = (STATUS_VALUES as readonly string[]).includes(rawStatus) ? rawStatus : ''
    yearFrom.value = qry.year_from ? parseInt(qry.year_from as string, 10) || null : null
    yearTo.value = qry.year_to ? parseInt(qry.year_to as string, 10) || null : null
    const newProviders = ((qry.providers as string) || '')
      .split(',')
      .map(s => s.trim().toLowerCase())
      .filter((p): p is Provider => PROVIDER_VALUES.includes(p as Provider))
    if (newProviders.join(',') !== providers.value.join(',')) {
      providers.value = newProviders
    }
    const rawSort = typeof qry.sort === 'string' ? (qry.sort as Sort) : 'popularity'
    sort.value = SORT_VALUES.includes(rawSort) ? rawSort : 'popularity'
    scoreMin.value = qry.score_min ? parseFloat(qry.score_min as string) || null : null
  }

  function writeUrl() {
    // Build the next query, preserving any keys the composable doesn't
    // own (e.g. page is owned by Browse.vue's pagination). We always
    // strip ?page since changing filters reverts to page 1.
    const next: Record<string, string | undefined> = { ...route.query, page: undefined } as Record<
      string,
      string | undefined
    >
    next.q = q.value || undefined
    next.genre = genres.value.length ? genres.value.join(',') : undefined
    next.kind = kind.value || undefined
    next.status = status.value || undefined
    next.year_from = yearFrom.value ? String(yearFrom.value) : undefined
    next.year_to = yearTo.value ? String(yearTo.value) : undefined
    next.providers = providers.value.length ? providers.value.join(',') : undefined
    next.sort = sort.value !== 'popularity' ? sort.value : undefined
    next.score_min = scoreMin.value ? String(scoreMin.value) : undefined
    suppressNextWatch = true
    router.replace({ query: next })
  }

  // Computed API params — feeds animeApi.getAnimeList. Mirrors the
  // backend whitelist exactly; sidebar values that fall outside the
  // whitelist are dropped at readUrl(), so we don't re-filter here.
  const apiParams = computed(() => {
    const p: Record<string, string | number> = { sort: sort.value }
    if (q.value) p.q = q.value
    if (genres.value.length) p.genre = genres.value.join(',')
    if (kind.value) p.kind = kind.value
    if (status.value) p.status = status.value
    if (yearFrom.value) p.year_from = yearFrom.value
    if (yearTo.value) p.year_to = yearTo.value
    if (providers.value.length) p.providers = providers.value.join(',')
    if (scoreMin.value) p.score_min = scoreMin.value
    return p
  })

  // Active filter count for the mobile toggle badge. The search query
  // and sort axis are intentionally EXCLUDED from the count — they are
  // not "narrowing filters" in the UX sense (sort never narrows; the
  // search input has its own input affordance).
  const activeCount = computed(() => {
    let n = 0
    if (genres.value.length) n++
    if (kind.value) n++
    if (status.value) n++
    if (yearFrom.value || yearTo.value) n++
    if (providers.value.length) n++
    if (scoreMin.value) n++
    return n
  })

  function reset() {
    q.value = ''
    genres.value = []
    kind.value = ''
    status.value = ''
    yearFrom.value = null
    yearTo.value = null
    providers.value = []
    scoreMin.value = null
    sort.value = 'popularity'
    writeUrl()
  }

  onMounted(readUrl)

  // Browser back/forward — re-read URL when ?route.query changes outside
  // our own writeUrl(). The suppressNextWatch guard prevents echoing.
  watch(
    () => route.query,
    () => {
      if (suppressNextWatch) {
        suppressNextWatch = false
        return
      }
      readUrl()
    },
    { deep: true },
  )

  return {
    q,
    genres,
    kind,
    status,
    yearFrom,
    yearTo,
    providers,
    sort,
    scoreMin,
    apiParams,
    activeCount,
    writeUrl,
    reset,
    readUrl,
  }
}
