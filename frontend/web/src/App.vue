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

    <!-- Mobile Bottom Navigation -->
    <MobileNav v-if="!isFullscreen" />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import Navbar from '@/components/layout/Navbar.vue'
import MobileNav from '@/components/layout/MobileNav.vue'

const route = useRoute()

const isFullscreen = computed(() => route.name === 'watch')

const mainClasses = computed(() => {
  if (isFullscreen.value) {
    return ''
  }
  return 'pb-20 md:pb-0' // Bottom padding for mobile nav
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
