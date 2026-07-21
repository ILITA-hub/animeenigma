<template>
  <template v-if="modelValue">
    <!-- Keep the highlighted page visible, but intercept accidental clicks
         while the modal tour owns interaction. -->
    <div class="fixed inset-0 z-[69]" aria-hidden="true" />
    <div
      v-if="targetRect"
      class="site-guide-spotlight"
      :style="spotlightStyle"
      aria-hidden="true"
      data-testid="site-guide-spotlight"
    />
    <div v-else class="fixed inset-0 z-[70] bg-black/80" aria-hidden="true" />

    <section
      ref="panelRef"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      :aria-describedby="bodyId"
      tabindex="-1"
      class="fixed z-[71] inset-x-4 bottom-4 sm:left-1/2 sm:right-auto sm:w-[min(32rem,calc(100vw-2rem))] sm:-translate-x-1/2 glass-elevated rounded-2xl border border-primary/30 p-4 sm:p-6 shadow-2xl"
      data-testid="site-guide-panel"
      @keydown="onKeydown"
    >
      <div class="flex items-start gap-3">
        <div class="size-10 rounded-xl bg-primary/15 text-primary flex items-center justify-center flex-shrink-0">
          <Compass class="size-5" aria-hidden="true" />
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
        <Button size="sm" data-testid="site-guide-next" @click="next">
          {{ currentIndex === steps.length - 1 ? t('siteGuide.finish') : t('common.next') }}
        </Button>
      </div>
    </section>
  </template>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, toRef, watch } from 'vue'
import { Compass } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui'
import { useFocusTrap } from '@/composables/useFocusTrap'

interface GuideStep {
  target: string
  titleKey: string
  bodyKey: string
}

interface TargetRect {
  top: number
  left: number
  width: number
  height: number
}

const props = defineProps<{ modelValue: boolean }>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
const { t } = useI18n()

const steps: GuideStep[] = [
  { target: 'brand', titleKey: 'siteGuide.steps.home.title', bodyKey: 'siteGuide.steps.home.body' },
  { target: 'search', titleKey: 'siteGuide.steps.search.title', bodyKey: 'siteGuide.steps.search.body' },
  { target: 'catalog', titleKey: 'siteGuide.steps.catalog.title', bodyKey: 'siteGuide.steps.catalog.body' },
  { target: 'schedule', titleKey: 'siteGuide.steps.schedule.title', bodyKey: 'siteGuide.steps.schedule.body' },
  { target: 'account', titleKey: 'siteGuide.steps.account.title', bodyKey: 'siteGuide.steps.account.body' },
  { target: 'feedback', titleKey: 'siteGuide.steps.feedback.title', bodyKey: 'siteGuide.steps.feedback.body' },
]

const currentIndex = ref(0)
const targetRect = ref<TargetRect | null>(null)
const panelRef = ref<HTMLElement | null>(null)
const titleId = 'site-guide-title'
const bodyId = 'site-guide-body'
let settleTimer: number | undefined

useFocusTrap({ active: toRef(props, 'modelValue'), container: panelRef })

const currentStep = computed(() => steps[currentIndex.value])
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

function measureTarget(): void {
  const el = visibleTarget(currentStep.value.target)
  if (!el) {
    targetRect.value = null
    return
  }

  const before = el.getBoundingClientRect()
  const outsideViewport = before.top < 12 || before.bottom > window.innerHeight - 12
  if (outsideViewport) el.scrollIntoView({ behavior: 'smooth', block: 'center' })

  const rect = el.getBoundingClientRect()
  const padding = 8
  targetRect.value = {
    top: Math.max(4, rect.top - padding),
    left: Math.max(4, rect.left - padding),
    width: Math.min(window.innerWidth - 8, rect.width + padding * 2),
    height: rect.height + padding * 2,
  }

  if (outsideViewport) {
    window.clearTimeout(settleTimer)
    settleTimer = window.setTimeout(measureTarget, 350)
  }
}

async function refreshTarget(): Promise<void> {
  await nextTick()
  measureTarget()
}

function close(): void {
  emit('update:modelValue', false)
}

function previous(): void {
  if (currentIndex.value === 0) return
  currentIndex.value -= 1
}

function next(): void {
  if (currentIndex.value === steps.length - 1) {
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
  () => props.modelValue,
  (open) => {
    if (!open) return
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
  window.clearTimeout(settleTimer)
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
