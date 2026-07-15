<template>
  <!-- Phase 11 / UX-23 — `navbar-root` class hook was added for Theater
       Mode on the anime detail view. Theater mode deliberately keeps the
       navbar visible (only the player section goes full-bleed, via
       `body.theater-mode [data-anime-player-wrapper]` in Anime.vue) — no
       rule targets `.navbar-root` to hide it. The hook remains mounted in
       case a future theater treatment needs it; Navbar itself stays
       stateless w.r.t. theater-mode.

       pr-[var(--scrollbar-width,0px)] consumes the CSS var reka-ui sets on
       <html> while a Select/DropdownMenu/Dialog body-locks scroll: the lock
       pads <body> right by the vanished scrollbar's width, and this fixed
       header must shrink by the same amount or its centered content visibly
       shifts. transition-transform (not -all) keeps the compensation
       instant — only the show/hide translate-y should animate. On mobile
       the scoped CSS below swaps the hide animation to an in-place fade
       (translate up would park the fixed capsule past the top edge and
       trigger iOS 26 Safari's opaque status-bar band — see style block).

       On mobile (< md) the header is a floating glass capsule — inset from
       the viewport edges like a browser URL bar — so content scrolls behind
       it through the blur. Desktop keeps the full-width glass-nav bar. The
       side inset (16px) matches the page gutter (px-4) so the capsule
       aligns with the content column; the bottom edge (8px top inset +
       56px row) matches the desktop header height, so page top offsets
       stay valid.

       The glass surface lives on an absolute CHILD, not on this fixed
       element: iOS 26 Safari samples background-color/backdrop-filter
       from fixed elements near the viewport edge to tint its status-bar
       chrome — glass directly on the fixed header gets sampled and
       painted as an opaque near-black band over the cutout zone,
       defeating viewport-fit=cover. A transparent fixed shell keeps
       Safari compositing real page pixels behind the status bar. -->
  <header
    :class="[
      'fixed top-2 inset-x-4 md:top-0 md:inset-x-0 z-50 pr-[var(--scrollbar-width,0px)] transition-transform duration-300 navbar-root',
      isVisible ? 'translate-y-0' : '-translate-y-full navbar-root--hidden',
    ]"
  >
    <div
      class="absolute inset-0 glass-nav glass-mobile-nav rounded-2xl border border-white/10 md:rounded-none md:border-0"
      aria-hidden="true"
    />
    <nav class="relative max-w-7xl mx-auto px-4 lg:px-8">
      <div class="flex items-center justify-between h-14 md:h-16">
        <!-- Left group: in-app Back (desktop PWA only) + brand logo -->
        <div class="flex items-center gap-1">
          <!-- Installed desktop PWA has no browser chrome (no back button),
               so provide an in-app Back control here. Standalone + desktop
               only: browser tabs keep their own back button, and mobile
               standalone relies on the OS back button + edge-swipe gesture. -->
          <button
            v-if="isStandalone && isDesktop"
            type="button"
            class="icon-btn-nt"
            data-test="app-back"
            :aria-label="$t('nav.back')"
            :title="$t('nav.back')"
            @click="router.back()"
          >
            <ArrowLeft class="size-5" />
          </button>
          <!-- Logo — BrandMark icon + wordmark -->
          <router-link to="/" class="brand-link">
            <BrandMark />
            <span class="brand-wordmark">
              <span class="brand-b1">Anime</span><span class="brand-b2">Enigma</span>
            </span>
            <span class="brand-beta">beta</span>
          </router-link>
        </div>

        <!-- Desktop Navigation -->
        <div class="hidden md:flex items-center gap-1">
          <router-link
            v-for="link in navLinks"
            :key="link.to"
            :to="link.to"
            class="nav-link-nt"
            :active-class="link.to === '/' ? '' : 'nav-link-nt--active'"
            :exact-active-class="link.to === '/' ? 'nav-link-nt--active' : ''"
          >
            {{ $t(link.label) }}
          </router-link>
          <!-- Gacha «Лудка» nav item — visibility resolved at runtime via the
               policy feed (useFeatureVisible / policy-service), not a build flag -->
          <router-link
            v-if="gachaVisible"
            to="/gacha"
            class="nav-link-nt"
            active-class="nav-link-nt--active"
          >
            {{ $t('gacha.nav_item') }}
          </router-link>
        </div>

        <!-- Right Section — Neon Tokyo tools row -->
        <div class="hidden md:flex items-center gap-1.5">
          <!-- Search -->
          <div class="relative" ref="searchContainerRef">
            <button
              v-if="!searchOpen"
              class="icon-btn-nt"
              :aria-label="$t('nav.search')"
              @click="openSearch"
            >
              <Search class="size-5" />
            </button>
            <div v-else class="flex items-center gap-2">
              <div class="relative w-64">
                <Input
                  ref="searchInputRef"
                  v-model="searchQuery"
                  type="text"
                  size="sm"
                  :placeholder="$t('search.placeholder')"
                  class="bg-white/10 border-white/20 focus:border-cyan-400/50 focus:bg-white/15"
                  @input="onSearchInput"
                  @keydown.enter="goToSearch"
                  @keydown.escape="closeSearch"
                  @keydown.down.prevent="highlightNext"
                  @keydown.up.prevent="highlightPrev"
                >
                  <template #prefix>
                    <Search class="size-4 text-white/60" aria-hidden="true" />
                  </template>
                </Input>
                <!-- Autocomplete Dropdown -->
                <Transition name="dropdown">
                  <div
                    v-if="searchResults.length > 0"
                    class="absolute top-full left-0 mt-2 min-w-[320px] rounded-xl overflow-hidden z-50 bg-background/95 backdrop-blur-xl border border-white/10 shadow-2xl"
                  >
                    <router-link
                      v-for="(result, index) in searchResults"
                      :key="result.id"
                      :to="`/anime/${result.id}`"
                      class="flex items-center gap-3 px-3 py-2.5 hover:bg-white/10 transition-colors"
                      :class="{ 'bg-white/10': highlightedIndex === index }"
                      @click="closeSearch"
                    >
                      <img
                        :src="result.coverImage"
                        :alt="result.title"
                        class="w-10 h-14 rounded-md object-cover flex-shrink-0"
                      />
                      <div class="flex-1 min-w-0">
                        <p class="text-white text-sm font-medium truncate">{{ result.title }}</p>
                        <p class="text-white/60 text-xs">
                          {{ result.releaseYear || '' }}{{ result.releaseYear && result.totalEpisodes ? ' \u00b7 ' : '' }}{{ result.totalEpisodes ? result.totalEpisodes + ' \u044d\u043f.' : '' }}
                        </p>
                      </div>
                      <span v-if="result.rating" class="flex items-center gap-0.5 text-warning text-xs font-medium flex-shrink-0">
                        <Star class="size-3" fill="currentColor" aria-hidden="true" />
                        {{ result.rating.toFixed(1) }}
                      </span>
                    </router-link>
                    <router-link
                      :to="{ path: '/browse', query: { q: searchQuery } }"
                      class="block px-3 py-2.5 text-center text-sm text-cyan-400 hover:bg-white/10 transition-colors border-t border-white/10"
                      @click="closeSearch"
                    >
                      {{ $t('search.viewAll') }}
                    </router-link>
                  </div>
                </Transition>
              </div>
              <button
                class="p-1.5 text-white/60 hover:text-white transition-colors"
                :aria-label="$t('nav.closeSearch')"
                @click="closeSearch"
              >
                <X class="size-4" />
              </button>
            </div>
          </div>

          <!-- Gacha balance chip — visibility resolved at runtime via the
               policy feed (useFeatureVisible / policy-service), not a build flag -->
          <router-link
            v-if="gachaVisible"
            to="/gacha"
            class="flex items-center gap-1.5 px-2.5 py-1 rounded-lg bg-orange-400/10 hover:bg-orange-400/20 transition-colors"
            :aria-label="$t('gacha.balance_chip_aria', { n: gachaBalance })"
            :title="$t('gacha.balance_tooltip')"
          >
            <Gem class="size-4 text-orange-400 flex-shrink-0" aria-hidden="true" />
            <span class="text-orange-300 text-sm font-medium tabular-nums">{{ gachaBalance }}</span>
          </router-link>

          <!-- Language Selector -->
          <div class="relative" ref="langDropdownRef">
            <button
              class="lang-pill-nt"
              @click="langDropdownOpen = !langDropdownOpen"
            >
              <span class="uppercase">{{ locale }}</span>
              <ChevronDown class="size-3.5" aria-hidden="true" />
            </button>
            <Transition name="dropdown">
              <div
                v-if="langDropdownOpen"
                class="absolute right-0 mt-2 py-2 w-32 rounded-xl bg-background/95 backdrop-blur-xl border border-white/10 shadow-2xl"
              >
                <button
                  v-for="lang in languages"
                  :key="lang.code"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-white/10 transition-colors"
                  :class="locale === lang.code ? 'text-cyan-400' : 'text-white/70'"
                  @click="setLocale(lang.code)"
                >
                  {{ lang.name }}
                </button>
              </div>
            </Transition>
          </div>

          <!-- Workstream notifications / Phase 3 — header bell. Visible only
               when logged in (anonymous users get no polling and no badge).
               Gated by the feature flag for rollback. -->
          <NotificationBell v-if="notifEnabled && authStore.isAuthenticated" />

          <!-- User Avatar / Login -->
          <template v-if="authStore.isAuthenticated">
            <router-link
              to="/profile"
              class="group flex-shrink-0"
              :aria-label="authStore.user?.username || $t('nav.profile')"
            >
              <Avatar
                :src="authStore.user?.avatar"
                :name="authStore.user?.username"
                size="sm"
                status="online"
                class="size-9 ring-2 ring-white/10 transition-shadow group-hover:ring-brand-cyan/30"
              />
            </router-link>
          </template>
          <template v-else>
            <Button size="sm" variant="outline" @click="showLogin">
              {{ $t('nav.login') }}
            </Button>
          </template>
        </div>

        <!-- Mobile Menu Button -->
        <button
          id="navbar-mobile-toggle"
          ref="hamburgerButtonRef"
          class="md:hidden p-3 min-w-[44px] min-h-[44px] flex items-center justify-center text-white/70 hover:text-white"
          @click="mobileMenuOpen = !mobileMenuOpen"
          :aria-label="mobileMenuOpen ? $t('nav.closeMenu') : $t('nav.openMenu')"
          :aria-expanded="mobileMenuOpen"
          aria-controls="mobile-drawer"
        >
          <Menu v-if="!mobileMenuOpen" class="size-6" />
          <X v-else class="size-6" />
        </button>
      </div>

      <!-- Mobile Menu -->
      <Transition name="mobile-menu">
        <div
          v-if="mobileMenuOpen"
          id="mobile-drawer"
          ref="drawerRef"
          role="dialog"
          aria-modal="true"
          aria-labelledby="mobile-drawer-title"
          class="md:hidden py-4 border-t border-white/10"
        >
          <h2 id="mobile-drawer-title" class="sr-only">{{ $t('nav.drawerTitle') }}</h2>
          <div class="flex flex-col gap-1">
            <router-link
              v-for="link in navLinks"
              :key="link.to"
              :to="link.to"
              class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              :active-class="link.to === '/' ? '' : 'text-cyan-400 bg-cyan-500/10'"
              :exact-active-class="link.to === '/' ? 'text-cyan-400 bg-cyan-500/10' : ''"
              @click="mobileMenuOpen = false"
            >
              {{ $t(link.label) }}
            </router-link>

            <!-- Gacha «Лудка» mobile link -->
            <router-link
              v-if="gachaVisible"
              to="/gacha"
              class="flex items-center gap-2 px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              active-class="text-orange-400 bg-orange-500/10"
              @click="mobileMenuOpen = false"
            >
              <Gem class="size-4 text-orange-400" aria-hidden="true" />
              {{ $t('gacha.nav_item') }}
              <span class="ml-auto text-orange-300 text-sm font-medium tabular-nums">{{ gachaBalance }}</span>
            </router-link>

            <!-- Divider -->
            <div class="my-1 border-t border-white/10" />

            <!-- Workstream notifications — full-width drawer row (matches
                 the Gacha row). The desktop bell's anchored dropdown cannot
                 fit a phone viewport from inside the drawer (it opened
                 off-screen left + below the fold), so tapping the row closes
                 the drawer and opens the list in the Modal below instead. -->
            <button
              v-if="notifEnabled && authStore.isAuthenticated"
              type="button"
              class="flex items-center gap-2 px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors text-left"
              :aria-label="notifAriaLabel"
              @click="openMobileNotifications"
            >
              <Bell class="size-4" aria-hidden="true" />
              {{ $t('notifications.bell.tooltip') }}
              <span
                v-if="notificationsStore.unreadCount > 0"
                aria-hidden="true"
                class="ml-auto min-w-[20px] h-5 px-1.5 rounded-full bg-pink-500 text-white text-xs font-semibold leading-none flex items-center justify-center tabular-nums"
              >
                {{ notifBadgeText }}
              </span>
            </button>

            <!-- Profile / Login -->
            <template v-if="authStore.isAuthenticated">
              <router-link
                to="/profile"
                class="flex items-center gap-3 px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
                active-class="text-cyan-400 bg-cyan-500/10"
                @click="mobileMenuOpen = false"
              >
                <Avatar
                  :src="authStore.user?.avatar"
                  :name="authStore.user?.username"
                  size="sm"
                  class="ring-1 ring-cyan-500/30"
                />
                {{ $t('nav.profile') }}
              </router-link>
            </template>
            <template v-else>
              <router-link
                to="/auth"
                class="px-4 py-3 text-cyan-400 hover:text-cyan-300 hover:bg-white/10 rounded-lg transition-colors font-medium"
                @click="() => { showLogin(); mobileMenuOpen = false }"
              >
                {{ $t('nav.login') }}
              </router-link>
            </template>

            <!-- Phase 14 / UX-30 — public About / FAQ link. No Footer
                 component exists in this project, so the drawer is the
                 canonical surface for the link. Mobile-only because the
                 desktop navbar is intentionally minimal; desktop users
                 reach /about via direct URL or footer pattern in later
                 phases. -->
            <router-link
              to="/about"
              class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              active-class="text-cyan-400 bg-cyan-500/10"
              @click="mobileMenuOpen = false"
            >
              {{ $t('nav.about') }}
            </router-link>

            <!-- Language -->
            <!-- UA-082 (UX-12 Phase 5): ButtonGroup wraps the mobile-drawer
                 language toggle with role="group" + aria-label; each lang
                 button binds aria-pressed to the active-locale state. -->
            <ButtonGroup
              :label="$t('nav.langToggleLabel')"
              container-class="flex items-center gap-2 px-4 py-3"
            >
              <button
                v-for="lang in languages"
                :key="lang.code"
                class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors"
                :class="locale === lang.code ? 'bg-cyan-500/20 text-cyan-400' : 'text-white/50 hover:text-white hover:bg-white/10'"
                :aria-pressed="locale === lang.code"
                @click="setLocale(lang.code)"
              >
                {{ lang.name }}
              </button>
            </ButtonGroup>
          </div>
        </div>
      </Transition>

      <!-- Mobile notifications host. Lives OUTSIDE the drawer so the panel
           survives the drawer closing; the Modal primitive already handles
           the scrim, Esc, refcounted scroll-lock, and the iOS 26
           fixed-element status-bar gotcha. -->
      <Modal
        v-if="notifEnabled && authStore.isAuthenticated"
        v-model="mobileNotifOpen"
        :title="$t('notifications.dropdown.title')"
        size="md"
      >
        <NotificationDropdown flat @close="mobileNotifOpen = false" />
      </Modal>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { setLocale as switchLocale } from '@/i18n'
import { useAuthStore } from '@/stores/auth'
import { onClickOutside, useDebounceFn, useEventListener, useMediaQuery } from '@vueuse/core'
import { animeApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { useBodyScrollLock } from '@/composables/useBodyScrollLock'
import { Search, X, ChevronDown, Menu, Gem, Star, Bell, ArrowLeft } from 'lucide-vue-next'
import Avatar from '@/components/ui/Avatar.vue'
import Button from '@/components/ui/Button.vue'
import ButtonGroup from '@/components/ui/ButtonGroup.vue'
import { Input, Modal } from '@/components/ui'
import NotificationBell from '@/components/NotificationBell.vue'
import NotificationDropdown from '@/components/NotificationDropdown.vue'
import BrandMark from '@/components/layout/BrandMark.vue'
import { useGachaVisible } from '@/utils/gachaGate'
import { useGachaStore } from '@/stores/gacha'
import { useNotificationsStore } from '@/stores/notifications'
import { useNotificationBadge } from '@/composables/useNotificationBadge'
import { downloadsNavVisible } from '@/offline/downloadGate'
import { useStandaloneDisplay } from '@/pwa/standalone'

const router = useRouter()
const { locale } = useI18n()
const authStore = useAuthStore()

// Installed-PWA detection — drives the desktop in-app Back button (a standalone
// desktop window has no browser chrome, so no OS-level back exists).
const isStandalone = useStandaloneDisplay()

// Gacha gate + balance chip
const gachaVisible = useGachaVisible()
const gachaStore = useGachaStore()
const gachaBalance = computed(() => gachaStore.balance)

// Wallet lifecycle (page-fetch optimization 2026-06-11): the persisted
// balance renders instantly; the network fetch only fires when the cached
// copy is stale. startBalanceWatch keeps the chip fresh via tab-return +
// watch-credit events instead of per-page-load fetches. Logout clears the
// persisted wallet so balances never leak across accounts.
watch(
  () => authStore.isAuthenticated,
  (authenticated, was) => {
    if (authenticated && gachaVisible.value) {
      void gachaStore.refreshWallet({ ifStale: true })
      gachaStore.startBalanceWatch()
    } else if (was && !authenticated) {
      gachaStore.clearWallet()
    }
  },
  { immediate: true },
)

// Workstream notifications / Phase 3 — feature flag mirrors App.vue. Build-
// time constant, no reactive dependency.
const notifEnabled = (import.meta.env.VITE_NOTIFICATIONS_ENABLED as string | undefined) !== 'false'

// Mobile notifications: the drawer row opens the list in a Modal (an
// anchored dropdown can't fit a phone viewport from inside the drawer).
// Store instantiation is side-effect-free — polling is App.vue's job.
const notificationsStore = useNotificationsStore()
const { badgeText: notifBadgeText, ariaLabel: notifAriaLabel } = useNotificationBadge()
const mobileNotifOpen = ref(false)

function openMobileNotifications(): void {
  // Close the drawer first so its focus trap + scroll lock unwind before
  // the Modal takes over; the Modal lives outside the drawer subtree so
  // it survives the unmount.
  mobileMenuOpen.value = false
  mobileNotifOpen.value = true
  // Same up-to-the-second refresh the desktop bell does on open.
  void notificationsStore.fetchNotifications()
}

// Hidden/legacy features (anidle, browser-view downloads, status) left the
// nav for the footer «Secret feature» roulette — utils/secretFeatures.ts.
// Downloads keeps its link only in the installed PWA, where offline playback
// lives; standalone-ness is reactive, hence computed.
const navLinks = computed(() => [
  { to: '/', label: 'nav.home' },
  { to: '/browse', label: 'nav.catalog' },
  { to: '/schedule', label: 'nav.schedule' },
  ...(downloadsNavVisible() ? [{ to: '/downloads', label: 'nav.downloads' }] : []),
])

const languages = [
  { code: 'ru', name: 'Русский' },
  { code: 'ja', name: '日本語' },
  { code: 'en', name: 'English' },
]

const isScrolled = ref(false)
const isVisible = ref(true)
const lastScrollY = ref(0)
const mobileMenuOpen = ref(false)
const langDropdownOpen = ref(false)
const langDropdownRef = ref<HTMLElement | null>(null)
const drawerRef = ref<HTMLElement | null>(null)
const hamburgerButtonRef = ref<HTMLButtonElement | null>(null)

// Focus-trap inside the mobile drawer; restores focus to the hamburger on close.
useFocusTrap({
  active: mobileMenuOpen,
  container: drawerRef,
  returnFocusTo: hamburgerButtonRef,
})

// Body scroll lock while drawer is open — refcount composable cooperates with
// Modal.vue and any other locker instead of stomping their overflow state.
useBodyScrollLock(mobileMenuOpen)

// WR-03: viewport resize from mobile to desktop while drawer is open would
// strand mobileMenuOpen === true (hamburger and drawer are both md:hidden, so
// the user can't toggle it). Force-close on entry to >= md so the scroll lock,
// focus trap, and body state all unwind cleanly via the existing watchers.
const isDesktop = useMediaQuery('(min-width: 768px)')
watch(isDesktop, (desktop) => {
  if (!desktop) return
  mobileMenuOpen.value = false
  // The notifications Modal is portaled (not md:hidden) — close it too, the
  // desktop bell has its own anchored dropdown.
  mobileNotifOpen.value = false
})

// WR-04: ESC must work globally while the drawer is open — not just when focus
// is inside the drawer subtree. Without this, clicking the hamburger or logo
// (siblings of the drawer) moves focus out of the drawer and the
// @keydown.escape template binding never fires. useEventListener auto-cleans
// on scope dispose.
useEventListener(document, 'keydown', (e: KeyboardEvent) => {
  if (e.key === 'Escape' && mobileMenuOpen.value) {
    mobileMenuOpen.value = false
  }
})

const setLocale = (code: string) => {
  // switchLocale lazy-loads the locale's messages on first use (en/ja are no
  // longer bundled in the entry) and sets i18n.global.locale once they land.
  void switchLocale(code)
  localStorage.setItem('locale', code)
  langDropdownOpen.value = false
}

const showLogin = () => {
  sessionStorage.setItem('returnUrl', router.currentRoute.value.fullPath)
  router.push('/auth')
}

// rAF-throttled: reading window.scrollY right after the reactive writes below
// invalidate style forces a synchronous reflow on EVERY scroll event (confirmed
// in the 2026-06-10 perf trace). One read per frame, batched at rAF start.
let scrollRafId = 0
const handleScroll = () => {
  if (scrollRafId) return
  scrollRafId = requestAnimationFrame(() => {
    scrollRafId = 0
    const currentScrollY = window.scrollY

    isScrolled.value = currentScrollY > 50
    isVisible.value = currentScrollY < lastScrollY.value || currentScrollY < 100

    lastScrollY.value = currentScrollY
  })
}

// Search state
const searchOpen = ref(false)
const searchQuery = ref('')
const searchResults = ref<Array<{ id: string; title: string; coverImage: string; releaseYear?: number; totalEpisodes?: number; rating?: number }>>([])
const highlightedIndex = ref(-1)
const searchInputRef = ref<{ focus: () => void } | null>(null)
const searchContainerRef = ref<HTMLElement | null>(null)

const openSearch = async () => {
  searchOpen.value = true
  await nextTick()
  searchInputRef.value?.focus()
}

const closeSearch = () => {
  searchOpen.value = false
  searchQuery.value = ''
  searchResults.value = []
  highlightedIndex.value = -1
}

const debouncedSearch = useDebounceFn(async (query: string) => {
  if (query.length < 2) {
    searchResults.value = []
    return
  }
  try {
    const response = await animeApi.search(query, undefined, 5)
    const data = response.data?.data || response.data
    const list = Array.isArray(data) ? data : []
    searchResults.value = list.map((a: Record<string, unknown>) => ({
      id: a.id as string,
      title: getLocalizedTitle(a.name as string, a.name_ru as string, a.name_jp as string),
      coverImage: getImageUrl(a.poster_url as string | undefined),
      releaseYear: a.year as number | undefined,
      totalEpisodes: a.episodes_count as number | undefined,
      rating: a.score as number | undefined,
    }))
  } catch {
    searchResults.value = []
  }
}, 300)

const onSearchInput = () => {
  highlightedIndex.value = -1
  debouncedSearch(searchQuery.value)
}

const goToSearch = () => {
  if (highlightedIndex.value >= 0 && highlightedIndex.value < searchResults.value.length) {
    const result = searchResults.value[highlightedIndex.value]
    router.push(`/anime/${result.id}`)
  } else if (searchQuery.value.trim()) {
    router.push({ path: '/browse', query: { q: searchQuery.value } })
  }
  closeSearch()
}

const highlightNext = () => {
  if (searchResults.value.length === 0) return
  highlightedIndex.value = (highlightedIndex.value + 1) % searchResults.value.length
}

const highlightPrev = () => {
  if (searchResults.value.length === 0) return
  highlightedIndex.value = highlightedIndex.value <= 0
    ? searchResults.value.length - 1
    : highlightedIndex.value - 1
}

onClickOutside(searchContainerRef, () => {
  if (searchOpen.value) closeSearch()
})

onClickOutside(langDropdownRef, () => {
  langDropdownOpen.value = false
})

onMounted(() => {
  window.addEventListener('scroll', handleScroll, { passive: true })
})

onUnmounted(() => {
  window.removeEventListener('scroll', handleScroll)
  // Scroll lock is released automatically by useBodyScrollLock's onScopeDispose.
})
</script>

<style scoped>
/* ------------------------------------------------------------------ */
/* Dropdown / mobile-menu transitions (unchanged)                       */
/* ------------------------------------------------------------------ */
.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.mobile-menu-enter-active,
.mobile-menu-leave-active {
  transition: opacity 0.2s ease, max-height 0.2s ease;
}

.mobile-menu-enter-from,
.mobile-menu-leave-to {
  opacity: 0;
  max-height: 0;
}

/* ------------------------------------------------------------------ */
/* Mobile capsule geometry. The glass treatment itself lives in        */
/* main.css (.glass-mobile-nav); only geometry quirks are local:       */
/*                                                                     */
/* 1. Safe-area offsets: with viewport-fit=cover the capsule must      */
/*    clear the status bar / Dynamic Island (top) and the landscape    */
/*    notch (sides). The scoped selector outranks the top-2/inset-x-4  */
/*    utilities; on cutout-less devices --safe-* are 0px and this      */
/*    matches the utilities exactly.                                   */
/* 2. Hide offset: -translate-y-full alone leaves the inset capsule's  */
/*    bottom sliver and its drop shadow peeking into the viewport      */
/*    (element top is --safe-top + 8px, shadow reaches ~40px past the  */
/*    bottom edge) — the hide offset needs that extra clearance.       */
/*    Desktop keeps plain -translate-y-full.                           */
/* ------------------------------------------------------------------ */
@media (max-width: 767px) {
  .navbar-root {
    top: calc(var(--safe-top) + 0.5rem);
    inset-inline: calc(var(--safe-left) + 1rem) calc(var(--safe-right) + 1rem);
    transition: opacity 0.3s, visibility 0s;
  }

  /* Hide by fading IN PLACE — never translate the fixed capsule up past the
     viewport top edge. iOS 26 Safari treats a fixed element at/above the top
     edge as a fixed header and permanently swaps the status-bar zone from
     "composite live page pixels behind the island" to an opaque root-color
     band (proven on-device by the /edge-m.html bisection: identical page
     minus the translate-up hide shows content behind the island; with it —
     black band). Parked at top+8px and visibility:hidden the capsule is
     paint-free, unfocusable and outside hit-testing, matching the state
     iOS 26 proved harmless. visibility transitions 0s AFTER the 0.3s fade
     out, and instantly on fade-in. */
  .navbar-root--hidden {
    translate: 0 0;
    opacity: 0;
    visibility: hidden;
    transition: opacity 0.3s, visibility 0s 0.3s;
  }
}

/* ------------------------------------------------------------------ */
/* Brand logo link                                                      */
/* ------------------------------------------------------------------ */
.brand-link {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
  flex-shrink: 0;
}

.brand-wordmark {
  font-family: var(--font-display);
  font-weight: 800;
  font-size: 18px;
  letter-spacing: -0.01em;
  line-height: 1;
}

.brand-b1 {
  color: var(--brand-cyan);
}

.brand-b2 {
  color: var(--foreground);
}

/* Small tilted "beta" tag next to the wordmark */
.brand-beta {
  align-self: flex-start;
  margin-left: -2px;
  margin-top: -4px;
  padding: 1px 5px;
  font-family: var(--font-sans);
  font-weight: 700;
  font-size: 9px;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  line-height: 1;
  color: var(--brand-pink);
  border: 1px solid var(--brand-pink);
  border-radius: var(--r-sm, 6px);
  transform: rotate(-8deg);
  transform-origin: left center;
}

/* ------------------------------------------------------------------ */
/* Desktop nav links — handoff .nav-link + .nav-link.active            */
/* ------------------------------------------------------------------ */
.nav-link-nt {
  position: relative;
  padding: 10px 14px;
  font-size: 14px;
  font-weight: 500;
  font-family: var(--font-sans);
  color: var(--muted-foreground);
  border-radius: var(--r-sm, 8px);
  transition: color 0.15s ease;
  text-decoration: none;
  white-space: nowrap;
}

.nav-link-nt:hover {
  color: var(--foreground);
}

/* Active state — colour lift + animated underline pill */
.nav-link-nt--active {
  color: var(--foreground);
}

.nav-link-nt--active::after {
  content: "";
  position: absolute;
  left: 14px;
  right: 14px;
  bottom: -16px;
  height: 2px;
  background: var(--brand-cyan);
  border-radius: 2px;
  box-shadow: 0 0 10px var(--brand-cyan);
  /* Animate in when active class is applied */
  transition: opacity 0.2s ease, box-shadow 0.2s ease;
}

/* ------------------------------------------------------------------ */
/* Icon buttons — handoff .icon-btn                                    */
/* ------------------------------------------------------------------ */
.icon-btn-nt {
  width: 36px;
  height: 36px;
  display: grid;
  place-items: center;
  border-radius: var(--r-md, 12px);
  color: var(--muted-foreground);
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  flex-shrink: 0;
}

.icon-btn-nt:hover {
  background: var(--white-a4);
  color: var(--foreground);
}

/* ------------------------------------------------------------------ */
/* Language pill — handoff .lang-pill                                  */
/* ------------------------------------------------------------------ */
.lang-pill-nt {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 10px;
  border-radius: var(--r-sm, 8px);
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.04em;
  color: var(--ink-2, var(--ink-2));
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  white-space: nowrap;
}

.lang-pill-nt:hover {
  background: var(--white-a4);
  color: var(--foreground);
}

</style>
