<template>
  <header
    :class="[
      'fixed top-0 left-0 right-0 z-50 transition-all duration-300',
      isVisible ? 'translate-y-0' : '-translate-y-full',
      isScrolled ? 'glass-nav' : 'bg-transparent'
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
          <button
            class="p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
            :aria-label="$t('nav.search')"
            @click="$router.push('/search')"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
            </svg>
          </button>

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
        <div v-if="mobileMenuOpen" class="md:hidden py-4 border-t border-white/10">
          <div class="flex flex-col gap-2">
            <router-link
              v-for="link in navLinks"
              :key="link.to"
              :to="link.to"
              class="px-4 py-3 text-white/70 hover:text-white hover:bg-white/5 rounded-lg transition-colors"
              active-class="text-cyan-400 bg-cyan-500/10"
              @click="mobileMenuOpen = false"
            >
              {{ $t(link.label) }}
            </router-link>
          </div>
        </div>
      </Transition>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { onClickOutside } from '@vueuse/core'
import Button from '@/components/ui/Button.vue'

const { locale } = useI18n()
const authStore = useAuthStore()

const navLinks = [
  { to: '/', label: 'nav.home' },
  { to: '/browse', label: 'nav.catalog' },
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
  // TODO: Implement login modal
  console.log('Show login modal')
}

const handleScroll = () => {
  const currentScrollY = window.scrollY

  isScrolled.value = currentScrollY > 50
  isVisible.value = currentScrollY < lastScrollY.value || currentScrollY < 100

  lastScrollY.value = currentScrollY
}

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
