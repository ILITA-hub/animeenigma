<!--
  /fanfics — admin-gated (VITE_FANFIC_ADMIN_ONLY) fanfic generation engine.

  Two tabs:
    - "Генерировать": GenerateForm -> fanficApi.generate() SSE stream ->
      reactive `content` ref rendered live by FanficReader (streaming caret).
      On `done`, shows a "saved" toast + Regenerate/Copy actions.
    - "Моя библиотека": LibraryGrid -> clicking a card fetches the full
      Fanfic and opens it in a FanficReader dialog.
-->
<script setup lang="ts">
import { ref, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { fanficApi } from '@/api/fanfic'
import { useToast } from '@/composables/useToast'
import { Tabs, Modal, Button, Alert } from '@/components/ui'
import GenerateForm from '@/components/fanfic/GenerateForm.vue'
import FanficReader from '@/components/fanfic/FanficReader.vue'
import LibraryGrid from '@/components/fanfic/LibraryGrid.vue'
import type { GenerateInput, Fanfic } from '@/types/fanfic'

const { t } = useI18n()
const { push: pushToast } = useToast()

const activeTab = ref<'generate' | 'library'>('generate')

// ── Generate tab: SSE streaming state ───────────────────────────────────────

const generating = ref(false)
const genTitle = ref('')
const content = ref('')
const genError = ref('')
const lastInput = ref<GenerateInput | null>(null)
const libraryGridRef = ref<InstanceType<typeof LibraryGrid> | null>(null)

let abortController: AbortController | null = null

async function onGenerate(input: GenerateInput): Promise<void> {
  lastInput.value = input
  generating.value = true
  genTitle.value = ''
  content.value = ''
  genError.value = ''

  abortController?.abort()
  abortController = new AbortController()

  await fanficApi.generate(
    input,
    {
      onDelta: (text) => {
        content.value += text
      },
      onDone: (_id, title) => {
        genTitle.value = title
        generating.value = false
        pushToast(t('fanfic.reader.saved'), 'success')
        // LibraryGrid only mounts on the "library" tab (Tabs renders a single
        // active named-slot, so switching tabs destroys/recreates it) — this
        // covers the case where it's already mounted; the remount-on-switch
        // path covers the far more common case of generating then browsing over.
        libraryGridRef.value?.refresh()
      },
      onError: (message) => {
        genError.value = message
        generating.value = false
      },
    },
    abortController.signal,
  )
  generating.value = false
}

function onRegenerate(): void {
  if (lastInput.value) void onGenerate(lastInput.value)
}

async function onCopy(): Promise<void> {
  try {
    await navigator.clipboard.writeText(content.value)
    pushToast(t('fanfic.reader.copied'), 'success')
  } catch {
    // Clipboard API can be unavailable/denied — fail silently, nothing to recover.
  }
}

onBeforeUnmount(() => {
  abortController?.abort()
})

// ── Library tab: reader dialog ──────────────────────────────────────────────

const readerOpen = ref(false)
const readerFanfic = ref<Fanfic | null>(null)

async function onOpenFanfic(id: string): Promise<void> {
  try {
    readerFanfic.value = await fanficApi.get(id)
    readerOpen.value = true
  } catch {
    // Fetch failed — stay on the grid, nothing to show.
  }
}

function onRemoveFanfic(id: string): void {
  if (readerFanfic.value?.id === id) {
    readerOpen.value = false
    readerFanfic.value = null
  }
}

// Exposed for src/views/__tests__/FanficsView.spec.ts — the streaming state
// lives entirely in this component (Tabs only mounts one of #generate/
// #library at a time, so a plain mount+DOM-only test can't reach the
// SSE-driven reactive state or the `libraryGridRef` seam directly).
defineExpose({
  activeTab,
  generating,
  content,
  genTitle,
  genError,
  libraryGridRef,
  onGenerate,
})
</script>

<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-5xl">
      <h1 class="text-3xl font-semibold text-white mb-1">{{ t('fanfic.title') }}</h1>

      <Tabs
        v-model="activeTab"
        class="mt-6"
        :tabs="[
          { value: 'generate', label: t('fanfic.tabs.generate') },
          { value: 'library', label: t('fanfic.tabs.library') },
        ]"
      >
        <template #generate>
          <div class="grid gap-6 lg:grid-cols-2">
            <div class="glass-card p-4 md:p-6 lg:p-8">
              <GenerateForm :disabled="generating" @generate="onGenerate" />
            </div>

            <div class="glass-card p-4 md:p-6 lg:p-8 min-h-[200px]">
              <template v-if="content || generating">
                <FanficReader :title="genTitle" :content="content" :streaming="generating" />
                <div v-if="!generating" class="flex justify-end gap-2 mt-6">
                  <Button variant="outline" size="sm" @click="onCopy">{{ t('fanfic.reader.copy') }}</Button>
                  <Button variant="outline" size="sm" @click="onRegenerate">{{ t('fanfic.reader.regenerate') }}</Button>
                </div>
              </template>
              <p v-else class="text-sm text-muted-foreground">{{ t('fanfic.reader.empty') }}</p>
              <Alert v-if="genError" variant="destructive" class="mt-4">{{ genError }}</Alert>
            </div>
          </div>
        </template>

        <template #library>
          <LibraryGrid ref="libraryGridRef" @open="onOpenFanfic" @remove="onRemoveFanfic" />
        </template>
      </Tabs>
    </div>

    <Modal v-model="readerOpen" :title="readerFanfic?.title" size="xl">
      <FanficReader v-if="readerFanfic" :title="readerFanfic.title" :content="readerFanfic.content" />
    </Modal>
  </div>
</template>
