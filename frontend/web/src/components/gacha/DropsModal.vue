<template>
  <!-- Drops modal: banner pool grouped SSR→N with static tier-rate headers.
       Unowned cards stay VISIBLE (dimmed + dashed border). Ported from v21. -->
  <Modal
    :model-value="modelValue"
    :title="$t('gacha.drops_title')"
    closable
    size="lg"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <p class="text-muted-foreground text-sm mb-2">{{ bannerName }}</p>

    <div data-testid="drops-body">
      <template v-for="tier in TIER_ORDER" :key="tier">
        <div v-if="cardsByTier[tier]?.length" class="tier-group">
          <div class="tier-head">
            <span :class="['tier-name', rarityTextClass(tier)]">{{ tier }}</span>
            <span class="tier-rate">{{ tierRateLabel(tier) }}</span>
          </div>
          <div class="pool">
            <div
              v-for="card in cardsByTier[tier]"
              :key="card.id"
              :data-testid="card.owned ? 'drops-card-owned' : 'drops-card-unowned'"
              class="pcard"
              :class="{ unowned: !card.owned }"
              role="button"
              tabindex="0"
              :aria-label="card.name"
              @click="openInspect(card)"
              @keydown.enter.space.prevent="openInspect(card)"
            >
              <div class="img">
                <img :src="cardPosterUrl(cardImageUrl(card.image_path), 256)" :alt="card.name" />
              </div>
              <div class="nm">
                <span class="truncate">{{ card.name }}</span>
                <span :class="rarityTextClass(card.rarity)">{{ card.rarity }}</span>
              </div>
              <span class="st" :class="{ owned: card.owned }">
                {{ card.owned ? $t('gacha.drops_owned') : $t('gacha.drops_unowned') }}
              </span>
            </div>
          </div>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex items-center justify-between w-full gap-3">
        <span class="text-muted-foreground text-sm">
          {{ $t('gacha.drops_stat', { owned: ownedCount, total: totalCount }) }}
        </span>
        <Button @click="$emit('update:modelValue', false)">{{ $t('gacha.drops_close') }}</Button>
      </div>
    </template>
  </Modal>

  <!-- 3D inspect viewer — whole pool in display order (SSR→N); starts at the
       clicked card. Teleports to body (z-95) above the Modal overlay (z-50). -->
  <CardViewer3D
    :active="inspectActive"
    :cards="inspectCards"
    :start-index="inspectStartIndex"
    mode="inspect"
    @done="inspectActive = false"
  />
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import CardViewer3D from './CardViewer3D.vue'
import {
  cardImageUrl,
  type BannerCardView,
  type GachaCard,
  type PulledCard,
  type Rarity,
} from '@/api/gacha'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = defineProps<{
  modelValue: boolean
  bannerName: string
  cards: BannerCardView[]
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const { t } = useI18n()

const TIER_ORDER: Rarity[] = ['SSR', 'SR', 'R', 'N']

// Static display rates matching the prod config defaults (plan §note).
const TIER_RATE: Record<Rarity, number> = { SSR: 1, SR: 8, R: 22, N: 69 }

const cardsByTier = computed(() => {
  const acc = {} as Record<Rarity, BannerCardView[]>
  for (const tier of TIER_ORDER) acc[tier] = []
  for (const card of props.cards) {
    if (!acc[card.rarity]) acc[card.rarity] = []
    acc[card.rarity].push(card)
  }
  return acc
})

const ownedCount = computed(() => props.cards.filter((c) => c.owned).length)
const totalCount = computed(() => props.cards.length)

// ── Inspect viewer state ─────────────────────────────────────────────────────
const inspectActive = ref(false)
const inspectStartIndex = ref(0)

/** Whole pool in display order (SSR→N) as PulledCard[] for the 3D viewer. */
const inspectCards = computed<PulledCard[]>(() =>
  TIER_ORDER.flatMap((tier) => (cardsByTier.value[tier] ?? []).map(dropToPulled)),
)

/** BannerCardView is a flat projection — rebuild the GachaCard shape the viewer reads. */
function dropToPulled(v: BannerCardView): PulledCard {
  const card: GachaCard = {
    id: v.id,
    name: v.name,
    source_title: '',
    image_path: v.image_path,
    back_path: v.back_path,
    rarity: v.rarity,
    enabled: true,
    created_at: '',
    updated_at: '',
  }
  return { card, new: false, count: 0 }
}

function openInspect(v: BannerCardView) {
  const idx = inspectCards.value.findIndex((p) => p.card.id === v.id)
  inspectStartIndex.value = idx >= 0 ? idx : 0
  inspectActive.value = true
}

function tierRateLabel(tier: Rarity): string {
  let label = t('gacha.drops_rate_label', { rate: TIER_RATE[tier] })
  if (tier === 'SSR') label += ` · ${t('gacha.drops_rate_ssr_extra')}`
  if (tier === 'SR') label += ` · ${t('gacha.drops_rate_sr_extra')}`
  return label
}

function rarityTextClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'r-ssr'
    case 'SR': return 'r-sr'
    case 'R': return 'r-r'
    default: return 'r-n'
  }
}
</script>

<style scoped>
.tier-group {
  margin-bottom: 0.5rem;
}
.tier-head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  margin: 1rem 0 0.55rem;
}
.tier-name {
  font-weight: 600;
  font-size: 0.95rem;
}
.tier-rate {
  font-size: 0.75rem;
  color: var(--ink-4);
}
.pool {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(110px, 1fr));
  gap: 0.9rem;
}
.pcard {
  border-radius: 0.9rem;
  overflow: hidden;
  border: 1px solid var(--line-strong);
  background: var(--surface-2);
  cursor: pointer;
}
.pcard .img {
  aspect-ratio: 3 / 4;
  overflow: hidden;
}
.pcard .img img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.pcard.unowned .img img {
  filter: saturate(0.45) brightness(0.75);
}
.pcard.unowned {
  border-style: dashed;
  border-color: var(--white-a20);
}
.pcard .nm {
  padding: 0.4rem 0.5rem 0.15rem;
  font-size: 0.74rem;
  font-weight: 500;
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.3rem;
}
.pcard .st {
  padding: 0 0.5rem 0.45rem;
  display: block;
  font-size: 0.66rem;
  color: var(--ink-4);
}
.pcard .st.owned {
  color: var(--brand-cyan);
}
.truncate {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.r-ssr { color: rgb(251, 146, 60); }
.r-sr  { color: rgb(129, 140, 248); }
.r-r   { color: rgb(45, 212, 191); }
.r-n   { color: var(--ink-4); }
</style>
