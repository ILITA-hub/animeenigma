<template>
  <div class="min-h-screen pt-24 pb-16 px-4">
    <div class="max-w-md mx-auto">
      <div class="glass-card rounded-2xl p-8">
        <h1 class="text-2xl font-bold text-white mb-2">{{ $t('admin.recs.title') }}</h1>
        <p class="text-white/60 text-sm mb-6">{{ $t('admin.recs.pickerHelp') }}</p>

        <!-- Phase 12 (UA-090/091/092/097/101): the picker is a single-input
             form (no live user search exists yet); listbox semantics are
             applied to the input + quick-action container so future live
             results can slot in without restructuring. Autofocus, focus
             rings, in-flight spinner, and self-row "You" badge land here. -->
        <form
          @submit.prevent="go"
          class="space-y-4"
          role="search"
          :aria-label="$t('admin.recs.picker.listboxLabel')"
        >
          <div>
            <label class="block text-white/70 text-sm mb-2" for="rec-user-id">
              {{ $t('admin.recs.pickerLabel') }}
            </label>
            <div class="relative">
              <Input
                id="rec-user-id"
                ref="searchInputRef"
                v-model="input"
                type="text"
                size="sm"
                :placeholder="$t('admin.recs.pickerPlaceholder')"
                autocomplete="off"
                spellcheck="false"
                :aria-busy="isSubmitting"
                class="bg-white/10 font-mono pr-9"
                required
              />
              <!-- Phase 12 / UA-092: spinner while the navigation is in flight. -->
              <div
                v-if="isSubmitting"
                class="absolute right-3 top-1/2 -translate-y-1/2 w-4 h-4 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin"
                aria-hidden="true"
              ></div>
            </div>
            <!-- Phase 12 / UA-097: empty/help text. Since the picker
                 doesn't have a live result list (single-input form), this
                 surfaces an empty-state hint when the user has typed
                 nothing. A future live-search drop-in can re-use the same
                 key to render under a populated listbox. -->
            <p
              v-if="!input.trim()"
              class="mt-2 text-white/40 text-xs italic"
            >
              {{ $t('admin.recs.picker.empty') }}
            </p>
          </div>
          <div
            class="flex items-center gap-3 flex-wrap"
            role="group"
            :aria-label="$t('admin.recs.picker.listboxLabel')"
          >
            <Button
              type="submit"
              variant="default"
              size="sm"
              :disabled="!input.trim() || isSubmitting"
            >
              {{ $t('admin.recs.pickerSubmit') }}
            </Button>
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
                   target is themselves. (No live result list exists yet
                   to filter; this is the closest applicable surface.) -->
              <span
                class="px-1.5 py-0.5 rounded text-[10px] font-mono bg-cyan-500/20 text-cyan-300"
              >
                {{ $t('admin.recs.picker.youBadge') }}
              </span>
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import Button from '@/components/ui/Button.vue'
import { Input } from '@/components/ui'

const router = useRouter()
const authStore = useAuthStore()
const input = ref('')
// Phase 12 / UA-091: autofocus the search input on mount.
const searchInputRef = ref<{ focus: () => void } | null>(null)
// Phase 12 / UA-092: track submit-in-flight so the spinner can render
// while the SPA router resolves the next route.
const isSubmitting = ref(false)

onMounted(() => {
  searchInputRef.value?.focus()
})

async function go() {
  const target = input.value.trim()
  if (!target) return
  isSubmitting.value = true
  try {
    await router.push(`/admin/recs/${encodeURIComponent(target)}`)
  } finally {
    isSubmitting.value = false
  }
}

async function goSelf() {
  if (!authStore.user?.id) return
  isSubmitting.value = true
  try {
    await router.push(`/admin/recs/${authStore.user.id}`)
  } finally {
    isSubmitting.value = false
  }
}
</script>
