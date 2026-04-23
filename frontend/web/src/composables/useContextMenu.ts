import { reactive } from 'vue'

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
  x: number
  y: number
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
    anime: null,
    listStatus: null,
    siteRating: null,
    episodesWatched: null,
    episodesTotal: null,
  })

  function open(event: MouseEvent, anime: ContextMenuAnime, opts?: OpenOpts) {
    event.preventDefault()
    contextMenu.x = event.clientX
    contextMenu.y = event.clientY
    contextMenu.anime = anime
    contextMenu.listStatus = opts?.listStatus ?? null
    contextMenu.siteRating = opts?.siteRating ?? null
    contextMenu.episodesWatched = opts?.episodesWatched ?? null
    contextMenu.episodesTotal = opts?.episodesTotal ?? null
    contextMenu.visible = true
  }

  function close() {
    contextMenu.visible = false
    // matches the addEventListener capture flag used in openAtElement
    document.removeEventListener('scroll', close, true)
  }

  function openAtElement(el: HTMLElement, anime: ContextMenuAnime, opts?: OpenOpts) {
    const r = el.getBoundingClientRect()
    contextMenu.x = r.right + 4
    contextMenu.y = r.top
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
      contextMenu.x = touch.clientX
      contextMenu.y = touch.clientY
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

  return { contextMenu, open, openAtElement, close, onTouchstart, onTouchmove, onTouchend }
}
