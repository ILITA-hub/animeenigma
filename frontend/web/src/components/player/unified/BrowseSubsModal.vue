<template>
  <!-- Scrim -->
  <div
    class="absolute inset-0 z-20 flex items-center justify-center p-6"
    style="background: rgba(0,0,0,0.6); backdrop-filter: blur(6px);"
    @click.self="emit('close')"
  >
    <!-- Dialog -->
    <div
      class="flex flex-col rounded-[var(--r-xl)] overflow-hidden w-[540px] max-w-full"
      style="max-height: 100%; box-shadow: 0 30px 70px rgba(0,0,0,0.6); background: var(--elevated);"
      role="dialog"
      aria-modal="true"
      aria-labelledby="browse-subs-title"
    >
      <!-- Head -->
      <div class="flex items-start justify-between px-5 pb-[14px] pt-[18px] border-b border-[var(--border)]">
        <div>
          <h2
            id="browse-subs-title"
            class="font-semibold text-[18px] text-white m-0"
            style="font-family: var(--font-display);"
          >
            Browse Subtitles
          </h2>
          <p class="text-[13px] text-[var(--muted-foreground)] mt-[3px] mb-0">
            {{ tracks.length }} track{{ tracks.length === 1 ? '' : 's' }} available
          </p>
        </div>
        <button
          class="w-8 h-8 flex items-center justify-center rounded-lg text-white/50 hover:text-white hover:bg-white/10 transition-colors border-0 bg-transparent"
          aria-label="Close"
          @click="emit('close')"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="M3 3L13 13M13 3L3 13" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/>
          </svg>
        </button>
      </div>

      <!-- Filters -->
      <div class="flex flex-col gap-2 px-5 py-3 border-b border-[var(--border)]">
        <!-- Search -->
        <div class="relative flex items-center">
          <span class="absolute left-3 text-white/50 pointer-events-none">
            <svg width="14" height="14" viewBox="0 0 14 14" fill="none" aria-hidden="true">
              <circle cx="6" cy="6" r="4.5" stroke="currentColor" stroke-width="1.5"/>
              <path d="M9.5 9.5L12.5 12.5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>
          </span>
          <input
            v-model="q"
            data-test="search"
            type="text"
            placeholder="Search by label or provider…"
            class="w-full py-[10px] pl-9 pr-9 rounded-[var(--r-md)] text-[14px] text-white placeholder-white/35 transition-all border focus:outline-none"
            style="background: rgba(255,255,255,0.06); border-color: var(--border);"
            @focus="($event.target as HTMLInputElement).style.borderColor = 'var(--brand-cyan)'"
            @blur="($event.target as HTMLInputElement).style.borderColor = 'var(--border)'"
          />
          <button
            v-if="q"
            class="absolute right-2 w-6 h-6 grid place-items-center rounded-full border-0 text-white"
            style="background: rgba(255,255,255,0.1);"
            aria-label="Clear search"
            @click="q = ''"
          >
            <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
              <path d="M2 2L8 8M8 2L2 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/>
            </svg>
          </button>
        </div>

        <!-- Provider chips -->
        <div v-if="distinctProviders.length > 1" class="flex flex-wrap items-center gap-[7px]">
          <span class="text-[11px] uppercase tracking-[0.05em] text-[var(--muted-foreground)] mr-[2px]">Provider</span>
          <button
            v-for="prov in distinctProviders"
            :key="prov"
            :class="[
              'px-3 py-[5px] rounded-full text-[13px] border transition-all',
              activeProvider === prov
                ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
                : 'bg-white/[0.06] border-transparent text-[var(--ink-2)] hover:bg-white/[0.12] hover:text-white',
            ]"
            :style="activeProvider === prov ? 'background: rgba(0,212,255,0.18)' : ''"
            @click="activeProvider = activeProvider === prov ? null : prov"
          >
            {{ prov }}
          </button>
        </div>

        <!-- Language chips -->
        <div v-if="distinctLangs.length > 1" class="flex flex-wrap items-center gap-[7px]">
          <span class="text-[11px] uppercase tracking-[0.05em] text-[var(--muted-foreground)] mr-[2px]">Lang</span>
          <button
            v-for="lang in distinctLangs"
            :key="lang"
            :class="[
              'px-3 py-[5px] rounded-full text-[13px] border transition-all',
              activeLang === lang
                ? 'border-[var(--accent-line)] text-[var(--brand-cyan)]'
                : 'bg-white/[0.06] border-transparent text-[var(--ink-2)] hover:bg-white/[0.12] hover:text-white',
            ]"
            :style="activeLang === lang ? 'background: rgba(0,212,255,0.18)' : ''"
            @click="activeLang = activeLang === lang ? null : lang"
          >
            {{ lang.toUpperCase() }}
          </button>
        </div>
      </div>

      <!-- Body -->
      <div class="overflow-y-auto px-5 py-[14px] pb-[18px]" style="scrollbar-width: thin;">
        <!-- Empty state -->
        <div v-if="groupedTracks.length === 0" class="text-center text-[var(--muted-foreground)] py-10 text-[14px]">
          No subtitles match your search.
        </div>

        <!-- Groups -->
        <div
          v-for="group in groupedTracks"
          :key="group.lang"
          data-test="lang-group"
          class="mb-4"
        >
          <h3 class="text-[14px] font-semibold text-white mb-2 m-0">
            {{ group.lang.toUpperCase() }}
            <span class="text-[var(--muted-foreground)] font-normal ml-1">({{ group.tracks.length }})</span>
          </h3>
          <ul class="list-none m-0 p-0 flex flex-col gap-[7px]">
            <li
              v-for="track in group.tracks"
              :key="track.url"
              data-test="track"
              :class="[
                'flex items-center gap-3 px-3 py-[11px] rounded-[var(--r-md)] border transition-all',
                track.url === selectedUrl
                  ? 'border-[var(--accent-line)]'
                  : 'bg-white/[0.05] border-transparent',
              ]"
              :style="track.url === selectedUrl ? 'background: rgba(0,212,255,0.12)' : ''"
            >
              <!-- Provider badge -->
              <span
                class="flex-shrink-0 px-[9px] py-[3px] rounded-full text-[11px] font-semibold"
                :style="providerBadgeStyle(track.provider)"
              >
                {{ track.provider }}
              </span>

              <!-- Track info -->
              <div class="flex-1 min-w-0 flex flex-col gap-[2px]">
                <span class="text-[14px] text-white truncate">{{ track.label }}</span>
                <span class="text-[11px] text-[var(--muted-foreground)]">{{ track.format.toUpperCase() }}</span>
              </div>

              <!-- Select button -->
              <button
                data-test="select"
                :disabled="track.url === selectedUrl"
                :class="[
                  'flex-shrink-0 px-[14px] py-[7px] rounded-[var(--r-sm)] border-0 text-[13px] font-semibold transition-all',
                  track.url === selectedUrl
                    ? 'cursor-default text-[var(--brand-cyan)]'
                    : 'text-white hover:bg-white/20',
                ]"
                :style="track.url === selectedUrl
                  ? 'background: rgba(0,212,255,0.25)'
                  : 'background: rgba(255,255,255,0.1)'"
                @click="emit('select', track)"
              >
                {{ track.url === selectedUrl ? 'Selected' : 'Select' }}
              </button>
            </li>
          </ul>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

export interface SubTrack {
  url: string
  provider: string
  lang: string
  label: string
  format: string
}

const props = defineProps<{
  tracks: SubTrack[]
  selectedUrl: string | null
}>()

const emit = defineEmits<{
  (e: 'select', track: SubTrack): void
  (e: 'close'): void
}>()

const q = ref('')
const activeProvider = ref<string | null>(null)
const activeLang = ref<string | null>(null)

const distinctProviders = computed(() =>
  [...new Set(props.tracks.map((t) => t.provider))],
)

const distinctLangs = computed(() =>
  [...new Set(props.tracks.map((t) => t.lang))],
)

const filteredTracks = computed(() => {
  const query = q.value.toLowerCase()
  return props.tracks.filter((t) => {
    const matchesSearch =
      !query ||
      t.label.toLowerCase().includes(query) ||
      t.provider.toLowerCase().includes(query)
    const matchesProvider = !activeProvider.value || t.provider === activeProvider.value
    const matchesLang = !activeLang.value || t.lang === activeLang.value
    return matchesSearch && matchesProvider && matchesLang
  })
})

interface TrackGroup {
  lang: string
  tracks: SubTrack[]
}

const groupedTracks = computed<TrackGroup[]>(() => {
  const map = new Map<string, SubTrack[]>()
  for (const track of filteredTracks.value) {
    const existing = map.get(track.lang)
    if (existing) {
      existing.push(track)
    } else {
      map.set(track.lang, [track])
    }
  }
  return [...map.entries()].map(([lang, tracks]) => ({ lang, tracks }))
})

// Simple provider → hue mapping for badges
const PROVIDER_HUES: Record<string, string> = {
  Jimaku: 'background: rgba(0,212,255,0.25); color: var(--brand-cyan)',
  OpenSubtitles: 'background: rgba(255,255,255,0.12); color: var(--ink-2)',
}

function providerBadgeStyle(provider: string): string {
  return PROVIDER_HUES[provider] ?? 'background: rgba(255,255,255,0.10); color: var(--ink-2)'
}
</script>
