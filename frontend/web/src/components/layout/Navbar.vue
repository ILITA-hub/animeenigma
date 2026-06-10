<template>
  <!-- Phase 11 / UX-23 — `navbar-root` class hook lets the global CSS rule
       in Anime.vue (`body.theater-mode .navbar-root { display: none }`)
       hide this header when Theater Mode is active on the anime detail
       view. Navbar itself stays stateless w.r.t. theater-mode; the body
       class is the contract. -->
  <header
    :class="[
      'fixed top-0 left-0 right-0 z-50 transition-all duration-300 navbar-root',
      isVisible ? 'translate-y-0' : '-translate-y-full',
      'glass-nav'
    ]"
  >
    <nav class="max-w-7xl mx-auto px-4 lg:px-8">
      <div class="flex items-center justify-between h-16">
        <!-- Logo — BrandMark icon + wordmark -->
        <router-link to="/" class="brand-link">
          <BrandMark />
          <span class="brand-wordmark">
            <span class="brand-b1">Anime</span><span class="brand-b2">Enigma</span>
          </span>
          <span class="brand-beta">beta</span>
        </router-link>

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
          <!-- Gacha «Лудка» nav item — gated by VITE_GACHA_ADMIN_ONLY -->
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
              <div class="relative">
                <input
                  ref="searchInputRef"
                  v-model="searchQuery"
                  type="text"
                  :placeholder="$t('search.placeholder')"
                  class="w-64 px-3 py-1.5 pl-9 bg-white/10 border border-white/20 rounded-lg text-white text-sm placeholder-white/40 focus:outline-none focus:border-cyan-400/50 focus:bg-white/15 transition-all"
                  @input="onSearchInput"
                  @keydown.enter="goToSearch"
                  @keydown.escape="closeSearch"
                  @keydown.down.prevent="highlightNext"
                  @keydown.up.prevent="highlightPrev"
                />
                <Search class="absolute left-2.5 top-1/2 -translate-y-1/2 size-4 text-white/60" aria-hidden="true" />
                <!-- Autocomplete Dropdown -->
                <Transition name="dropdown">
                  <div
                    v-if="searchResults.length > 0"
                    class="absolute top-full left-0 mt-2 rounded-xl overflow-hidden z-50 bg-gray-950/95 backdrop-blur-xl border border-white/10 shadow-2xl"
                    style="min-width: 320px"
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

          <!-- Gacha balance chip — gated by VITE_GACHA_ADMIN_ONLY -->
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
                class="absolute right-0 mt-2 py-2 w-32 rounded-xl bg-gray-950/95 backdrop-blur-xl border border-white/10 shadow-2xl"
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
          class="md:hidden py-4 border-t border-white/10 glass-nav rounded-b-2xl"
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

            <!-- Workstream notifications / Phase 3 — mobile bell. Same
                 NotificationBell component as desktop; dropdown is
                 max-w-[calc(100vw-1.5rem)] so it fits the narrow viewport. -->
            <div
              v-if="notifEnabled && authStore.isAuthenticated"
              class="px-3 py-2"
            >
              <NotificationBell />
            </div>

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
    </nav>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { onClickOutside, useDebounceFn, useEventListener, useMediaQuery } from '@vueuse/core'
import { animeApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { useBodyScrollLock } from '@/composables/useBodyScrollLock'
import { Search, X, ChevronDown, Menu, Gem, Star } from 'lucide-vue-next'
import Avatar from '@/components/ui/Avatar.vue'
import Button from '@/components/ui/Button.vue'
import ButtonGroup from '@/components/ui/ButtonGroup.vue'
import NotificationBell from '@/components/NotificationBell.vue'
import BrandMark from '@/components/layout/BrandMark.vue'
import { useGachaVisible } from '@/utils/gachaGate'
import { useGachaStore } from '@/stores/gacha'

const router = useRouter()
const { locale } = useI18n()
const authStore = useAuthStore()

// Gacha gate + balance chip
const gachaVisible = useGachaVisible()
const gachaStore = useGachaStore()
const gachaBalance = computed(() => gachaStore.balance)

// Refresh wallet once on login (whenever isAuthenticated flips true and gacha is visible)
watch(
  () => authStore.isAuthenticated,
  (authenticated) => {
    if (authenticated && gachaVisible.value) {
      gachaStore.refreshWallet()
    }
  },
  { immediate: true },
)

// Workstream notifications / Phase 3 — feature flag mirrors App.vue. Build-
// time constant, no reactive dependency.
const notifEnabled = (import.meta.env.VITE_NOTIFICATIONS_ENABLED as string | undefined) !== 'false'

const navLinks = [
  { to: '/', label: 'nav.home' },
  { to: '/browse', label: 'nav.catalog' },
  { to: '/schedule', label: 'nav.schedule' },
]

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
  if (desktop && mobileMenuOpen.value) {
    mobileMenuOpen.value = false
  }
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
  locale.value = code
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
const searchInputRef = ref<HTMLInputElement | null>(null)
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
  font-family: var(--f-display, "Manrope", "Inter", system-ui, sans-serif);
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
  font-family: var(--f-ui, "Inter", system-ui, sans-serif);
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
  font-family: var(--f-ui, "Inter", system-ui, sans-serif);
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
  background: rgba(255, 255, 255, 0.05);
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
  color: var(--ink-2, rgba(255, 255, 255, 0.78));
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  white-space: nowrap;
}

.lang-pill-nt:hover {
  background: rgba(255, 255, 255, 0.05);
  color: var(--foreground);
}

</style>
