<template>
  <!-- Reusable card picker used inside both the banner and group edit dialogs.
       Parent renders this in the "picker view" of its modal.
       Props:
         excludeIds     — card ids already in the target (shown disabled with ✓)
         allCards       — full card list (already fetched by parent)
         groups         — group list for the group filter dropdown
         alreadyInLabel — tooltip text when a card is already included
         search         — (v-model:search) controlled search string
         selected       — (v-model:selected) controlled Set<string> of selected ids
       Emits:
         update:search      — when search input changes
         update:selected    — when selection changes
         confirm(ids)       — user confirmed selection (Add button in parent footer)
         cancel()           — user navigated back
  -->
  <div data-testid="gacha-card-picker">
    <!-- Filters -->
    <div class="flex flex-wrap gap-2 mb-4">
      <Input
        :value="search"
        :placeholder="$t('gacha.admin.banner_picker_search_placeholder')"
        class="flex-1 min-w-40"
        data-testid="picker-search"
        @input="$emit('update:search', ($event.target as HTMLInputElement).value)"
      />
      <Select
        v-model="rarityFilter"
        :options="rarityOptions"
        class="w-36"
      />
      <Select
        v-model="groupFilter"
        :options="groupOptions"
        class="w-40"
      />
      <button
        type="button"
        :class="[
          'flex items-center gap-1.5 px-3 rounded-lg border text-xs font-medium transition-colors',
          hideExcluded
            ? 'border-cyan-400/60 bg-cyan-400/10 text-cyan-400'
            : 'border-white/20 bg-white/5 text-white/60 hover:text-white hover:border-white/40',
        ]"
        data-testid="picker-hide-added"
        :aria-pressed="hideExcluded"
        @click="hideExcluded = !hideExcluded"
      >
        <EyeOff v-if="hideExcluded" class="size-3.5" aria-hidden="true" />
        <Eye v-else class="size-3.5" aria-hidden="true" />
        {{ $t('gacha.admin.picker_hide_added') }}
      </button>
    </div>

    <!-- Card grid -->
    <div
      v-if="filteredCards.length === 0"
      class="py-8 text-center text-muted-foreground text-sm"
    >
      {{ $t('gacha.admin.banner_picker_empty') }}
    </div>
    <div
      v-else
      class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6 gap-3 max-h-96 overflow-y-auto pr-1"
      data-testid="picker-card-grid"
    >
      <button
        v-for="card in filteredCards"
        :key="card.id"
        type="button"
        :disabled="excludeIds.includes(card.id)"
        :class="[
          'relative flex flex-col items-center gap-1 rounded-xl border transition-all focus:outline-none',
          excludeIds.includes(card.id)
            ? 'border-white/10 opacity-50 cursor-not-allowed bg-white/5'
            : selected.has(card.id)
              ? 'border-cyan-400 bg-cyan-400/10 ring-2 ring-cyan-400/30'
              : 'border-white/20 bg-white/5 hover:border-white/40 hover:bg-white/10',
        ]"
        :data-testid="`picker-card-${card.id}`"
        @click="toggle(card.id)"
      >
        <img
          :src="cardPosterUrl(cardImageUrl(card.image_path), 128)"
          :alt="card.name"
          class="w-full aspect-[3/4] object-cover rounded-t-xl"
        />
        <div class="w-full px-1.5 pb-1.5">
          <p class="text-white text-xs font-medium truncate leading-tight">{{ card.name }}</p>
          <div class="flex items-center justify-between mt-0.5">
            <span :class="['text-xs font-semibold px-1 py-0.5 rounded', rarityBadgeClass(card.rarity)]">
              {{ card.rarity }}
            </span>
            <span
              v-if="excludeIds.includes(card.id)"
              class="text-xs text-white/40"
              :title="alreadyInLabel"
            >✓</span>
          </div>
        </div>
        <!-- Selected checkmark overlay -->
        <div
          v-if="selected.has(card.id)"
          class="absolute top-1 right-1 size-5 rounded-full bg-cyan-400 flex items-center justify-center"
        >
          <Check class="size-3 text-black" aria-hidden="true" />
        </div>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Eye, EyeOff } from 'lucide-vue-next'
import { cardImageUrl, gachaAdminApi, type GachaCard, type GachaGroup, type Rarity } from '@/api/gacha'
import { cardPosterUrl } from '@/composables/useImageProxy'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'

// ── Props ─────────────────────────────────────────────────────────────────────
const props = defineProps<{
  /** Card ids that are already in the target (disabled with ✓). */
  excludeIds: string[]
  /** Full card list, already fetched by the parent. */
  allCards: GachaCard[]
  /** Group list for the group filter dropdown. */
  groups: GachaGroup[]
  /** Tooltip text shown on already-included card thumbnails. */
  alreadyInLabel: string
  /** Controlled search string (v-model:search). */
  search: string
  /** Controlled selection set (v-model:selected). */
  selected: Set<string>
}>()

// ── Emits ─────────────────────────────────────────────────────────────────────
const emit = defineEmits<{
  (e: 'update:search', value: string): void
  (e: 'update:selected', value: Set<string>): void
  (e: 'confirm', ids: string[]): void
  (e: 'cancel'): void
}>()

// ── i18n ──────────────────────────────────────────────────────────────────────
const { t } = useI18n()

// ── Internal filter state (not controlled by parent) ──────────────────────────
const rarityFilter = ref('all')
const groupFilter = ref('all')
const hideExcluded = ref(false)

// Group membership is not carried on the card list response, so the group
// filter loads the selected group's card ids server-side (listCards group_id)
// and filters client-side against that set. null = no group filter active.
const groupCardIDs = ref<Set<string> | null>(null)
watch(groupFilter, async (g) => {
  if (g === 'all') {
    groupCardIDs.value = null
    return
  }
  try {
    const res = await gachaAdminApi.listCards({ group_id: g })
    groupCardIDs.value = new Set((res.data.data ?? []).map(c => c.id))
  } catch {
    groupCardIDs.value = new Set() // failed load → empty (shows none, not wrong)
  }
})

// ── Options ───────────────────────────────────────────────────────────────────
const rarityOptions = computed(() => [
  { value: 'all', label: t('gacha.admin.banner_picker_filter_rarity') },
  { value: 'N',   label: 'N' },
  { value: 'R',   label: 'R' },
  { value: 'SR',  label: 'SR' },
  { value: 'SSR', label: 'SSR' },
])

const groupOptions = computed(() => [
  { value: 'all', label: t('gacha.admin.banner_picker_filter_group') },
  ...props.groups.map(g => ({ value: g.id, label: g.name })),
])

// ── Filtered cards ────────────────────────────────────────────────────────────
const filteredCards = computed(() => {
  const q = props.search.toLowerCase().trim()
  return props.allCards.filter(c => {
    if (rarityFilter.value !== 'all' && c.rarity !== rarityFilter.value) return false
    if (groupCardIDs.value !== null && !groupCardIDs.value.has(c.id)) return false
    if (hideExcluded.value && props.excludeIds.includes(c.id)) return false
    if (q && !c.name.toLowerCase().includes(q) && !c.source_title.toLowerCase().includes(q)) return false
    return true
  })
})

// ── Actions ───────────────────────────────────────────────────────────────────
function toggle(cardId: string) {
  if (props.excludeIds.includes(cardId)) return
  const next = new Set(props.selected)
  if (next.has(cardId)) {
    next.delete(cardId)
  } else {
    next.add(cardId)
  }
  emit('update:selected', next)
}

function selectAllVisible() {
  const next = new Set(props.selected)
  for (const card of filteredCards.value) {
    if (!props.excludeIds.includes(card.id)) {
      next.add(card.id)
    }
  }
  emit('update:selected', next)
}

function confirmSelection() {
  emit('confirm', Array.from(props.selected))
}

function cancel() {
  emit('cancel')
}

// ── Rarity badge ──────────────────────────────────────────────────────────────
function rarityBadgeClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'bg-orange-400/20 text-orange-400'
    case 'SR':  return 'bg-indigo-400/20 text-indigo-400'
    case 'R':   return 'bg-teal-400/20 text-teal-400'
    default:    return 'bg-white/10 text-white/60'
  }
}

// ── Reset — called by parent when opening the picker ─────────────────────────
function reset() {
  rarityFilter.value = 'all'
  groupFilter.value = 'all'
  groupCardIDs.value = null
  hideExcluded.value = false
  emit('update:search', '')
  emit('update:selected', new Set())
}

// ── Expose public surface ─────────────────────────────────────────────────────
defineExpose({
  filteredCards,
  reset,
  selectAllVisible,
  confirmSelection,
  cancel,
})
</script>
