<template>
  <!-- Compact "My List" row. Replaces the old fixed 9-column <table> row:
       on md+ everything sits on one line (controls right-aligned, fixed widths
       so they column-align across rows); below md the controls wrap onto a
       second line indented under the title instead of forcing a horizontal
       page scroll. -->
  <div class="flex flex-wrap items-center gap-x-3 gap-y-2 px-2 py-2.5 rounded-xl border-b border-white/5 transition-colors hover:bg-white/[0.03]">
    <label v-if="selectable" class="flex items-center pr-2 self-center cursor-pointer" @click.stop>
      <Checkbox :model-value="!!selected" @update:model-value="() => emit('toggleSelect')" />
    </label>

    <span class="hidden md:block w-6 shrink-0 text-right text-xs text-white/40 tabular-nums">{{ index }}</span>

    <router-link :to="href" class="block w-11 shrink-0" tabindex="-1" aria-hidden="true">
      <!-- proxy-width MUST match PosterCard's 384: the grid and this list show
           the same posters, and a different `w` bucket = a different URL =
           a full re-download on every grid↔list switch. -->
      <PosterImage :src="entry.anime?.poster_url || ''" :alt="title" ratio="2/3" rounded="lg" :proxy-width="384" />
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
      <!-- Score — ONE circle for own + public rows (own adds click-to-edit),
           so the two variants can't drift apart visually. -->
      <div class="w-10 shrink-0 flex justify-center">
        <Input
          v-if="isOwn && editingScore"
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
        <component
          :is="isOwn ? 'button' : 'span'"
          v-else
          :type="isOwn ? 'button' : undefined"
          data-testid="score-button"
          :title="isOwn ? t('profile.table.score') : undefined"
          class="inline-flex items-center justify-center w-8 h-8 rounded-full transition-colors"
          :class="[
            entry.score && entry.score > 0 ? 'bg-cyan-500/20 text-cyan-400 font-semibold' : 'text-white/30',
            isOwn ? 'cursor-pointer ' + (entry.score && entry.score > 0 ? 'hover:bg-cyan-500/30' : 'hover:bg-white/10 hover:text-white/60') : '',
          ]"
          @click="isOwn && (editingScore = true)"
        >
          {{ entry.score && entry.score > 0 ? entry.score : '-' }}
        </component>
      </div>

      <!-- Progress — DS Stepper primitive (own); read-only text (public) -->
      <div class="shrink-0">
        <Stepper
          v-if="isOwn"
          :model-value="entry.episodes || 0"
          :min="0"
          :max="totalEpisodes || 9999"
          :suffix="`/ ${totalEpisodes || '?'}`"
          :label="t('profile.table.progress')"
          input-width="32px"
          @update:model-value="(n: number) => emit('updateEpisodes', n)"
        />
        <span v-else class="text-sm whitespace-nowrap">
          <span class="text-white">{{ entry.episodes || 0 }}</span>
          <span class="text-white/60"> / {{ totalEpisodes || '?' }}</span>
        </span>
      </div>

      <!-- Rewatch tally — muted ↻ N, hidden at 0 (own AND public); stepper on
           own profile once count ≥ 1. The 0→1 bump lives in the anime-page
           status menu. Design 2026-06-05, tightened 2026-07-05. -->
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
          <DatePicker
            :model-value="formatDateForInput(entry.started_at)"
            :placeholder="t('profile.table.startDate')"
            :title="t('profile.table.startDate')"
            class="w-32"
            @update:model-value="(v: string) => emit('updateDate', 'started_at', v)"
          />
          <DatePicker
            :model-value="formatDateForInput(entry.completed_at)"
            :placeholder="t('profile.table.endDate')"
            :title="t('profile.table.endDate')"
            class="w-32"
            @update:model-value="(v: string) => emit('updateDate', 'completed_at', v)"
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

      <!-- Remove — DS Button, ghost/icon (border dropped: this is a quiet row action) -->
      <Button
        v-if="isOwn"
        variant="ghost"
        size="icon-sm"
        data-testid="row-remove"
        class="shrink-0 border-transparent bg-transparent text-white/30 hover:text-destructive hover:bg-destructive/20 hover:border-transparent"
        :title="t('profile.actions.removeFromList')"
        :aria-label="t('profile.actions.removeFromList')"
        @click="emit('remove')"
      >
        <Trash2 class="size-4" />
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Trash2 } from 'lucide-vue-next'
import { Badge, Button, DatePicker, Input, Select, Stepper, type SelectOption } from '@/components/ui'
import Checkbox from '@/components/ui/Checkbox.vue'
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
  selectable?: boolean
  selected?: boolean
}>()

const emit = defineEmits<{
  editScore: [value: string]
  updateEpisodes: [episodes: number]
  updateRewatchCount: [count: number]
  updateDate: [field: 'started_at' | 'completed_at', value: string]
  updateStatus: [status: string]
  remove: []
  toggleSelect: []
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
