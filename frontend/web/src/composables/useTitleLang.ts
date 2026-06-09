import { ref } from 'vue'

/**
 * Catalog title-language preference. Lets users see English/romaji anime titles
 * even when the UI locale is Russian (Russian titles aren't always recognizable).
 *
 * 'auto' defers to the global UI locale; 'ru'/'en' pin a language. The value is a
 * module-level singleton ref so the toggle reactively re-renders every anime card
 * title across the app (catalog, home, continue-watching) through getLocalizedTitle.
 */
export type TitleLang = 'auto' | 'ru' | 'en'

const STORAGE_KEY = 'titleLang'

function readInitial(): TitleLang {
  const v = localStorage.getItem(STORAGE_KEY)
  return v === 'ru' || v === 'en' ? v : 'auto'
}

const titleLang = ref<TitleLang>(readInitial())

export function setTitleLang(v: TitleLang): void {
  titleLang.value = v
  if (v === 'auto') localStorage.removeItem(STORAGE_KEY)
  else localStorage.setItem(STORAGE_KEY, v)
}

export function useTitleLang() {
  return { titleLang, setTitleLang }
}
