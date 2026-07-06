<template>
  <!-- Installed-PWA bottom tab bar. In standalone mode there is no browser
       chrome (no back button, no URL bar), so navigation must live in-app.
       Mobile-only + standalone-only by design: the browser tab keeps its own
       chrome and the desktop PWA keeps the top navbar. -->
  <nav
    v-if="isStandalone && isMobile"
    class="fixed inset-x-0 bottom-0 z-40 px-4"
    :aria-label="$t('nav.drawerTitle')"
    data-test="mobile-tabbar"
  >
    <div class="tabbar-capsule glass-nav glass-mobile-nav rounded-2xl border border-white/10 flex items-stretch">
      <button
        type="button"
        class="tab-item"
        data-test="tab-back"
        :aria-label="$t('nav.back')"
        @click="router.back()"
      >
        <ArrowLeft class="size-5" aria-hidden="true" />
        <span class="tab-label">{{ $t('nav.back') }}</span>
      </button>
      <RouterLink
        v-for="it in items"
        :key="it.to"
        :to="it.to"
        class="tab-item"
        :class="{ 'tab-item--active': isActive(it.to) }"
        :data-test="`tab-${it.key}`"
      >
        <component :is="it.icon" class="size-5" aria-hidden="true" />
        <span class="tab-label">{{ $t(it.label) }}</span>
      </RouterLink>
    </div>
  </nav>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter, RouterLink } from 'vue-router'
import { ArrowLeft, Home, LayoutGrid, Download, User } from 'lucide-vue-next'
import { useStandaloneDisplay } from '@/pwa/standalone'
import { useMobilePlayer } from '@/composables/aePlayer/useMobilePlayer'
import { downloadsNavVisible } from '@/offline/downloadGate'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const authStore = useAuthStore()
const isStandalone = useStandaloneDisplay()
const { isMobile } = useMobilePlayer()

const items = computed(() => [
  { key: 'home', to: '/', icon: Home, label: 'nav.home' },
  { key: 'browse', to: '/browse', icon: LayoutGrid, label: 'nav.catalog' },
  // Browser view hides downloads (secret-feature pool covers it); the
  // installed PWA keeps the tab — offline playback lives there.
  ...(downloadsNavVisible()
    ? [{ key: 'downloads', to: '/downloads', icon: Download, label: 'nav.downloads' }]
    : []),
  authStore.isAuthenticated
    ? { key: 'profile', to: '/profile', icon: User, label: 'nav.profile' }
    : { key: 'profile', to: '/auth', icon: User, label: 'nav.login' },
])

function isActive(to: string): boolean {
  if (to === '/') return route.path === '/'
  return route.path === to || route.path.startsWith(`${to}/`)
}
</script>

<style scoped>
.tabbar-capsule {
  margin-bottom: max(env(safe-area-inset-bottom), 12px);
}

.tab-item {
  flex: 1;
  min-width: 0;
  min-height: 56px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  border: 0;
  background: transparent;
  color: var(--muted-foreground);
  cursor: pointer;
  transition: color 0.15s;
}

.tab-item:hover {
  color: var(--foreground);
}

.tab-item--active {
  color: var(--brand-cyan);
}

.tab-label {
  font-size: 10.5px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  max-width: 100%;
}
</style>
