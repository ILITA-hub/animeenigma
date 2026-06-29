<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ShowcaseBlock } from '@/types/showcase'
import { spanClasses, sizeFor } from '@/types/showcase'
import { showcaseApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import ShowcaseBlockView from './ShowcaseBlockView.vue'
import ShowcaseEditor from './ShowcaseEditor.vue'

const props = defineProps<{ userId: string; isOwner: boolean; autoEdit?: boolean }>()
const emit = defineEmits<{
  loaded: [number]
  change: [{ enabled: boolean; count: number }]
  editorClosed: []
}>()

const { t } = useI18n()
const toast = useToast()

const blocks = ref<ShowcaseBlock[]>([])
const enabled = ref(false)
const editing = ref(false)
const loading = ref(true)

function cellClass(b: ShowcaseBlock): string {
  const s = sizeFor(b.type, b.variant)
  return spanClasses(b.w || s.defW, b.h || s.defH)
}

async function load() {
  loading.value = true
  try {
    const res = await showcaseApi.getShowcase(props.userId)
    const data = 'data' in res.data
      ? (res.data as { data: { blocks: ShowcaseBlock[]; enabled: boolean } }).data
      : (res.data as { blocks: ShowcaseBlock[]; enabled: boolean })
    blocks.value = data.blocks ?? []
    enabled.value = !!data.enabled
  } catch {
    blocks.value = []
    enabled.value = false
  } finally {
    loading.value = false
    emit('loaded', blocks.value.length)
  }
}

async function onSave(next: ShowcaseBlock[], nextEnabled: boolean) {
  try {
    const res = await showcaseApi.saveShowcase(next, nextEnabled)
    const data = 'data' in res.data
      ? (res.data as { data: { blocks: ShowcaseBlock[]; enabled: boolean } }).data
      : (res.data as { blocks: ShowcaseBlock[]; enabled: boolean })
    // Backend coerces enabled=false for an empty showcase — trust its echo.
    const coerced = !!data.enabled
    blocks.value = next
    enabled.value = coerced
    editing.value = false
    toast.push(t('showcase.saved'), 'success')
    emit('change', { enabled: coerced, count: next.length })
    emit('editorClosed')
  } catch {
    toast.push(t('showcase.save_error'), 'error')
  }
}

function onCancel() {
  editing.value = false
  emit('editorClosed')
}

onMounted(async () => {
  if (props.autoEdit) editing.value = true
  await load()
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <h2 class="text-xl font-semibold text-foreground">{{ $t('showcase.title') }}</h2>
      <button
        v-if="isOwner && !editing"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="editing = true"
      >
        {{ $t('showcase.edit') }}
      </button>
    </div>

    <ShowcaseEditor
      v-if="editing"
      :user-id="userId"
      :model-value="blocks"
      :enabled="enabled"
      @save="onSave"
      @cancel="onCancel"
    />

    <template v-else>
      <p v-if="!loading && !blocks.length" class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
      <div v-else class="grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense] [grid-auto-rows:165px] md:[grid-auto-rows:190px]">
        <div v-for="(b, i) in blocks" :key="i" :data-showcase-cell="b.type" :class="['h-full', cellClass(b)]">
          <ShowcaseBlockView :block="b" :user-id="userId" :is-owner="isOwner" />
        </div>
      </div>
    </template>
  </section>
</template>
