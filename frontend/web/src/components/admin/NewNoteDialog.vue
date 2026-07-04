<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Modal, Select, Button } from '@/components/ui'
import { adminApi } from '@/api/client'
import { FEEDBACK_CATEGORIES } from '@/types/feedback'
import type { FeedbackCategory } from '@/types/feedback'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [boolean]; created: [string] }>()

const { t } = useI18n()

const category = ref<FeedbackCategory | '__none__'>('__none__')
const description = ref('')
const submitting = ref(false)
const errorMsg = ref('')

const categoryOptions = [
  { value: '__none__', label: t('admin.feedback.newNote.categoryNone') },
  ...FEEDBACK_CATEGORIES.map((c) => ({ value: c, label: t(`admin.feedback.category.${c}`) })),
]

function reset(): void {
  category.value = '__none__'
  description.value = ''
  errorMsg.value = ''
}

function close(): void {
  emit('update:open', false)
}

function unwrapId(res: unknown): string {
  const d = (res as { data?: unknown }).data
  const inner = (d as { data?: { id?: string }; id?: string })
  return inner?.data?.id ?? inner?.id ?? ''
}

async function submit(): Promise<void> {
  if (!description.value.trim()) {
    errorMsg.value = t('admin.feedback.newNote.error')
    return
  }
  submitting.value = true
  errorMsg.value = ''
  try {
    const res = await adminApi.createNote({
      category: category.value && category.value !== '__none__' ? category.value : undefined,
      description: description.value.trim(),
    })
    emit('created', unwrapId(res))
    reset()
    close()
  } catch {
    errorMsg.value = t('admin.feedback.newNote.error')
  } finally {
    submitting.value = false
  }
}

defineExpose({ category, description, submit })
</script>

<template>
  <Modal
    :model-value="props.open"
    :title="t('admin.feedback.newNote.title')"
    size="sm"
    @update:model-value="(v: boolean) => !v && close()"
  >
    <div class="space-y-4">
      <div>
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.categoryLabel') }}</label>
        <Select v-model="category" size="sm" :options="categoryOptions" />
      </div>
      <div>
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.descriptionLabel') }}</label>
        <textarea
          v-model="description"
          rows="4"
          class="w-full rounded-lg bg-white/5 border border-white/10 px-3 py-2 text-sm text-white placeholder:text-white/30 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 resize-y"
          :placeholder="t('admin.feedback.newNote.descriptionPlaceholder')"
        ></textarea>
      </div>
      <p v-if="errorMsg" class="text-sm text-destructive">{{ errorMsg }}</p>
    </div>

    <template #footer>
      <Button variant="ghost" @click="close">{{ t('admin.feedback.newNote.cancel') }}</Button>
      <Button :disabled="submitting" @click="submit">{{ t('admin.feedback.newNote.submit') }}</Button>
    </template>
  </Modal>
</template>
