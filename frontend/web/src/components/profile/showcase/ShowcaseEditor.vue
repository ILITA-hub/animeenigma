<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import Select from '@/components/ui/Select.vue'
import type { SelectOption } from '@/components/ui/Select.vue'
import type { AboutConfig, FavoriteAnimeConfig, CardCollectionConfig, OpEdConfig, ShowcaseBlock, ShowcaseBlockType } from '@/types/showcase'
import { MAX_SHOWCASE_BLOCKS, SHOWCASE_VARIANTS, defaultVariant, sizeFor, clampSize, spanClasses } from '@/types/showcase'
import { userApi } from '@/api/client'
import { gachaApi } from '@/api/gacha'
import ShowcaseBlockView from './ShowcaseBlockView.vue'
import ShowcaseConfigDialog from './ShowcaseConfigDialog.vue'

// Narrow an 'about' block's config to AboutConfig for v-model binding. Returns
// the SAME object reference, so v-model assignments still mutate element.config.
// (vue-tsc cannot parse an inline `as` cast inside a v-model expression.)
function aboutConfig(el: ShowcaseBlock): AboutConfig {
  return el.config as AboutConfig
}

const props = defineProps<{ userId: string; modelValue: ShowcaseBlock[] }>()
const emit = defineEmits<{ save: [ShowcaseBlock[]]; cancel: [] }>()

const { t } = useI18n()
const toast = useToast()

const local = ref<ShowcaseBlock[]>(props.modelValue.map((b) => ({ ...b })))

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

const AUTO_TYPES: ShowcaseBlockType[] = ['continue_watching', 'anime_dna', 'compatibility']

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
  const renumbered = local.value.map((b, i) => ({ ...b, order: i, variant: b.variant ?? defaultVariant(b.type) }))
  emit('save', renumbered)
}

function variantOptions(type: ShowcaseBlockType): SelectOption[] {
  return SHOWCASE_VARIANTS[type].map((v) => ({ value: v, label: v }))
}

async function autoFillAnime(el: ShowcaseBlock) {
  try {
    const res = await userApi.getWatchlist({ sort: 'score', order: 'desc', per_page: 12 })
    const items = (res.data?.data ?? res.data) as Array<{ anime_id: string; score?: number }>
    const sorted = [...items].sort((a, b) => (b.score ?? 0) - (a.score ?? 0)).slice(0, 12)
    ;(el.config as FavoriteAnimeConfig).anime_ids = sorted.map((i) => i.anime_id)
  } catch {
    toast.push(t('showcase.auto_fill_error'), 'error')
  }
}

async function autoFillCards(el: ShowcaseBlock) {
  try {
    const res = await gachaApi.getCollection()
    const view = res.data?.data ?? res.data
    const RARITY_ORDER: Record<string, number> = { SSR: 4, SR: 3, R: 2, N: 1 }
    const owned = view.cards
      .filter((c: { owned: boolean }) => c.owned)
      .sort(
        (
          a: { card: { rarity: string; created_at: string } },
          b: { card: { rarity: string; created_at: string } },
        ) => {
          const rd = (RARITY_ORDER[b.card.rarity] ?? 0) - (RARITY_ORDER[a.card.rarity] ?? 0)
          if (rd !== 0) return rd
          return new Date(b.card.created_at).getTime() - new Date(a.card.created_at).getTime()
        },
      )
      .slice(0, 12)
    ;(el.config as CardCollectionConfig).card_ids = owned.map((c: { card: { id: string } }) => c.card.id)
  } catch {
    toast.push(t('showcase.auto_fill_error'), 'error')
  }
}

const newThemeId = ref<Record<number, string>>({})

function addThemeId(el: ShowcaseBlock, index: number, id: string) {
  const cfg = el.config as OpEdConfig
  const trimmed = id.trim()
  if (trimmed && !cfg.theme_ids.includes(trimmed)) cfg.theme_ids.push(trimmed)
  newThemeId.value[index] = ''
}

function removeThemeId(el: ShowcaseBlock, id: string) {
  const cfg = el.config as OpEdConfig
  cfg.theme_ids = cfg.theme_ids.filter((t) => t !== id)
}

function opEdConfig(el: ShowcaseBlock): OpEdConfig {
  return el.config as OpEdConfig
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

        <!-- Corner resize handle — hidden for fixed-size variants and on touch devices -->
        <button
          v-if="!isFixed(element)"
          type="button"
          class="showcase-resize absolute bottom-1 right-1 grid h-6 w-6 place-items-center rounded-lg border border-border text-brand-cyan cursor-nwse-resize touch-none z-10"
          :data-test="`showcase-resize-${index}`"
          @pointerdown="startResize($event, index)"
        >◢</button>

        <!-- Config overlay anchored to bottom -->
        <div class="absolute inset-x-0 bottom-0 flex flex-col gap-1 bg-card/90 p-2 backdrop-blur-sm">
          <div class="flex items-center justify-between">
            <span class="showcase-drag-handle cursor-grab text-xs font-semibold text-foreground">
              ⠿ {{ $t(`showcase.block.${element.type}`) }}
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                :data-test="`showcase-config-${index}`"
                class="text-xs font-medium text-brand-cyan hover:text-foreground"
                @click.stop="openConfig(index)"
              >⚙</button>
              <button
                type="button"
                :data-test="`showcase-remove-${index}`"
                class="text-xs font-medium text-destructive"
                @click="removeBlock(index)"
              >
                {{ $t('showcase.remove_block') }}
              </button>
            </div>
          </div>

          <!-- Variant picker — only for types with >1 variant -->
          <div
            v-if="SHOWCASE_VARIANTS[element.type as ShowcaseBlockType].length > 1"
          >
            <Select
              :model-value="element.variant ?? SHOWCASE_VARIANTS[element.type as ShowcaseBlockType][0]"
              :options="variantOptions(element.type)"
              :label="$t('showcase.variant_label')"
              @update:model-value="element.variant = $event as string"
            />
          </div>

          <!-- About block inline editor -->
          <div v-if="element.type === 'about'" class="space-y-1">
            <input
              v-model="aboutConfig(element).title"
              :placeholder="$t('showcase.about_title_placeholder')"
              maxlength="64"
              class="w-full rounded-lg border border-border bg-background px-3 py-1 text-xs"
            />
            <textarea
              v-model="aboutConfig(element).text"
              :placeholder="$t('showcase.about_placeholder')"
              rows="2"
              maxlength="2000"
              class="w-full rounded-lg border border-border bg-background px-3 py-1 text-xs"
            />
          </div>

          <!-- favorite_anime: picker hint + Auto button -->
          <div v-else-if="element.type === 'favorite_anime'" class="flex items-center gap-2">
            <p class="flex-1 text-xs text-muted-foreground">{{ $t('showcase.pick_anime') }}</p>
            <button
              type="button"
              :data-test="`showcase-auto-anime-${index}`"
              class="rounded-lg border border-border px-2 py-0.5 text-xs font-medium text-foreground hover:bg-accent"
              @click="autoFillAnime(element)"
            >
              {{ $t('showcase.auto_fill') }}
            </button>
          </div>

          <!-- favorite_character: picker hint -->
          <div v-else-if="element.type === 'favorite_character'">
            <p class="text-xs text-muted-foreground">{{ $t('showcase.pick_character') }}</p>
          </div>

          <!-- card_collection: picker hint + Auto button -->
          <div v-else-if="element.type === 'card_collection'" class="flex items-center gap-2">
            <p class="flex-1 text-xs text-muted-foreground">{{ $t('showcase.pick_cards') }}</p>
            <button
              type="button"
              :data-test="`showcase-auto-cards-${index}`"
              class="rounded-lg border border-border px-2 py-0.5 text-xs font-medium text-foreground hover:bg-accent"
              @click="autoFillCards(element)"
            >
              {{ $t('showcase.auto_fill') }}
            </button>
          </div>

          <!-- op_ed: theme ID list + add input -->
          <div v-else-if="element.type === 'op_ed'" class="space-y-1">
            <p class="text-xs font-medium text-muted-foreground">{{ $t('showcase.pick_theme') }}</p>
            <div class="flex flex-wrap gap-1">
              <span
                v-for="tid in opEdConfig(element).theme_ids"
                :key="tid"
                class="flex items-center gap-1 rounded-md border border-border px-2 py-0.5 text-xs"
              >
                {{ tid }}
                <button
                  type="button"
                  class="text-destructive"
                  :data-test="`showcase-remove-theme-${tid}`"
                  @click="removeThemeId(element, tid)"
                >
                  ×
                </button>
              </span>
            </div>
            <div class="flex gap-2">
              <input
                :value="newThemeId[index] ?? ''"
                :placeholder="$t('showcase.op_ed_add_theme')"
                class="flex-1 rounded-lg border border-border bg-background px-2 py-0.5 text-xs"
                data-test="showcase-theme-input"
                @input="newThemeId[index] = ($event.target as HTMLInputElement).value"
                @keydown.enter.prevent="addThemeId(element, index, newThemeId[index] ?? '')"
              />
              <button
                type="button"
                class="rounded-lg border border-border px-2 py-0.5 text-xs font-medium text-foreground hover:bg-accent"
                data-test="showcase-theme-add"
                @click="addThemeId(element, index, newThemeId[index] ?? '')"
              >
                +
              </button>
            </div>
          </div>

          <!-- Auto types (continue_watching, anime_dna, compatibility) -->
          <div v-else-if="AUTO_TYPES.includes(element.type)">
            <p class="text-xs text-muted-foreground">{{ $t('showcase.auto_block_info') }}</p>
          </div>

          <!-- stats fallback -->
          <div v-else>
            <p class="text-xs text-muted-foreground">{{ $t('showcase.pick_anime') }}</p>
          </div>
        </div>
      </div>
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
