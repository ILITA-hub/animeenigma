import { onBeforeUnmount, type Ref } from 'vue'

export interface LazySection {
  /** Always-rendered element (or sentinel) the observer watches. */
  el: Ref<HTMLElement | null>
  /**
   * Fired once as the element approaches the viewport (and in the jsdom /
   * no-IntersectionObserver fallback). Must carry its own once-guard.
   */
  trigger: () => void
}

/**
 * Lazy below-the-fold loaders (page-fetch optimization 2026-06-11 — extracted
 * from Anime.vue). The reviews feed / related rail / characters rail render
 * far below the player; fetching them on mount put multiple requests (plus a
 * ~500ms Shikimori upstream call) on the critical path. One
 * IntersectionObserver with a generous rootMargin fires each trigger once as
 * the user approaches.
 */
export function useLazyAnimeSections(sections: LazySection[]) {
  let lazySectionObserver: IntersectionObserver | null = null

  function disarmLazySections() {
    lazySectionObserver?.disconnect()
    lazySectionObserver = null
  }

  function armLazySections() {
    disarmLazySections()
    if (typeof IntersectionObserver === 'undefined') {
      // jsdom / ancient browser — keep the eager behavior.
      for (const s of sections) s.trigger()
      return
    }
    lazySectionObserver = new IntersectionObserver(
      (entries) => {
        for (const entry of entries) {
          if (!entry.isIntersecting) continue
          const section = sections.find((s) => s.el.value === entry.target)
          if (section) {
            section.trigger()
            lazySectionObserver?.unobserve(entry.target)
          }
        }
      },
      // Prefetch well before the section enters the viewport so the content is
      // there by the time the user arrives (and the QuickNav anchor exists).
      { rootMargin: '1200px 0px' },
    )
    for (const s of sections) {
      if (s.el.value) lazySectionObserver.observe(s.el.value)
    }
  }

  onBeforeUnmount(disarmLazySections)

  return { armLazySections, disarmLazySections }
}
