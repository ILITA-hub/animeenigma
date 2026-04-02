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
}

export function useContextMenu() {
  const contextMenu = reactive<ContextMenuState>({
    visible: false,
    x: 0,
    y: 0,
    anime: null,
    listStatus: null,
    siteRating: null,
  })

  function open(
    event: MouseEvent,
    anime: ContextMenuAnime,
    opts?: { listStatus?: string | null; siteRating?: { average_score: number; total_reviews: number } | null }
  ) {
    event.preventDefault()
    contextMenu.x = event.clientX
    contextMenu.y = event.clientY
    contextMenu.anime = anime
    contextMenu.listStatus = opts?.listStatus ?? null
    contextMenu.siteRating = opts?.siteRating ?? null
    contextMenu.visible = true
  }

  function close() {
    contextMenu.visible = false
  }

  // Long-press helpers for mobile
  let longPressTimer: number | null = null

  function onTouchstart(
    event: TouchEvent,
    anime: ContextMenuAnime,
    opts?: { listStatus?: string | null; siteRating?: { average_score: number; total_reviews: number } | null }
  ) {
    longPressTimer = window.setTimeout(() => {
      const touch = event.touches[0]
      contextMenu.x = touch.clientX
      contextMenu.y = touch.clientY
      contextMenu.anime = anime
      contextMenu.listStatus = opts?.listStatus ?? null
      contextMenu.siteRating = opts?.siteRating ?? null
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

  return { contextMenu, open, close, onTouchstart, onTouchmove, onTouchend }
}
