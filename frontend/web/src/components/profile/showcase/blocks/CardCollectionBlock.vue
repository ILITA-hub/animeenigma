<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import type { CardCollectionConfig } from '@/types/showcase'
import { defaultVariant } from '@/types/showcase'
import { gachaApi, cardImageUrl, type GachaCard } from '@/api/gacha'
import { cardPosterUrl } from '@/composables/useImageProxy'

const props = defineProps<{ config: CardCollectionConfig; variant?: string; userId?: string }>()

// ─── State ────────────────────────────────────────────────────────────────────

const cards = ref<GachaCard[]>([])
const dialogCard = ref<GachaCard | null>(null)
const dialogOpen = computed(() => dialogCard.value !== null)

const v = computed(() => props.variant || defaultVariant('card_collection'))

// Reduced-motion preference
const prefersReducedMotion =
  typeof window !== 'undefined' && typeof window.matchMedia === 'function'
    ? window.matchMedia('(prefers-reduced-motion: reduce)').matches
    : false

// ─── Data resolution ─────────────────────────────────────────────────────────

onMounted(async () => {
  const ids = new Set(props.config.card_ids ?? [])
  if (!ids.size) return
  try {
    const res = await gachaApi.getCollection()
    const view = res.data?.data ?? res.data
    const all = (view as { cards: Array<{ card: GachaCard; owned: boolean }> }).cards ?? []
    cards.value = all
      .filter((entry) => entry.owned && ids.has(entry.card.id))
      .map((entry) => entry.card)
  } catch {
    cards.value = []
  }
})

// ─── Dialog ───────────────────────────────────────────────────────────────────

function openDialog(card: GachaCard) {
  dialogCard.value = card
}

function closeDialog() {
  dialogCard.value = null
}

function onBackdropClick(e: MouseEvent) {
  if ((e.target as HTMLElement).classList.contains('cc-dialog-backdrop')) {
    closeDialog()
  }
}

function onEsc(e: KeyboardEvent) {
  if (e.key === 'Escape' && dialogOpen.value) closeDialog()
}

onMounted(() => {
  document.addEventListener('keydown', onEsc)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onEsc)
})

// ─── 3D Tilt handler ─────────────────────────────────────────────────────────

function onTiltMove(e: MouseEvent) {
  if (prefersReducedMotion) return
  const card = e.currentTarget as HTMLElement
  const rect = card.getBoundingClientRect()
  const px = (e.clientX - rect.left) / rect.width
  const py = (e.clientY - rect.top) / rect.height
  card.style.transform = `rotateY(${(px - 0.5) * 18}deg) rotateX(${(0.5 - py) * 18}deg) translateY(-6px) scale(1.04)`
  card.style.setProperty('--mx', `${px * 100}%`)
  card.style.setProperty('--my', `${py * 100}%`)
}

function onTiltLeave(e: MouseEvent) {
  const card = e.currentTarget as HTMLElement
  card.style.transform = ''
}

// ─── Rarity helpers ───────────────────────────────────────────────────────────

const rarityClass: Record<string, string> = {
  SSR: 'frame-ssr',
  SR: 'frame-sr',
  R: 'frame-r',
  N: 'frame-n',
}

const rarityBadgeClass: Record<string, string> = {
  SSR: 'rar-ssr',
  SR: 'rar-sr',
  R: 'rar-r',
  N: 'rar-n',
}

const rarityGlow: Record<string, string> = {
  SSR: 'shadow-[0_0_18px_rgba(255,214,0,0.55)]',
  SR: 'shadow-[0_0_16px_rgba(167,139,250,0.5)]',
  R: '',
  N: '',
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col overflow-hidden rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 shrink-0 text-lg font-semibold text-foreground">
      {{ $t('showcase.block.card_collection') }}
    </h3>

    <!-- ── ROW (default) ─────────────────────────────────────────── -->
    <div v-if="v === 'row' && cards.length" class="cc-row flex min-h-0 flex-1 items-stretch gap-3 overflow-x-auto pb-2">
      <div
        v-for="c in cards"
        :key="c.id"
        class="cc-gcard flex-none"
        :class="rarityGlow[c.rarity]"
        @click="openDialog(c)"
      >
        <div class="cc-frame" :class="rarityClass[c.rarity]" />
        <img v-if="c.image_path" :src="cardPosterUrl(cardImageUrl(c.image_path), 256)" :alt="c.name" class="cc-img" />
        <div class="cc-holo" />
        <div class="cc-cg" />
        <span class="cc-rar" :class="rarityBadgeClass[c.rarity]">{{ c.rarity }}</span>
        <div class="cc-cap">
          <p class="truncate text-[10px] font-medium text-foreground">{{ c.name }}</p>
        </div>
      </div>
    </div>

    <!-- ── FAN ───────────────────────────────────────────────────── -->
    <div
      v-else-if="v === 'fan' && cards.length"
      class="relative flex min-h-0 flex-1 items-center justify-center overflow-hidden"
    >
      <div
        v-for="(c, idx) in cards.slice(0, 5)"
        :key="c.id"
        class="cc-gcard cc-fan-card absolute"
        :class="[`cc-fan-pos-${idx}`, rarityGlow[c.rarity]]"
        @click="openDialog(c)"
      >
        <div class="cc-frame" :class="rarityClass[c.rarity]" />
        <img v-if="c.image_path" :src="cardPosterUrl(cardImageUrl(c.image_path), 256)" :alt="c.name" class="cc-img" />
        <div class="cc-holo" />
        <div class="cc-cg" />
        <span class="cc-rar" :class="rarityBadgeClass[c.rarity]">{{ c.rarity }}</span>
        <div class="cc-cap">
          <p class="truncate text-[10px] font-medium text-foreground">{{ c.name }}</p>
        </div>
      </div>
    </div>

    <!-- ── GRID ──────────────────────────────────────────────────── -->
    <div
      v-else-if="v === 'grid' && cards.length"
      class="cc-grid grid min-h-0 flex-1 grid-cols-3 gap-3 overflow-hidden fade-clip sm:grid-cols-4 md:grid-cols-5"
    >
      <div
        v-for="c in cards"
        :key="c.id"
        class="cc-gcard"
        :class="rarityGlow[c.rarity]"
        @click="openDialog(c)"
      >
        <div class="cc-frame" :class="rarityClass[c.rarity]" />
        <img v-if="c.image_path" :src="cardPosterUrl(cardImageUrl(c.image_path), 256)" :alt="c.name" class="cc-img" />
        <div class="cc-holo" />
        <div class="cc-cg" />
        <span class="cc-rar" :class="rarityBadgeClass[c.rarity]">{{ c.rarity }}</span>
        <div class="cc-cap">
          <p class="truncate text-[10px] font-medium text-foreground">{{ c.name }}</p>
        </div>
      </div>
    </div>

    <!-- ── HERO ──────────────────────────────────────────────────── -->
    <div
      v-else-if="v === 'hero' && cards.length"
      class="grid min-h-0 flex-1 content-center gap-6 overflow-hidden md:grid-cols-[180px_1fr]"
    >
      <!-- Featured card -->
      <div
        class="cc-gcard cc-hero-feat"
        :class="rarityGlow[cards[0].rarity]"
        @click="openDialog(cards[0])"
      >
        <div class="cc-frame" :class="rarityClass[cards[0].rarity]" />
        <img
          v-if="cards[0].image_path"
          :src="cardPosterUrl(cardImageUrl(cards[0].image_path), 256)"
          :alt="cards[0].name"
          class="cc-img"
        />
        <div class="cc-holo" />
        <div class="cc-cg" />
        <span class="cc-rar" :class="rarityBadgeClass[cards[0].rarity]">{{ cards[0].rarity }}</span>
        <div class="cc-cap">
          <p class="truncate text-[10px] font-medium text-foreground">{{ cards[0].name }}</p>
        </div>
      </div>

      <!-- Info panel -->
      <div class="cc-hero-panel flex flex-col justify-center gap-3">
        <span class="cc-lab inline-flex w-fit items-center rounded-full border border-border px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
          {{ $t('showcase.cardCollection.favCard') }}
        </span>
        <p class="text-xl font-semibold text-foreground">{{ cards[0].name }}</p>
        <div class="flex items-center gap-2">
          <span
            class="rounded px-1.5 py-0.5 text-[10px] font-semibold"
            :class="{
              'bg-warning/20 text-warning': cards[0].rarity === 'SSR',
              'cc-sr-badge': cards[0].rarity === 'SR',
              'cc-r-badge': cards[0].rarity === 'R',
              'bg-muted text-muted-foreground': cards[0].rarity === 'N',
            }"
          >{{ cards[0].rarity }}</span>
          <span class="text-sm font-medium text-muted-foreground">{{ cards[0].source_title }}</span>
        </div>
        <p v-if="cards.length > 1" class="text-xs font-medium text-muted-foreground">
          {{ $t('showcase.cardCollection.others') }}
        </p>
        <div v-if="cards.length > 1" class="flex flex-wrap gap-2">
          <div
            v-for="c in cards.slice(1, 6)"
            :key="c.id"
            class="cc-gcard cc-hero-mini"
            :class="rarityGlow[c.rarity]"
            @click="openDialog(c)"
          >
            <div class="cc-frame" :class="rarityClass[c.rarity]" />
            <img v-if="c.image_path" :src="cardPosterUrl(cardImageUrl(c.image_path), 256)" :alt="c.name" class="cc-img" />
            <div class="cc-holo" />
            <div class="cc-cg" />
          </div>
        </div>
      </div>
    </div>

    <!-- ── TILT3D ─────────────────────────────────────────────────── -->
    <div
      v-else-if="v === 'tilt3d' && cards.length"
      class="cc-tilt3d flex min-h-0 flex-1 flex-wrap content-center justify-center gap-6 overflow-hidden fade-clip"
      style="perspective: 1000px"
    >
      <div
        v-for="c in cards"
        :key="c.id"
        class="cc-gcard cc-tilt-card"
        :class="rarityGlow[c.rarity]"
        @mousemove="onTiltMove"
        @mouseleave="onTiltLeave"
        @click="openDialog(c)"
      >
        <div class="cc-frame" :class="rarityClass[c.rarity]" />
        <img v-if="c.image_path" :src="cardPosterUrl(cardImageUrl(c.image_path), 256)" :alt="c.name" class="cc-img" />
        <div class="cc-holo" />
        <div class="cc-sheen" />
        <div class="cc-cg" />
        <span class="cc-rar" :class="rarityBadgeClass[c.rarity]">{{ c.rarity }}</span>
        <div class="cc-cap">
          <p class="truncate text-[10px] font-medium text-foreground">{{ c.name }}</p>
        </div>
      </div>
    </div>

    <!-- ── EMPTY ─────────────────────────────────────────────────── -->
    <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
  </div>

  <!-- ── DIALOG ────────────────────────────────────────────────────── -->
  <Teleport to="body">
    <Transition name="cc-dlg">
      <div
        v-if="dialogOpen && dialogCard"
        class="cc-dialog cc-dialog-backdrop fixed inset-0 z-50 flex items-center justify-center p-6"
        role="dialog"
        aria-modal="true"
        @click="onBackdropClick"
      >
        <div class="cc-dialog-inner relative flex flex-col items-center gap-4">
          <button
            class="absolute -right-3 -top-3 z-10 flex size-7 items-center justify-center rounded-full bg-card text-muted-foreground transition-colors hover:text-foreground"
            aria-label="Close"
            @click="closeDialog"
          >
            <svg
              xmlns="http://www.w3.org/2000/svg"
              width="14"
              height="14"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>

          <!-- Card preview -->
          <div
            class="cc-gcard cc-dialog-card"
            :class="[rarityClass[dialogCard.rarity], rarityGlow[dialogCard.rarity]]"
          >
            <div class="cc-frame" :class="rarityClass[dialogCard.rarity]" />
            <img
              v-if="dialogCard.image_path"
              :src="cardPosterUrl(cardImageUrl(dialogCard.image_path), 256)"
              :alt="dialogCard.name"
              class="cc-img"
            />
            <div class="cc-holo" />
            <div class="cc-cg" />
            <span class="cc-rar" :class="rarityBadgeClass[dialogCard.rarity]">{{ dialogCard.rarity }}</span>
          </div>

          <!-- Card info -->
          <div class="text-center">
            <p class="text-base font-semibold text-foreground">{{ dialogCard.name }}</p>
            <p class="mt-0.5 text-sm font-medium text-muted-foreground">{{ dialogCard.source_title }}</p>
            <span
              class="mt-2 inline-block rounded px-2 py-0.5 text-xs font-semibold"
              :class="{
                'bg-warning/20 text-warning': dialogCard.rarity === 'SSR',
                'cc-sr-badge': dialogCard.rarity === 'SR',
                'cc-r-badge': dialogCard.rarity === 'R',
                'bg-muted text-muted-foreground': dialogCard.rarity === 'N',
              }"
            >{{ dialogCard.rarity }}</span>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
/* ── Base card ────────────────────────────────────────────────────────────── */
.cc-gcard {
  position: relative;
  aspect-ratio: 5 / 7;
  border-radius: 16px;
  overflow: hidden;
  cursor: pointer;
  border: 1.5px solid transparent;
  transition: transform 0.2s ease-out, box-shadow 0.2s ease-out;
  background-color: hsl(var(--card));
  width: 120px;
  flex-shrink: 0;
}

.cc-gcard:hover {
  transform: rotateY(-4deg) rotateX(2deg) translateY(-4px);
}

@media (prefers-reduced-motion: reduce) {
  .cc-gcard {
    transition: none;
  }
  .cc-gcard:hover {
    transform: none;
  }
}

/* ── Card internals ───────────────────────────────────────────────────────── */
.cc-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.cc-holo {
  position: absolute;
  inset: 0;
  background: linear-gradient(
    115deg,
    transparent 30%,
    rgba(255, 255, 255, 0.18) 48%,
    transparent 60%
  );
  mix-blend-mode: overlay;
  pointer-events: none;
}

.cc-cg {
  position: absolute;
  inset: 0;
  background: linear-gradient(180deg, transparent 45%, rgba(0, 0, 0, 0.8));
  pointer-events: none;
}

.cc-rar {
  position: absolute;
  top: 6px;
  left: 6px;
  font-size: 9px;
  font-weight: 600;
  padding: 1px 5px;
  border-radius: 4px;
  line-height: 1.4;
  letter-spacing: 0.04em;
}

.cc-cap {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  padding: 4px 6px;
}

/* ── Rarity frames ────────────────────────────────────────────────────────── */
.cc-frame {
  position: absolute;
  inset: 0;
  border-radius: 16px;
  pointer-events: none;
  z-index: 2;
}

.frame-ssr {
  border: 1.5px solid transparent;
  background: linear-gradient(hsl(var(--card)), hsl(var(--card))) padding-box,
    linear-gradient(135deg, var(--warning), #ff9d00, var(--warning)) border-box;
}

.frame-sr {
  border: 1.5px solid transparent;
  background: linear-gradient(hsl(var(--card)), hsl(var(--card))) padding-box,
    linear-gradient(135deg, var(--brand-violet), var(--brand-pink)) border-box;
}

.frame-r {
  border: 1.5px solid transparent;
  background: linear-gradient(hsl(var(--card)), hsl(var(--card))) padding-box,
    linear-gradient(135deg, var(--brand-cyan), #009dcc) border-box;
}

.frame-n {
  border: 1.5px solid hsl(var(--border));
}

/* ── Rarity badges ────────────────────────────────────────────────────────── */
.rar-ssr {
  background: rgba(255, 214, 0, 0.2);
  color: var(--warning);
}

.rar-sr {
  background: rgba(167, 139, 250, 0.2);
  color: var(--brand-violet);
}

.rar-r {
  background: rgba(34, 211, 238, 0.2);
  color: var(--brand-cyan);
}

.rar-n {
  background: hsl(var(--muted));
  color: hsl(var(--muted-foreground));
}

/* ── Rarity badge utility classes (in panel/dialog) ──────────────────────── */
.cc-sr-badge {
  background: rgba(167, 139, 250, 0.2);
  color: var(--brand-violet);
}

.cc-r-badge {
  background: rgba(34, 211, 238, 0.2);
  color: var(--brand-cyan);
}

/* ── ROW variant ─────────────────────────────────────────────────────────── */
.cc-row {
  scrollbar-width: thin;
  scrollbar-color: hsl(var(--border)) transparent;
}

/* Row cards scale to the cell height (aspect 5/7 derives the width) so the
   strip never overflows the fixed bento row. */
.cc-row .cc-gcard {
  height: 100%;
  width: auto;
}

/* Clean-clip overflow: fade the bottom edge so cards beyond the cell cut
   cleanly (no scrollbar). `black`/`transparent` = alpha mask, not a color. */
.fade-clip {
  -webkit-mask-image: linear-gradient(to bottom, black 84%, transparent);
  mask-image: linear-gradient(to bottom, black 84%, transparent);
}

/* ── FAN variant ─────────────────────────────────────────────────────────── */
.cc-fan-card {
  width: 120px;
  transform-origin: bottom center;
}

.cc-fan-card:hover {
  transform: translateY(-30px) rotate(0deg) scale(1.08) !important;
  z-index: 10 !important;
}

.cc-fan-pos-0 {
  transform: rotate(-22deg) translateX(-130px) translateY(20px);
  z-index: 1;
}
.cc-fan-pos-1 {
  transform: rotate(-11deg) translateX(-65px) translateY(4px);
  z-index: 2;
}
.cc-fan-pos-2 {
  transform: rotate(0deg) translateY(-6px);
  z-index: 5;
}
.cc-fan-pos-3 {
  transform: rotate(11deg) translateX(65px) translateY(4px);
  z-index: 2;
}
.cc-fan-pos-4 {
  transform: rotate(22deg) translateX(130px) translateY(20px);
  z-index: 1;
}

@media (prefers-reduced-motion: reduce) {
  .cc-fan-card:hover {
    transform: none !important;
  }
}

/* ── GRID variant ─────────────────────────────────────────────────────────── */
.cc-grid .cc-gcard {
  width: 100%;
}

.cc-grid .cc-gcard:hover {
  transform: translateY(-5px) scale(1.03);
}

/* ── HERO variant ─────────────────────────────────────────────────────────── */
.cc-hero-feat {
  width: 180px;
}

.cc-hero-mini {
  width: 60px;
  border-radius: 10px;
}

.cc-hero-mini:hover {
  transform: translateY(-4px);
}

/* ── TILT3D variant ───────────────────────────────────────────────────────── */
.cc-tilt-card {
  width: 150px;
  will-change: transform;
}

.cc-tilt-card:hover {
  transform: none; /* JS override takes over */
}

.cc-sheen {
  position: absolute;
  inset: 0;
  border-radius: 16px;
  background: radial-gradient(
    circle at var(--mx, 50%) var(--my, 50%),
    rgba(255, 255, 255, 0.4),
    transparent 45%
  );
  opacity: 0;
  transition: opacity 0.2s;
  pointer-events: none;
  z-index: 3;
}

.cc-tilt-card:hover .cc-sheen {
  opacity: 1;
}

@media (prefers-reduced-motion: reduce) {
  .cc-tilt-card {
    will-change: auto;
  }
  .cc-sheen {
    display: none;
  }
}

/* ── DIALOG ───────────────────────────────────────────────────────────────── */
.cc-dialog-backdrop {
  background: rgba(4, 4, 10, 0.72);
  backdrop-filter: blur(8px);
}

.cc-dialog-inner {
  pointer-events: auto;
}

.cc-dialog-card {
  width: 180px;
  cursor: default;
}

.cc-dialog-card:hover {
  transform: none;
}

/* Dialog transition */
.cc-dlg-enter-active,
.cc-dlg-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.cc-dlg-enter-from,
.cc-dlg-leave-to {
  opacity: 0;
  transform: scale(0.95);
}

@media (prefers-reduced-motion: reduce) {
  .cc-dlg-enter-active,
  .cc-dlg-leave-active {
    transition: opacity 0.1s;
  }
  .cc-dlg-enter-from,
  .cc-dlg-leave-to {
    transform: none;
  }
}
</style>
