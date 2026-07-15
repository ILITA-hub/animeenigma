import { ref, type Ref } from 'vue'
import type { Anime } from '@/composables/useAnime'
import { useAuthStore } from '@/stores/auth'
import { animeApi, adminApi } from '@/api/client'
import type { AnimeWithExtras } from './types'

/**
 * Admin-only tools for the anime page (extracted from Anime.vue): the admin
 * kebab (Refresh / Hide / Shikimori ID), hidden-status toggle, and the inline
 * Shikimori-ID edit panel.
 */
export function useAnimeAdmin(
  anime: Ref<Anime | null>,
  fetchAnime: (id: string) => Promise<Anime | null>,
) {
  const authStore = useAuthStore()

  const refreshing = ref(false)
  const isHidden = ref(false)
  const showShikimoriEdit = ref(false)
  // Admin kebab (Refresh / Hide / Shikimori ID) — admin-only, grouped out of the
  // user action row. Controlled open state for the DropdownMenu #trigger.
  const showAdminMenu = ref(false)
  const editShikimoriId = ref('')
  const savingShikimoriId = ref(false)

  const fetchHiddenStatus = () => {
    // Hidden status comes from the anime object itself
    if (anime.value) {
      isHidden.value = (anime.value as AnimeWithExtras).hidden || false
      editShikimoriId.value = (anime.value as AnimeWithExtras).shikimoriId || ''
    }
  }

  const toggleHidden = async () => {
    if (!anime.value || !authStore.isAdmin) return

    try {
      if (isHidden.value) {
        await adminApi.unhideAnime(anime.value.id)
        isHidden.value = false
      } else {
        await adminApi.hideAnime(anime.value.id)
        isHidden.value = true
      }
    } catch (err) {
      console.error('Failed to toggle hidden status:', err)
    }
  }

  const saveShikimoriId = async () => {
    if (!anime.value || !authStore.isAdmin || savingShikimoriId.value) return

    savingShikimoriId.value = true
    try {
      await adminApi.updateShikimoriId(anime.value.id, editShikimoriId.value)
      showShikimoriEdit.value = false
      // Refresh anime data to get updated translations
      await fetchAnime(anime.value.id)
    } catch (err) {
      console.error('Failed to update Shikimori ID:', err)
    } finally {
      savingShikimoriId.value = false
    }
  }

  const refreshAnimeData = async () => {
    if (!anime.value || refreshing.value) return

    refreshing.value = true
    try {
      await animeApi.refresh(anime.value.id)
      // Refetch anime data to show updated info
      await fetchAnime(anime.value.id)
    } catch (err) {
      console.error('Failed to refresh anime data:', err)
    } finally {
      refreshing.value = false
    }
  }

  return {
    refreshing,
    isHidden,
    showShikimoriEdit,
    showAdminMenu,
    editShikimoriId,
    savingShikimoriId,
    fetchHiddenStatus,
    toggleHidden,
    saveShikimoriId,
    refreshAnimeData,
  }
}
