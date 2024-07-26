import { createRouter, createWebHistory } from "vue-router"; 
import MainPage from "./pages/MainPage.vue"; 
import AuthPage from "./pages/AuthPage.vue"; 
import RoomsPage from "./pages/RoomsPage.vue"; 
import CreateRoom from "./pages/CreateRoom.vue";
import CollectionsPage from "./pages/CollectionsPage.vue";
import CreateCollection from "./pages/CreateCollection.vue";
import UserPage from "./pages/UserPage.vue";
import CollectSelect from "./pages/CollectSelect.vue";
import { useAuthStore } from './stores/authStore';

const router = createRouter({ 
  history: createWebHistory(), 
  routes: [ 
    { path: '/main', component: MainPage, alias: '/' },
    { path: '/auth', component: AuthPage }, 
    { path: '/rooms', component: RoomsPage},
    { path: '/createroom', component: CreateRoom},
    { path: '/collections', component: CollectionsPage},
    { path: '/custom-collections', component: CreateCollection},
    { path: '/user', component: UserPage },
    { path: '/collect-select', component: CollectSelect }, 
  ] 
});

let isDirectNavigation = true;

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore(); 

  if (isDirectNavigation) {
    to.meta.isDirectNavigation = true;
  } else {
    to.meta.isDirectNavigation = false;
  }
  isDirectNavigation = false;
  if (to.path === '/user' && !authStore.isAuthenticated) {
    next('/auth');
  } else {
    next();
  }
});

export default router;
