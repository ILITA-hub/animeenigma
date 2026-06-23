<template>
  <div class="flex flex-col min-w-[264px]">
    <!-- Header -->
    <div class="px-2.5 pt-2 pb-1">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitles
      </span>
    </div>

    <!-- Fast chooser: [provider] Off RU EN JP -->
    <div class="flex flex-wrap items-center gap-[5px] px-2.5 py-[9px]">
      <!-- Provider chip (bundled subs from the resolved stream) -->
      <button
        v-if="providerChip"
        data-test="sub-provider-chip"
        :title="$t('player.aePlayer.subs.fromProvider', { provider: providerChip.provider })"
        :class="pillClass(providerActive)"
        :style="providerActive ? 'background: var(--cyan-a20)' : ''"
        @click="emit('select-provider')"
      >
        {{ providerChip.provider }}
      </button>

      <!-- Off -->
      <button
        data-test="subs-off"
        :class="pillClass(subLang === 'off')"
        :style="subLang === 'off' ? 'background: var(--cyan-a20)' : ''"
        @click="emit('pick-lang', 'off')"
      >
        {{ $t('player.aePlayer.subs.off') }}
      </button>

      <!-- Fixed RU / EN / JP fast buttons -->
      <button
        v-for="lang in FAST_LANGS"
        :key="lang"
        data-test="fast-lang"
        :data-lang="lang"
        :disabled="!availableSubLangs.includes(lang)"
        :title="availableSubLangs.includes(lang) ? undefined : $t('player.aePlayer.subs.noTrack')"
        :class="[
          pillClass(!providerActive && subLang === lang),
          availableSubLangs.includes(lang) ? '' : 'opacity-40 cursor-not-allowed',
        ]"
        :style="(!providerActive && subLang === lang) ? 'background: var(--cyan-a20)' : ''"
        @click="onFast(lang)"
      >
        {{ FAST_LABELS[lang] }}
      </button>
    </div>

    <!-- Hardsub note: no soft tracks at all, subs are burned in by the provider -->
    <div
      v-if="hardsubNote && availableSubLangs.length === 0"
      class="px-2.5 pb-1.5 text-[11px] leading-snug text-[var(--muted-foreground)]"
      data-test="hardsub-note"
    >
      {{ hardsubNote }}
    </div>

    <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

    <!-- Subtitle settings sub-section -->
    <div class="px-2.5 pb-1 pt-0.5">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitle settings
      </span>
    </div>

    <!-- Text size -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Text size</label>
      <input
        type="range"
        :value="subSize"
        min="50"
        max="200"
        step="5"
        class="flex-1"
        style="accent-color: var(--brand-cyan);"
        @input="emit('update:subSize', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="text-[12px] text-[var(--muted-foreground)] w-8 text-right flex-shrink-0">
        {{ subSize }}%
      </span>
    </div>

    <!-- Background -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Background</label>
      <input
        type="range"
        :value="subBg"
        min="0"
        max="100"
        step="5"
        class="flex-1"
        style="accent-color: var(--brand-cyan);"
        @input="emit('update:subBg', Number(($event.target as HTMLInputElement).value))"
      />
      <span class="text-[12px] text-[var(--muted-foreground)] w-8 text-right flex-shrink-0">
        {{ subBg }}%
      </span>
    </div>

    <!-- Timing offset (DS Stepper primitive) -->
    <div class="flex items-center gap-3 px-2.5 py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Timing</label>
      <div class="flex flex-col items-end gap-1 ml-auto">
        <Stepper
          :model-value="subOffset"
          :step="0.1"
          suffix="s"
          label="offset"
          @update:model-value="emit('update:subOffset', $event)"
        />
        <!-- Hint text -->
        <span class="text-[11px] text-[var(--muted-foreground)]">
          {{ offsetHint }}
          <button
            v-if="subOffset !== 0"
            class="ml-1 text-[var(--brand-cyan)] bg-transparent border-0 text-[11px] underline p-0 cursor-pointer"
            @click="emit('update:subOffset', 0)"
          >Reset</button>
        </span>
      </div>
    </div>

    <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

    <!-- Browse all subtitles -->
    <button
      data-test="open-browse"
      class="w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-left transition-colors hover:bg-white/[0.08] hover:text-white text-[var(--brand-cyan)]"
      @click="emit('open-browse')"
    >
      <List class="size-4 flex-shrink-0" aria-hidden="true" />
      <span class="flex-1 text-[var(--brand-cyan)]">Browse all subtitles</span>
      <ChevronRight class="size-3" aria-hidden="true" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { List, ChevronRight } from 'lucide-vue-next'
import Stepper from '@/components/ui/Stepper.vue'

const { t: $t } = useI18n()

const props = defineProps<{
  subLang: string
  /** Real distinct languages that have a loaded track. */
  availableSubLangs: string[]
  /** Bundled-subs provider from the resolved stream; null hides the chip. */
  providerChip: { provider: string } | null
  /** True when the active subtitle is the provider's bundled track. */
  providerActive: boolean
  /** Shown only when there are NO soft tracks but the stream has burned-in subs. */
  hardsubNote?: string | null
  subSize: number
  subBg: number
  subOffset: number
}>()

const emit = defineEmits<{
  (e: 'pick-lang', value: string): void
  (e: 'select-provider'): void
  (e: 'update:subSize', value: number): void
  (e: 'update:subBg', value: number): void
  (e: 'update:subOffset', value: number): void
  (e: 'open-browse'): void
}>()

const FAST_LANGS = ['ru', 'en', 'ja'] as const
const FAST_LABELS: Record<string, string> = { ru: 'RU', en: 'EN', ja: 'JP' }

function pillClass(active: boolean): string[] {
  return [
    'px-[9px] py-1 rounded-full text-[12px] font-medium border transition-all',
    active
      ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
      : 'bg-white/[0.08] border-transparent text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white',
  ]
}

function onFast(lang: string) {
  if (!props.availableSubLangs.includes(lang)) return
  emit('pick-lang', lang)
}

const offsetHint = computed(() => {
  const v = props.subOffset
  if (v === 0) return 'In sync'
  const abs = Math.abs(v).toFixed(1)
  return v > 0 ? `${abs}s later` : `${abs}s earlier`
})
</script>
