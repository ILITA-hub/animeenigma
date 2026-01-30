import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { MotionPlugin } from '@vueuse/motion'
import router from './router'
import App from './App.vue'

// Styles
import './styles/main.css'
import 'video.js/dist/video-js.css'

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

// Create i18n instance
const i18n = createI18n({
  legacy: false,
  locale: getDefaultLocale(),
  fallbackLocale: 'en',
  messages: { ru, ja, en },
})

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(i18n)
app.use(MotionPlugin)

app.mount('#app')
