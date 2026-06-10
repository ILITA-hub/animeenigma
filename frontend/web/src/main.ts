import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import i18n from './i18n'
import App from './App.vue'
import { tryReloadOnChunkError } from './utils/chunk-reload'

// Styles
import './styles/main.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(i18n)
app.mount('#app')

window.addEventListener('unhandledrejection', (event) => {
  // defineAsyncComponent failures after a deploy surface here as
  // "Unable to preload CSS for ..." or "Failed to fetch dynamically imported
  // module" — reload to pick up the new hashed asset names.
  if (tryReloadOnChunkError(event.reason)) {
    event.preventDefault()
    return
  }
  console.error('[Unhandled Promise Rejection]', event.reason)
})

// Defer diagnostics init to after first paint to reduce long task duration
const deferInit = window.requestIdleCallback || ((cb: () => void) => setTimeout(cb, 100))
deferInit(() => {
  import('./utils/diagnostics').then(({ initDiagnostics }) => initDiagnostics())
})

// Idle-load the Noto Sans JP @font-face declarations (230 unicode-range
// slices, ~250KB of CSS text) — keeping them out of the render-blocking
// main stylesheet. font-display:swap + unicode-range means JP glyphs render
// with the fallback font for a moment and swap in once this lands.
deferInit(() => {
  import('./styles/noto-sans-jp.css')
})

// Defer analytics (clickstream) init too — flag-gated, default on (only the
// string 'false' disables it). Lazy import keeps it off the critical path.
deferInit(() => {
  if (import.meta.env.VITE_ANALYTICS_ENABLED !== 'false') {
    import('./analytics').then(({ analytics }) => {
      const base = import.meta.env.VITE_API_URL || '/api'
      analytics.init({ endpoint: `${base}/analytics/collect` })
    })
  }
})
