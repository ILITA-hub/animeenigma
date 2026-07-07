/**
 * RBAC-and-roulette P4 — feature-visibility store.
 *
 * Boot-loaded once from `GET /api/policy/features/mine`
 * (`featuresApi.getFeaturesMine`, api/client.ts). `useFeatureVisible`
 * (composables/useFeatureVisible.ts) and the Task 2 router guard both read
 * this store to decide what the current user sees — nav items, dark-ship
 * routes (fanfic/gacha/profile-wall), and the footer roulette pool
 * (Task 3) — replacing the old VITE_*_ADMIN_ONLY build flags and the
 * client-side secret-features eligibility facade.
 *
 * `ready` resolves once the FIRST `load()` settles, success OR failure, so
 * awaiters (e.g. a router guard deciding whether to allow a dark-ship route)
 * always proceed — on failure `loaded` stays false and callers fall back to
 * the D1 fail-open rule (see resolveVisible in useFeatureVisible.ts).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { featuresApi } from '@/api/client'

export const useFeatureVisibilityStore = defineStore('featureVisibility', () => {
  const visible = ref<Set<string>>(new Set())
  const roulette = ref<string[]>([])
  const rouletteEnabled = ref(false)
  const loaded = ref(false)

  let readyResolve: () => void = () => {}
  const ready: Promise<void> = new Promise((resolve) => {
    readyResolve = resolve
  })

  // Guards against concurrent/double fetch: the first call kicks off the
  // request; every call after that (still in flight, or already settled)
  // gets the same promise back instead of re-fetching.
  let loadPromise: Promise<void> | null = null

  function load(): Promise<void> {
    if (loadPromise) return loadPromise
    loadPromise = (async () => {
      try {
        const data = await featuresApi.getFeaturesMine()
        visible.value = new Set(data.visible)
        roulette.value = data.roulette
        rouletteEnabled.value = data.rouletteEnabled
        loaded.value = true
      } catch (err) {
        // Fail-open: leave loaded=false so useFeatureVisible/resolveVisible
        // fall back to the D1 per-key failSafe. This is a boot-time
        // best-effort fetch, not a user-facing action — swallow, don't throw.
        console.error('featureVisibility: failed to load /policy/features/mine', err)
      } finally {
        readyResolve()
      }
    })()
    return loadPromise
  }

  return { visible, roulette, rouletteEnabled, loaded, ready, load }
})
