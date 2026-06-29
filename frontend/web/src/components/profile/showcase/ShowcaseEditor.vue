<script setup lang="ts">
import { ref } from 'vue'
import type { ShowcaseBlock, ShowcaseBlockType } from '@/types/showcase'
import { MAX_SHOWCASE_BLOCKS, defaultVariant, sizeFor, clampSize, spanClasses } from '@/types/showcase'
import ShowcaseBlockView from './ShowcaseBlockView.vue'
import ShowcaseConfigDialog from './ShowcaseConfigDialog.vue'
import Switch from '@/components/ui/Switch.vue'

const props = withDefaults(
  defineProps<{ userId: string; modelValue: ShowcaseBlock[]; enabled?: boolean }>(),
  { enabled: false },
)
const emit = defineEmits<{ save: [ShowcaseBlock[], boolean]; cancel: [] }>()

const local = ref<ShowcaseBlock[]>(props.modelValue.map((b) => ({ ...b })))
const enabled = ref(props.enabled)

// Non-blocking nudge: shown after a Save with content but visibility OFF.
const showHiddenNudge = ref(false)

const ADDABLE: ShowcaseBlockType[] = [
  'about',
  'favorite_anime',
  'stats',
  'favorite_character',
  'card_collection',
  'continue_watching',
  'op_ed',
  'anime_dna',
  'compatibility',
]

function addBlock(type: ShowcaseBlockType) {
  if (local.value.length >= MAX_SHOWCASE_BLOCKS) return
  let config: ShowcaseBlock['config']
  if (type === 'about') {
    config = { title: '', text: '' }
  } else if (type === 'op_ed') {
    config = { theme_ids: [] }
  } else {
    config = {}
  }
  const variant = defaultVariant(type)
  const s = sizeFor(type, variant)
  local.value.push({ type, order: local.value.length, variant, w: s.defW, h: s.defH, config })
}

function removeBlock(i: number) {
  local.value.splice(i, 1)
}

function swapBlocks(i: number, j: number) {
  const a = local.value[i]
  const b = local.value[j]
  if (!a || !b) return
  const ca = clampSize(a.type, a.variant, b.w ?? 0, b.h ?? 0)
  const cb = clampSize(b.type, b.variant, a.w ?? 0, a.h ?? 0)
  a.w = ca.w
  a.h = ca.h
  b.w = cb.w
  b.h = cb.h
  local.value[i] = b
  local.value[j] = a
}

function save() {
  // Empty showcase can never be visible — keep the toggle honest locally too
  // (the backend coerces enabled=false when blocks is empty).
  if (local.value.length === 0) enabled.value = false
  const renumbered = local.value.map((b, i) => ({ ...b, order: i, variant: b.variant ?? defaultVariant(b.type) }))
  // Nudge: saving real content while hidden — offer one-click enable, but still
  // let the plain save through (saves as hidden).
  showHiddenNudge.value = renumbered.length >= 1 && enabled.value === false
  emit('save', renumbered, enabled.value)
}

// Inline "Enable" action from the nudge: flip visibility on, then re-save.
function enableAndSave() {
  enabled.value = true
  showHiddenNudge.value = false
  const renumbered = local.value.map((b, i) => ({ ...b, order: i, variant: b.variant ?? defaultVariant(b.type) }))
  emit('save', renumbered, enabled.value)
}

// ── Resize helpers ────────────────────────────────────────────────
function isFixed(b: ShowcaseBlock): boolean {
  const s = sizeFor(b.type, b.variant)
  return s.minW === s.maxW && s.minH === s.maxH
}

function applyResize(i: number, dCols: number, dRows: number) {
  const b = local.value[i]
  if (!b) return
  const c = clampSize(b.type, b.variant, (b.w ?? 0) + dCols, (b.h ?? 0) + dRows)
  b.w = c.w
  b.h = c.h
}

function applyResizeAbsolute(i: number, w: number, h: number) {
  const b = local.value[i]
  if (!b) return
  const c = clampSize(b.type, b.variant, w, h)
  b.w = c.w
  b.h = c.h
}

function startResize(e: PointerEvent, i: number) {
  e.preventDefault()
  e.stopPropagation()
  const grid = (e.currentTarget as HTMLElement).closest('[data-showcase-grid]') as HTMLElement
  const cols = window.innerWidth < 768 ? 2 : 4
  const gap = 12
  const cellW = (grid.clientWidth - (cols - 1) * gap) / cols
  const rowH = window.innerWidth < 768 ? 165 : 190
  const sx = e.clientX
  const sy = e.clientY
  const sw = local.value[i].w ?? 0
  const sh = local.value[i].h ?? 0
  ;(e.target as HTMLElement).setPointerCapture(e.pointerId)
  const move = (ev: PointerEvent) => {
    applyResizeAbsolute(
      i,
      sw + Math.round((ev.clientX - sx) / (cellW + gap)),
      sh + Math.round((ev.clientY - sy) / (rowH + gap)),
    )
  }
  const up = () => {
    document.removeEventListener('pointermove', move)
    document.removeEventListener('pointerup', up)
  }
  document.addEventListener('pointermove', move)
  document.addEventListener('pointerup', up)
}

// ── Add-block picker ─────────────────────────────────────────────
const pickerOpen = ref(false)
function usedTypes(): Set<string> { return new Set(local.value.map((b) => b.type)) }
function pick(type: ShowcaseBlockType) { addBlock(type); pickerOpen.value = false }

// ── Config dialog ─────────────────────────────────────────────────
const configIdx = ref<number | null>(null)
function openConfig(i: number) { configIdx.value = i }
function closeConfig() { configIdx.value = null }
function onBlockUpdate(updated: ShowcaseBlock) {
  if (configIdx.value !== null) {
    local.value[configIdx.value] = updated
  }
}

// Drag-to-swap state (native HTML5 drag events — no new packages)
const dragSrcIdx = ref<number | null>(null)

function onDragStart(index: number) {
  dragSrcIdx.value = index
}

function onDragEnd() {
  dragSrcIdx.value = null
}

function onDragOver(e: DragEvent) {
  e.preventDefault()
}

function onDrop(targetIdx: number) {
  if (dragSrcIdx.value !== null && dragSrcIdx.value !== targetIdx) {
    swapBlocks(dragSrcIdx.value, targetIdx)
  }
  dragSrcIdx.value = null
}

function blockSpanClasses(el: ShowcaseBlock): string {
  const s = sizeFor(el.type, el.variant)
  return spanClasses(el.w || s.defW, el.h || s.defH)
}

defineExpose({ swapBlocks, applyResize, isFixed, local, pickerOpen, usedTypes, openConfig, closeConfig, configIdx })
</script>

<template>
  <div class="space-y-4">
    <!-- Add-block picker trigger -->
    <div class="relative">
      <button
        type="button"
        data-test="showcase-open-picker"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        :disabled="local.length >= MAX_SHOWCASE_BLOCKS"
        @click="pickerOpen = true"
      >
        + {{ $t('showcase.add_block') }}
      </button>

      <!-- Picker overlay -->
      <div
        v-if="pickerOpen"
        class="absolute left-0 top-8 z-50 flex flex-col gap-1 rounded-xl border border-border bg-card p-3 shadow-lg"
      >
        <p class="mb-1 text-xs font-semibold text-muted-foreground">{{ $t('showcase.add_block_title') }}</p>
        <button
          v-for="type in ADDABLE"
          :key="type"
          type="button"
          :data-test="`picker-${type}`"
          :disabled="usedTypes().has(type)"
          class="rounded-lg border border-border px-3 py-1 text-left text-sm font-medium text-foreground hover:bg-accent disabled:opacity-40 disabled:pointer-events-none"
          @click="pick(type)"
        >
          {{ $t(`showcase.block.${type}`) }}
        </button>
        <button
          type="button"
          class="mt-1 text-xs font-medium text-muted-foreground hover:text-foreground"
          @click="pickerOpen = false"
        >
          {{ $t('showcase.cancel') }}
        </button>
      </div>
      <!-- Backdrop to close picker -->
      <div v-if="pickerOpen" class="fixed inset-0 z-40" @click="pickerOpen = false" />
    </div>

    <!-- Bento grid editor with drag-to-swap -->
    <div data-showcase-grid class="grid grid-cols-2 md:grid-cols-4 gap-3 [grid-auto-flow:dense] [grid-auto-rows:190px]">
      <div
        v-for="(element, index) in local"
        :key="element.order + '-' + element.type"
        :class="['relative rounded-xl border border-border bg-card overflow-hidden cursor-grab', blockSpanClasses(element)]"
        draggable="true"
        @dragstart="onDragStart(index)"
        @dragend="onDragEnd"
        @dragover="onDragOver"
        @drop="onDrop(index)"
      >
        <!-- Live preview of block content -->
        <div class="pointer-events-none h-full w-full opacity-60">
          <ShowcaseBlockView :block="element" :user-id="userId" :is-owner="true" />
        </div>

        <!-- Top-right controls: configure + remove (in line with the block title) -->
        <div class="absolute right-1.5 top-1.5 z-20 flex items-center gap-1.5">
          <button
            type="button"
            :data-test="`showcase-config-${index}`"
            :title="$t('showcase.variant_label')"
            class="grid h-7 w-7 place-items-center rounded-lg border border-border bg-card/90 text-sm text-brand-cyan backdrop-blur-sm hover:text-foreground"
            @click.stop="openConfig(index)"
          >⚙</button>
          <button
            type="button"
            :data-test="`showcase-remove-${index}`"
            :title="$t('showcase.remove_block')"
            class="grid h-7 w-7 place-items-center rounded-lg border border-border bg-card/90 text-sm text-destructive backdrop-blur-sm hover:bg-destructive/10"
            @click.stop="removeBlock(index)"
          >✕</button>
        </div>

        <!-- Corner resize handle — bottom-right; hidden for fixed-size variants and on touch devices -->
        <button
          v-if="!isFixed(element)"
          type="button"
          class="showcase-resize absolute bottom-1.5 right-1.5 grid h-7 w-7 place-items-center rounded-lg border border-border bg-card/90 text-brand-cyan backdrop-blur-sm cursor-nwse-resize touch-none z-20"
          :data-test="`showcase-resize-${index}`"
          @pointerdown="startResize($event, index)"
        >◢</button>
      </div>
    </div>

    <!-- Visibility toggle — publish/unpublish the showcase -->
    <div class="flex items-center justify-between gap-3 rounded-lg border border-border bg-card px-4 py-3">
      <div class="min-w-0">
        <p class="text-sm font-medium text-foreground">{{ $t('showcase.visibleToggle') }}</p>
        <p v-if="local.length === 0" class="text-xs text-muted-foreground">{{ $t('showcase.disabledEmptyHint') }}</p>
      </div>
      <Switch
        v-model="enabled"
        :disabled="local.length === 0"
        data-test="showcase-visible-toggle"
        :aria-label="$t('showcase.visibleToggle')"
      />
    </div>

    <!-- Hidden-with-content nudge — non-blocking; offers one-click enable -->
    <div
      v-if="showHiddenNudge"
      data-test="showcase-hidden-nudge"
      class="flex items-center justify-between gap-3 rounded-lg border border-warning/40 bg-warning/10 px-4 py-3 text-sm text-warning"
    >
      <span class="min-w-0">{{ $t('showcase.hiddenNotice') }}</span>
      <button
        type="button"
        data-test="showcase-enable-now"
        class="shrink-0 rounded-lg border border-warning/50 px-3 py-1 text-sm font-medium text-warning hover:bg-warning/20"
        @click="enableAndSave"
      >
        {{ $t('showcase.enableNow') }}
      </button>
    </div>

    <div class="flex gap-2">
      <button
        type="button"
        data-test="showcase-save"
        class="rounded-lg bg-primary px-4 py-2 text-sm font-semibold text-primary-foreground"
        @click="save"
      >
        {{ $t('showcase.save') }}
      </button>
      <button
        type="button"
        class="rounded-lg border border-border px-4 py-2 text-sm font-medium text-foreground"
        @click="emit('cancel')"
      >
        {{ $t('showcase.cancel') }}
      </button>
    </div>

    <!-- Per-block config dialog — teleport to body to escape overflow clips -->
    <Teleport to="body">
      <ShowcaseConfigDialog
        v-if="configIdx !== null && local[configIdx]"
        :block="local[configIdx]"
        :user-id="userId"
        @update:block="onBlockUpdate"
        @close="closeConfig"
      />
    </Teleport>
  </div>
</template>

<style scoped>
@media (pointer: coarse) {
  .showcase-resize {
    display: none;
  }
}
</style>
