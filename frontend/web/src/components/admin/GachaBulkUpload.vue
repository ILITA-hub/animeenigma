<template>
  <!-- Bulk cover upload: every picked image becomes a DISABLED draft card
       (rarity N, name = file stem) via the existing upload+create endpoints. -->
  <Modal
    :model-value="modelValue"
    :title="$t('gacha.admin.bulk_upload_title')"
    :closable="!running"
    :close-on-backdrop="!running"
    :close-on-esc="!running"
    @update:model-value="v => { if (v || !running) emit('update:modelValue', v) }"
  >
    <div
      class="border-2 border-dashed border-white/20 rounded-lg p-6 text-center cursor-pointer transition-colors"
      :class="dragOver ? 'border-white/60 bg-white/5' : 'hover:border-white/40'"
      data-testid="bulk-drop-zone"
      @click="fileInput?.click()"
      @dragover.prevent="dragOver = true"
      @dragleave.prevent="dragOver = false"
      @drop.prevent="onDrop"
    >
      <Upload class="size-8 mx-auto mb-2 text-white/40" />
      <p class="text-white/70 text-sm">{{ $t('gacha.admin.bulk_upload_hint') }}</p>
      <input
        ref="fileInput"
        type="file"
        accept="image/*"
        multiple
        class="hidden"
        data-testid="bulk-file-input"
        @change="onPick"
      />
    </div>

    <div v-if="items.length > 0" class="mt-4">
      <p class="text-white/70 text-sm mb-2">
        {{ $t('gacha.admin.bulk_upload_progress', { done: doneCount, total: items.length }) }}
      </p>
      <ul class="max-h-48 overflow-y-auto space-y-1 text-sm">
        <li v-for="(item, i) in items" :key="i" class="flex items-center gap-2">
          <Spinner v-if="item.status === 'uploading'" class="size-3" />
          <span v-else-if="item.status === 'done'" class="text-teal-400">✓</span>
          <span v-else-if="item.status === 'error'" class="text-destructive">✗</span>
          <span v-else class="text-white/40">•</span>
          <span class="truncate text-white/80">{{ item.file.name }}</span>
        </li>
      </ul>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <Button
          v-if="errorCount > 0 && !running"
          variant="outline"
          data-testid="bulk-retry-btn"
          @click="retryFailed"
        >
          {{ $t('gacha.admin.bulk_upload_retry', { n: errorCount }) }}
        </Button>
        <Button variant="outline" :disabled="running" @click="emit('update:modelValue', false)">
          {{ $t('gacha.admin.bulk_upload_close') }}
        </Button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Upload } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { gachaAdminApi } from '@/api/gacha'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import Spinner from '@/components/ui/Spinner.vue'

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()

const { t } = useI18n()

type ItemStatus = 'pending' | 'uploading' | 'done' | 'error'
interface UploadItem {
  file: File
  status: ItemStatus
}

const items = ref<UploadItem[]>([])
const running = ref(false)
const dragOver = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)

const doneCount = computed(() => items.value.filter(i => i.status === 'done').length)
const errorCount = computed(() => items.value.filter(i => i.status === 'error').length)

// Fresh queue on every reopen.
watch(() => props.modelValue, open => {
  if (open) items.value = []
})

function onPick(e: Event) {
  const input = e.target as HTMLInputElement
  if (input.files) addFiles(Array.from(input.files))
  input.value = ''
}

function onDrop(e: DragEvent) {
  dragOver.value = false
  const files = e.dataTransfer?.files
  if (files) addFiles(Array.from(files).filter(f => f.type.startsWith('image/')))
}

function addFiles(files: File[]) {
  if (files.length === 0) return
  items.value.push(...files.map(file => ({ file, status: 'pending' as ItemStatus })))
  void run()
}

/** Card name = file stem; backend rejects empty names, so fall back. */
function nameFromFile(file: File): string {
  const stem = file.name.replace(/\.[^.]+$/, '').trim()
  return stem || t('gacha.admin.bulk_unnamed')
}

async function processItem(item: UploadItem) {
  // Status flips to 'uploading' synchronously — that is the claim that stops
  // a sibling worker from picking the same item (single-threaded JS: no await
  // between a worker's find() and this line).
  item.status = 'uploading'
  try {
    const res = await gachaAdminApi.uploadFile(item.file, 'cards')
    const data = (res as { data?: { data?: { image_path?: string } } }).data
    const imagePath = data?.data?.image_path ?? ''
    if (!imagePath) throw new Error('empty image_path')
    await gachaAdminApi.createCard({
      name: nameFromFile(item.file),
      source_title: '',
      rarity: 'N',
      enabled: false,
      image_path: imagePath,
      back_path: '',
      group_ids: [],
    })
    item.status = 'done'
  } catch {
    item.status = 'error'
  }
}

/** Drain all pending items with at most 3 concurrent workers. */
async function run() {
  if (running.value) return
  running.value = true
  try {
    const workers = Array.from({ length: 3 }, async () => {
      for (;;) {
        const next = items.value.find(i => i.status === 'pending')
        if (!next) return
        await processItem(next)
      }
    })
    await Promise.all(workers)
  } finally {
    running.value = false
  }
}

function retryFailed() {
  for (const item of items.value) {
    if (item.status === 'error') item.status = 'pending'
  }
  void run()
}
</script>
