<template>
  <div class="min-h-screen pt-24 pb-16 px-4">
    <div class="max-w-md mx-auto">
      <div class="glass-card rounded-2xl p-8">
        <h1 class="text-2xl font-bold text-white mb-2">{{ $t('admin.recs.title') }}</h1>
        <p class="text-white/60 text-sm mb-6">{{ $t('admin.recs.pickerHelp') }}</p>

        <form @submit.prevent="go" class="space-y-4">
          <div>
            <label class="block text-white/70 text-sm mb-2" for="rec-user-id">
              {{ $t('admin.recs.pickerLabel') }}
            </label>
            <input
              id="rec-user-id"
              v-model="input"
              type="text"
              :placeholder="$t('admin.recs.pickerPlaceholder')"
              autocomplete="off"
              spellcheck="false"
              class="w-full px-3 py-2 rounded-lg bg-white/10 border border-white/20 text-white font-mono text-sm placeholder-white/30 focus:outline-none focus:border-cyan-400/60"
              required
            />
          </div>
          <div class="flex items-center gap-3">
            <button
              type="submit"
              class="px-4 py-2 rounded-md bg-cyan-500/80 hover:bg-cyan-500 text-white font-medium text-sm transition disabled:opacity-50"
              :disabled="!input.trim()"
            >
              {{ $t('admin.recs.pickerSubmit') }}
            </button>
            <button
              v-if="authStore.user?.id"
              type="button"
              class="text-cyan-300 hover:text-cyan-200 text-sm"
              @click="goSelf"
            >
              {{ $t('admin.recs.pickerSelf') }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const authStore = useAuthStore()
const input = ref('')

function go() {
  const target = input.value.trim()
  if (!target) return
  router.push(`/admin/recs/${encodeURIComponent(target)}`)
}

function goSelf() {
  if (authStore.user?.id) {
    router.push(`/admin/recs/${authStore.user.id}`)
  }
}
</script>
