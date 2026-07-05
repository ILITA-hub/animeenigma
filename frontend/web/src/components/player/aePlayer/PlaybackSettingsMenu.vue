<template>
  <!-- Root menu or sub-view -->
  <div class="flex flex-col min-w-[250px]">

    <!-- Sub-view: Quality -->
    <template v-if="view === 'quality'">
      <button
        class="w-full flex items-center gap-2 px-2.5 py-1.5 pb-2.5 bg-transparent border-0 text-white text-[13px] font-semibold"
        @click="view = 'root'"
      >
        <ChevronLeft class="size-[14px]" aria-hidden="true" />
        {{ $t('player.aePlayer.quality') }}
      </button>
      <div class="flex flex-col gap-[3px] pb-1">
        <button
          v-for="q in qualities"
          :key="q"
          :class="[
            'w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-left transition-colors',
            quality === q
              ? 'text-[var(--brand-cyan)]'
              : 'text-[var(--ink-2)] hover:bg-white/[0.08] hover:text-white',
          ]"
          @click="select('quality', q)"
        >
          <Check v-if="quality === q" class="size-3 flex-shrink-0" aria-hidden="true" />
          <span v-else class="w-3 flex-shrink-0" aria-hidden="true"/>
          {{ q }}
          <span v-if="q !== 'Auto' && q === (qualities.find(x => x !== 'Auto') ?? '')" class="ml-auto text-[10px] font-semibold uppercase" style="background: var(--brand-cyan); padding: 1px 5px; border-radius: 4px; color: var(--color-base);">HD</span>
        </button>
      </div>
    </template>

    <!-- Sub-view: Speed -->
    <template v-else-if="view === 'speed'">
      <button
        class="w-full flex items-center gap-2 px-2.5 py-1.5 pb-2.5 bg-transparent border-0 text-white text-[13px] font-semibold"
        @click="view = 'root'"
      >
        <ChevronLeft class="size-[14px]" aria-hidden="true" />
        {{ $t('player.aePlayer.speed') }}
      </button>
      <div class="flex gap-1 px-1.5 pb-1.5 flex-wrap">
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
            ? 'background: var(--cyan-a20)'
            : 'background: var(--line)'"
          @click="select('speed', s)"
        >
          {{ s === 1 ? $t('player.aePlayer.normal') : `${s}×` }}
        </button>
      </div>
    </template>

    <!-- Root menu -->
    <template v-else>
      <!-- Header -->
      <div class="px-2.5 pt-2 pb-1">
        <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
          {{ $t('player.aePlayer.playback') }}
        </span>
      </div>

      <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

      <!-- Quality row -->
      <button
        class="w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-[var(--ink-2)] text-left transition-colors hover:bg-white/[0.08] hover:text-white"
        @click="view = 'quality'"
      >
        <MonitorPlay class="size-4 flex-shrink-0" aria-hidden="true" />
        <span class="flex-1">{{ $t('player.aePlayer.quality') }}</span>
        <span class="inline-flex items-center gap-1.5 text-[13px] text-[var(--muted-foreground)] mr-1">
          {{ qualityDisplay ?? quality }}
          <ChevronDown class="size-3" aria-hidden="true" />
        </span>
      </button>

      <!-- Speed row -->
      <button
        class="w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-[var(--ink-2)] text-left transition-colors hover:bg-white/[0.08] hover:text-white"
        @click="view = 'speed'"
      >
        <Gauge class="size-4 flex-shrink-0" aria-hidden="true" />
        <span class="flex-1">{{ $t('player.aePlayer.speed') }}</span>
        <span class="inline-flex items-center gap-1.5 text-[13px] text-[var(--muted-foreground)] mr-1">
          {{ speed === 1 ? $t('player.aePlayer.normal') : `${speed}×` }}
          <ChevronDown class="size-3" aria-hidden="true" />
        </span>
      </button>

      <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

      <!-- Autoplay next toggle -->
      <div class="flex items-center gap-2.5 px-2.5 py-[9px]">
        <SkipForward class="size-4 flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true" />
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">{{ $t('player.aePlayer.autoplayNext') }}</span>
        <Switch
          :model-value="autoNext"
          @update:model-value="emit('update:autoNext', $event)"
        />
      </div>

      <!-- Auto-skip intro toggle -->
      <div class="flex items-center gap-2.5 px-2.5 py-[9px]">
        <FastForward class="size-4 flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true" />
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">{{ $t('player.aePlayer.autoSkipIntro') }}</span>
        <Switch
          :model-value="autoSkip"
          @update:model-value="emit('update:autoSkip', $event)"
        />
      </div>

      <!-- Hacker mode toggle -->
      <div class="flex items-center gap-2.5 px-2.5 py-[9px]">
        <Terminal class="size-4 flex-shrink-0 text-[var(--ink-2)]" aria-hidden="true" />
        <span class="flex-1 text-[14px] text-[var(--ink-2)]">{{ $t('player.aePlayer.hackerMode') }}</span>
        <Switch
          :model-value="hackerMode"
          @update:model-value="emit('update:hackerMode', $event)"
        />
      </div>

      <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>

      <!-- Share this moment — copies a link with the exact source/episode/time -->
      <button
        class="w-full flex items-center gap-2.5 px-2.5 py-[9px] rounded-[var(--r-sm)] bg-transparent border-0 text-[14px] text-[var(--ink-2)] text-left transition-colors hover:bg-white/[0.08] hover:text-white"
        data-test="share-moment"
        @click="emit('share')"
      >
        <Link class="size-4 flex-shrink-0" aria-hidden="true" />
        <span class="flex-1">{{ $t('player.aePlayer.shareMoment') }}</span>
      </button>

      <!-- Live debug mini-stats (hacker mode only) -->
      <template v-if="hackerMode && debugStats">
        <div class="h-px mx-1 my-1.5 bg-[var(--border)]"/>
        <div class="px-2.5 pb-2 font-mono text-[11px] leading-[1.8] text-[var(--success)]" data-test="debug-stats">
          <div>BW   {{ debugStats.bw }}</div>
          <div>BUF  {{ debugStats.buffer }}</div>
          <div>LVL  {{ debugStats.level }}</div>
          <div>FRAG {{ debugStats.frag }}</div>
        </div>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Switch from '@/components/ui/Switch.vue'
import { ChevronLeft, ChevronDown, Check, MonitorPlay, Gauge, SkipForward, FastForward, Terminal, Link } from 'lucide-vue-next'

defineProps<{
  quality: string
  qualities: string[]
  /** e.g. "Auto · 720p" while auto-switching; falls back to `quality` */
  qualityDisplay?: string
  speed: number
  speeds: number[]
  autoNext: boolean
  autoSkip: boolean
  hackerMode: boolean
  /** compact live debug numbers; null hides the section */
  debugStats?: { bw: string; buffer: string; level: string; frag: string } | null
}>()

const emit = defineEmits<{
  (e: 'update:quality', value: string): void
  (e: 'update:speed', value: number): void
  (e: 'update:autoNext', value: boolean): void
  (e: 'update:autoSkip', value: boolean): void
  (e: 'update:hackerMode', value: boolean): void
  (e: 'share'): void
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
