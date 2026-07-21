<template>
  <template v-if="modelValue">
    <div class="fixed inset-0 z-[69]" aria-hidden="true" data-testid="site-guide-blocker" />
    <div
      v-if="targetRect"
      class="site-guide-spotlight"
      :style="spotlightStyle"
      aria-hidden="true"
      data-testid="site-guide-spotlight"
    />
    <div v-else class="fixed inset-0 z-[70] bg-black/80" aria-hidden="true" data-testid="site-guide-backdrop" />

    <section
      ref="panelRef"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      :aria-describedby="bodyId"
      tabindex="-1"
      class="fixed z-[71] inset-x-4 sm:left-1/2 sm:right-auto sm:w-[min(32rem,calc(100vw-2rem))] sm:-translate-x-1/2 glass-elevated rounded-2xl border border-primary/30 p-4 sm:p-6 shadow-2xl"
      :class="panelPlacement"
      :data-guide-mode="mode"
      data-testid="site-guide-panel"
      @keydown="onKeydown"
    >
      <div class="flex items-start gap-3">
        <div class="size-10 rounded-xl bg-primary/15 text-primary flex items-center justify-center flex-shrink-0">
          <Clapperboard v-if="mode === 'player'" class="size-5" aria-hidden="true" />
          <Compass v-else class="size-5" aria-hidden="true" />
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center justify-between gap-3 mb-1">
            <p class="text-xs text-primary font-medium tabular-nums">
              {{ t('siteGuide.progress', { current: currentIndex + 1, total: steps.length }) }}
            </p>
            <button
              type="button"
              class="text-xs text-muted-foreground hover:text-white transition-colors"
              @click="close"
            >
              {{ t('siteGuide.skip') }}
            </button>
          </div>
          <h2 :id="titleId" class="text-lg font-semibold text-white">
            {{ t(currentStep.titleKey) }}
          </h2>
          <p :id="bodyId" class="mt-2 text-sm text-white/70 leading-relaxed">
            {{ t(currentStep.bodyKey) }}
          </p>
        </div>
      </div>

      <div class="mt-5 flex items-center justify-between gap-3">
        <Button
          variant="soft"
          size="sm"
          data-testid="site-guide-back"
          :disabled="currentIndex === 0"
          @click="previous"
        >
          {{ t('common.back') }}
        </Button>
        <div class="flex gap-1.5" aria-hidden="true">
          <span
            v-for="(_, index) in steps"
            :key="index"
            class="h-1.5 rounded-full transition-all"
            :class="index === currentIndex ? 'w-5 bg-primary' : 'w-1.5 bg-white/20'"
          />
        </div>
        <Button
          size="sm"
          data-testid="site-guide-next"
          :loading="playerLaunchPending && isSiteFinal"
          @click="next"
        >
          {{ nextLabel }}
        </Button>
      </div>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { Clapperboard, Compass } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui'
import { useFocusTrap } from '@/composables/useFocusTrap'
import { playerGuideMenu, type PlayerGuideMenu } from '@/composables/siteGuideState'

export type SiteGuideMode = 'site' | 'player'

interface GuideStep {
  target: string
  titleKey: string
  bodyKey: string
  menu?: PlayerGuideMenu
}

interface TargetRect {
  top: number
  left: number
  width: number
  height: number
}

const props = withDefaults(defineProps<{
  modelValue: boolean
  mode?: SiteGuideMode
  playerLaunchPending?: boolean
}>(), {
  mode: 'site',
  playerLaunchPending: false,
})
const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'start-player': []
}>()
const { t } = useI18n()

const SITE_STEPS: GuideStep[] = [
  { target: 'brand', titleKey: 'siteGuide.steps.home.title', bodyKey: 'siteGuide.steps.home.body' },
  { target: 'search', titleKey: 'siteGuide.steps.search.title', bodyKey: 'siteGuide.steps.search.body' },
  { target: 'catalog', titleKey: 'siteGuide.steps.catalog.title', bodyKey: 'siteGuide.steps.catalog.body' },
  { target: 'schedule', titleKey: 'siteGuide.steps.schedule.title', bodyKey: 'siteGuide.steps.schedule.body' },
  { target: 'account', titleKey: 'siteGuide.steps.account.title', bodyKey: 'siteGuide.steps.account.body' },
  { target: 'feedback', titleKey: 'siteGuide.steps.feedback.title', bodyKey: 'siteGuide.steps.feedback.body' },
]

const PLAYER_STEPS: GuideStep[] = [
  { target: 'player-screen', titleKey: 'siteGuide.playerSteps.screen.title', bodyKey: 'siteGuide.playerSteps.screen.body' },
  { target: 'player-menu-episodes', titleKey: 'siteGuide.playerSteps.episodes.title', bodyKey: 'siteGuide.playerSteps.episodes.body', menu: 'episodes' },
  { target: 'player-menu-source', titleKey: 'siteGuide.playerSteps.source.title', bodyKey: 'siteGuide.playerSteps.source.body', menu: 'source' },
  { target: 'player-menu-subs', titleKey: 'siteGuide.playerSteps.subtitles.title', bodyKey: 'siteGuide.playerSteps.subtitles.body', menu: 'subs' },
  { target: 'player-menu-settings', titleKey: 'siteGuide.playerSteps.settings.title', bodyKey: 'siteGuide.playerSteps.settings.body', menu: 'settings' },
  { target: 'player-view', titleKey: 'siteGuide.playerSteps.view.title', bodyKey: 'siteGuide.playerSteps.view.body' },
]

const currentIndex = ref(0)
const targetRect = ref<TargetRect | null>(null)
const panelRef = ref<HTMLElement | null>(null)
const titleId = 'site-guide-title'
const bodyId = 'site-guide-body'
const trapActive = computed(() => props.modelValue)
let settleTimer: number | undefined
let retryTimer: number | undefined
let retryCount = 0

useFocusTrap({ active: trapActive, container: panelRef })

const steps = computed(() => {
  if (props.mode === 'player') return PLAYER_STEPS
  return SITE_STEPS
})
const currentStep = computed(() => steps.value[currentIndex.value])
const isSiteFinal = computed(() => props.mode === 'site' && currentIndex.value === steps.value.length - 1)
const nextLabel = computed(() => {
  if (isSiteFinal.value) return t('siteGuide.continuePlayer')
  return currentIndex.value === steps.value.length - 1 ? t('siteGuide.finish') : t('common.next')
})
const spotlightStyle = computed(() => {
  if (!targetRect.value) return undefined
  const r = targetRect.value
  return {
    top: `${r.top}px`,
    left: `${r.left}px`,
    width: `${r.width}px`,
    height: `${r.height}px`,
  }
})
const panelPlacement = computed(() => {
  if (targetRect.value && targetRect.value.top + targetRect.value.height > window.innerHeight * 0.55) {
    return 'top-[calc(var(--header-offset)+1rem)]'
  }
  return 'bottom-4'
})

function visibleTarget(name: string): HTMLElement | null {
  const candidates = document.querySelectorAll<HTMLElement>(`[data-site-guide="${name}"]`)
  for (const el of candidates) {
    const rect = el.getBoundingClientRect()
    const style = window.getComputedStyle(el)
    if (rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden') {
      return el
    }
  }
  return null
}

function retryMissingTarget(): void {
  if (retryCount >= 30 || !props.modelValue) return
  window.clearTimeout(retryTimer)
  retryCount += 1
  retryTimer = window.setTimeout(measureTarget, 300)
}

function measureTarget(): void {
  const target = currentStep.value.target
  const el = target ? visibleTarget(target) : null
  if (!el) {
    targetRect.value = null
    retryMissingTarget()
    return
  }

  retryCount = 0
  window.clearTimeout(retryTimer)
  const before = el.getBoundingClientRect()
  const outsideViewport = before.top < 12 || before.bottom > window.innerHeight - 12
  if (outsideViewport) el.scrollIntoView({ behavior: 'smooth', block: 'center' })

  const rect = el.getBoundingClientRect()
  const padding = 8
  const left = Math.max(4, rect.left - padding)
  targetRect.value = {
    top: Math.max(4, rect.top - padding),
    left,
    width: Math.min(window.innerWidth - left - 4, rect.width + padding * 2),
    height: rect.height + padding * 2,
  }

  if (outsideViewport) {
    window.clearTimeout(settleTimer)
    settleTimer = window.setTimeout(measureTarget, 350)
  }
}

async function refreshTarget(): Promise<void> {
  targetRect.value = null
  retryCount = 0
  window.clearTimeout(retryTimer)
  playerGuideMenu.value = props.mode === 'player' ? (currentStep.value.menu ?? null) : null
  await nextTick()
  measureTarget()
}

function close(): void {
  playerGuideMenu.value = null
  emit('update:modelValue', false)
}

function previous(): void {
  if (currentIndex.value === 0) return
  currentIndex.value -= 1
}

function next(): void {
  if (isSiteFinal.value) {
    emit('start-player')
    return
  }
  if (currentIndex.value === steps.value.length - 1) {
    close()
    return
  }
  currentIndex.value += 1
}

function onKeydown(e: KeyboardEvent): void {
  if (e.key === 'Escape') {
    e.preventDefault()
    close()
  } else if (e.key === 'ArrowRight') {
    e.preventDefault()
    next()
  } else if (e.key === 'ArrowLeft') {
    e.preventDefault()
    previous()
  }
}

watch(
  [() => props.modelValue, () => props.mode],
  ([open]) => {
    document.body.classList.toggle('site-guide-player-active', Boolean(open && props.mode === 'player'))
    if (!open) {
      playerGuideMenu.value = null
      return
    }
    currentIndex.value = 0
    void refreshTarget()
  },
  { immediate: true },
)

watch(currentIndex, () => void refreshTarget())

function onViewportChange(): void {
  if (props.modelValue) measureTarget()
}

window.addEventListener('resize', onViewportChange)
window.addEventListener('scroll', onViewportChange, true)

onBeforeUnmount(() => {
  document.body.classList.remove('site-guide-player-active')
  window.clearTimeout(settleTimer)
  window.clearTimeout(retryTimer)
  window.removeEventListener('resize', onViewportChange)
  window.removeEventListener('scroll', onViewportChange, true)
})
</script>

<style scoped>
.site-guide-spotlight {
  position: fixed;
  z-index: 70;
  border: 2px solid var(--brand-cyan);
  border-radius: var(--r-xl);
  box-shadow: 0 0 0 9999px var(--black-a80), var(--glow-cyan);
  pointer-events: none;
  transition: inset 0.2s ease, width 0.2s ease, height 0.2s ease;
}
</style>
