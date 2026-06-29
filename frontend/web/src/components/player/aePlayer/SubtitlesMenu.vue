<template>
  <div class="flex flex-col min-w-[264px]">
    <!-- ─── Captions face ─────────────────────────────────────────────── -->
    <template v-if="face === 'caps'">
      <!-- Header with appearance toggle -->
      <div class="flex items-center justify-between px-3 pt-2 pb-1">
        <span class="text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
          {{ $t('player.aePlayer.subs.title') }}
        </span>
        <button
          data-test="style-toggle"
          class="inline-flex items-center gap-1.5 rounded-full border border-[var(--border)] bg-white/[0.06] px-2.5 py-1 text-[12px] text-[var(--ink-2)] transition-colors hover:bg-white/[0.12] hover:text-white"
          @click="face = 'style'"
        >
          <Settings2 class="size-3.5" aria-hidden="true" />
          {{ $t('player.aePlayer.subs.appearance') }}
        </button>
      </div>

      <!-- Off row -->
      <button
        data-test="subs-off"
        :class="rowClass(subLang === 'off')"
        @click="emit('pick-lang', 'off')"
      >
        <span :class="radioClass(subLang === 'off')" aria-hidden="true" />
        <span class="flex-1" :class="subLang === 'off' ? 'text-[var(--brand-cyan)] font-semibold' : 'text-white'">
          {{ $t('player.aePlayer.subs.off') }}
        </span>
      </button>

      <!-- Language rows: native autonym + source meta -->
      <button
        v-for="lang in FAST_LANGS"
        :key="lang"
        data-test="lang-row"
        :data-lang="lang"
        :disabled="!availableSubLangs.includes(lang)"
        :class="[
          rowClass(subLang === lang),
          availableSubLangs.includes(lang) ? '' : 'opacity-45 cursor-not-allowed',
        ]"
        @click="onPick(lang)"
      >
        <span :class="radioClass(subLang === lang)" aria-hidden="true" />
        <span
          class="flex-1"
          :class="subLang === lang ? 'text-[var(--brand-cyan)] font-semibold' : 'text-white'"
        >{{ AUTONYMS[lang] }}</span>
        <span class="text-[12px] text-[var(--muted-foreground)] truncate max-w-[120px]">
          {{ availableSubLangs.includes(lang)
            ? (langSources[lang] || '')
            : $t('player.aePlayer.subs.noTrack') }}
        </span>
      </button>

      <!-- Hardsub note: no soft tracks, subs burned in by the provider -->
      <div
        v-if="hardsubNote && availableSubLangs.length === 0"
        class="px-3 pb-1.5 text-[11px] leading-snug text-[var(--muted-foreground)]"
        data-test="hardsub-note"
      >
        {{ hardsubNote }}
      </div>

      <div class="h-px mx-1 my-1.5 bg-[var(--border)]" />

      <!-- Browse all subtitles -->
      <button
        data-test="open-browse"
        class="flex w-full items-center gap-2.5 rounded-[var(--r-sm)] px-3 py-2 text-left text-[14px] text-[var(--brand-cyan)] transition-colors hover:bg-white/[0.08]"
        @click="emit('open-browse')"
      >
        <List class="size-4 flex-shrink-0" aria-hidden="true" />
        <span class="flex-1">{{ browseLabel }}</span>
        <ChevronRight class="size-3" aria-hidden="true" />
      </button>
    </template>

    <!-- ─── Appearance face ───────────────────────────────────────────── -->
    <template v-else>
      <!-- Back -->
      <div class="flex items-center gap-2 px-3 pt-2 pb-1">
        <button
          data-test="style-back"
          class="inline-flex items-center gap-1 bg-transparent text-[13px] text-[var(--ink-2)] transition-colors hover:text-white"
          @click="face = 'caps'"
        >
          <ChevronLeft class="size-3.5" aria-hidden="true" />
          {{ $t('player.aePlayer.subs.back') }}
        </button>
        <span class="ml-auto text-[11px] font-semibold uppercase tracking-[0.06em] text-[var(--muted-foreground)]">
          {{ $t('player.aePlayer.subs.appearance') }}
        </span>
      </div>

      <!-- Live preview -->
      <div class="relative mx-3 mb-3 h-[88px] overflow-hidden rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--background)]">
        <div class="absolute inset-x-0 bottom-3 px-3 text-center">
          <span
            class="inline rounded-[5px] px-2 py-0.5 font-medium leading-[1.7] text-white [text-shadow:0_1px_3px_var(--black-a80)] [box-decoration-break:clone] [-webkit-box-decoration-break:clone]"
            :style="previewStyle"
          >{{ $t('player.aePlayer.subs.sample') }}</span>
        </div>
      </div>

      <!-- Text size -->
      <div class="flex items-center gap-3 px-3 py-2">
        <label class="w-[72px] flex-shrink-0 text-[13px] text-[var(--ink-2)]">{{ $t('player.aePlayer.subs.textSize') }}</label>
        <input
          data-test="sub-size"
          type="range"
          :value="subSize"
          min="50"
          max="200"
          step="5"
          class="flex-1 [accent-color:var(--brand-cyan)]"
          @input="emit('update:subSize', Number(($event.target as HTMLInputElement).value))"
        />
        <span class="w-9 flex-shrink-0 text-right text-[12px] text-[var(--muted-foreground)]">{{ subSize }}%</span>
      </div>

      <!-- Background -->
      <div class="flex items-center gap-3 px-3 py-2">
        <label class="w-[72px] flex-shrink-0 text-[13px] text-[var(--ink-2)]">{{ $t('player.aePlayer.subs.background') }}</label>
        <input
          data-test="sub-bg"
          type="range"
          :value="subBg"
          min="0"
          max="100"
          step="5"
          class="flex-1 [accent-color:var(--brand-cyan)]"
          @input="emit('update:subBg', Number(($event.target as HTMLInputElement).value))"
        />
        <span class="w-9 flex-shrink-0 text-right text-[12px] text-[var(--muted-foreground)]">{{ subBg }}%</span>
      </div>

      <!-- Auto-sync -->
      <div class="flex items-center gap-3 px-3 py-2">
        <div class="flex-1">
          <div class="text-[13px] text-[var(--ink-2)]">{{ $t('player.aePlayer.subs.autoSync') }}</div>
          <div class="text-[11px] text-[var(--muted-foreground)]">{{ $t('player.aePlayer.subs.autoSyncHint') }}</div>
        </div>
        <span data-test="autosync-switch">
          <Switch :model-value="autoSync" @update:model-value="emit('update:autoSync', $event)" />
        </span>
      </div>

      <!-- Auto-sync debug (hacker mode) -->
      <div
        v-if="autoSyncInfo"
        data-test="autosync-debug"
        class="px-3 pb-2 font-mono text-[11px] leading-[1.7] text-[var(--success)]"
      >
        <div>{{ $t('player.aePlayer.subs.autoSyncDebug.state', {
          status: autoSyncInfo.status,
          offset: autoSyncInfo.offset.toFixed(1),
          conf: Math.round(autoSyncInfo.confidence * 100),
        }) }}</div>
        <div v-for="(ev, i) in autoSyncInfo.events" :key="i">
          {{ $t('player.aePlayer.subs.autoSyncDebug.event', {
            delta: (ev.delta >= 0 ? '+' : '') + ev.delta.toFixed(1),
            from: fmtResume(ev.windowStart),
            to: fmtResume(ev.windowEnd),
            conf: Math.round(ev.confidence * 100),
          }) }}
        </div>
      </div>

      <!-- Timing offset -->
      <div class="flex items-center gap-3 px-3 py-2">
        <label class="w-[72px] flex-shrink-0 text-[13px] text-[var(--ink-2)]">{{ $t('player.aePlayer.subs.timing') }}</label>
        <div class="ml-auto flex flex-col items-end gap-1">
          <Stepper
            :model-value="subOffset"
            :step="0.1"
            suffix="s"
            :label="$t('player.aePlayer.subs.timing')"
            @update:model-value="emit('update:subOffset', $event)"
          />
          <span class="text-[11px] text-[var(--muted-foreground)]">
            {{ offsetHint }}
            <button
              v-if="subOffset !== 0"
              class="ml-1 cursor-pointer border-0 bg-transparent p-0 text-[11px] text-[var(--brand-cyan)] underline"
              @click="emit('update:subOffset', 0)"
            >{{ $t('player.aePlayer.subs.reset') }}</button>
          </span>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { List, ChevronRight, ChevronLeft, Settings2 } from 'lucide-vue-next'
import Stepper from '@/components/ui/Stepper.vue'
import Switch from '@/components/ui/Switch.vue'
import { fmtResume } from '@/composables/aePlayer/episodeProgress'
import type { SyncEvent } from '@/composables/aePlayer/subtitleAlign'

const { t: $t } = useI18n()

const props = defineProps<{
  subLang: string
  /** Real distinct languages that have a loaded track. */
  availableSubLangs: string[]
  /** lang → best track's source label (fansub group / official), shown as row meta. */
  langSources: Record<string, string>
  /** Total track count across all languages, for the Browse footer. */
  browseCount?: number
  /** Shown only when there are NO soft tracks but the stream has burned-in subs. */
  hardsubNote?: string | null
  subSize: number
  subBg: number
  subOffset: number
  autoSync: boolean
  autoSyncInfo?: { status: string; offset: number; confidence: number; events: SyncEvent[] } | null
}>()

const emit = defineEmits<{
  (e: 'pick-lang', value: string): void
  (e: 'update:subSize', value: number): void
  (e: 'update:subBg', value: number): void
  (e: 'update:subOffset', value: number): void
  (e: 'open-browse'): void
  (e: 'update:autoSync', value: boolean): void
}>()

const face = ref<'caps' | 'style'>('caps')

const FAST_LANGS = ['ru', 'en', 'ja'] as const
// Native autonyms — locale-independent, clearer than RU/EN/JP codes.
const AUTONYMS: Record<string, string> = { ru: 'Русский', en: 'English', ja: '日本語' }

function rowClass(active: boolean): string[] {
  return [
    'flex w-full items-center gap-3 px-3 py-2 text-left text-[14px] transition-colors',
    active ? 'bg-[var(--accent-soft)]' : 'hover:bg-white/[0.06]',
  ]
}

function radioClass(active: boolean): string[] {
  return [
    'relative size-4 flex-shrink-0 rounded-full border',
    active
      ? 'border-[var(--brand-cyan)] after:absolute after:inset-[3px] after:rounded-full after:bg-[var(--brand-cyan)]'
      : 'border-[var(--ink-4)]',
  ]
}

function onPick(lang: string) {
  if (!props.availableSubLangs.includes(lang)) return
  emit('pick-lang', lang)
}

const browseLabel = computed(() =>
  props.browseCount != null
    ? $t('player.aePlayer.subs.browseAll', { count: props.browseCount })
    : $t('player.aePlayer.subs.browseAllNoCount'),
)

const offsetHint = computed(() => {
  const v = props.subOffset
  if (v === 0) return $t('player.aePlayer.subs.inSync')
  const seconds = Math.abs(v).toFixed(1)
  return v > 0
    ? $t('player.aePlayer.subs.offsetLater', { seconds })
    : $t('player.aePlayer.subs.offsetEarlier', { seconds })
})

// Live preview: scale font size and background opacity from the sliders.
// Dynamic object binding (not a literal) — DS-lint inline-color rule does not apply.
const previewStyle = computed(() => ({
  fontSize: `${(18 * props.subSize) / 100}px`,
  background: `rgba(0, 0, 0, ${((props.subBg / 100) * 0.85).toFixed(2)})`,
}))
</script>
