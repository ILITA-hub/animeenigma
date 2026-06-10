<template>
  <!-- Gacha collection album — OWNED cards ONLY, per-rarity sections SSR→N with
       per-rarity progress and ×N dupe counts. No unowned cards / silhouettes. -->
  <div>
    <!-- Loading state -->
    <div v-if="loadingCollection" class="flex justify-center py-10">
      <Spinner />
    </div>

    <!-- Error state -->
    <Alert v-else-if="collectionError" variant="destructive" class="mb-4">
      {{ collectionError }}
    </Alert>

    <!-- Empty state -->
    <div
      v-else-if="!collection || ownedCards.length === 0"
      class="glass-card p-8 text-center text-muted-foreground"
    >
      {{ $t('gacha.collection_empty') }}
    </div>

    <template v-else>
      <!-- Per-rarity sections, highest first: SSR → SR → R → N.
           Only sections with at least one OWNED card are rendered. -->
      <div
        v-for="rarity in RARITY_SECTIONS"
        v-show="cardsByRarity[rarity]?.length"
        :key="rarity"
        class="mb-8"
      >
        <!-- Section header -->
        <div class="flex items-center gap-3 mb-3">
          <span :class="['text-base font-semibold', rarityTextClass(rarity)]">
            {{ $t(`gacha.collection_section_${rarity.toLowerCase()}`) }}
          </span>
          <span class="text-muted-foreground text-sm">
            {{
              $t('gacha.collection_progress', {
                owned: collection.progress[rarity]?.owned ?? 0,
                total: collection.progress[rarity]?.total ?? 0,
              })
            }}
          </span>
          <!-- Progress bar -->
          <div class="flex-1 h-1 bg-white/10 rounded-full overflow-hidden">
            <div
              :class="['h-full rounded-full transition-all', rarityBarClass(rarity)]"
              :style="{
                width: progressPercent(
                  collection.progress[rarity]?.owned ?? 0,
                  collection.progress[rarity]?.total ?? 0,
                ) + '%',
              }"
            />
          </div>
        </div>

        <!-- Owned card grid for this rarity -->
        <div
          v-if="cardsByRarity[rarity]?.length"
          class="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-6 gap-3"
        >
          <div
            v-for="entry in cardsByRarity[rarity]"
            :key="entry.card.id"
            data-testid="collection-card-owned"
            class="relative rounded-lg overflow-hidden aspect-[2/3] bg-white/5"
          >
            <img
              :src="cardImageUrl(entry.card.image_path)"
              :alt="entry.card.name"
              class="w-full h-full object-cover"
            />

            <!-- Rarity border -->
            <div :class="['absolute inset-0 rounded-lg ring-1 ring-inset', rarityRingClass(rarity)]" />

            <!-- Name + dupe count -->
            <div class="absolute bottom-0 left-0 right-0 bg-black/60 px-1 py-0.5">
              <p class="text-white text-[10px] font-medium truncate">{{ entry.card.name }}</p>
            </div>
            <span
              v-if="entry.count > 1"
              class="absolute top-1 right-1 bg-white/20 text-white text-[10px] font-semibold px-1 rounded"
            >×{{ entry.count }}</span>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useGachaStore } from '@/stores/gacha'
import { cardImageUrl, type Rarity } from '@/api/gacha'
import Spinner from '@/components/ui/Spinner.vue'
import Alert from '@/components/ui/Alert.vue'

const { t: _t } = useI18n()
void _t // keep vue-i18n composable active for template $t

const store = useGachaStore()
const loadingCollection = computed(() => store.loadingCollection)
const collectionError = computed(() => store.collectionError)
const collection = computed(() => store.collection)

const RARITY_SECTIONS: Rarity[] = ['SSR', 'SR', 'R', 'N']

// Owned cards only — the album shows what the user has actually pulled.
const ownedCards = computed(() => (collection.value?.cards ?? []).filter((e) => e.owned))

// Group OWNED cards by rarity
const cardsByRarity = computed(() => {
  const c = collection.value
  if (!c) return {} as Record<Rarity, NonNullable<typeof c>['cards']>
  return ownedCards.value.reduce(
    (acc, entry) => {
      const r = entry.card.rarity
      if (!acc[r]) acc[r] = []
      acc[r].push(entry)
      return acc
    },
    {} as Record<Rarity, NonNullable<typeof c>['cards']>,
  )
})

function progressPercent(owned: number, total: number): number {
  if (total === 0) return 0
  return Math.round((owned / total) * 100)
}

// ── Rarity styling (exempt hues: teal/indigo/orange) ──────────────────────
function rarityTextClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'text-orange-400'
    case 'SR':  return 'text-indigo-400'
    case 'R':   return 'text-teal-400'
    default:    return 'text-muted-foreground'
  }
}

function rarityRingClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'ring-orange-400/40'
    case 'SR':  return 'ring-indigo-400/40'
    case 'R':   return 'ring-teal-400/40'
    default:    return 'ring-white/10'
  }
}

function rarityBarClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'bg-orange-400'
    case 'SR':  return 'bg-indigo-400'
    case 'R':   return 'bg-teal-400'
    default:    return 'bg-white/40'
  }
}

onMounted(() => {
  store.fetchCollection()
})
</script>
