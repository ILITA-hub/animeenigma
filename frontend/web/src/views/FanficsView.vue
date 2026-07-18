<!--
  /fanfics — fanfic generation engine. Route visibility resolved at runtime
  via the policy feed (useFeatureVisible / policy-service, key 'fanfic'),
  not a build flag.

  Two tabs:
    - "Генерировать": GenerateForm -> fanficApi.generate() SSE stream ->
      reactive `content` ref rendered live by FanficReader (streaming caret).
      On `done`, shows a "saved" toast + Regenerate/Copy actions.
    - "Моя библиотека": LibraryGrid -> clicking a card fetches the full
      Fanfic and opens it in a FanficReader dialog.
-->
<script setup lang="ts">
import { ref, watch, onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { fanficApi } from '@/api/fanfic'
import { useToast } from '@/composables/useToast'
import { useFanficVisible } from '@/utils/fanficGate'
import { Tabs, Modal, Button, Alert } from '@/components/ui'
import GenerateForm from '@/components/fanfic/GenerateForm.vue'
import FanficReader from '@/components/fanfic/FanficReader.vue'
import LibraryGrid from '@/components/fanfic/LibraryGrid.vue'
import type { GenerateInput, Fanfic } from '@/types/fanfic'

const { t } = useI18n()
const { push: pushToast } = useToast()
const route = useRoute()
const router = useRouter()

// The daily-fanfic deep link (?daily=1) bypasses the route guard's fanfic
// gate — the daily reader is public. Viewers WITHOUT the fanfic feature get
// only the reader dialog: the authoring tabs stay hidden, and leaving the
// dialog (or failing to open it) routes them home instead of stranding them
// on an empty authoring shell.
const fanficVisible = useFanficVisible()

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
  continueAbort?.abort()
})

// ── Library tab: reader dialog ──────────────────────────────────────────────

const readerOpen = ref(false)
const readerFanfic = ref<Fanfic | null>(null)
// True only when the reader was opened via the daily-spotlight deep link
// (openDailyFanfic below) — gates the Modal footer's owner-scoped
// "Продолжить" button off, since fanficApi.getDaily()'s fanfic is very
// rarely the current user's own (see its doc comment in src/api/fanfic.ts).
const readerIsDaily = ref(false)

async function onOpenFanfic(id: string): Promise<void> {
  try {
    readerFanfic.value = await fanficApi.get(id)
    readerIsDaily.value = false
    readerOpen.value = true
  } catch {
    // Fetch failed — stay on the grid, nothing to show.
  }
}

// Deep-link entry point for DailyFanficCard's "Читать" CTA
// (`/fanfics?daily=1`, see DailyFanficCard.vue). Never opens the reader with
// empty/gated content — an explicit pick surfaces a toast instead (login nudge
// for anon readers, an explicit-content notice for logged-in ones the backend
// still gates). Fired once from onMounted below, not from a query watcher.
async function openDailyFanfic(): Promise<void> {
  try {
    const daily = await fanficApi.getDaily()
    if (daily.gated || !daily.content) {
      pushToast(
        daily.gate_reason === 'login' ? t('fanfic.daily.loginRequired') : t('fanfic.daily.gated'),
        'info',
      )
      leaveIfReaderOnly()
      return
    }
    readerFanfic.value = daily
    readerIsDaily.value = true
    readerOpen.value = true
  } catch {
    pushToast(t('fanfic.daily.loadError'), 'error')
    leaveIfReaderOnly()
  }
}

// Reader-only viewers (no fanfic feature) have nothing else on this page —
// send them home once the daily dialog is gone. Toasts are app-level, so a
// nudge pushed just before still shows after the navigation.
function leaveIfReaderOnly(): void {
  if (!fanficVisible.value) void router.replace({ name: 'home' })
}

watch(readerOpen, (open) => {
  if (!open) leaveIfReaderOnly()
})

onMounted(() => {
  if (route.query.daily === '1') {
    void openDailyFanfic()
  }
})

function onRemoveFanfic(id: string): void {
  if (readerFanfic.value?.id === id) {
    readerOpen.value = false
    readerFanfic.value = null
  }
}

const continuing = ref(false)
let continueAbort: AbortController | null = null

async function onContinueFanfic(): Promise<void> {
  const f = readerFanfic.value
  if (!f || continuing.value) return
  continuing.value = true
  continueAbort?.abort()
  continueAbort = new AbortController()
  await fanficApi.continueStory(
    f.id,
    {
      onDelta: (text) => {
        if (readerFanfic.value) readerFanfic.value.content += text
      },
      onDone: (_id, _title, _usage, part) => {
        if (readerFanfic.value && part) readerFanfic.value.part_count = part
        continuing.value = false
        libraryGridRef.value?.refresh()
      },
      onError: () => {
        continuing.value = false
      },
    },
    continueAbort.signal,
  )
  continuing.value = false
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
  continuing,
  onContinueFanfic,
  readerOpen,
  readerFanfic,
  readerIsDaily,
  openDailyFanfic,
  onOpenFanfic,
})
</script>

<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-5xl">
      <h1 class="text-3xl font-semibold text-white mb-1">{{ t('fanfic.title') }}</h1>

      <Tabs
        v-if="fanficVisible"
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
      <FanficReader v-if="readerFanfic" :title="readerFanfic.title" :content="readerFanfic.content" :streaming="continuing" />
      <template v-if="readerFanfic && readerFanfic.status === 'complete' && !readerIsDaily" #footer>
        <Button :loading="continuing" @click="onContinueFanfic">{{ t('fanfic.reader.continue') }}</Button>
      </template>
    </Modal>
  </div>
</template>
