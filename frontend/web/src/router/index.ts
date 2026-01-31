import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import Home from '@/views/Home.vue'
import Browse from '@/views/Browse.vue'
import Anime from '@/views/Anime.vue'
import Watch from '@/views/Watch.vue'
import Game from '@/views/Game.vue'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: Home,
    meta: { title: 'Home' }
  },
  {
    path: '/auth',
    name: 'auth',
    component: () => import('@/views/Auth.vue'),
    meta: { title: 'Вход' }
  },
  {
    path: '/browse',
    name: 'browse',
    component: Browse,
    meta: { title: 'Browse Anime' }
  },
  {
    path: '/search',
    name: 'search',
    component: Browse,
    meta: { title: 'Search Anime' }
  },
  {
    path: '/anime/:id',
    name: 'anime',
    component: Anime,
    meta: { title: 'Anime Details' }
  },
  {
    path: '/watch/:animeId/:episodeId',
    name: 'watch',
    component: Watch,
    meta: { title: 'Watch', requiresAuth: false }
  },
  {
    path: '/profile',
    name: 'profile',
    component: () => import('@/views/Profile.vue'),
    meta: { title: 'Profile', requiresAuth: true }
  },
  {
    path: '/game',
    name: 'game',
    component: Game,
    meta: { title: 'Game Rooms' }
  },
  {
    path: '/game/:roomId',
    name: 'game-room',
    component: Game,
    meta: { title: 'Game Room' }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFound.vue'),
    meta: { title: 'Not Found' }
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior(_to, _from, savedPosition) {
    if (savedPosition) {
      return savedPosition
    } else {
      return { top: 0 }
    }
  }
})

// Navigation guards
router.beforeEach((to, _from, next) => {
  const authStore = useAuthStore()

  // Update page title
  document.title = to.meta.title
    ? `${to.meta.title} - AnimeEnigma`
    : 'AnimeEnigma'

  // Check authentication
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    next({ name: 'home' })
  } else {
    next()
  }
})

export default router
