import { createRouter, createWebHashHistory } from "vue-router"; 
import MainPage from "./pages/MainPage.vue"; 
import AuthPage from "./pages/AuthPage.vue"; 

export default createRouter ({ 
  history: createWebHashHistory(), 
  routes: [ 
    { path: '/main', component: MainPage, alias: '/' },
    { path: '/auth', component: AuthPage }, 
    // Другие маршруты...
  ] 
})
