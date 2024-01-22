import './assets/main.css'
import 'vuetify/styles'

import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import vuetify from '@/plugins/vuetify.js'
import axios from '@/plugins/axios.ts'
import sockets from '@/plugins/sockets.ts'

const app = createApp(App) 

import VueSocketIO from 'vue-socket.io'

app.use(createPinia())
app.use(router)
app.use(vuetify)
app.use(axios, {
  baseUrl: '/api/',
})
app.use(sockets, {
  baseUrl: '/ws/',
})

app.mount('#app')
