import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { OfflineDownload } from '@/offline/types'
import { listDownloads, putDownload } from '@/offline/registry'
import { engineState, markEvicted, removeDownload, pauseDownload, enqueueDownload, storageEstimate, isEngineWorking } from '@/offline/downloadEngine'
import { useProviderResolver } from '@/composables/aePlayer/useProviderResolver'
import { makeExternalSubResolver } from '@/offline/externalSubs'

export const useDownloadsStore = defineStore('downloads', () => {
  const entries = ref<OfflineDownload[]>([])
  const storage = ref<{ usage: number; quota: number } | null>(null)
  const loading = ref(false)

  async function refresh(): Promise<void> {
    loading.value = true
    try {
      const listed = await markEvicted(await listDownloads())
      for (const d of listed) {
        if ((d.state === 'downloading' || d.state === 'queued') && !isEngineWorking(d.id)) {
          d.state = 'paused' // interrupted by reload/crash — resumable via the existing button
          await putDownload(d)
        }
      }
      entries.value = listed
      storage.value = await storageEstimate() // via the OfflineMediaStore port (Task 7b)
    } finally {
      loading.value = false
    }
  }

  async function remove(id: string): Promise<void> {
    await removeDownload(id)
    await refresh()
  }

  function pause(id: string): void {
    pauseDownload(id)
    void refresh()
  }

  /** Resume a paused/failed download — needs network: re-resolves the stream
   *  via the live resolver with the entry's frozen combo; cached resources
   *  are skipped by the engine. */
  async function resume(d: OfflineDownload): Promise<void> {
    const resolver = useProviderResolver()
    await enqueueDownload({
      animeId: d.animeId,
      animeTitle: d.animeTitle,
      episode: d.episode,
      combo: d.combo,
      quality: d.quality,
      resolve: () => resolver.resolveStream(d.combo.provider, d.animeId, d.episode, d.combo),
      subPref: d.subPref,
      resolveSubs: makeExternalSubResolver(d.animeId, d.subPref)?.(d.episode),
    })
    await refresh()
  }

  /** animeId → downloads, newest anime first. */
  const byAnime = computed(() => {
    const groups = new Map<string, OfflineDownload[]>()
    for (const d of [...entries.value].sort((a, b) => a.episode.number - b.episode.number)) {
      const g = groups.get(d.animeId) ?? []
      g.push(d)
      groups.set(d.animeId, g)
    }
    return [...groups.entries()].sort(
      (a, b) => Math.max(...b[1].map((d) => d.createdAt)) - Math.max(...a[1].map((d) => d.createdAt)),
    )
  })

  return { entries, storage, loading, refresh, remove, pause, resume, byAnime, progress: engineState.progress }
})
