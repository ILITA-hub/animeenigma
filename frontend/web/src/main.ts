import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import i18n from './i18n'
import App from './App.vue'

// Styles
import './styles/main.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(i18n)
app.mount('#app')

// Defer diagnostics init to after first paint to reduce long task duration
const deferInit = window.requestIdleCallback || ((cb: () => void) => setTimeout(cb, 100))
deferInit(() => {
  import('./utils/diagnostics').then(({ initDiagnostics }) => initDiagnostics())
})
