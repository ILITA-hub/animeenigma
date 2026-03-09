import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { MotionPlugin } from '@vueuse/motion'
import router from './router'
import i18n from './i18n'
import App from './App.vue'

// Diagnostics (must init before other imports that use console)
import { initDiagnostics } from './utils/diagnostics'
initDiagnostics()

// Styles
import './styles/main.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(i18n)
app.use(MotionPlugin)

app.mount('#app')
