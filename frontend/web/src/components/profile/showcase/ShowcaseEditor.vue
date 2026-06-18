<script setup lang="ts">
import { ref } from 'vue'
import draggable from 'vuedraggable'
import Select from '@/components/ui/Select.vue'
import type { SelectOption } from '@/components/ui/Select.vue'
import type { AboutConfig, FavoriteAnimeConfig, CardCollectionConfig, OpEdConfig, ShowcaseBlock, ShowcaseBlockType } from '@/types/showcase'
import { MAX_SHOWCASE_BLOCKS, SHOWCASE_VARIANTS, defaultVariant } from '@/types/showcase'
import { userApi } from '@/api/client'
import { gachaApi } from '@/api/gacha'

// Narrow an 'about' block's config to AboutConfig for v-model binding. Returns
// the SAME object reference, so v-model assignments still mutate element.config.
// (vue-tsc cannot parse an inline `as` cast inside a v-model expression.)
function aboutConfig(el: ShowcaseBlock): AboutConfig {
  return el.config as AboutConfig
}

const props = defineProps<{ userId: string; modelValue: ShowcaseBlock[] }>()
const emit = defineEmits<{ save: [ShowcaseBlock[]]; cancel: [] }>()

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
  local.value.push({ type, order: local.value.length, variant: defaultVariant(type), config })
}

function removeBlock(i: number) {
  local.value.splice(i, 1)
}

function save() {
  const renumbered = local.value.map((b, i) => ({ ...b, order: i, variant: b.variant ?? defaultVariant(b.type) }))
  emit('save', renumbered)
}

function variantOptions(type: ShowcaseBlockType): SelectOption[] {
  return SHOWCASE_VARIANTS[type].map((v) => ({ value: v, label: v }))
}

async function autoFillAnime(el: ShowcaseBlock) {
  const res = await userApi.getWatchlist({ sort: 'score', order: 'desc', per_page: 12 })
  const items = (res.data?.data ?? res.data) as Array<{ anime_id: string; score?: number }>
  const sorted = [...items].sort((a, b) => (b.score ?? 0) - (a.score ?? 0)).slice(0, 12)
  ;(el.config as FavoriteAnimeConfig).anime_ids = sorted.map((i) => i.anime_id)
}

async function autoFillCards(el: ShowcaseBlock) {
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
}

const newThemeId = ref('')

function addThemeId(el: ShowcaseBlock, id: string) {
  const cfg = el.config as OpEdConfig
  const trimmed = id.trim()
  if (trimmed && !cfg.theme_ids.includes(trimmed)) cfg.theme_ids.push(trimmed)
  newThemeId.value = ''
}

function removeThemeId(el: ShowcaseBlock, id: string) {
  const cfg = el.config as OpEdConfig
  cfg.theme_ids = cfg.theme_ids.filter((t) => t !== id)
}

function opEdConfig(el: ShowcaseBlock): OpEdConfig {
  return el.config as OpEdConfig
}
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <button
        v-for="t in ADDABLE"
        :key="t"
        type="button"
        class="rounded-lg border border-border px-3 py-1 text-sm font-medium text-foreground hover:bg-accent"
        @click="addBlock(t)"
      >
        + {{ $t(`showcase.block.${t}`) }}
      </button>
    </div>

    <draggable v-model="local" item-key="order" handle=".showcase-drag-handle">
      <template #item="{ element, index }">
        <div class="mb-3 rounded-xl border border-border bg-card p-3">
          <div class="mb-2 flex items-center justify-between">
            <span class="showcase-drag-handle cursor-grab text-sm font-semibold text-foreground">
              ⠿ {{ $t(`showcase.block.${element.type}`) }}
            </span>
            <button
              type="button"
              :data-test="`showcase-remove-${index}`"
              class="text-sm font-medium text-destructive"
              @click="removeBlock(index)"
            >
              {{ $t('showcase.remove_block') }}
            </button>
          </div>

          <!-- Variant picker — only for types with >1 variant -->
          <div
            v-if="SHOWCASE_VARIANTS[element.type].length > 1"
            class="mb-2"
          >
            <Select
              :model-value="element.variant ?? SHOWCASE_VARIANTS[element.type][0]"
              :options="variantOptions(element.type)"
              :label="$t('showcase.variant_label')"
              @update:model-value="element.variant = $event as string"
            />
          </div>

          <!-- About block inline editor -->
          <div v-if="element.type === 'about'" class="space-y-2">
            <input
              v-model="aboutConfig(element).title"
              :placeholder="$t('showcase.about_title_placeholder')"
              maxlength="64"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
            <textarea
              v-model="aboutConfig(element).text"
              :placeholder="$t('showcase.about_placeholder')"
              rows="4"
              maxlength="2000"
              class="w-full rounded-lg border border-border bg-background px-3 py-2 text-sm"
            />
          </div>

          <!-- favorite_anime: picker hint + Auto button -->
          <div v-else-if="element.type === 'favorite_anime'" class="flex items-center gap-2">
            <p class="flex-1 text-xs text-muted-foreground">{{ $t('showcase.pick_anime') }}</p>
            <button
              type="button"
              :data-test="`showcase-auto-anime-${index}`"
              class="rounded-lg border border-border px-3 py-1 text-xs font-medium text-foreground hover:bg-accent"
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
              class="rounded-lg border border-border px-3 py-1 text-xs font-medium text-foreground hover:bg-accent"
              @click="autoFillCards(element)"
            >
              {{ $t('showcase.auto_fill') }}
            </button>
          </div>

          <!-- op_ed: theme ID list + add input -->
          <div v-else-if="element.type === 'op_ed'" class="space-y-2">
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
                v-model="newThemeId"
                :placeholder="$t('showcase.op_ed_add_theme')"
                class="flex-1 rounded-lg border border-border bg-background px-3 py-1.5 text-xs"
                data-test="showcase-theme-input"
                @keydown.enter.prevent="addThemeId(element, newThemeId)"
              />
              <button
                type="button"
                class="rounded-lg border border-border px-3 py-1 text-xs font-medium text-foreground hover:bg-accent"
                data-test="showcase-theme-add"
                @click="addThemeId(element, newThemeId)"
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
      </template>
    </draggable>

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
  </div>
</template>
