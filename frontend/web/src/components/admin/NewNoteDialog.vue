<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Modal, Select, Button } from '@/components/ui'
import { adminApi } from '@/api/client'
import type { FeedbackKind } from '@/types/feedback'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ 'update:open': [boolean]; created: [string] }>()

const { t } = useI18n()

const kind = ref<FeedbackKind>('todo')
const category = ref('')
const description = ref('')
const submitting = ref(false)
const errorMsg = ref('')

const kindOptions = [
  { value: 'todo', label: t('admin.feedback.kind.todo') },
  { value: 'idea', label: t('admin.feedback.kind.idea') },
  { value: 'feedback', label: t('admin.feedback.kind.feedback') },
]
const categoryOptions = [
  { value: '', label: t('admin.feedback.newNote.categoryNone') },
  { value: 'bug', label: t('admin.feedback.category.bug') },
  { value: 'issue', label: t('admin.feedback.category.issue') },
  { value: 'feature', label: t('admin.feedback.category.feature') },
]

function reset(): void {
  kind.value = 'todo'
  category.value = ''
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
      kind: kind.value,
      category: category.value || undefined,
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

defineExpose({ kind, category, description, submit })
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
        <label class="block text-sm font-medium text-white/70 mb-2">{{ t('admin.feedback.newNote.kindLabel') }}</label>
        <Select v-model="kind" size="sm" :options="kindOptions" />
      </div>
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
