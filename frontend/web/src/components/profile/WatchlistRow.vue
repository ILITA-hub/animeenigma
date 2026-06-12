<template>
  <!-- Compact "My List" row. Replaces the old fixed 9-column <table> row:
       on md+ everything sits on one line (controls right-aligned, fixed widths
       so they column-align across rows); below md the controls wrap onto a
       second line indented under the title instead of forcing a horizontal
       page scroll. -->
  <div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-2 py-2.5 rounded-xl border-b border-white/5 transition-colors hover:bg-white/[0.03]">
    <span class="hidden md:block w-6 shrink-0 text-right text-xs text-white/40 tabular-nums">{{ index }}</span>

    <router-link :to="href" class="block w-11 shrink-0" tabindex="-1" aria-hidden="true">
      <PosterImage :src="entry.anime?.poster_url || ''" :alt="title" ratio="2/3" rounded="lg" :proxy-width="88" />
    </router-link>

    <div class="flex-1 min-w-0">
      <router-link
        :to="href"
        class="block text-sm font-medium text-white truncate hover:text-cyan-400 transition-colors"
        data-testid="row-title"
      >
        {{ title }}
      </router-link>
      <!-- Dates: read-only line under the title below lg (the editors live in the lg+ controls row) -->
      <p v-if="datesLine" class="lg:hidden mt-0.5 text-[11px] text-white/40 truncate">{{ datesLine }}</p>
    </div>

    <!-- Controls: same line on md+, wrapped second line (indented under the title) on mobile -->
    <div class="flex items-center gap-2 basis-full md:basis-auto pl-14 md:pl-0">
      <!-- Score -->
      <div class="w-10 shrink-0 flex justify-center">
        <template v-if="isOwn">
          <Input
            v-if="editingScore"
            type="number"
            size="sm"
            min="0" max="10"
            :model-value="String(entry.score || 0)"
            class="text-center text-cyan-400"
            data-testid="score-input"
            @blur="(e: Event) => { editingScore = false; emit('editScore', (e.target as HTMLInputElement).value) }"
            @keydown.enter="(e: KeyboardEvent) => (e.target as HTMLInputElement).blur()"
            @keydown.escape="editingScore = false"
          />
          <button
            v-else
            type="button"
            data-testid="score-button"
            :title="t('profile.table.score')"
            class="inline-flex items-center justify-center w-8 h-8 rounded-full transition-colors cursor-pointer"
            :class="entry.score && entry.score > 0 ? 'bg-cyan-500/20 text-cyan-400 font-semibold hover:bg-cyan-500/30' : 'text-white/30 hover:bg-white/10 hover:text-white/60'"
            @click="editingScore = true"
          >
            {{ entry.score && entry.score > 0 ? entry.score : '-' }}
          </button>
        </template>
        <span
          v-else-if="entry.score && entry.score > 0"
          class="inline-flex items-center justify-center w-8 h-8 rounded-full bg-cyan-500/20 text-cyan-400 font-semibold"
        >{{ entry.score }}</span>
        <span v-else class="text-white/30">-</span>
      </div>

      <!-- Progress -->
      <div class="flex items-center gap-1 shrink-0">
        <template v-if="isOwn">
          <button
            type="button"
            data-testid="ep-minus"
            class="w-6 h-6 rounded flex items-center justify-center bg-white/10 text-white/60 hover:bg-white/20 hover:text-white transition-colors disabled:opacity-40"
            :disabled="(entry.episodes || 0) <= 0"
            @click="emit('updateEpisodes', (entry.episodes || 0) - 1)"
          >-</button>
          <div class="w-12">
            <Input
              type="number"
              size="sm"
              :model-value="String(entry.episodes || 0)"
              min="0"
              :max="totalEpisodes || 9999"
              class="h-6 py-0 text-center text-xs bg-white/10"
              @blur="(e: Event) => emit('updateEpisodes', parseInt((e.target as HTMLInputElement).value) || 0)"
              @keydown.enter="(e: KeyboardEvent) => (e.target as HTMLInputElement).blur()"
            />
          </div>
          <span class="text-white/60 text-sm">/ {{ totalEpisodes || '?' }}</span>
          <button
            type="button"
            data-testid="ep-plus"
            class="w-6 h-6 rounded flex items-center justify-center bg-white/10 text-white/60 hover:bg-white/20 hover:text-white transition-colors disabled:opacity-40"
            :disabled="totalEpisodes ? (entry.episodes || 0) >= totalEpisodes : false"
            @click="emit('updateEpisodes', (entry.episodes || 0) + 1)"
          >+</button>
        </template>
        <span v-else class="text-sm whitespace-nowrap">
          <span class="text-white">{{ entry.episodes || 0 }}</span>
          <span class="text-white/60"> / {{ totalEpisodes || '?' }}</span>
        </span>
      </div>

      <!-- Rewatch tally — muted ↻ N; stepper on own profile, ghost badge
           (hidden at 0) on public ones. Design 2026-06-05. -->
      <div class="shrink-0">
        <RewatchCounter
          :count="entry.rewatch_count || 0"
          :editable="isOwn"
          @update:count="(n: number) => emit('updateRewatchCount', n)"
        />
      </div>

      <!-- Dates: editable inputs (own) / text (public), lg+ only -->
      <div class="hidden lg:flex items-center gap-1.5 shrink-0">
        <template v-if="isOwn">
          <Input
            type="date"
            size="sm"
            :title="t('profile.table.startDate')"
            :model-value="formatDateForInput(entry.started_at)"
            class="text-xs py-1 w-32"
            @change="(e: Event) => emit('updateDate', 'started_at', (e.target as HTMLInputElement).value)"
          />
          <Input
            type="date"
            size="sm"
            :title="t('profile.table.endDate')"
            :model-value="formatDateForInput(entry.completed_at)"
            class="text-xs py-1 w-32"
            @change="(e: Event) => emit('updateDate', 'completed_at', (e.target as HTMLInputElement).value)"
          />
        </template>
        <span v-else class="text-xs text-white/60 whitespace-nowrap">{{ datesLine || '-' }}</span>
      </div>

      <!-- Status -->
      <div v-if="isOwn" class="w-28 shrink-0">
        <Select
          :model-value="entry.status"
          :options="statusOptions"
          size="xs"
          @change="(val: string | number) => emit('updateStatus', String(val))"
        />
      </div>
      <Badge v-else :variant="statusVariant" size="sm" class="shrink-0">{{ statusLabel }}</Badge>

      <!-- Remove -->
      <button
        v-if="isOwn"
        type="button"
        data-testid="row-remove"
        class="p-1.5 rounded hover:bg-destructive/20 text-white/30 hover:text-destructive transition-colors shrink-0"
        :title="t('profile.actions.removeFromList')"
        @click="emit('remove')"
      >
        <Trash2 class="size-4" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import { Badge, Input, Select, type SelectOption } from '@/components/ui'
import PosterImage from '@/components/anime/PosterImage.vue'
import RewatchCounter from '@/components/anime/RewatchCounter.vue'
import { getLocalizedTitle } from '@/utils/title'

interface WatchlistRowEntry {
  anime_id: string
  anime?: {
    name?: string
    name_ru?: string
    name_jp?: string
    poster_url?: string
    episodes_count?: number
  }
  status: string
  score?: number
  episodes?: number
  rewatch_count?: number
  started_at?: string | null
  completed_at?: string | null
}

const props = defineProps<{
  entry: WatchlistRowEntry
  /** Absolute 1-based position (pagination offset applied by the parent). */
  index: number
  isOwn: boolean
  statusOptions: SelectOption[]
}>()

const emit = defineEmits<{
  editScore: [value: string]
  updateEpisodes: [episodes: number]
  updateRewatchCount: [count: number]
  updateDate: [field: 'started_at' | 'completed_at', value: string]
  updateStatus: [status: string]
  remove: []
}>()

const { t, locale } = useI18n()

const editingScore = ref(false)

const href = computed(() => `/anime/${props.entry.anime_id}`)

const title = computed(() => {
  // Track locale so the title re-resolves when the UI language flips.
  void locale.value
  return getLocalizedTitle(props.entry.anime?.name, props.entry.anime?.name_ru, props.entry.anime?.name_jp) || 'Anime'
})

const totalEpisodes = computed(() => props.entry.anime?.episodes_count || 0)

const localeMap: Record<string, string> = { ru: 'ru-RU', en: 'en-US', ja: 'ja-JP' }

function formatDateForInput(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  try {
    return new Date(dateStr).toISOString().split('T')[0]
  } catch {
    return ''
  }
}

function formatDateDisplay(dateStr: string | null | undefined): string {
  if (!dateStr) return ''
  try {
    return new Date(dateStr).toLocaleDateString(localeMap[locale.value] || 'en-US', { day: '2-digit', month: '2-digit', year: 'numeric' })
  } catch {
    return ''
  }
}

const datesLine = computed(() => {
  const start = formatDateDisplay(props.entry.started_at)
  const end = formatDateDisplay(props.entry.completed_at)
  if (start && end) return `${start} – ${end}`
  return start || end
})

const statusVariant = computed<'primary' | 'success' | 'warning' | 'destructive' | 'info' | 'default'>(() => {
  switch (props.entry.status) {
    case 'watching': return 'primary'
    case 'completed': return 'success'
    case 'on_hold': return 'warning'
    case 'dropped': return 'destructive'
    default: return 'default'
  }
})

const statusLabel = computed(() => {
  const map: Record<string, string> = {
    watching: 'profile.watchlist.watching',
    plan_to_watch: 'profile.watchlist.planToWatch',
    completed: 'profile.watchlist.completed',
    on_hold: 'profile.watchlist.onHold',
    dropped: 'profile.watchlist.dropped',
  }
  const key = map[props.entry.status]
  return key ? t(key) : props.entry.status
})
</script>
