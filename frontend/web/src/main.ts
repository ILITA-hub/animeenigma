import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import i18n from './i18n'
import App from './App.vue'
import { useAuthStore } from './stores/auth'
import { tryReloadOnChunkError } from './utils/chunk-reload'
import { reportFeError, installFeErrorTraps } from './utils/feErrorLog'
import { initAssetEdge } from './utils/assetEdge'

// Styles
import './styles/main.css'

const app = createApp(App)

// app.config.errorHandler catches Vue errors NOT intercepted by the App.vue
// onErrorCaptured boundary (e.g. errors during the boundary's own render).
app.config.errorHandler = (err, _instance, info) => {
  const e = err instanceof Error ? err : new Error(String(err))
  if (tryReloadOnChunkError(e)) return
  reportFeError({ kind: 'vue', message: e.message, stack: e.stack, source: String(info) })
  console.error('[Vue Error]', err, info)
}

const pinia = createPinia()
app.use(pinia)
app.use(router)
app.use(i18n)

// Cross-domain magic-link SSO landing: magic-link-login set the httpOnly session
// cookies plus a readable one-shot `ae_sso=1` marker cookie. THIS origin's
// localStorage is empty (the user logged in on a different domain, e.g.
// animeenigma.ru), so without a nudge the app renders logged-out despite valid
// cookies. When the marker is present we adopt the session from the
// refresh_token cookie (one /auth/refresh) BEFORE mount — so router guards and
// the first render see the authenticated state — then delete the marker cookie.
// The URL is never touched (no ?ae_sso clutter). Non-SSO loads are untouched.
function readCookie(name: string): string | undefined {
  return document.cookie
    .split('; ')
    .find((c) => c.startsWith(name + '='))
    ?.slice(name.length + 1)
}

async function bootstrap() {
  if (readCookie('ae_sso') === '1') {
    // One-shot: clear the marker immediately so a reload won't re-trigger.
    document.cookie = 'ae_sso=; Path=/; Max-Age=0; Secure; SameSite=Lax'
    const auth = useAuthStore(pinia)
    if (!auth.token) {
      try {
        await auth.refreshAccessToken()
      } catch {
        // Stale/absent cookie — fall through and render anonymously.
      }
    }
  }
  app.mount('#app')
}

void bootstrap()

// Uncaught window errors → backend log (gated + volume-capped inside the util).
installFeErrorTraps()

// RU static-edge asset routing: probe origin vs edge once and cache the
// decision (applied by the inline bootstrap in index.html on the next load).
// No-op unless VITE_MSK_ASSET_HOST was set at build time.
initAssetEdge()

window.addEventListener('unhandledrejection', (event) => {
  // defineAsyncComponent failures after a deploy surface here as
  // "Unable to preload CSS for ..." or "Failed to fetch dynamically imported
  // module" — reload to pick up the new hashed asset names.
  if (tryReloadOnChunkError(event.reason)) {
    event.preventDefault()
    return
  }
  const reason = event.reason
  reportFeError({
    kind: 'unhandledrejection',
    message: reason instanceof Error ? reason.message : String(reason),
    stack: reason instanceof Error ? reason.stack : undefined,
  })
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

// Defer PWA/offline service-worker registration too — same idle window as
// diagnostics/analytics above.
deferInit(() => {
  void import('./pwa/registerPwa').then((m) => m.initPwa())
})
