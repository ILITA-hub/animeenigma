<template>
  <div class="inline-flex">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 text-white/60 hover:text-white/80 text-sm transition-colors"
      :aria-label="$t('footer.feedback.button')"
      @click="handleClick"
    >
      <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 10h.01M12 10h.01M16 10h.01M21 12c0 4.418-4.03 8-9 8a9.96 9.96 0 01-4.84-1.23L3 20l1.25-3.74A7.96 7.96 0 013 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
      </svg>
      {{ $t('footer.feedback.button') }}
    </button>

    <Modal v-model="showModal" :title="$t('footer.feedback.title')" size="lg">
      <div v-if="submitted" class="text-center py-4">
        <svg class="w-12 h-12 mx-auto mb-3 text-success" fill="none" stroke="currentColor" viewBox="0 0 24 24" aria-hidden="true">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
        </svg>
        <p class="text-white text-lg font-medium">{{ $t('footer.feedback.sent') }}</p>
        <p class="text-white/60 mt-1 text-sm">{{ $t('footer.feedback.thankYou') }}</p>
      </div>

      <template v-else>
        <fieldset class="mb-4">
          <legend class="text-sm text-white/60 mb-2">{{ $t('footer.feedback.categoryLabel') }}</legend>
          <div class="grid grid-cols-3 gap-2">
            <label
              v-for="opt in categories"
              :key="opt.value"
              class="flex flex-col items-center gap-1 px-3 py-3 rounded-lg border cursor-pointer transition-colors"
              :class="
                category === opt.value
                  ? 'bg-brand-violet/20 border-brand-violet/50 text-white'
                  : 'bg-white/5 border-white/10 text-white/60 hover:bg-white/10 hover:text-white/80'
              "
            >
              <input
                v-model="category"
                type="radio"
                :value="opt.value"
                name="feedback-category"
                class="sr-only"
              >
              <span class="text-xl" aria-hidden="true">{{ opt.icon }}</span>
              <span class="text-sm font-medium">{{ $t(opt.labelKey) }}</span>
            </label>
          </div>
        </fieldset>

        <textarea
          v-model="description"
          class="w-full h-24 bg-white/5 border border-white/10 rounded-lg p-3 text-white text-sm placeholder-white/30 focus:outline-none focus:border-white/20 resize-none"
          :placeholder="$t(descriptionPlaceholderKey)"
        />

        <p class="mt-2 text-xs text-white/30">
          {{ $t('footer.feedback.disclaimer') }}
        </p>

        <div v-if="submitError" class="mt-2 text-sm text-pink-400">
          {{ submitError }}
        </div>
      </template>

      <template #footer>
        <Button
          v-if="!submitted"
          variant="soft"
          size="sm"
          @click="showModal = false"
        >
          {{ $t('common.cancel') }}
        </Button>
        <button
          v-if="!submitted"
          class="px-4 py-2 text-sm rounded-lg font-medium transition-colors disabled:opacity-50 bg-brand-violet/20 text-brand-violet hover:bg-brand-violet/30"
          :disabled="submitting"
          @click="submitReport"
        >
          <span v-if="submitting" class="flex items-center gap-2">
            <span class="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
            {{ $t('footer.feedback.sending') }}
          </span>
          <span v-else>{{ $t('footer.feedback.submit') }}</span>
        </button>
        <Button
          v-if="submitted"
          variant="soft"
          size="sm"
          @click="closeAfterSuccess"
        >
          {{ $t('common.close') }}
        </Button>
      </template>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { userApi } from '@/api/client'
import { collectDiagnostics } from '@/utils/diagnostics'
import Button from '@/components/ui/Button.vue'
import Modal from '@/components/ui/Modal.vue'

type FeedbackCategory = 'bug' | 'issue' | 'feature'

const authStore = useAuthStore()
const router = useRouter()
const { t } = useI18n()

const showModal = ref(false)
const description = ref('')
const category = ref<FeedbackCategory>('bug')
const submitting = ref(false)
const submitted = ref(false)
const submitError = ref<string | null>(null)

const categories: { value: FeedbackCategory; labelKey: string; icon: string }[] = [
  { value: 'bug',     labelKey: 'footer.feedback.categoryBug',     icon: '🐛' },
  { value: 'issue',   labelKey: 'footer.feedback.categoryIssue',   icon: '❓' },
  { value: 'feature', labelKey: 'footer.feedback.categoryFeature', icon: '💡' },
]

const descriptionPlaceholderKey = computed(() => {
  switch (category.value) {
    case 'feature': return 'footer.feedback.descriptionPlaceholderFeature'
    case 'issue':   return 'footer.feedback.descriptionPlaceholderIssue'
    default:        return 'footer.feedback.descriptionPlaceholderBug'
  }
})

function handleClick() {
  if (authStore.isAuthenticated) {
    showModal.value = true
  } else {
    sessionStorage.setItem('returnUrl', router.currentRoute.value.fullPath)
    router.push({ name: 'auth' })
  }
}

function closeAfterSuccess() {
  showModal.value = false
  submitted.value = false
  description.value = ''
  category.value = 'bug'
}

async function submitReport() {
  submitting.value = true
  submitError.value = null

  try {
    const report = collectDiagnostics(
      {
        playerType: 'feedback',
        animeId: '',
        animeName: '',
        episodeNumber: null,
        serverName: null,
        streamUrl: null,
        errorMessage: null,
        scraperProvider: null,
        triedChain: [],
      },
      description.value,
      authStore.user?.id ?? null,
      authStore.user?.username ?? null,
      category.value,
    )

    await userApi.reportError(report as unknown as Record<string, unknown>)
    submitted.value = true
  } catch (err: unknown) {
    const e = err as { response?: { data?: { error?: { message?: string } } }; message?: string }
    submitError.value = e.response?.data?.error?.message || e.message || t('footer.feedback.submitError')
  } finally {
    submitting.value = false
  }
}
</script>
