import { createI18n } from 'vue-i18n'
import { watch } from 'vue'

// Locale messages
import ru from './locales/ru.json'
import ja from './locales/ja.json'
import en from './locales/en.json'

// Detect user's preferred language
function getDefaultLocale(): string {
  const saved = localStorage.getItem('locale')
  if (saved && ['ru', 'ja', 'en'].includes(saved)) {
    return saved
  }

  const browserLang = navigator.language.split('-')[0]
  if (['ru', 'ja', 'en'].includes(browserLang)) {
    return browserLang
  }

  return 'ru' // Default to Russian
}

const initialLocale = getDefaultLocale()

// Create i18n instance
const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: 'en',
  messages: { ru, ja, en },
})

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
