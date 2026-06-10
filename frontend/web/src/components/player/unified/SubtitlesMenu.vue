<template>
  <div class="flex flex-col" style="min-width: 264px;">
    <!-- Header -->
    <div class="px-[10px] pt-[8px] pb-[4px]">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitles
      </span>
    </div>

    <!-- Language selection -->
    <div class="flex items-center gap-[10px] px-[10px] py-[9px]">
      <span class="text-[14px] text-[var(--ink-2)] flex-shrink-0">Language</span>
      <div class="flex flex-wrap gap-[5px] justify-end flex-1">
        <!-- Off option -->
        <button
          :class="[
            'px-[9px] py-1 rounded-full text-[12px] font-medium border transition-all',
            subLang === 'off'
              ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
              : 'bg-white/[0.08] border-transparent text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white',
          ]"
          :style="subLang === 'off' ? 'background: rgba(0,212,255,0.18)' : ''"
          @click="emit('update:subLang', 'off')"
        >
          Off
        </button>
        <button
          v-for="lang in subLangs"
          :key="lang"
          :class="[
            'px-[9px] py-1 rounded-full text-[12px] font-medium border transition-all',
            subLang === lang
              ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
              : 'bg-white/[0.08] border-transparent text-[var(--muted-foreground)] hover:bg-white/[0.14] hover:text-white',
          ]"
          :style="subLang === lang ? 'background: rgba(0,212,255,0.18)' : ''"
          @click="emit('update:subLang', lang)"
        >
          {{ langLabel(lang) }}
        </button>
      </div>
    </div>

    <div class="h-px mx-1 my-[6px]" style="background: var(--border);"/>

    <!-- Subtitle settings sub-section -->
    <div class="px-[10px] pb-[4px] pt-[2px]">
      <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
        Subtitle settings
      </span>
    </div>

    <!-- Text size -->
    <div class="flex items-center gap-3 px-[10px] py-2">
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
    <div class="flex items-center gap-3 px-[10px] py-2">
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

    <!-- Timing offset stepper -->
    <div class="flex items-center gap-3 px-[10px] py-2">
      <label class="text-[13px] text-[var(--ink-2)] w-[86px] flex-shrink-0">Timing</label>
      <div class="flex flex-col items-end gap-[4px] ml-auto">
        <!-- Stepper control -->
        <div
          class="inline-flex items-center gap-[2px] rounded-[var(--r-md)] p-[2px]"
          style="background: rgba(255,255,255,0.06);"
        >
          <button
            class="w-[26px] h-[26px] rounded-[var(--r-sm)] border-0 text-white text-[16px] leading-none transition-colors hover:text-[var(--brand-cyan)]"
            style="background: rgba(255,255,255,0.08);"
            aria-label="Decrease offset"
            @click="adjustOffset(-0.1)"
            @mousedown.prevent
          >−</button>
          <div
            class="inline-flex items-baseline rounded-[var(--r-sm)] px-2"
            style="background: rgba(0,0,0,0.25);"
          >
            <input
              type="number"
              :value="subOffset"
              step="0.1"
              class="text-right text-[14px] text-white border-0 bg-transparent py-[5px] focus:outline-none"
              style="width: 46px; -moz-appearance: textfield;"
              @change="onOffsetChange"
            />
            <span class="text-[12px] text-[var(--muted-foreground)] ml-[1px]">s</span>
          </div>
          <button
            class="w-[26px] h-[26px] rounded-[var(--r-sm)] border-0 text-white text-[16px] leading-none transition-colors hover:text-[var(--brand-cyan)]"
            style="background: rgba(255,255,255,0.08);"
            aria-label="Increase offset"
            @click="adjustOffset(0.1)"
            @mousedown.prevent
          >+</button>
        </div>
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

    <div class="h-px mx-1 my-[6px]" style="background: var(--border);"/>

    <!-- Browse all subtitles -->
    <button
      class="w-full flex items-center gap-[10px] px-[10px] py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-left transition-colors hover:bg-white/[0.08] hover:text-white text-[var(--brand-cyan)]"
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
import { List, ChevronRight } from 'lucide-vue-next'

const props = defineProps<{
  subLang: string
  subLangs: string[]
  subSize: number
  subBg: number
  subOffset: number
}>()

const emit = defineEmits<{
  (e: 'update:subLang', value: string): void
  (e: 'update:subSize', value: number): void
  (e: 'update:subBg', value: number): void
  (e: 'update:subOffset', value: number): void
  (e: 'open-browse'): void
}>()

const LANG_LABELS: Record<string, string> = {
  en: 'English',
  ru: 'Русский',
  ja: '日本語',
}

function langLabel(code: string): string {
  return LANG_LABELS[code] ?? code.toUpperCase()
}

const offsetHint = computed(() => {
  const v = props.subOffset
  if (v === 0) return 'In sync'
  const abs = Math.abs(v).toFixed(1)
  return v > 0 ? `${abs}s later` : `${abs}s earlier`
})

function adjustOffset(delta: number) {
  const next = Math.round((props.subOffset + delta) * 10) / 10
  emit('update:subOffset', next)
}

function onOffsetChange(e: Event) {
  const val = parseFloat((e.target as HTMLInputElement).value)
  if (!isNaN(val)) {
    emit('update:subOffset', Math.round(val * 10) / 10)
  }
}
</script>

<style scoped>
input[type='number']::-webkit-outer-spin-button,
input[type='number']::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
</style>
