import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import i18n from '@/i18n'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/Home.vue'),
    meta: { title: 'Home' }
  },
  {
    path: '/auth',
    name: 'auth',
    component: () => import('@/views/Auth.vue'),
    meta: { titleKey: 'nav.login' }
  },
  {
    path: '/browse',
    name: 'browse',
    component: () => import('@/views/Browse.vue'),
    meta: { title: 'Browse Anime' }
  },
  {
    path: '/search',
    redirect: (to) => ({ path: '/browse', query: to.query }),
  },
  {
    path: '/anime/:id',
    name: 'anime',
    component: () => import('@/views/Anime.vue'),
    meta: { title: 'Anime Details' }
  },
  {
    path: '/watch/:animeId/:episodeId',
    name: 'watch',
    component: () => import('@/views/Watch.vue'),
    meta: { title: 'Watch', requiresAuth: false }
  },
  {
    path: '/profile',
    name: 'profile',
    component: () => import('@/views/ProfileSetup.vue'),
    meta: { titleKey: 'nav.profile', requiresAuth: true },
    beforeEnter: (_to, _from, next) => {
      const authStore = useAuthStore()
      if (authStore.user?.public_id) {
        next(`/user/${authStore.user.public_id}`)
      } else {
        next()
      }
    }
  },
  {
    path: '/schedule',
    name: 'schedule',
    component: () => import('@/views/Schedule.vue'),
    meta: { titleKey: 'schedule.title' }
  },
  {
    path: '/themes',
    name: 'themes',
    component: () => import('@/views/Themes.vue'),
    meta: { title: 'Openings & Endings' }
  },
  {
    path: '/game',
    name: 'game',
    component: () => import('@/views/Game.vue'),
    meta: { title: 'Game Rooms' }
  },
  {
    path: '/game/:roomId',
    name: 'game-room',
    component: () => import('@/views/Game.vue'),
    meta: { title: 'Game Room' }
  },
  {
    path: '/user/:publicId',
    name: 'public-profile',
    component: () => import('@/views/Profile.vue'),
    meta: { titleKey: 'nav.profile' }
  },
  {
    path: '/status',
    name: 'status',
    component: () => import('@/views/StatusPage.vue'),
    meta: { titleKey: 'status.title' }
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
  const titleKey = to.meta.titleKey as string | undefined
  const title = to.meta.title as string | undefined
  if (titleKey) {
    document.title = `${i18n.global.t(titleKey)} - AnimeEnigma`
  } else if (title) {
    document.title = `${title} - AnimeEnigma`
  } else {
    document.title = 'AnimeEnigma'
  }

  // Check authentication
  if (to.meta.requiresAuth && !authStore.isAuthenticated) {
    sessionStorage.setItem('returnUrl', to.fullPath)
    next({ name: 'auth' })
  } else {
    next()
  }
})

export default router
