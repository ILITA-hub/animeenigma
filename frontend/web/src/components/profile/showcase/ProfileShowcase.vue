<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ShowcaseBlock } from '@/types/showcase'
import { spanClasses, sizeFor } from '@/types/showcase'
import { showcaseApi } from '@/api/client'
import { useToast } from '@/composables/useToast'
import ShowcaseBlockView from './ShowcaseBlockView.vue'
import ShowcaseEditor from './ShowcaseEditor.vue'

const props = defineProps<{ userId: string; isOwner: boolean }>()
const emit = defineEmits<{ loaded: [number] }>()

const { t } = useI18n()
const toast = useToast()

const blocks = ref<ShowcaseBlock[]>([])
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
      ? (res.data as { data: { blocks: ShowcaseBlock[] } }).data
      : res.data
    blocks.value = data.blocks ?? []
  } catch {
    blocks.value = []
  } finally {
    loading.value = false
    emit('loaded', blocks.value.length)
  }
}

async function onSave(next: ShowcaseBlock[]) {
  try {
    await showcaseApi.saveShowcase(next)
    blocks.value = next
    editing.value = false
    toast.push(t('showcase.saved'), 'success')
  } catch {
    toast.push(t('showcase.save_error'), 'error')
  }
}

onMounted(load)
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
      @save="onSave"
      @cancel="editing = false"
    />

    <template v-else>
      <p v-if="!loading && !blocks.length" class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
      <div v-else class="grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense] [grid-auto-rows:165px] md:[grid-auto-rows:190px]">
        <template v-for="(b, i) in blocks" :key="i">
          <div :data-showcase-cell="b.type" :class="['h-full', cellClass(b)]">
            <ShowcaseBlockView :block="b" :user-id="userId" :is-owner="isOwner" />
          </div>
        </template>
      </div>
    </template>
  </section>
</template>
