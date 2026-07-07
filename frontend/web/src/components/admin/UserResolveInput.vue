<template>
  <div class="flex items-start gap-2" data-testid="user-resolve-input">
    <div class="flex-1">
      <Input
        v-model="query"
        type="text"
        size="sm"
        :placeholder="t('admin.userResolve.placeholder')"
        :disabled="isResolving"
        autocomplete="off"
        spellcheck="false"
        data-testid="user-resolve-field"
        @keydown.enter="handleResolve"
      />
      <p
        v-if="errorMessage"
        class="mt-1 text-sm text-destructive"
        data-testid="user-resolve-error"
      >
        {{ errorMessage }}
      </p>
    </div>
    <Button
      type="button"
      size="sm"
      :disabled="isResolving || !query.trim()"
      :aria-label="isResolving ? t('admin.userResolve.resolving') : t('admin.userResolve.add')"
      data-testid="user-resolve-submit"
      @click="handleResolve"
    >
      <Spinner v-if="isResolving" size="sm" tone="mono" aria-hidden="true" />
      <Plus v-else class="size-4" aria-hidden="true" />
    </Button>
  </div>
</template>

<script setup lang="ts">
// Shared admin component (RBAC-and-roulette P3, Task 4): turns a typed
// identifier (username / public_id / Telegram ID / UUID) into a resolved
// user via GET /api/admin/users/resolve. Consumed by the recs picker
// (Task 5, mode="nav") and the policy admin view (Task 7, mode="chip").
//
// Deliberately dumb: no router, no store, no navigation — the parent
// decides what to do with the resolved user via @resolve. `mode` is purely
// informational for the parent (e.g. to pick an icon/copy variant); this
// component's own behavior does not branch on it.
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus } from 'lucide-vue-next'
import { Input, Button, Spinner } from '@/components/ui'
import { adminApi, type ResolvedUser } from '@/api/client'

withDefaults(defineProps<{ mode?: 'chip' | 'nav' }>(), { mode: 'chip' })

const emit = defineEmits<{ (e: 'resolve', user: ResolvedUser): void }>()

const { t } = useI18n()

const query = ref('')
const isResolving = ref(false)
const errorMessage = ref('')

// Backend wraps responses in { success, data } via httputil.OK (mirrors
// useAdminFeedback.ts / useAdminRecs.ts unwrap pattern).
function unwrap(data: unknown): ResolvedUser {
  const d = data as { data?: ResolvedUser }
  return d && typeof d === 'object' && 'data' in d ? (d.data as ResolvedUser) : (data as ResolvedUser)
}

async function handleResolve(): Promise<void> {
  const q = query.value.trim()
  if (!q) return
  isResolving.value = true
  errorMessage.value = ''
  try {
    const res = await adminApi.resolveUser(q)
    const user = unwrap(res.data)
    emit('resolve', user)
    query.value = ''
    errorMessage.value = ''
  } catch (e: unknown) {
    const status = (e as { response?: { status?: number } })?.response?.status
    if (status === 404) {
      errorMessage.value = t('admin.userResolve.notFound', { q })
    } else {
      errorMessage.value = t('admin.errors.serverError')
    }
  } finally {
    isResolving.value = false
  }
}

defineExpose({ query, isResolving, errorMessage, handleResolve })
</script>
