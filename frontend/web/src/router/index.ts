import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import i18n from '@/i18n'
import { tryReloadOnChunkError } from '@/utils/chunk-reload'
import { GACHA_ADMIN_ONLY } from '@/utils/gachaGate'

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
    // Workstream notifications / Phase 3 — backend ships watch_url as
    // /anime/{id}/watch?player=&episode=&translation= but the frontend
    // route is /anime/:id (which consumes the same query params). This
    // alias redirects without 404'ing for any code path that pushes the
    // raw watch_url (e.g. future email/Telegram deep links). The store's
    // translateWatchUrl helper produces the canonical /anime/:id?... shape
    // directly; this alias is belt + suspenders.
    path: '/anime/:id/watch',
    redirect: (to) => ({ path: `/anime/${to.params.id}`, query: to.query }),
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
    // Workstream watch-together / Phase 2 (WT-SHELL-01) — synchronized
    // watch-with-friends room. The view at /views/WatchTogetherView.vue
    // fetches the room snapshot via REST, opens a WS through the
    // useWatchTogetherRoom composable, and dispatches to one of the 5
    // existing <*Player> components based on room.player.
    //
    // NOT requiresAuth: a logged-out user opening an invite link must be able
    // to JOIN the room as a guest. WatchTogetherView mints an ephemeral guest
    // identity (auth.ensureGuestToken → POST /auth/guest) before connecting;
    // the guest token is kept out of the global auth.token so isAuthenticated
    // stays false (guests can sync + chat + react, but can't create rooms —
    // the create/Invite button stays gated on isAuthenticated). The room host
    // still logs in to create the room in the first place.
    //
    // Lazy-loaded so the WatchTogether dependency graph (composable +
    // sidebar + 5 player components via defineAsyncComponent) is
    // chunk-isolated and only paid for on this route — WT-NF-04 budget.
    path: '/watch/room/:roomId',
    name: 'watch-together-room',
    component: () => import('@/views/WatchTogetherView.vue'),
    meta: { titleKey: 'watch_together.title' }
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
    // Phase 14 / UX-30 — public About / FAQ page.
    path: '/about',
    name: 'about',
    component: () => import('@/views/About.vue'),
    meta: { titleKey: 'about.title' }
  },
  {
    // Admin dashboard landing — Vue replacement for the old hardcoded gateway
    // HTML page. Lists every admin tool as a styled card. Non-strict matching
    // covers both `/admin` and `/admin/`. Auth enforced by the guard below
    // (meta.requiresAdmin) AND upstream by the gateway /admin JWT group.
    path: '/admin',
    name: 'admin-dashboard',
    component: () => import('@/views/admin/AdminDashboard.vue'),
    meta: { titleKey: 'admin.dashboard.title', requiresAuth: true, requiresAdmin: true }
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
    // Phase 17 (UX-33) — editorial collections admin list.
    path: '/admin/collections',
    name: 'admin-collections',
    component: () => import('@/views/admin/AdminCollections.vue'),
    meta: { titleKey: 'admin.collections.title', requiresAuth: true, requiresAdmin: true }
  },
  {
    // Phase 17 (UX-33) — editorial collections edit form.
    // :id === 'new' triggers the create flow.
    path: '/admin/collections/:id',
    name: 'admin-collection-edit',
    component: () => import('@/views/admin/AdminCollectionEdit.vue'),
    meta: { titleKey: 'admin.collections.editTitle', requiresAuth: true, requiresAdmin: true }
  },
  {
    // Workstream raw-jp v0.2 Phase 05 (LIB-09): raw-library admin UI.
    // Operator-only surface for the self-hosted torrent → HLS pipeline:
    // search Nyaa + AnimeTosho, queue magnets, monitor jobs, link orphans.
    path: '/admin/raw-library',
    name: 'admin-raw-library',
    component: () => import('@/views/admin/RawLibrary.vue'),
    meta: { titleKey: 'player.adminLibrary.title', requiresAuth: true, requiresAdmin: true }
  },
  {
    // User-facing "my feedback" page — own messages + triage status.
    path: '/my-feedback',
    name: 'my-feedback',
    component: () => import('@/views/MyFeedback.vue'),
    meta: { titleKey: 'myFeedback.title', requiresAuth: true }
  },
  {
    // Admin feedback browser — read + triage user feedback / error reports.
    path: '/admin/feedback',
    name: 'admin-feedback',
    component: () => import('@/views/admin/AdminFeedback.vue'),
    meta: { titleKey: 'admin.feedback.title', requiresAuth: true, requiresAdmin: true }
  },
  {
    // Phase 17 (UX-33) — public editorial collection detail page.
    path: '/collections/:slug',
    name: 'collection-detail',
    component: () => import('@/views/Collections.vue'),
    meta: { titleKey: 'collections.title' }
  },
  // ── Gacha «Лудка» routes (dark-shipped via VITE_GACHA_ADMIN_ONLY) ──────────
  {
    path: '/gacha',
    name: 'gacha',
    component: () => import('@/views/Gacha.vue'),
    meta: { titleKey: 'gacha.nav_item', requiresAuth: true, gachaGated: true }
  },
  {
    // Legacy per-banner route → all-in-one page with the banner preselected.
    path: '/gacha/:id',
    name: 'gacha-banner',
    redirect: (to) => ({ path: '/gacha', query: { banner: String(to.params.id) } })
  },
  {
    path: '/admin/gacha',
    name: 'admin-gacha',
    component: () => import('@/views/admin/AdminGacha.vue'),
    meta: { titleKey: 'gacha.admin.title', requiresAuth: true, requiresAdmin: true }
  },
  {
    path: '/:pathMatch(.*)*',
    name: 'not-found',
    component: () => import('@/views/NotFound.vue'),
    meta: { titleKey: 'notFound.title' }
  }
]

if (typeof window !== 'undefined' && 'scrollRestoration' in window.history) {
  window.history.scrollRestoration = 'manual'
}

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
  //
  // Phase 12 / UA-100: surface a "not admin" message before the silent
  // redirect. The router runs outside any component tree, so we hand off
  // via sessionStorage; App.vue picks it up on the next mount and shows a
  // dismissible red banner. Cleared after one read.
  if (to.meta.requiresAdmin && !authStore.isAdmin) {
    try {
      sessionStorage.setItem('admin_redirect_reason', 'admin.errors.notAdmin')
    } catch {
      // sessionStorage can throw in privacy modes — silent failure is OK.
    }
    next({ name: 'home' })
    return
  }

  // Gacha «Лудка» gate: gachaGated routes are only visible to admins when
  // VITE_GACHA_ADMIN_ONLY is true (dark-ship), or to any authenticated user
  // when false (global release). Non-eligible users are redirected home.
  if (to.meta.gachaGated) {
    const gachaVisible = GACHA_ADMIN_ONLY ? authStore.isAdmin : authStore.isAuthenticated
    if (!gachaVisible) {
      next({ name: 'home' })
      return
    }
  }

  next()
})

// Auto-recover when lazy-loaded route chunks fail after a deploy (old JS/CSS
// files are replaced with new hashed versions). vue-router aborts the failed
// navigation WITHOUT committing the URL, so we hand the intended destination
// (`to.fullPath`) to the recovery helper — it does a full page load of that
// route, landing the user on the page they clicked rather than reloading the
// origin route (which dumped them back on "/"). Only catches errors during
// route navigation; defineAsyncComponent failures inside views surface as
// unhandledrejection — see main.ts.
router.onError((error, to) => {
  tryReloadOnChunkError(error, to?.fullPath)
})

// Clickstream pageview on every successful navigation (Plan 2). Lazy import
// keeps analytics out of the router's critical path.
router.afterEach((to) => {
  if (import.meta.env.VITE_ANALYTICS_ENABLED === 'false') return
  import('@/analytics').then(({ analytics }) => {
    analytics.page({ route: typeof to.name === 'string' ? to.name : undefined })
  }).catch(() => undefined)
})

export default router
