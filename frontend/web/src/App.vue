<template>
  <div id="app" class="min-h-screen bg-base">
    <!-- A11y: first Tab stop. Visually hidden until keyboard-focused. -->
    <a
      href="#main-content"
      class="sr-only focus:not-sr-only focus:fixed focus:top-3 focus:left-3 focus:z-[100] focus:px-4 focus:py-2 focus:rounded-lg focus:bg-brand-violet focus:text-white focus:text-sm focus:font-medium"
    >
      {{ $t('a11y.skipToContent') }}
    </a>
    <!-- Design-system Phase 3 / Wave 3: a single app-root TooltipProvider so any
         <Tooltip> primitive has its required ancestor app-wide. Reka's
         TooltipProvider renders as a transparent Slot (no extra DOM node), so it
         does NOT introduce a wrapper element or shift layout — #app stays the
         single root. -->
    <TooltipProvider :delay-duration="300">
    <!-- Desktop Navbar -->
    <Navbar />

    <!-- Error Fallback -->
    <div v-if="appError" class="min-h-screen flex items-center justify-center px-4">
      <div class="text-center max-w-md">
        <TriangleAlert class="size-16 mx-auto mb-4 text-pink-400" aria-hidden="true" />
        <h2 class="text-xl font-semibold text-white mb-2">Something went wrong</h2>
        <p class="text-white/60 text-sm mb-6">{{ appError.message }}</p>
        <button
          @click="reloadPage"
          class="px-6 py-2.5 bg-brand-violet hover:bg-brand-violet/90 text-white rounded-lg font-medium transition-colors"
        >
          Reload
        </button>
      </div>
    </div>

    <!-- Phase 12 / UA-100: admin-guard redirect notice.
         The router redirects non-admin users home on /admin/* and stashes
         a key in sessionStorage; we surface a brief dismissible banner so
         the redirect isn't silent. Auto-dismisses after 6 s. -->
    <div
      v-if="adminRedirectKey"
      role="alert"
      class="fixed top-20 left-1/2 -translate-x-1/2 z-50 max-w-md w-[calc(100%-2rem)] bg-destructive/90 text-white px-4 py-3 rounded-lg shadow-lg flex items-start gap-3"
    >
      <span class="flex-1 text-sm">{{ $t(adminRedirectKey) }}</span>
      <button
        type="button"
        class="text-white/80 hover:text-white text-lg leading-none"
        :aria-label="$t('system.statusBanner.dismiss')"
        @click="adminRedirectKey = null"
      >
        ×
      </button>
    </div>

    <!-- Main Content.
         The ONE place that offsets pages below the fixed Navbar (value =
         --header-offset in main.css; body already pads by safe-top). Pages
         must NOT re-add their own header offset. Routes whose design runs
         behind the transparent header (full-bleed hero) opt out via route
         meta.fullBleed and own their clearance.
         v-if (not v-else): a v-else would pair with the conditional banner
         above and unmount the page while the banner shows. -->
    <main
      v-if="!appError"
      id="main-content"
      tabindex="-1"
      :class="{ 'pt-[var(--header-offset)]': !route.meta.fullBleed, 'pb-24': tabBarVisible }"
    >
      <router-view v-slot="{ Component }">
        <Transition name="page">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>

    <!-- Phase 13 / UX-27: global toast renderer for optimistic-action rollbacks -->
    <Toaster />

    <!-- Global promise-based confirm() host (useConfirm) — themed replacement
         for native window.confirm(). Mounted once, like <Toaster />. -->
    <ConfirmDialogHost />

    <!-- Card-launched season download flow (context menu → quality dialog → engine). -->
    <SeasonDownloadHost />

    <!-- Installed-PWA bottom navigation (standalone mode has no browser chrome). -->
    <MobileTabBar v-if="!appError" />

    <!-- Workstream notifications / Phase 3 — slide-in toast for the latest
         undismissed notification. Mounted at App-root so it survives route
         transitions. Gated by the feature flag so VITE_NOTIFICATIONS_ENABLED=
         false fully disables the surface. -->
    <NotificationToast v-if="notifEnabled" />

    <!-- Footer -->
    <footer v-if="!appError" class="py-8 px-4 text-center border-t border-white/10" :class="{ 'pb-24': tabBarVisible }">
      <div class="max-w-7xl mx-auto flex flex-wrap items-center justify-center gap-x-3 gap-y-2">
        <p class="text-white/60 text-sm">
          &copy; {{ new Date().getFullYear() }} AnimeEnigma. {{ $t('footer.rights') }}
        </p>
        <template v-if="commitHash">
          <span class="text-brand-cyan/30 text-sm select-none" aria-hidden="true">&bull;</span>
          <a
            :href="commitUrl"
            target="_blank"
            rel="noopener noreferrer"
            :title="$t('footer.build')"
            class="text-white/60 hover:text-white/80 text-sm font-mono transition-colors"
          >
            {{ $t('footer.build') }} {{ commitHash }}
          </a>
        </template>
        <span class="text-brand-cyan/30 text-sm select-none" aria-hidden="true">&bull;</span>
        <FeedbackButton />
        <template v-if="MY_FEEDBACK_ENABLED && authStore.isAuthenticated">
          <span class="text-brand-cyan/30 text-sm select-none" aria-hidden="true">&bull;</span>
          <router-link
            to="/my-feedback"
            class="inline-flex items-center gap-1.5 text-white/60 hover:text-white/80 text-sm transition-colors"
          >
            <Inbox class="size-4" aria-hidden="true" />
            {{ $t('footer.feedback.viewMine') }}
          </router-link>
        </template>
        <div class="flex items-center gap-3 sm:ml-auto">
          <a
            href="https://t.me/anime_enigma"
            target="_blank"
            rel="noopener noreferrer"
            :aria-label="$t('footer.social.telegram')"
            class="text-muted-foreground hover:text-brand-cyan transition-colors"
          >
            <Send class="size-4" aria-hidden="true" />
          </a>
          <a
            href="https://github.com/ILITA-hub/animeenigma"
            target="_blank"
            rel="noopener noreferrer"
            :aria-label="$t('footer.social.github')"
            class="text-muted-foreground hover:text-brand-cyan transition-colors"
          >
            <Github class="size-4" aria-hidden="true" />
          </a>
          <a
            href="mailto:info@animeenigma.org"
            :aria-label="$t('footer.social.email')"
            class="text-muted-foreground hover:text-brand-cyan transition-colors"
          >
            <Mail class="size-4" aria-hidden="true" />
          </a>
        </div>
      </div>
    </footer>

    </TooltipProvider>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onErrorCaptured, ref, watch } from 'vue'
import { TriangleAlert, Inbox, Send, Github, Mail } from 'lucide-vue-next'
import { TooltipProvider } from 'reka-ui'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { useNotificationsStore } from '@/stores/notifications'
import Navbar from '@/components/layout/Navbar.vue'
import FeedbackButton from '@/components/layout/FeedbackButton.vue'
import Toaster from '@/components/ui/Toaster.vue'
import ConfirmDialogHost from '@/components/ui/ConfirmDialogHost.vue'
import NotificationToast from '@/components/NotificationToast.vue'
import MobileTabBar from '@/components/layout/MobileTabBar.vue'
import SeasonDownloadHost from '@/components/SeasonDownloadHost.vue'
import { useStandaloneDisplay } from '@/pwa/standalone'
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
import { tryReloadOnChunkError } from '@/utils/chunk-reload'
import { reportFeError } from '@/utils/feErrorLog'

const authStore = useAuthStore()
const router = useRouter()
const route = useRoute()
const notifStore = useNotificationsStore()

// Workstream notifications / Phase 3 — feature flag baked at build time.
// When false (rollback), skip mounting the toast AND skip starting the
// store's polling lifecycle. Defaults to true.
const notifEnabled = (import.meta.env.VITE_NOTIFICATIONS_ENABLED as string | undefined) !== 'false'

// Installed-PWA bottom tab bar (standalone has no browser chrome). When it is
// visible, main/footer get bottom clearance so content never hides behind it.
const isStandalone = useStandaloneDisplay()
const { isMobile: isMobileViewport } = useMobilePlayer()
const tabBarVisible = computed(() => isStandalone.value && isMobileViewport.value)

// "My feedback" footer link re-enabled per owner approval of AUTO-436
// (2026-06-11, in-chat). FeedbackButton.vue has the same flag.
const MY_FEEDBACK_ENABLED = true

// Short git commit hash of the deployed build, baked in by `make redeploy-web`
// (Dockerfile ARG VITE_GIT_COMMIT). Empty in dev / builds without a checkout.
const commitHash = (import.meta.env.VITE_GIT_COMMIT ?? '').trim()
const commitUrl = commitHash
  ? `https://github.com/ILITA-hub/animeenigma/commit/${commitHash}`
  : ''

// Auth-driven lifecycle: start polling on login, stop + clear state on
// logout. immediate=true so an already-authenticated tab on page-load
// also kicks the engine. The store's start() is idempotent.
watch(
  () => authStore.isAuthenticated,
  (isAuth) => {
    if (!notifEnabled) return
    if (isAuth) {
      notifStore.start()
    } else {
      notifStore.stop()
    }
  },
  { immediate: true },
)

const appError = ref<Error | null>(null)

// Phase 12 / UA-100: pick up the admin-guard redirect reason that
// router/index.ts stashed in sessionStorage before redirecting home.
// Cleared on first read so the banner doesn't persist across navigation.
const adminRedirectKey = ref<string | null>(null)

function consumeAdminRedirectKey() {
  try {
    const key = sessionStorage.getItem('admin_redirect_reason')
    if (key) {
      sessionStorage.removeItem('admin_redirect_reason')
      adminRedirectKey.value = key
      // Auto-dismiss after 6 s so the banner doesn't linger forever.
      window.setTimeout(() => {
        adminRedirectKey.value = null
      }, 6000)
    }
  } catch {
    // sessionStorage may throw in privacy modes — silent failure is OK.
  }
}

onErrorCaptured((err) => {
  // Stale-chunk recovery: lazy-loaded async components (e.g. the per-player
  // defineAsyncComponent imports) that fail because their hashed chunk was
  // replaced by a newer deploy surface HERE, not at the global
  // unhandledrejection handler in main.ts — this boundary intercepts them
  // first. Without this, the user dead-ends on the generic "Something went
  // wrong" screen showing "Failed to fetch dynamically imported module".
  // Reload to the fresh index.html + new hashed asset names instead. The
  // helper's cooldown guards against an infinite reload loop when the asset
  // is genuinely gone server-side.
  if (tryReloadOnChunkError(err)) {
    return false
  }
  appError.value = err instanceof Error ? err : new Error(String(err))
  reportFeError({
    kind: 'vue',
    message: appError.value.message,
    stack: appError.value.stack,
  })
  console.error('[App Error]', err)
  return false // prevent propagation
})

const reloadPage = () => window.location.reload()

// Initialize auth state - fetch user if we have token but no user data
onMounted(async () => {
  consumeAdminRedirectKey()
  if (authStore.token && !authStore.user) {
    await authStore.fetchUser()
  }
})

// Phase 12 / UA-100: also surface after route changes, since the
// admin-guard sets sessionStorage during navigation (not initial mount).
router.afterEach(() => {
  consumeAdminRedirectKey()
})
</script>

<style scoped>
.page-enter-active {
  transition: opacity 0.2s ease;
}

.page-enter-from {
  opacity: 0;
}

main:focus-visible {
  box-shadow: none;
}
</style>
