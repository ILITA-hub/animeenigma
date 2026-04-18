import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import i18n from '@/i18n'

const routes: RouteRecordRaw[] = [
  {
    path: '/',
    name: 'home',
    component: () => import('@/views/Home.vue'),
    meta: { titleKey: 'nav.home' }
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
    meta: { titleKey: 'nav.catalog' }
  },
  {
    path: '/search',
    redirect: (to) => ({ path: '/browse', query: to.query }),
  },
  {
    path: '/anime/:id',
    name: 'anime',
    component: () => import('@/views/Anime.vue'),
    meta: { titleKey: 'anime.detailsTitle' }
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
    meta: { titleKey: 'themes.title' }
  },
  {
    path: '/game',
    name: 'game',
    component: () => import('@/views/Game.vue'),
    meta: { titleKey: 'rooms.title' }
  },
  {
    path: '/game/:roomId',
    name: 'game-room',
    component: () => import('@/views/Game.vue'),
    meta: { titleKey: 'rooms.title' }
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
    meta: { titleKey: 'notFound.title' }
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

// Auto-reload when lazy-loaded chunks fail after a deploy
// (old JS/CSS files are replaced with new hashed versions)
router.onError((error, to) => {
  const chunkFailed = /Loading (CSS )?chunk [\w-]+ failed|Failed to fetch dynamically imported module|error loading dynamically imported module/i.test(error.message)
  if (chunkFailed) {
    window.location.assign(to.fullPath)
  }
})

export default router
