<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useToast } from '@/composables/useToast'
import type { ShowcaseBlock, ShowcaseBlockType, AboutConfig, FavoriteAnimeConfig, CardCollectionConfig, OpEdConfig } from '@/types/showcase'
import { SHOWCASE_VARIANTS, clampSize } from '@/types/showcase'
import { userApi } from '@/api/client'
import { gachaApi } from '@/api/gacha'

const props = defineProps<{ block: ShowcaseBlock; userId: string }>()
const emit = defineEmits<{
  'update:block': [block: ShowcaseBlock]
  close: []
}>()

const { t } = useI18n()
const toast = useToast()

// Operate on a deep copy — never mutate the prop.
// Seed config from props.block.config (persistence fix).
const draft = reactive<ShowcaseBlock>({
  ...props.block,
  config: { ...props.block.config },
})

const AUTO_TYPES: ShowcaseBlockType[] = ['continue_watching', 'anime_dna', 'compatibility']

// ── Variant picker ────────────────────────────────────────────────
function setVariant(v: string) {
  draft.variant = v
  const c = clampSize(draft.type, v, draft.w ?? 0, draft.h ?? 0)
  draft.w = c.w
  draft.h = c.h
  emit('update:block', { ...draft, config: { ...(draft.config as object) } } as ShowcaseBlock)
}

// ── Config helpers ────────────────────────────────────────────────
function aboutConfig(): AboutConfig {
  return draft.config as AboutConfig
}

function opEdConfig(): OpEdConfig {
  return draft.config as OpEdConfig
}

function emitUpdate() {
  emit('update:block', { ...draft, config: { ...(draft.config as object) } } as ShowcaseBlock)
}

// ── Auto-fill ─────────────────────────────────────────────────────
async function autoFillAnime() {
  try {
    const res = await userApi.getWatchlist({ sort: 'score', order: 'desc', per_page: 12 })
    const items = (res.data?.data ?? res.data) as Array<{ anime_id: string; score?: number }>
    const sorted = [...items].sort((a, b) => (b.score ?? 0) - (a.score ?? 0)).slice(0, 12)
    ;(draft.config as FavoriteAnimeConfig).anime_ids = sorted.map((i) => i.anime_id)
    emitUpdate()
  } catch {
    toast.push(t('showcase.auto_fill_error'), 'error')
  }
}

async function autoFillCards() {
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
    ;(draft.config as CardCollectionConfig).card_ids = owned.map((c: { card: { id: string } }) => c.card.id)
    emitUpdate()
  } catch {
    toast.push(t('showcase.auto_fill_error'), 'error')
  }
}

// ── Op-Ed theme management ────────────────────────────────────────
const newThemeId = ref('')

function addTheme(id: string) {
  const cfg = opEdConfig()
  const trimmed = id.trim()
  if (trimmed && !cfg.theme_ids.includes(trimmed)) {
    cfg.theme_ids.push(trimmed)
    emitUpdate()
  }
  newThemeId.value = ''
}

function removeTheme(id: string) {
  const cfg = opEdConfig()
  cfg.theme_ids = cfg.theme_ids.filter((t) => t !== id)
  emitUpdate()
}
</script>

<template>
  <!-- Backdrop -->
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" @click.self="emit('close')">
    <!-- Dialog panel -->
    <div class="relative w-full max-w-md rounded-xl border border-border bg-card p-5 shadow-xl">
      <!-- Header -->
      <div class="mb-4 flex items-center justify-between">
        <h3 class="text-sm font-semibold text-foreground">
          {{ $t(`showcase.block.${block.type}`) }}
        </h3>
        <button
          type="button"
          data-test="dialog-close"
          class="text-xs font-medium text-muted-foreground hover:text-foreground"
          @click="emit('close')"
        >
          ✕
        </button>
      </div>

      <!-- Variant chips — only for types with >1 variant -->
      <div v-if="SHOWCASE_VARIANTS[block.type as ShowcaseBlockType].length > 1" class="mb-4">
        <p class="mb-2 text-xs font-medium text-muted-foreground">{{ $t('showcase.variant_label') }}</p>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="v in SHOWCASE_VARIANTS[block.type as ShowcaseBlockType]"
            :key="v"
            type="button"
            :data-test="`variant-${v}`"
            :class="[
              'rounded-lg border px-3 py-1 text-sm font-medium',
              draft.variant === v
                ? 'border-brand-cyan text-brand-cyan'
                : 'border-border text-muted-foreground hover:text-foreground',
            ]"
            @click="setVariant(v)"
          >{{ v }}</button>
        </div>
      </div>

      <!-- About block config -->
      <div v-if="block.type === 'about'" class="space-y-2">
        <input
          v-model="aboutConfig().title"
          data-test="about-title"
          :placeholder="$t('showcase.about_title_placeholder')"
          maxlength="64"
          class="w-full rounded-lg border border-border bg-background px-3 py-1.5 text-xs"
          @input="emitUpdate()"
        />
        <textarea
          v-model="aboutConfig().text"
          :placeholder="$t('showcase.about_placeholder')"
          rows="3"
          maxlength="2000"
          class="w-full rounded-lg border border-border bg-background px-3 py-1.5 text-xs"
          @input="emitUpdate()"
        />
      </div>

      <!-- favorite_anime config -->
      <div v-else-if="block.type === 'favorite_anime'" class="flex items-center gap-2">
        <p class="flex-1 text-xs text-muted-foreground">{{ $t('showcase.pick_anime') }}</p>
        <button
          type="button"
          data-test="dialog-auto-anime"
          class="rounded-lg border border-border px-2 py-1 text-xs font-medium text-foreground hover:bg-accent"
          @click="autoFillAnime()"
        >
          {{ $t('showcase.auto_fill') }}
        </button>
      </div>

      <!-- favorite_character config -->
      <div v-else-if="block.type === 'favorite_character'">
        <p class="text-xs text-muted-foreground">{{ $t('showcase.pick_character') }}</p>
      </div>

      <!-- card_collection config -->
      <div v-else-if="block.type === 'card_collection'" class="flex items-center gap-2">
        <p class="flex-1 text-xs text-muted-foreground">{{ $t('showcase.pick_cards') }}</p>
        <button
          type="button"
          data-test="dialog-auto-cards"
          class="rounded-lg border border-border px-2 py-1 text-xs font-medium text-foreground hover:bg-accent"
          @click="autoFillCards()"
        >
          {{ $t('showcase.auto_fill') }}
        </button>
      </div>

      <!-- op_ed config -->
      <div v-else-if="block.type === 'op_ed'" class="space-y-2">
        <p class="text-xs font-medium text-muted-foreground">{{ $t('showcase.pick_theme') }}</p>
        <div class="flex flex-wrap gap-1">
          <span
            v-for="tid in opEdConfig().theme_ids"
            :key="tid"
            class="flex items-center gap-1 rounded-md border border-border px-2 py-0.5 text-xs"
          >
            {{ tid }}
            <button
              type="button"
              :data-test="`dialog-remove-theme-${tid}`"
              class="text-destructive"
              @click="removeTheme(tid)"
            >×</button>
          </span>
        </div>
        <div class="flex gap-2">
          <input
            v-model="newThemeId"
            :placeholder="$t('showcase.op_ed_add_theme')"
            class="flex-1 rounded-lg border border-border bg-background px-2 py-0.5 text-xs"
            data-test="dialog-theme-input"
            @keydown.enter.prevent="addTheme(newThemeId)"
          />
          <button
            type="button"
            class="rounded-lg border border-border px-2 py-0.5 text-xs font-medium text-foreground hover:bg-accent"
            data-test="dialog-theme-add"
            @click="addTheme(newThemeId)"
          >+</button>
        </div>
      </div>

      <!-- Auto types: continue_watching, anime_dna, compatibility -->
      <div v-else-if="AUTO_TYPES.includes(block.type)">
        <p class="text-xs text-muted-foreground">{{ $t('showcase.auto_block_info') }}</p>
      </div>

      <!-- stats / other fallback -->
      <div v-else>
        <p class="text-xs text-muted-foreground">{{ $t('showcase.pick_anime') }}</p>
      </div>
    </div>
  </div>
</template>
