import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import i18n from '@/i18n'
import { tryReloadOnChunkError } from '@/utils/chunk-reload'

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
    // Phase 14 (REC-ADMIN-01 / REC-ADMIN-02): admin debug page for the recs
    // engine. Route guard rejects non-admin users via meta.requiresAdmin.
    path: '/admin/recs/:user_id',
    name: 'admin-recs',
    component: () => import('@/views/admin/AdminRecs.vue'),
    meta: { titleKey: 'admin.recs.title', requiresAuth: true, requiresAdmin: true }
  },
  {
    // Picker view for the admin recs debug page. Reachable from the gateway
    // /admin landing HTML when the admin clicks "Rec Engine Debug" without
    // a known user_id in hand. Accepts username, public_id, or UUID and
    // redirects to /admin/recs/{input}.
    path: '/admin/recs',
    name: 'admin-recs-picker',
    component: () => import('@/views/admin/AdminRecsPicker.vue'),
    meta: { titleKey: 'admin.recs.title', requiresAuth: true, requiresAdmin: true }
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
  scrollBehavior(to, from, savedPosition) {
    if (savedPosition) {
      return savedPosition
    }
    // Same route, only query/hash changed (e.g. ?ugc= tab switch) — preserve scroll position
    if (to.path === from.path) {
      return false
    }
    return { top: 0 }
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
    return
  }

  // Phase 14: requiresAdmin gate. Non-admin users are sent home.
  // The route guard is purely UX — the actual security boundary is the
  // gateway + player AdminRoleMiddleware (defense-in-depth).
  if (to.meta.requiresAdmin && !authStore.isAdmin) {
    next({ name: 'home' })
    return
  }

  next()
})

// Auto-reload when lazy-loaded chunks fail after a deploy
// (old JS/CSS files are replaced with new hashed versions).
// Only catches errors during route navigation; defineAsyncComponent failures
// inside views surface as unhandledrejection — see main.ts.
router.onError((error) => {
  tryReloadOnChunkError(error)
})

export default router
