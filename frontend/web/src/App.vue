<template>
  <div id="app" class="min-h-screen bg-base">
    <!-- Desktop Navbar -->
    <Navbar v-if="!isFullscreen" />

    <!-- Error Fallback -->
    <div v-if="appError" class="min-h-screen flex items-center justify-center px-4">
      <div class="text-center max-w-md">
        <svg class="w-16 h-16 mx-auto mb-4 text-pink-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-3L13.732 4c-.77-1.333-2.694-1.333-3.464 0L3.34 16c-.77 1.333.192 3 1.732 3z" />
        </svg>
        <h2 class="text-xl font-semibold text-white mb-2">Something went wrong</h2>
        <p class="text-white/60 text-sm mb-6">{{ appError.message }}</p>
        <button
          @click="reloadPage"
          class="px-6 py-2.5 bg-purple-500 hover:bg-purple-600 text-white rounded-lg font-medium transition-colors"
        >
          Reload
        </button>
      </div>
    </div>

    <!-- Main Content -->
    <main v-else :class="mainClasses">
      <router-view v-slot="{ Component }">
        <Transition name="page">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>

    <!-- Footer -->
    <footer v-if="!isFullscreen && !appError" class="py-8 px-4 text-center border-t border-white/10">
      <div class="max-w-7xl mx-auto flex items-center justify-center gap-4">
        <p class="text-white/40 text-sm">
          &copy; {{ new Date().getFullYear() }} AnimeEnigma. {{ $t('footer.rights') }}
        </p>
        <router-link to="/status" class="text-white/40 hover:text-white/60 text-sm transition-colors">
          {{ $t('status.title') }}
        </router-link>
      </div>
    </footer>

  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onErrorCaptured, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import Navbar from '@/components/layout/Navbar.vue'

const route = useRoute()
const authStore = useAuthStore()

const appError = ref<Error | null>(null)

onErrorCaptured((err) => {
  appError.value = err
  console.error('[App Error]', err)
  return false // prevent propagation
})

const reloadPage = () => window.location.reload()

const isFullscreen = computed(() => route.name === 'watch')

const mainClasses = computed(() => {
  if (isFullscreen.value) return ''
  return '' // Pages handle their own top padding; glass nav floats over backgrounds
})

// Initialize auth state - fetch user if we have token but no user data
onMounted(async () => {
  if (authStore.token && !authStore.user) {
    await authStore.fetchUser()
  }
})
</script>

<style scoped>
.page-enter-active {
  transition: opacity 0.2s ease;
}

.page-enter-from {
  opacity: 0;
}
</style>
