import { createRouter, createWebHashHistory } from "vue-router"; 
import MainPage from "./pages/MainPage.vue"; 
import AuthPage from "./pages/AuthPage.vue"; 
import RoomsPage from "./pages/RoomsPage.vue"; 


export default createRouter ({ 
  history: createWebHashHistory(), 
  routes: [ 
    { path: '/main', component: MainPage, alias: '/' },
    { path: '/auth', component: AuthPage }, 
    { path: '/rooms', component: RoomsPage},
  ] 
})
