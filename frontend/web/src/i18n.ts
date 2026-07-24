import { createI18n } from 'vue-i18n'
import { watch } from 'vue'
import { setInterfaceLocale } from '@/composables/useInterfaceLocale'

// Bundle-size optimization (2026-06-11): only the default locale ships in the
// entry bundle. en/ja (~130KB raw JSON) used to be statically imported here,
// putting all three locale files in the critical-path entry chunk for every
// visitor. Non-ru locales now lazy-load via setLocale; their chunks are
// content-hashed + immutable-cached, so the cost is one parallel request on
// the FIRST visit only.
import ru from './locales/ru.json'

export const SUPPORTED_LOCALES = ['ru', 'en', 'ja'] as const

// Russian pluralization (one / few / many). vue-i18n's built-in rule is
// English-style (singular vs. plural), which mis-picks branches for Russian
// — e.g. it would render "1 отзывов" instead of "1 отзыв". Russian count
// strings must carry three branches in this exact order:
//   one  → "{n} отзыв"    (1, 21, 31, … but NOT 11)
//   few  → "{n} отзыва"   (2–4, 22–24, … but NOT 12–14)
//   many → "{n} отзывов"  (0, 5–20, 25–30, …)
// Returns the zero-based branch index. Two-branch messages (rare in ru)
// fall back to one/other so they never throw.
function russianPlural(choice: number, choicesLength: number): number {
  const n = Math.abs(choice)
  const mod10 = n % 10
  const mod100 = n % 100

  if (choicesLength < 3) {
    return n === 1 ? 0 : 1
  }
  if (mod10 === 1 && mod100 !== 11) return 0 // one
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return 1 // few
  return 2 // many
}

// Lazy loaders — explicit map (not a template-string import) so Vite doesn't
// also emit a duplicate chunk for ru.json, which stays statically bundled.
const localeLoaders: Record<string, () => Promise<{ default: unknown }>> = {
  en: () => import('./locales/en.json'),
  ja: () => import('./locales/ja.json'),
}

// Detect user's preferred language
function getDefaultLocale(): string {
  const saved = localStorage.getItem('locale')
  if (saved && (SUPPORTED_LOCALES as readonly string[]).includes(saved)) {
    return saved
  }

  const browserLang = navigator.language.split('-')[0]
  if ((SUPPORTED_LOCALES as readonly string[]).includes(browserLang)) {
    return browserLang
  }

  return 'ru' // Default to Russian
}

const initialLocale = getDefaultLocale()

// Create i18n instance. The instance always BOOTS in ru (the only locale
// whose messages are available synchronously); a saved/browser en/ja
// preference is applied via setLocale below as soon as its messages land —
// at worst a sub-second flash of Russian on a cold first visit.
// fallbackLocale is ru (was en): ru is the primary, most complete locale and
// the only one guaranteed loaded.
const i18n = createI18n({
  legacy: false,
  locale: 'ru',
  fallbackLocale: 'ru',
  messages: { ru },
  // Per-locale plural-branch selection. Only ru needs a custom rule; en/ja
  // keep vue-i18n's default (en: singular/plural, ja: no plurals).
  pluralRules: { ru: russianPlural },
})

const loadedLocales = new Set<string>(['ru'])

/**
 * Switch the active locale, lazy-loading its messages on first use.
 * The single entry point for locale changes (Navbar language menu + boot).
 */
export async function setLocale(code: string): Promise<void> {
  if (!(SUPPORTED_LOCALES as readonly string[]).includes(code)) return
  if (!loadedLocales.has(code)) {
    try {
      const messages = await localeLoaders[code]()
      // en/ja share ru's key schema (vue-i18n types messages from the
      // statically-imported ru) — the locale files are hand-kept in parity.
      i18n.global.setLocaleMessage(code, messages.default as typeof ru)
      loadedLocales.add(code)
    } catch {
      // Chunk fetch failed (offline / mid-deploy) — stay on the current
      // locale rather than rendering bare translation keys.
      return
    }
  }
  const localeRef = i18n.global.locale as unknown as { value: string }
  localeRef.value = code
  setInterfaceLocale(code)
}

if (initialLocale !== 'ru') {
  void setLocale(initialLocale)
}

// Keep <html lang> in sync with the active locale (accessibility, SEO, browser translation)
if (typeof document !== 'undefined') {
  document.documentElement.lang = initialLocale
  watch(
    () => (i18n.global.locale as unknown as { value: string }).value,
    (newLocale) => {
      if (newLocale) document.documentElement.lang = newLocale
    },
  )
}

export default i18n
