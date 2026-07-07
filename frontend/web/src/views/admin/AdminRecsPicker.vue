<template>
  <div class="min-h-screen pt-4 pb-16 px-4">
    <div class="max-w-md mx-auto">
      <div class="glass-card rounded-2xl p-8">
        <h1 class="text-2xl font-semibold text-white mb-2">{{ $t('admin.recs.title') }}</h1>
        <p class="text-white/60 text-sm mb-6">{{ $t('admin.recs.pickerHelp') }}</p>

        <!-- RBAC-and-roulette Task 5: the picker now delegates identifier
             resolution (username / public_id / Telegram ID / UUID) to the
             shared UserResolveInput (mode="nav"), which validates before
             emitting — no more free-text submit landing on a 404 route. -->
        <div
          class="space-y-4"
          role="search"
          :aria-label="$t('admin.recs.picker.listboxLabel')"
        >
          <div>
            <span class="block text-white/70 text-sm mb-2">
              {{ $t('admin.recs.pickerLabel') }}
            </span>
            <UserResolveInput mode="nav" @resolve="onResolve" />
          </div>
          <div
            class="flex items-center gap-3 flex-wrap"
            role="group"
            :aria-label="$t('admin.recs.picker.listboxLabel')"
          >
            <button
              v-if="authStore.user?.id"
              type="button"
              role="option"
              tabindex="0"
              :aria-selected="false"
              class="text-cyan-300 hover:text-cyan-200 text-sm inline-flex items-center gap-2 focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400 rounded px-1"
              @click="goSelf"
            >
              {{ $t('admin.recs.pickerSelf') }}
              <!-- Phase 12 / UA-090: mark the admin's own quick-action with
                   a "You" badge so operators can see at a glance that the
                   target is themselves. -->
              <span
                class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-cyan-500/20 text-cyan-300"
              >
                {{ $t('admin.recs.picker.youBadge') }}
              </span>
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import UserResolveInput from '@/components/admin/UserResolveInput.vue'
import type { ResolvedUser } from '@/api/client'

const router = useRouter()
const authStore = useAuthStore()

function onResolve(u: ResolvedUser) {
  router.push(`/admin/recs/${u.id}`)
}

function goSelf() {
  if (!authStore.user?.id) return
  router.push(`/admin/recs/${authStore.user.id}`)
}
</script>
