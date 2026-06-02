import { reactive } from 'vue'
import type { ReferenceElement } from 'reka-ui'

interface ContextMenuAnime {
  id: string | number
  title: string
  name?: string
  nameRu?: string
  nameJp?: string
  coverImage: string
  rating?: number
  releaseYear?: number
  episodes?: number
  status?: string
  genres?: string[]
  rawGenres?: { name?: string; nameRu?: string }[]
}

interface ContextMenuState {
  visible: boolean
  // x/y retained for back-compat with the consumer prop bindings (:x/:y);
  // positioning now flows through `anchorEl` (Reka anchored mode — DS-LIB-08).
  x: number
  y: number
  // Reka anchored-positioning source: the kebab element (click) or a virtual
  // element at the touch point (mobile long-press). Forwarded to DropdownMenu
  // :reference so the menu anchors to the trigger instead of the cursor.
  anchorEl: ReferenceElement | null
  anime: ContextMenuAnime | null
  listStatus: string | null
  siteRating: { average_score: number; total_reviews: number } | null
  episodesWatched: number | null
  episodesTotal: number | null
}

interface OpenOpts {
  listStatus?: string | null
  siteRating?: { average_score: number; total_reviews: number } | null
  episodesWatched?: number | null
  episodesTotal?: number | null
}

export function useContextMenu() {
  const contextMenu = reactive<ContextMenuState>({
    visible: false,
    x: 0,
    y: 0,
    anchorEl: null,
    anime: null,
    listStatus: null,
    siteRating: null,
    episodesWatched: null,
    episodesTotal: null,
  })

  function close() {
    contextMenu.visible = false
    // matches the addEventListener capture flag used in openAtElement
    document.removeEventListener('scroll', close, true)
  }

  function openAtElement(el: HTMLElement, anime: ContextMenuAnime, opts?: OpenOpts) {
    const r = el.getBoundingClientRect()
    contextMenu.x = r.right + 4
    contextMenu.y = r.top
    // Anchor the Reka menu to the kebab element itself.
    contextMenu.anchorEl = el
    contextMenu.anime = anime
    contextMenu.listStatus = opts?.listStatus ?? null
    contextMenu.siteRating = opts?.siteRating ?? null
    contextMenu.episodesWatched = opts?.episodesWatched ?? null
    contextMenu.episodesTotal = opts?.episodesTotal ?? null
    contextMenu.visible = true
    // capture phase on document so nested scroll containers (Home column max-h
    // overflow-y-auto) also dismiss the menu — scroll events do not bubble.
    document.addEventListener('scroll', close, { passive: true, once: true, capture: true })
  }

  // Long-press helpers for mobile
  let longPressTimer: number | null = null

  function onTouchstart(event: TouchEvent, anime: ContextMenuAnime, opts?: OpenOpts) {
    longPressTimer = window.setTimeout(() => {
      const touch = event.touches[0]
      const x = touch.clientX
      const y = touch.clientY
      contextMenu.x = x
      contextMenu.y = y
      // Build a zero-size virtual reference element at the touch point so the
      // Reka anchored menu opens there — a menu, NOT a cursor menu. No
      // preventDefault on the native context menu (long-press on touch does not
      // conflict with desktop right-click — DS-LIB-08 keeps native right-click).
      contextMenu.anchorEl = {
        getBoundingClientRect: () =>
          ({ x, y, top: y, left: x, right: x, bottom: y, width: 0, height: 0 }) as DOMRect,
      }
      contextMenu.anime = anime
      contextMenu.listStatus = opts?.listStatus ?? null
      contextMenu.siteRating = opts?.siteRating ?? null
      contextMenu.episodesWatched = opts?.episodesWatched ?? null
      contextMenu.episodesTotal = opts?.episodesTotal ?? null
      contextMenu.visible = true
    }, 500)
  }

  function onTouchmove() {
    if (longPressTimer) {
      clearTimeout(longPressTimer)
      longPressTimer = null
    }
  }

  function onTouchend() {
    if (longPressTimer) {
      clearTimeout(longPressTimer)
      longPressTimer = null
    }
  }

  return { contextMenu, openAtElement, close, onTouchstart, onTouchmove, onTouchend }
}
