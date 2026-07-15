import { ref, computed, onMounted, onUnmounted, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Anime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { useWatchlistStore } from '@/stores/watchlist'
import { useToast } from '@/composables/useToast'
import { userApi } from '@/api/client'

/**
 * Watchlist status + rewatch-count state for the anime page (extracted from
 * Anime.vue): the status dropdown (with click-outside close), optimistic
 * status set/remove, the manual rewatch-count stepper, and the legacy
 * per-endpoint watchlist fetch fallback.
 */
export function useAnimeListStatus(anime: Ref<Anime | null>) {
  const { t } = useI18n()
  const authStore = useAuthStore()
  const watchlistStore = useWatchlistStore()
  const toast = useToast()

  const currentListStatus = ref<string | null>(null)
  const currentRewatchCount = ref(0)
  const showStatusDropdown = ref(false)
  const dropdownRef = ref<HTMLElement | null>(null)

  const statusLabels = computed((): Record<string, string> => ({
    watching: t('profile.watchlist.watching'),
    plan_to_watch: t('profile.watchlist.planToWatch'),
    completed: t('profile.watchlist.completed'),
    on_hold: t('profile.watchlist.onHold'),
    dropped: t('profile.watchlist.dropped'),
  }))

  // Manual rewatch-count stepper (row inside the status dropdown menu).
  // Optimistic; the PUT carries the current status so the entry isn't moved.
  async function setRewatchCount(n: number) {
    if (!anime.value || !currentListStatus.value) return
    const prior = currentRewatchCount.value
    currentRewatchCount.value = n
    try {
      await userApi.updateWatchlistEntry({
        anime_id: anime.value.id,
        status: currentListStatus.value,
        rewatch_count: n,
      })
    } catch (err) {
      console.error('Failed to update rewatch count:', err)
      currentRewatchCount.value = prior
      toast.push(t('watchlist.errors.updateFailed'))
    }
  }

  const fetchWatchlistStatus = async () => {
    if (!authStore.isAuthenticated || !anime.value) return

    try {
      await watchlistStore.fetchWatchlist()
      const entries = watchlistStore.entries

      // Direct UUID match
      let entry = entries.find((e) => e.anime_id === anime.value?.id)

      // If not found, check for mal_XXXXX entries that match this anime's MAL ID
      if (!entry && anime.value.malId) {
        const malAnimeId = `mal_${anime.value.malId}`
        entry = entries.find((e) => e.anime_id === malAnimeId)

        if (entry) {
          // Auto-migrate from mal_XXXXX to real UUID
          try {
            await userApi.migrateListEntry(
              malAnimeId,
              anime.value.id
            )
          } catch (e) {
            console.warn('Auto-migration of MAL entry failed:', e)
          }
        }
      }

      if (entry) {
        currentListStatus.value = entry.status
        currentRewatchCount.value = (entry as { rewatch_count?: number }).rewatch_count ?? 0
      } else {
        currentListStatus.value = null
        currentRewatchCount.value = 0
      }
    } catch (err) {
      console.error('Failed to fetch watchlist status:', err)
    }
  }

  const setListStatus = async (status: string) => {
    if (!anime.value) return
    const animeId = anime.value.id
    const prior = currentListStatus.value
    // Optimistic: flip the visible status + close the dropdown immediately.
    currentListStatus.value = status
    showStatusDropdown.value = false
    try {
      await watchlistStore.setStatusOptimistic(animeId, status)
    } catch (err) {
      console.error('Failed to update list status:', err)
      // Rollback the view-local mirror (store action already rolled back its map).
      currentListStatus.value = prior
      toast.push(t('watchlist.errors.updateFailed'))
    }
  }

  const removeFromList = async () => {
    if (!anime.value) return
    const animeId = anime.value.id
    const prior = currentListStatus.value
    const priorCount = currentRewatchCount.value
    // Optimistic: clear the visible status + close the dropdown immediately.
    currentListStatus.value = null
    currentRewatchCount.value = 0
    showStatusDropdown.value = false
    try {
      await watchlistStore.removeEntryOptimistic(animeId)
    } catch (err) {
      console.error('Failed to remove from list:', err)
      currentListStatus.value = prior
      currentRewatchCount.value = priorCount
      toast.push(t('watchlist.errors.removeFailed'))
    }
  }

  // Close dropdown on click outside
  const handleClickOutside = (event: MouseEvent) => {
    if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
      showStatusDropdown.value = false
    }
  }

  onMounted(() => {
    document.addEventListener('click', handleClickOutside)
  })

  onUnmounted(() => {
    document.removeEventListener('click', handleClickOutside)
  })

  /** Per-anime reset (route param change) — mirrors the loadAnimeData reset block. */
  function reset() {
    currentListStatus.value = null
    currentRewatchCount.value = 0
    showStatusDropdown.value = false
  }

  return {
    currentListStatus,
    currentRewatchCount,
    showStatusDropdown,
    dropdownRef,
    statusLabels,
    setRewatchCount,
    fetchWatchlistStatus,
    setListStatus,
    removeFromList,
    reset,
  }
}
