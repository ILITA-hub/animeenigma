<template>
  <header
    :class="[
      'fixed top-0 left-0 right-0 z-50 transition-all duration-300',
      isVisible ? 'translate-y-0' : '-translate-y-full',
      'glass-nav'
    ]"
  >
    <nav class="max-w-7xl mx-auto px-4 lg:px-8">
      <div class="flex items-center justify-between h-16">
        <!-- Logo -->
        <router-link to="/" class="flex items-center gap-2 text-xl font-bold">
          <span class="text-cyan-400">Anime</span>
          <span class="text-white">Enigma</span>
        </router-link>

        <!-- Desktop Navigation -->
        <div class="hidden md:flex items-center gap-6">
          <router-link
            v-for="link in navLinks"
            :key="link.to"
            :to="link.to"
            class="text-white/70 hover:text-white transition-colors relative py-2"
            active-class="text-cyan-400"
          >
            {{ $t(link.label) }}
            <span
              v-if="$route.path === link.to"
              class="absolute bottom-0 left-0 right-0 h-0.5 bg-cyan-400 rounded-full"
            />
          </router-link>
        </div>

        <!-- Right Section -->
        <div class="hidden md:flex items-center gap-4">
          <!-- Search -->
          <div class="relative" ref="searchContainerRef">
            <button
              v-if="!searchOpen"
              class="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              :aria-label="$t('nav.search')"
              @click="openSearch"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
              </svg>
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
                <svg class="absolute left-2.5 top-1/2 -translate-y-1/2 w-4 h-4 text-white/40" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
                <!-- Autocomplete Dropdown -->
                <Transition name="dropdown">
                  <div
                    v-if="searchResults.length > 0"
                    class="absolute top-full left-0 mt-2 glass-elevated rounded-xl overflow-hidden z-50"
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
                        <p class="text-white/40 text-xs">
                          {{ result.releaseYear || '' }}{{ result.releaseYear && result.totalEpisodes ? ' \u00b7 ' : '' }}{{ result.totalEpisodes ? result.totalEpisodes + ' \u044d\u043f.' : '' }}
                        </p>
                      </div>
                      <span v-if="result.rating" class="text-cyan-400 text-xs font-medium flex-shrink-0">
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
                class="p-1.5 text-white/40 hover:text-white transition-colors"
                @click="closeSearch"
              >
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
                </svg>
              </button>
            </div>
          </div>

          <!-- Language Selector -->
          <div class="relative" ref="langDropdownRef">
            <button
              class="flex items-center gap-1 px-3 py-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              @click="langDropdownOpen = !langDropdownOpen"
            >
              <span class="uppercase text-sm font-medium">{{ locale }}</span>
              <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
              </svg>
            </button>
            <Transition name="dropdown">
              <div
                v-if="langDropdownOpen"
                class="absolute right-0 mt-2 py-2 w-32 glass-elevated rounded-xl"
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

          <!-- User Avatar / Login -->
          <template v-if="authStore.isAuthenticated">
            <router-link
              to="/profile"
              class="w-10 h-10 rounded-full bg-cyan-500/20 border border-cyan-500/30 flex items-center justify-center text-cyan-400 hover:bg-cyan-500/30 transition-colors"
            >
              <span class="text-sm font-medium">{{ userInitials }}</span>
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
          class="md:hidden p-2 text-white/70 hover:text-white"
          @click="mobileMenuOpen = !mobileMenuOpen"
          :aria-label="mobileMenuOpen ? 'Close menu' : 'Open menu'"
        >
          <svg v-if="!mobileMenuOpen" class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
          </svg>
          <svg v-else class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </button>
      </div>

      <!-- Mobile Menu -->
      <Transition name="mobile-menu">
        <div v-if="mobileMenuOpen" class="md:hidden py-4 border-t border-white/10 glass-nav rounded-b-2xl">
          <div class="flex flex-col gap-1">
            <router-link
              v-for="link in navLinks"
              :key="link.to"
              :to="link.to"
              class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              active-class="text-cyan-400 bg-cyan-500/10"
              @click="mobileMenuOpen = false"
            >
              {{ $t(link.label) }}
            </router-link>

            <!-- Divider -->
            <div class="my-1 border-t border-white/10" />

            <!-- Profile / Login -->
            <template v-if="authStore.isAuthenticated">
              <router-link
                to="/profile"
                class="flex items-center gap-3 px-4 py-3 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
                active-class="text-cyan-400 bg-cyan-500/10"
                @click="mobileMenuOpen = false"
              >
                <div class="w-8 h-8 rounded-full bg-cyan-500/20 border border-cyan-500/30 flex items-center justify-center text-cyan-400 text-sm font-medium flex-shrink-0">
                  {{ userInitials }}
                </div>
                {{ $t('nav.profile') }}
              </router-link>
            </template>
            <template v-else>
              <router-link
                to="/auth"
                class="px-4 py-3 text-cyan-400 hover:text-cyan-300 hover:bg-white/10 rounded-lg transition-colors font-medium"
                @click="mobileMenuOpen = false"
              >
                {{ $t('nav.login') }}
              </router-link>
            </template>

            <!-- Language -->
            <div class="flex items-center gap-2 px-4 py-3">
              <button
                v-for="lang in languages"
                :key="lang.code"
                class="px-3 py-1.5 rounded-lg text-sm font-medium transition-colors"
                :class="locale === lang.code ? 'bg-cyan-500/20 text-cyan-400' : 'text-white/50 hover:text-white hover:bg-white/10'"
                @click="setLocale(lang.code)"
              >
                {{ lang.name }}
              </button>
            </div>
          </div>
        </div>
      </Transition>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { onClickOutside, useDebounceFn } from '@vueuse/core'
import { animeApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import Button from '@/components/ui/Button.vue'

const router = useRouter()
const { locale } = useI18n()
const authStore = useAuthStore()

const navLinks = [
  { to: '/', label: 'nav.home' },
  { to: '/browse', label: 'nav.catalog' },
  { to: '/themes', label: 'nav.themes' },
  { to: '/game', label: 'nav.rooms' },
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

const userInitials = computed(() => {
  const user = authStore.user
  if (!user?.username) return '?'
  return user.username.slice(0, 2).toUpperCase()
})

const setLocale = (code: string) => {
  locale.value = code
  localStorage.setItem('locale', code)
  langDropdownOpen.value = false
}

const showLogin = () => {
  router.push('/auth')
}

const handleScroll = () => {
  const currentScrollY = window.scrollY

  isScrolled.value = currentScrollY > 50
  isVisible.value = currentScrollY < lastScrollY.value || currentScrollY < 100

  lastScrollY.value = currentScrollY
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
      coverImage: (a.poster_url || '') as string,
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
})
</script>

<style scoped>
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
</style>
