<template>
  <!-- Sequential fullscreen 3D card reveal. Fly-in from depth w/ spin, shockwave
       + particles tinted by tier, rotating godrays (SSR/SR), drag-rotate via rAF
       (cos-damped vertical, spring-back), sin-based holo shine, branded card back.
       Ported 1:1 from the v21 mock.

       mode='reveal' (default) — pull-reveal flow: Skip all + Next/To results.
       mode='inspect' — collection viewer: Prev/Next (wrap-around) + Close; no Skip all. -->
  <Teleport to="body">
    <div
      v-if="active"
      ref="viewer"
      class="viewer on"
      :class="`t-${current?.card.rarity ?? 'N'}`"
      data-testid="card-viewer-3d"
    >
      <span class="vcount">
        {{
          isInspect
            ? $t('gacha.viewer_inspect_count', { i: index + 1, total: cards.length })
            : $t('gacha.viewer_count', { i: index + 1, total: cards.length })
        }}
      </span>
      <div ref="stage" class="stage">
        <div
          ref="card3d"
          class="card3d"
          :class="[`f-${current?.card.rarity ?? 'N'}`, { slowIn: current?.card.rarity === 'SSR' }]"
          @pointerdown="onPointerDown"
          @pointermove="onPointerMove"
          @pointerup="onPointerUp"
          @pointercancel="onPointerUp"
          @pointerleave="onPointerLeave"
          @dragstart.prevent
          @selectstart.prevent
        >
          <div class="cimg">
            <img v-if="current" :src="cardImageUrl(current.card.image_path)" :alt="current.card.name" draggable="false" />
          </div>
          <div class="holo" />
          <div class="cname">
            <span class="truncate">{{ current?.card.name }}</span>
            <span :class="rarityTextClass(current?.card.rarity)">{{ current?.card.rarity }}</span>
          </div>
          <!-- Card back: uploaded image, else branded default -->
          <div class="cardback">
            <img v-if="backImage" :src="backImage" alt="" class="cardback-img" draggable="false" />
            <template v-else>
              <div class="emblem">
                <div class="ringb" />
                <div class="ringb b2" />
                <span class="dia">◆</span>
              </div>
              <span class="wordmark">{{ $t('gacha.viewer_card_back_wordmark') }}</span>
            </template>
          </div>
          <!-- reveal: NEW badge; inspect: always show ×N dupe count -->
          <span v-if="!isInspect && current?.new" class="vtagNEW">{{ $t('gacha.viewer_new_badge') }}</span>
          <span v-else-if="current && current.count > 1" class="vtagDUP">
            {{ $t('gacha.viewer_dupe_badge', { n: current.count }) }}
          </span>
        </div>
      </div>
      <span class="vhint">{{ $t('gacha.viewer_hint') }}</span>

      <!-- reveal mode footer -->
      <div v-if="!isInspect" class="vbtns">
        <Button variant="outline" size="sm" data-testid="viewer-skip-all" @click="skipAll">
          {{ $t('gacha.viewer_skip_all') }}
        </Button>
        <Button size="sm" data-testid="viewer-next" @click="next">
          {{ isLast ? $t('gacha.viewer_to_results') : $t('gacha.viewer_next') }}
        </Button>
      </div>

      <!-- inspect mode footer -->
      <div v-else class="vbtns">
        <Button
          variant="outline"
          size="sm"
          data-testid="viewer-inspect-prev"
          :aria-label="$t('gacha.viewer_inspect_prev_aria')"
          @click="inspectPrev"
        >
          {{ $t('gacha.viewer_inspect_prev') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          data-testid="viewer-inspect-flip"
          :aria-label="$t('gacha.viewer_flip_aria')"
          @click="flipCard"
        >
          {{ $t('gacha.viewer_flip') }}
        </Button>
        <Button
          variant="outline"
          size="sm"
          data-testid="viewer-inspect-close"
          :aria-label="$t('gacha.viewer_inspect_close_aria')"
          @click="finish"
        >
          {{ $t('gacha.viewer_inspect_close') }}
        </Button>
        <Button
          size="sm"
          data-testid="viewer-inspect-next"
          :aria-label="$t('gacha.viewer_inspect_next_aria')"
          @click="inspectNext"
        >
          {{ $t('gacha.viewer_inspect_next') }}
        </Button>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, onMounted, nextTick } from 'vue'
import Button from '@/components/ui/Button.vue'
import { cardImageUrl, type PulledCard, type Rarity } from '@/api/gacha'

const props = defineProps<{
  active: boolean
  cards: PulledCard[]
  /**
   * 'reveal' (default) — sequential pull-reveal: Skip all + Next/To results.
   * 'inspect' — collection browser: Prev/Next wrap-around + Close; no Skip all.
   */
  mode?: 'reveal' | 'inspect'
  /**
   * Index to start at when the viewer opens (inspect mode).
   * Defaults to 0. Changes while active are ignored (use reactive cards order).
   */
  startIndex?: number
}>()

const emit = defineEmits<{
  /** All cards viewed (next on last), skip-all, or inspect close pressed. */
  done: []
}>()

const isInspect = computed(() => props.mode === 'inspect')

const TEASE: Record<Rarity, string> = {
  N: 'var(--muted-foreground)',
  R: 'rgb(45,212,191)',
  SR: 'rgb(129,140,248)',
  SSR: 'rgb(251,146,60)',
}

const viewer = ref<HTMLElement | null>(null)
const stage = ref<HTMLElement | null>(null)
const card3d = ref<HTMLElement | null>(null)

const index = ref(0)
const current = computed(() => props.cards[index.value] ?? null)
const isLast = computed(() => index.value >= props.cards.length - 1)
const backImage = computed(() =>
  current.value?.card.back_path ? cardImageUrl(current.value.card.back_path) : '',
)

// Inspect rests on whichever face is nearest — front OR back — so the uploaded
// card back can actually be examined; reveal keeps its ceremonial always-front
// snap (step 360 makes every formula below collapse to the original behavior).
const faceStep = computed(() => (isInspect.value ? 180 : 360))

function nearestFace(deg: number): number {
  return Math.round(deg / faceStep.value) * faceStep.value
}

// ── Drag-rotate state ───────────────────────────────────────────────────────
let drag = false
let lx = 0
let ly = 0
// Total pointer travel since pointerdown — distinguishes a tap (flip in
// inspect mode) from a drag (free rotate).
let travel = 0
let rx = 0
let ry = 0
let raf: number | null = null
let fxTimer: ReturnType<typeof setTimeout> | null = null
let fxClear: ReturnType<typeof setTimeout> | null = null

function applyTilt() {
  const c = card3d.value
  if (!c) return
  c.style.transform = `rotateX(${rx}deg) rotateY(${ry}deg)`
  // Holo position via sine of angle: periodic, never tiles past gradient edges.
  const hx = 50 - Math.sin((ry * Math.PI) / 180) * 52
  const hy = 50 + Math.sin((rx * Math.PI) / 180) * 52
  c.style.setProperty('--hx', `${hx}%`)
  c.style.setProperty('--hy', `${hy}%`)
}

function tick() {
  applyTilt()
  raf = drag ? requestAnimationFrame(tick) : null
}

function onPointerDown(e: PointerEvent) {
  const c = card3d.value
  if (!c) return
  drag = true
  lx = e.clientX
  ly = e.clientY
  travel = 0
  c.setPointerCapture(e.pointerId)
  c.style.transition = 'none'
  if (!raf) raf = requestAnimationFrame(tick)
}

function onPointerMove(e: PointerEvent) {
  const c = card3d.value
  if (!c) return
  if (drag) {
    // Vertical contribution fades to 0 at the edge (90°) and flips sign past it —
    // continuous, no jump, unlike a hard inversion.
    const k = Math.cos((ry * Math.PI) / 180)
    travel += Math.abs(e.clientX - lx) + Math.abs(e.clientY - ly)
    ry += (e.clientX - lx) * 0.42
    rx -= (e.clientY - ly) * 0.42 * k
    lx = e.clientX
    ly = e.clientY
    rx = Math.max(-32, Math.min(32, rx))
  } else {
    // Hover tilt is relative to the resting face, so an inspect card resting
    // on its back is not yanked to the front by a mere mouse-over.
    const r = c.getBoundingClientRect()
    const px = (e.clientX - r.left) / r.width - 0.5
    const py = (e.clientY - r.top) / r.height - 0.5
    ry = nearestFace(ry) + px * 22
    rx = -py * 22
    applyTilt()
  }
}

// springTo animates Y to target, levels X, then normalizes the angle.
function springTo(target: number) {
  const c = card3d.value
  if (!c) return
  c.style.transition = 'transform .5s cubic-bezier(.2,1.2,.4,1)'
  ry = target
  rx = 0
  applyTilt()
  setTimeout(() => {
    c.style.transition = 'transform .08s linear'
    ry = ry % 360
  }, 520)
}

// flipCard turns the card to its other face (inspect footer button + tap).
function flipCard() {
  springTo(nearestFace(ry) + 180)
}

function release() {
  const c = card3d.value
  if (!c || !drag) return
  drag = false
  // A motionless tap in inspect mode flips the card — the discoverable way to
  // see the рубашка without mastering a 90°+ drag. Real drags spring to the
  // nearest face: front-only in reveal (step 360), front OR back in inspect.
  if (isInspect.value && travel < 6) {
    flipCard()
    return
  }
  springTo(nearestFace(ry))
}

function onPointerUp() {
  release()
}

function onPointerLeave() {
  if (!drag) {
    rx = 0
    ry = nearestFace(ry)
    applyTilt()
  }
}

// ── Card transitions ────────────────────────────────────────────────────────
function resetTilt() {
  rx = 0
  ry = 0
  const c = card3d.value
  if (c) {
    c.style.transition = 'transform .08s linear'
    c.style.transform = ''
  }
}

function restartFlyIn() {
  const c = card3d.value
  if (!c) return
  // Force CSS-animation restart: the keyframe already ran while hidden.
  c.style.animation = 'none'
  void c.offsetWidth
  c.style.animation = ''
}

function spawnLandingFx(tier: Rarity) {
  const s = stage.value
  if (!s) return
  s.querySelectorAll('.shock,.pt').forEach((e) => e.remove())
  if (fxTimer) clearTimeout(fxTimer)
  if (fxClear) clearTimeout(fxClear)
  const delay = tier === 'SSR' ? 900 : 520
  const tease = TEASE[tier] ?? TEASE.N
  fxTimer = setTimeout(() => {
    const mk = (cls: string) => {
      const d = document.createElement('div')
      d.className = cls
      d.style.setProperty('--tease', tease)
      s.appendChild(d)
      return d
    }
    mk('shock')
    mk('shock s2')
    const n = tier === 'SSR' ? 14 : tier === 'SR' ? 10 : 7
    for (let i = 0; i < n; i++) {
      const a = ((Math.PI * 2) / n) * i + Math.random() * 0.5
      const r = 90 + Math.random() * 120
      const d = mk('pt')
      d.textContent = tier === 'SSR' ? '✦' : '◆'
      d.style.setProperty('--dx', `${Math.cos(a) * r}px`)
      d.style.setProperty('--dy', `${Math.sin(a) * r}px`)
      d.style.setProperty('--rot', `${Math.random() * 240 - 120}deg`)
      d.style.animationDelay = `${Math.random() * 0.1}s`
    }
    fxClear = setTimeout(() => s.querySelectorAll('.shock,.pt').forEach((e) => e.remove()), 1100)
  }, delay)
}

async function renderCurrent() {
  resetTilt()
  await nextTick()
  restartFlyIn()
  const tier = current.value?.card.rarity ?? 'N'
  // Set the viewer godray tease var.
  viewer.value?.style.setProperty('--tease', TEASE[tier] ?? TEASE.N)
  spawnLandingFx(tier)
}

function next() {
  if (isLast.value) {
    finish()
    return
  }
  index.value++
  renderCurrent()
}

function skipAll() {
  finish()
}

/** inspect mode: go to previous card, wrapping around. */
function inspectPrev() {
  index.value = (index.value - 1 + props.cards.length) % props.cards.length
  renderCurrent()
}

/** inspect mode: go to next card, wrapping around. */
function inspectNext() {
  index.value = (index.value + 1) % props.cards.length
  renderCurrent()
}

function finish() {
  cleanupFx()
  emit('done')
}

function onKeyDown(e: KeyboardEvent) {
  if (!props.active) return
  if (e.key === 'Escape') finish()
}

function cleanupFx() {
  if (raf) {
    cancelAnimationFrame(raf)
    raf = null
  }
  if (fxTimer) clearTimeout(fxTimer)
  if (fxClear) clearTimeout(fxClear)
  drag = false
  stage.value?.querySelectorAll('.shock,.pt').forEach((e) => e.remove())
}

function rarityTextClass(rarity?: Rarity): string {
  switch (rarity) {
    case 'SSR': return 'r-ssr'
    case 'SR': return 'r-sr'
    case 'R': return 'r-r'
    default: return 'r-n'
  }
}

watch(
  () => props.active,
  (on) => {
    if (on) {
      index.value = props.startIndex ?? 0
      renderCurrent()
    } else {
      cleanupFx()
    }
  },
  { immediate: true },
)

// Register ESC key handler globally when mounted.
onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
})

onBeforeUnmount(() => {
  cleanupFx()
  window.removeEventListener('keydown', onKeyDown)
})
</script>

<style scoped>
.viewer {
  position: fixed;
  inset: 0;
  z-index: 95;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 1.4rem;
  touch-action: none;
  transition: background 0.6s ease;
  background: radial-gradient(ellipse at 50% 42%, var(--vglow, rgba(40, 40, 60, 0.5)), rgba(2, 2, 8, 0.96) 72%);
}
.viewer.t-N { --vglow: rgba(120, 120, 140, 0.28); }
.viewer.t-R { --vglow: rgba(45, 212, 191, 0.3); }
.viewer.t-SR { --vglow: rgba(129, 140, 248, 0.38); }
.viewer.t-SSR { --vglow: rgba(251, 146, 60, 0.45); }
.viewer::before {
  content: '';
  position: absolute;
  left: 50%;
  top: 42%;
  width: 240vmax;
  height: 240vmax;
  margin: -120vmax 0 0 -120vmax;
  border-radius: 50%;
  pointer-events: none;
  opacity: 0;
  transition: opacity 0.5s;
  background: repeating-conic-gradient(from 0deg at 50% 50%, var(--white-a4) 0deg 6deg, transparent 6deg 14deg);
}
.viewer.t-SSR::before,
.viewer.t-SR::before { opacity: 1; }
.viewer.t-SSR::before {
  background: repeating-conic-gradient(from 0deg at 50% 50%, rgba(251, 146, 60, 0.1) 0deg 6deg, transparent 6deg 14deg);
  animation: rays 14s linear infinite;
}
@keyframes rays {
  to { transform: rotate(360deg); }
}
.stage {
  perspective: 900px;
  position: relative;
}
.card3d {
  position: relative;
  width: min(64vw, 300px);
  aspect-ratio: 3 / 4;
  border-radius: 1.1rem;
  transform-style: preserve-3d;
  transition: transform 0.08s linear;
  cursor: grab;
  /* НИКАКОЙ собственной отрисовки: фон/рамка контейнера легли бы плоскостью
     на z=0 — между лицом (+2px) и рубашкой (−2px) — и закрывали бы рубашку
     при перевороте, пряча и торцы. Грани красятся сами. */
  border: none;
  background: transparent;
  animation: flyIn 0.8s cubic-bezier(0.16, 1, 0.3, 1);
;user-select:none;-webkit-user-select:none;-webkit-user-drag:none}
.card3d.slowIn {
  animation: flyIn 1.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.card3d:active { cursor: grabbing; }
@keyframes flyIn {
  0% { transform: translateZ(-1100px) rotateY(900deg) scale(0.25); opacity: 0; filter: brightness(3) blur(6px); }
  55% { opacity: 1; filter: brightness(1.6) blur(1px); }
  78% { transform: translateZ(46px) rotateY(-14deg) scale(1.05); filter: brightness(1.15) blur(0); }
  100% { transform: none; filter: none; }
}
.card3d.f-SSR .cimg, .card3d.f-SSR .cardback { border-color: rgb(251, 146, 60); box-shadow: 0 0 60px rgba(251, 146, 60, 0.55); }
.card3d.f-SR .cimg, .card3d.f-SR .cardback { border-color: rgb(129, 140, 248); box-shadow: 0 0 34px rgba(129, 140, 248, 0.4); }
.card3d.f-R .cimg, .card3d.f-R .cardback { border-color: rgb(45, 212, 191); box-shadow: 0 0 20px rgba(45, 212, 191, 0.3); }
.card3d.f-N .cimg, .card3d.f-N .cardback { border-color: var(--white-a20); }
.card3d .cimg,
.card3d .holo,
.card3d .cname { backface-visibility: hidden; }
.card3d .cimg {
  position: absolute;
  inset: 0;
  border-radius: 1.1rem;
  border: 3px solid;
  background: var(--elevated);
  overflow: hidden;
}
.card3d .cimg img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.card3d .holo {
  position: absolute;
  inset: 0;
  border-radius: 0.9rem;
  background: linear-gradient(115deg, transparent 30%, var(--white-a30) 47%, var(--cyan-a20) 52%, transparent 68%);
  background-size: 240% 240%;
  background-position: var(--hx, 50%) var(--hy, 50%);
  mix-blend-mode: screen;
  pointer-events: none;
  background-repeat: no-repeat;
}
.card3d .cname {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 1.6rem 0.9rem 0.8rem;
  background: linear-gradient(transparent, var(--black-a80));
  display: flex;
  justify-content: space-between;
  align-items: end;
  font-weight: 600;
  border-radius: 0 0 0.85rem 0.85rem;
  gap: 0.5rem;
}
/* ── card back ── */
.cardback {
  position: absolute;
  inset: 0;
  border-radius: 1.1rem;
  border: 3px solid;
  transform: rotateY(180deg);
  backface-visibility: hidden;
  overflow: hidden;
  background:
    radial-gradient(ellipse at 50% 50%, var(--accent-soft), transparent 70%),
    repeating-linear-gradient(45deg, var(--cyan-a08) 0 2px, transparent 2px 18px),
    repeating-linear-gradient(-45deg, rgba(124, 58, 237, 0.05) 0 2px, transparent 2px 18px),
    linear-gradient(160deg, rgb(13, 13, 28), rgb(19, 19, 38) 55%, rgb(11, 11, 24));
}
.cardback-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.cardback::before {
  content: '';
  position: absolute;
  inset: 10px;
  border: 1px solid var(--accent-line);
  border-radius: 0.6rem;
  box-shadow: inset 0 0 24px var(--cyan-a08);
}
.cardback .emblem {
  position: absolute;
  left: 50%;
  top: 50%;
  transform: translate(-50%, -50%);
  display: grid;
  place-items: center;
}
.cardback .emblem .dia {
  font-size: 3.6rem;
  color: var(--brand-cyan);
  text-shadow: 0 0 18px var(--brand-cyan), 0 0 48px var(--cyan-a40);
}
.cardback .emblem .ringb {
  position: absolute;
  width: 120px;
  height: 120px;
  border-radius: 50%;
  border: 1px solid var(--cyan-a40);
  animation: backRing 3.2s ease-in-out infinite;
}
.cardback .emblem .ringb.b2 {
  width: 150px;
  height: 150px;
  border-style: dashed;
  opacity: 0.5;
  animation-duration: 4.4s;
  animation-direction: reverse;
}
@keyframes backRing {
  0%, 100% { transform: scale(1); opacity: 0.7; }
  50% { transform: scale(1.08); opacity: 1; }
}
.cardback .wordmark {
  position: absolute;
  left: 0;
  right: 0;
  bottom: 14px;
  text-align: center;
  font-size: 0.6rem;
  font-weight: 600;
  letter-spacing: 0.35em;
  color: var(--white-a30);
}
/* shockwave + particle fx (created imperatively, hence :deep) */
.stage :deep(.shock) {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 20px;
  height: 20px;
  margin: -10px;
  border-radius: 50%;
  border: 3px solid var(--tease, var(--brand-cyan));
  opacity: 0;
  pointer-events: none;
  animation: shock 0.6s ease-out forwards;
}
.stage :deep(.shock.s2) {
  animation-delay: 0.12s;
  border-width: 2px;
}
@keyframes shock {
  0% { transform: scale(1); opacity: 0; }
  55% { opacity: 0.8; }
  100% { transform: scale(22); opacity: 0; }
}
.stage :deep(.pt) {
  position: absolute;
  left: 50%;
  top: 50%;
  font-size: 1rem;
  color: var(--tease, var(--brand-cyan));
  text-shadow: 0 0 10px var(--tease, var(--brand-cyan));
  opacity: 0;
  pointer-events: none;
  animation: ptFly 0.9s ease-out forwards;
}
@keyframes ptFly {
  0% { transform: translate(-50%, -50%) scale(0.4); opacity: 0; }
  18% { opacity: 1; }
  100% { transform: translate(calc(-50% + var(--dx)), calc(-50% + var(--dy))) scale(1.1) rotate(var(--rot)); opacity: 0; }
}
.vcount { font-size: 0.82rem; color: var(--ink-4); }
.vhint { font-size: 0.78rem; color: var(--ink-4); }
.vbtns { display: flex; gap: 0.7rem; }
.vtagNEW {
  backface-visibility: hidden;
  position: absolute;
  top: 0.6rem;
  left: 0.6rem;
  background: var(--brand-cyan);
  color: rgb(0, 0, 0);
  font-size: 0.75rem;
  font-weight: 600;
  border-radius: 0.4rem;
  padding: 0.15rem 0.5rem;
  z-index: 2;
}
.vtagDUP {
  backface-visibility: hidden;
  position: absolute;
  top: 0.6rem;
  right: 0.6rem;
  background: var(--black-a60);
  color: var(--ink-2);
  font-size: 0.75rem;
  font-weight: 600;
  border-radius: 0.4rem;
  padding: 0.15rem 0.5rem;
  z-index: 2;
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
.card3d img{user-select:none;-webkit-user-select:none;-webkit-user-drag:none;pointer-events:none}
</style>
