<template>
  <div id="app" class="min-h-screen bg-base">
    <!-- Desktop Navbar -->
    <Navbar v-if="!isFullscreen" />

    <!-- Main Content -->
    <main :class="mainClasses">
      <router-view v-slot="{ Component }">
        <Transition name="page">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>

    <!-- Footer -->
    <footer v-if="!isFullscreen" class="py-8 px-4 text-center border-t border-white/10">
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
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import Navbar from '@/components/layout/Navbar.vue'

const route = useRoute()
const authStore = useAuthStore()

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
