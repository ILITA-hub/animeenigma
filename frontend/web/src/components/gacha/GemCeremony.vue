<template>
  <!-- Gem summon ceremony: spiral sparks → charge → crack seams → burst → flash.
       Rarity-tease color; SSR is gold + slower. Ported 1:1 from v21. -->
  <Teleport to="body">
    <div
      v-if="active"
      class="summon"
      :class="[phaseClass, { gold: topTier === 'SSR' }]"
      :style="{ '--tease': teaseColor }"
      data-testid="gem-ceremony"
    >
      <div ref="gemwrap" class="gemwrap">
        <div class="gem">◆</div>
        <div v-for="(s, i) in sparks" :key="`s${i}`" class="spark" :style="s" />
        <div v-for="(s, i) in seams" :key="`m${i}`" class="seam" :style="s" />
      </div>
      <div class="summon-hint">{{ hintText }}</div>
      <button type="button" class="skip" data-testid="ceremony-skip" @click="skip">
        {{ $t('gacha.ceremony_skip') }}
      </button>
    </div>
    <div ref="flashEl" class="flash" :class="{ go: flashing }" />
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Rarity } from '@/api/gacha'

const props = defineProps<{
  /** When true, the ceremony runs. Parent flips this on a pull. */
  active: boolean
  /** Highest rarity in the pull — drives tease color + duration. */
  topTier: Rarity
}>()

const emit = defineEmits<{
  /** Fired when the ceremony completes (or is skipped). */
  done: [skipped: boolean]
}>()

const { t } = useI18n()

const TEASE: Record<Rarity, string> = {
  N: 'rgba(255,255,255,.55)',
  R: 'rgb(45,212,191)',
  SR: 'rgb(129,140,248)',
  SSR: 'rgb(251,146,60)',
}

const phase = ref<'idle' | 'charge' | 'crack' | 'burst'>('idle')
const flashing = ref(false)
const gemwrap = ref<HTMLElement | null>(null)
const flashEl = ref<HTMLElement | null>(null)

const teaseColor = computed(() => TEASE[props.topTier] ?? TEASE.N)
const phaseClass = computed(() => (phase.value === 'idle' ? '' : phase.value))
const hintText = computed(() =>
  props.topTier === 'SSR' && (phase.value === 'charge' || phase.value === 'crack')
    ? t('gacha.ceremony_hint_ssr')
    : t('gacha.ceremony_hint'),
)

// Spiral spark + crack-seam styles (computed once per run).
const sparks = ref<Record<string, string>[]>([])
const seams = ref<Record<string, string>[]>([])

let timers: ReturnType<typeof setTimeout>[] = []

function clearTimers() {
  timers.forEach(clearTimeout)
  timers = []
}

function buildParticles(ssr: boolean) {
  const n = ssr ? 26 : 16
  sparks.value = Array.from({ length: n }, (_, i) => ({
    '--a': `${(360 / n) * i + Math.random() * 20}deg`,
    '--dur': `${0.8 + Math.random() * 0.5}s`,
    '--del': `${Math.random() * 0.6}s`,
  }))
  seams.value = Array.from({ length: 7 }, (_, i) => ({
    '--sa': `${Math.random() * 360}deg`,
    'animation-delay': `${i * 0.05}s`,
  }))
}

function triggerFlash() {
  // Force CSS animation restart: remove + reflow + add.
  flashing.value = false
  void flashEl.value?.offsetWidth
  flashing.value = true
}

function finish(skipped: boolean) {
  clearTimers()
  phase.value = 'idle'
  triggerFlash()
  emit('done', skipped)
}

function skip() {
  finish(true)
}

async function run() {
  clearTimers()
  const ssr = props.topTier === 'SSR'
  buildParticles(ssr)
  phase.value = 'idle'
  await nextTick()

  const chargeAt = ssr ? 1200 : 700
  const crackAt = chargeAt + (ssr ? 900 : 500)
  const burstAt = crackAt + 450
  const doneAt = burstAt + 420

  timers = [
    setTimeout(() => { phase.value = 'charge' }, chargeAt),
    setTimeout(() => { phase.value = 'crack' }, crackAt),
    setTimeout(() => { phase.value = 'burst' }, burstAt),
    setTimeout(() => finish(false), doneAt),
  ]
}

watch(
  () => props.active,
  (on) => {
    if (on) run()
    else clearTimers()
  },
  { immediate: true },
)

onBeforeUnmount(clearTimers)
</script>

<style scoped>
.summon {
  position: fixed;
  inset: 0;
  z-index: 96;
  background: rgba(2, 2, 8, 0.93);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-direction: column;
  gap: 1.6rem;
}
.summon.gold {
  background: radial-gradient(ellipse at 50% 45%, rgba(124, 45, 18, 0.45), rgba(2, 2, 8, 0.96) 70%);
}
.gemwrap {
  position: relative;
  width: 300px;
  height: 300px;
  display: grid;
  place-items: center;
}
.gem {
  font-size: 6.5rem;
  color: var(--tease);
  text-shadow: 0 0 34px var(--tease), 0 0 90px var(--tease);
  animation: gemIdle 1.2s ease-in-out infinite;
  position: relative;
  z-index: 2;
}
@keyframes gemIdle {
  0%, 100% { transform: scale(1); }
  50% { transform: scale(1.07); }
}
.summon.charge .gem {
  animation: gemCharge 0.42s ease-in-out infinite;
}
@keyframes gemCharge {
  0%, 100% { transform: scale(1) rotate(-1.2deg); }
  50% { transform: scale(1.13) rotate(1.2deg); }
}
.summon.crack .gem {
  animation: gemShake 0.09s linear infinite;
}
@keyframes gemShake {
  0% { transform: translate(-3px, 1px); }
  25% { transform: translate(3px, -2px); }
  50% { transform: translate(-2px, -1px); }
  75% { transform: translate(2px, 2px); }
}
.summon.burst .gem {
  animation: gemBurst 0.5s ease-out forwards;
}
@keyframes gemBurst {
  0% { transform: scale(1.1); opacity: 1; filter: blur(0); }
  100% { transform: scale(4.2); opacity: 0; filter: blur(10px); }
}
.spark {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 7px;
  height: 7px;
  margin: -3.5px;
  border-radius: 50%;
  background: var(--tease);
  box-shadow: 0 0 10px var(--tease);
  opacity: 0;
  transform: rotate(var(--a)) translateX(190px) scale(0.3);
  animation: suckIn var(--dur, 1s) var(--del, 0s) cubic-bezier(0.5, 0, 0.8, 0.4) infinite;
}
@keyframes suckIn {
  0% { opacity: 0; transform: rotate(var(--a)) translateX(190px) scale(0.3); }
  25% { opacity: 1; }
  100% { opacity: 0; transform: rotate(calc(var(--a) + 260deg)) translateX(4px) scale(1.15); }
}
.seam {
  position: absolute;
  left: 50%;
  top: 50%;
  width: 3px;
  height: 0;
  background: linear-gradient(rgb(255, 255, 255), var(--tease));
  box-shadow: 0 0 14px var(--tease);
  transform-origin: top center;
  transform: translate(-50%, -50%) rotate(var(--sa));
  opacity: 0;
  z-index: 3;
}
.summon.crack .seam,
.summon.burst .seam {
  animation: seamGrow 0.3s ease-out forwards;
}
@keyframes seamGrow {
  0% { height: 0; opacity: 0; }
  40% { opacity: 1; }
  100% { height: 130px; opacity: 0.95; }
}
.summon-hint {
  font-size: 0.85rem;
  color: var(--ink-2);
}
.skip {
  font-size: 0.8rem;
  color: var(--ink-4);
  cursor: pointer;
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 999px;
  padding: 0.35rem 1rem;
  background: none;
}
.skip:hover {
  color: rgb(255, 255, 255);
  border-color: rgba(255, 255, 255, 0.45);
}
.flash {
  position: fixed;
  inset: 0;
  z-index: 97;
  background: rgb(255, 255, 255);
  opacity: 0;
  pointer-events: none;
}
.flash.go {
  animation: flashOut 0.5s ease-out;
}
@keyframes flashOut {
  0% { opacity: 0.95; }
  100% { opacity: 0; }
}
</style>
