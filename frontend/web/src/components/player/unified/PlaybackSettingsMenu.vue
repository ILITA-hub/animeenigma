<template>
  <!-- Root menu or sub-view -->
  <div class="flex flex-col" style="min-width: 250px;">

    <!-- Sub-view: Quality -->
    <template v-if="view === 'quality'">
      <button
        class="w-full flex items-center gap-2 px-[10px] py-[6px] pb-[10px] bg-transparent border-0 text-white text-[13px] font-semibold"
        @click="view = 'root'"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M9 2L4 7L9 12" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        Quality
      </button>
      <div class="flex flex-col gap-[3px] pb-1">
        <button
          v-for="q in qualities"
          :key="q"
          :class="[
            'w-full flex items-center gap-[10px] px-[10px] py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-left transition-colors',
            quality === q
              ? 'text-[var(--brand-cyan)]'
              : 'text-[var(--ink-2)] hover:bg-white/[0.08] hover:text-white',
          ]"
          @click="select('quality', q)"
        >
          <svg
            v-if="quality === q"
            width="12" height="12" viewBox="0 0 12 12" fill="none"
            class="flex-shrink-0" aria-hidden="true"
          >
            <path d="M2 6L5 9L10 3" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
          <span v-else class="w-3 flex-shrink-0" aria-hidden="true"/>
          {{ q }}
          <span v-if="q === qualities[0]" class="ml-auto text-[10px] font-semibold uppercase text-white" style="background: var(--brand-cyan); padding: 1px 5px; border-radius: 4px; color: var(--color-base);">HD</span>
        </button>
      </div>
    </template>

    <!-- Sub-view: Speed -->
    <template v-else-if="view === 'speed'">
      <button
        class="w-full flex items-center gap-2 px-[10px] py-[6px] pb-[10px] bg-transparent border-0 text-white text-[13px] font-semibold"
        @click="view = 'root'"
      >
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
          <path d="M9 2L4 7L9 12" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        Speed
      </button>
      <div class="flex gap-1 px-[6px] pb-[6px] flex-wrap">
        <button
          v-for="s in speeds"
          :key="s"
          :class="[
            'flex-1 py-[7px] rounded-[var(--r-sm)] border-0 text-[12px] font-mono transition-colors',
            speed === s
              ? 'text-[var(--brand-cyan)]'
              : 'text-[var(--ink-2)] hover:bg-white/[0.12] hover:text-white',
          ]"
          :style="speed === s
            ? 'background: rgba(0,212,255,0.2)'
            : 'background: rgba(255,255,255,0.06)'"
          @click="select('speed', s)"
        >
          {{ s === 1 ? 'Normal' : `${s}×` }}
        </button>
      </div>
    </template>

    <!-- Root menu -->
    <template v-else>
      <!-- Header -->
      <div class="px-[10px] pt-[8px] pb-[4px]">
        <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
          Playback
        </span>
      </div>

      <div class="h-px mx-1 my-[6px]" style="background: var(--border);"/>

      <!-- Quality row -->
      <button
        class="w-full flex items-center gap-[10px] px-[10px] py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-[var(--ink-2)] text-left transition-colors hover:bg-white/[0.08] hover:text-white"
        @click="view = 'quality'"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" class="flex-shrink-0" aria-hidden="true">
          <rect x="2" y="5" width="12" height="8" rx="1.5" stroke="currentColor" stroke-width="1.5"/>
          <path d="M5 5V4a1 1 0 011-1h4a1 1 0 011 1v1" stroke="currentColor" stroke-width="1.5"/>
        </svg>
        <span class="flex-1">Quality</span>
        <span class="inline-flex items-center gap-[6px] text-[13px] text-[var(--muted-foreground)] mr-1">
          {{ quality }}
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
            <path d="M4 5L6 7L8 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </span>
      </button>

      <!-- Speed row -->
      <button
        class="w-full flex items-center gap-[10px] px-[10px] py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-[var(--ink-2)] text-left transition-colors hover:bg-white/[0.08] hover:text-white"
        @click="view = 'speed'"
      >
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" class="flex-shrink-0" aria-hidden="true">
          <path d="M8 3v2M8 11v2M3 8H1M15 8h-2M4.93 4.93L3.51 3.51M12.49 12.49l-1.42-1.42M4.93 11.07l-1.42 1.42M12.49 3.51l-1.42 1.42" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
          <circle cx="8" cy="8" r="2.5" stroke="currentColor" stroke-width="1.5"/>
        </svg>
        <span class="flex-1">Speed</span>
        <span class="inline-flex items-center gap-[6px] text-[13px] text-[var(--muted-foreground)] mr-1">
          {{ speed === 1 ? 'Normal' : `${speed}×` }}
          <svg width="12" height="12" viewBox="0 0 12 12" fill="none" aria-hidden="true">
            <path d="M4 5L6 7L8 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </span>
      </button>

      <div class="h-px mx-1 my-[6px]" style="background: var(--border);"/>

      <!-- Autoplay next toggle -->
      <div class="flex items-center gap-[10px] px-[10px] py-[9px]">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" class="flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true">
          <path d="M3 4l7 4-7 4V4z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
          <path d="M12 4v8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
        </svg>
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">Autoplay next</span>
        <Switch
          :model-value="autoNext"
          @update:model-value="emit('update:autoNext', $event)"
        />
      </div>

      <!-- Auto-skip intro toggle -->
      <div class="flex items-center gap-[10px] px-[10px] py-[9px]">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" class="flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true">
          <path d="M3 4l6 4-6 4V4z" stroke="currentColor" stroke-width="1.5" stroke-linejoin="round"/>
          <path d="M11 4l2 4-2 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
        </svg>
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">Auto-skip intro</span>
        <Switch
          :model-value="autoSkip"
          @update:model-value="emit('update:autoSkip', $event)"
        />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Switch from '@/components/ui/Switch.vue'

defineProps<{
  quality: string
  qualities: string[]
  speed: number
  speeds: number[]
  autoNext: boolean
  autoSkip: boolean
}>()

const emit = defineEmits<{
  (e: 'update:quality', value: string): void
  (e: 'update:speed', value: number): void
  (e: 'update:autoNext', value: boolean): void
  (e: 'update:autoSkip', value: boolean): void
}>()

type View = 'root' | 'quality' | 'speed'
const view = ref<View>('root')

function select(type: 'quality' | 'speed', value: string | number) {
  if (type === 'quality') {
    emit('update:quality', value as string)
  } else {
    emit('update:speed', value as number)
  }
  view.value = 'root'
}
</script>
