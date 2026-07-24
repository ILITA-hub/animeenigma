<template>
  <!-- Summary grid (5 cols, x10 = 5×2; mobile 3 cols) with face-down flip reveal.
       SSR flips last for suspense; pinned-visible via opacity:1 + shake class.
       Ported from the v21 result modal. -->
  <Modal
    :model-value="modelValue"
    :title="title"
    closable
    size="lg"
    @update:model-value="$emit('update:modelValue', $event)"
  >
    <p class="text-muted-foreground text-sm mb-3">{{ bannerName }}</p>

    <div class="rgrid" data-testid="summary-grid">
      <button
        type="button"
        v-for="(pulled, i) in cards"
        :key="i"
        class="rcard"
        :class="[`f-${pulled.card.rarity}`, { facedown: facedown[i], 'ssr-pop': revealedSsr[i] }]"
        :style="{ animationDelay: `${i * 60}ms` }"
        :data-testid="`summary-card-${pulled.card.rarity}`"
        @click="reveal(i)"
      >
        <div class="flip">
          <div class="face">
            <div class="img">
              <img :src="cardPosterUrl(cardImageUrl(pulled.card.image_path), 384)" :alt="pulled.card.name" />
            </div>
            <div class="nm">
              <span class="truncate">{{ pulled.card.name }}</span>
              <span :class="rarityTextClass(pulled.card.rarity)">{{ pulled.card.rarity }}</span>
            </div>
          </div>
          <div class="back">◆</div>
        </div>
        <span v-if="pulled.new" class="tagNEW" :style="{ opacity: facedown[i] ? 0 : 1 }">
          {{ $t('gacha.viewer_new_badge') }}
        </span>
        <span
          v-else-if="pulled.count > 1"
          class="tagDUP"
          :style="{ opacity: facedown[i] ? 0 : 1 }"
        >
          {{ $t('gacha.viewer_dupe_badge', { n: pulled.count }) }}
        </span>
      </button>
    </div>

    <template #footer>
      <div class="flex items-center justify-between w-full gap-3 flex-wrap">
        <span class="text-muted-foreground text-sm">
          {{ $t('gacha.summary_balance', { balance, pity: `${pity} / ${pityThreshold}` }) }}
        </span>
        <div class="flex gap-2">
          <Button
            variant="outline"
            size="sm"
            :disabled="balance < costX10"
            data-testid="summary-again"
            @click="$emit('again')"
          >
            {{ $t('gacha.summary_again_x10', { n: costX10 }) }}
          </Button>
          <Button size="sm" data-testid="summary-done" @click="$emit('update:modelValue', false)">
            {{ $t('gacha.summary_done') }}
          </Button>
        </div>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/ui/Modal.vue'
import Button from '@/components/ui/Button.vue'
import { cardImageUrl, type PulledCard, type Rarity } from '@/api/gacha'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = defineProps<{
  modelValue: boolean
  cards: PulledCard[]
  bannerName: string
  balance: number
  pity: number
  pityThreshold: number
  costX10: number
  /** When true (skipped ceremony/viewer), reveal everything immediately. */
  instant?: boolean
}>()

defineEmits<{
  'update:modelValue': [value: boolean]
  again: []
}>()

const { t } = useI18n()

const TIER_RANK: Record<Rarity, number> = { N: 0, R: 1, SR: 2, SSR: 3 }

const facedown = ref<boolean[]>([])
const revealedSsr = ref<boolean[]>([])
let timers: ReturnType<typeof setTimeout>[] = []

const title = computed(() =>
  props.cards.length === 10 ? t('gacha.summary_title_x10') : t('gacha.summary_title_x1'),
)

function clearTimers() {
  timers.forEach(clearTimeout)
  timers = []
}

function reveal(i: number) {
  if (!facedown.value[i]) return
  facedown.value[i] = false
  if (props.cards[i]?.card.rarity === 'SSR') revealedSsr.value[i] = true
}

function start() {
  clearTimers()
  facedown.value = props.cards.map(() => true)
  revealedSsr.value = props.cards.map(() => false)
  props.cards.forEach((o, i) => {
    // SSR flips last for suspense; N/R quick.
    const delay = props.instant ? 0 : 300 + TIER_RANK[o.card.rarity] * 420 + Math.random() * 180
    timers.push(setTimeout(() => reveal(i), delay))
  })
}

function rarityTextClass(rarity: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'r-ssr'
    case 'SR': return 'r-sr'
    case 'R': return 'r-r'
    default: return 'r-n'
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) start()
    else clearTimers()
  },
)

watch(
  () => props.cards,
  () => {
    if (props.modelValue) start()
  },
)
</script>

<style scoped>
.rgrid {
  display: grid;
  grid-template-columns: repeat(5, 1fr);
  gap: 0.8rem;
  margin-bottom: 0.3rem;
}
@media (max-width: 640px) {
  .rgrid {
    grid-template-columns: repeat(3, 1fr);
  }
}
.rcard {
  position: relative;
  border-radius: 0.9rem;
  overflow: hidden;
  border: 2px solid;
  background: var(--elevated);
  opacity: 0;
  transform: translateY(14px) scale(0.92);
  animation: pop 0.42s forwards;
  perspective: 600px;
  cursor: pointer;
  display: block;
  width: 100%;
  padding: 0;
  text-align: left;
  font: inherit;
}
@keyframes pop {
  to { opacity: 1; transform: none; }
}
.rcard.f-SSR { border-color: rgb(251, 146, 60); box-shadow: 0 0 18px rgba(251, 146, 60, 0.45); }
.rcard.f-SR { border-color: rgb(129, 140, 248); box-shadow: 0 0 12px rgba(129, 140, 248, 0.35); }
.rcard.f-R { border-color: rgb(45, 212, 191); }
.rcard.f-N { border-color: var(--white-a20); }
.rcard .flip {
  position: relative;
  width: 100%;
  transform-style: preserve-3d;
  transition: transform 0.55s cubic-bezier(0.3, 1.4, 0.4, 1);
}
.rcard.facedown .flip {
  transform: rotateY(180deg);
}
.face,
.back {
  backface-visibility: hidden;
}
.face .img {
  aspect-ratio: 3 / 4;
  overflow: hidden;
}
.face .img img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.face .nm {
  padding: 0.35rem 0.45rem 0.5rem;
  font-size: 0.72rem;
  font-weight: 500;
  display: flex;
  justify-content: space-between;
  gap: 0.2rem;
}
.back {
  position: absolute;
  inset: 0;
  transform: rotateY(180deg);
  background: linear-gradient(150deg, rgb(26, 26, 46), rgb(16, 16, 28) 60%);
  display: grid;
  place-items: center;
  font-size: 1.8rem;
  color: var(--cyan-a60);
}
.rcard.ssr-pop {
  opacity: 1;
  transform: none;
  animation: ssrShake 0.5s 0.1s;
}
@keyframes ssrShake {
  0%, 100% { transform: none; }
  25% { transform: translateX(-3px) rotate(-1.5deg); }
  75% { transform: translateX(3px) rotate(1.5deg); }
}
.tagNEW {
  position: absolute;
  top: 0.35rem;
  left: 0.35rem;
  background: var(--brand-cyan);
  color: rgb(0, 0, 0);
  font-size: 0.62rem;
  font-weight: 600;
  border-radius: 0.35rem;
  padding: 0.08rem 0.35rem;
  transition: opacity 0.3s;
}
.tagDUP {
  position: absolute;
  top: 0.35rem;
  right: 0.35rem;
  background: var(--black-a60);
  backdrop-filter: blur(4px);
  color: var(--ink-2);
  font-size: 0.62rem;
  font-weight: 600;
  border-radius: 0.35rem;
  padding: 0.08rem 0.35rem;
  transition: opacity 0.3s;
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
