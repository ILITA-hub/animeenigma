<template>
  <div id="app" class="min-h-screen bg-base">
    <!-- Desktop Navbar -->
    <Navbar v-if="!isFullscreen" />

    <!-- Main Content -->
    <main :class="mainClasses">
      <router-view v-slot="{ Component }">
        <Transition name="page" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </main>

    <!-- Footer -->
    <footer v-if="!isFullscreen" class="py-8 px-4 text-center border-t border-white/10">
      <div class="max-w-7xl mx-auto">
        <p class="text-white/40 text-sm">
          &copy; {{ new Date().getFullYear() }} AnimeEnigma. {{ $t('footer.rights') }}
        </p>
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
  if (isFullscreen.value) {
    return ''
  }
  return 'pt-16' // Top padding for fixed navbar
})

// Initialize auth state - fetch user if we have token but no user data
onMounted(async () => {
  if (authStore.token && !authStore.user) {
    await authStore.fetchUser()
  }
})
</script>

<style scoped>
.page-enter-active,
.page-leave-active {
  transition: opacity 0.2s ease;
}

.page-enter-from,
.page-leave-to {
  opacity: 0;
}
</style>
