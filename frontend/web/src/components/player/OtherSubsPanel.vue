<template>
  <Modal
    :model-value="modelValue"
    :title="$t('player.otherSubs.title')"
    size="full"
    @update:model-value="(v) => emit('update:modelValue', v)"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex items-center justify-center py-12">
      <Spinner size="lg" />
    </div>

    <div v-else-if="error" class="py-8 text-center text-destructive text-sm">
      {{ error }}
    </div>

    <div v-else class="space-y-4">
      <!-- Filter bar: provider + language. Filters the already-fetched
           .all() response client-side; no extra requests, no extra quota. -->
      <div class="space-y-3 sticky top-0 z-20 bg-black/60 backdrop-blur-sm py-2 -mx-1 px-1 rounded-lg">
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-white/50 text-xs uppercase tracking-wide mr-1">{{ $t('player.otherSubs.filter.provider') }}</span>
          <button
            v-for="p in providerOptions"
            :key="p"
            type="button"
            :data-provider="p"
            :aria-pressed="providerFilter === p"
            class="px-3 py-1 rounded-full text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/60"
            :class="providerFilter === p ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="providerFilter = p"
          >
            {{ p === 'all' ? $t('player.otherSubs.filter.all') : providerLabel(p) }}
          </button>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <span class="text-white/50 text-xs uppercase tracking-wide mr-1">{{ $t('player.otherSubs.filter.language') }}</span>
          <button
            type="button"
            :data-lang="'all'"
            :aria-pressed="langFilter === 'all'"
            class="px-3 py-1 rounded-full text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/60"
            :class="langFilter === 'all' ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="langFilter = 'all'"
          >
            {{ $t('player.otherSubs.filter.all') }}
          </button>
          <button
            v-for="l in languageOptions"
            :key="l.lang"
            type="button"
            :data-lang="l.lang"
            :aria-pressed="langFilter === l.lang"
            class="px-3 py-1 rounded-full text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-cyan-400/60"
            :class="langFilter === l.lang ? 'bg-cyan-500/30 text-cyan-100 ring-1 ring-cyan-400/40' : 'bg-white/5 text-white/70 hover:bg-white/10'"
            @click="langFilter = l.lang"
          >
            {{ languageHeader(l.lang) }} ({{ l.count }})
          </button>
        </div>
      </div>

      <p v-if="filteredGroups.length === 0" class="py-12 text-center text-white/60 text-sm">
        {{ $t('player.otherSubs.empty') }}
      </p>

      <section
        v-for="group in filteredGroups"
        :key="group.lang"
        class="space-y-2"
      >
        <header class="flex items-center gap-2 py-1">
          <h3 class="text-white font-semibold">
            {{ languageHeader(group.lang) }} ({{ group.tracks.length }})
          </h3>
        </header>

        <ul class="space-y-1.5">
          <li
            v-for="track in group.tracks"
            :key="track.url"
            class="flex items-center gap-3 rounded-lg p-3 transition-colors"
            :class="isSelected(track)
              ? 'bg-cyan-500/15 ring-1 ring-cyan-400/40'
              : 'bg-white/5 hover:bg-white/10'"
          >
            <Badge :variant="providerVariant(track.provider)" size="sm" class="shrink-0">
              {{ providerLabel(track.provider) }}
            </Badge>

            <div class="flex-1 min-w-0">
              <div class="text-white text-sm truncate" :title="track.label || track.release">
                {{ track.label || track.release || track.url }}
              </div>
              <div class="text-white/40 text-xs">
                {{ track.format?.toUpperCase() || $t('player.otherSubs.genericFormat') }}
              </div>
            </div>

            <button
              type="button"
              class="px-3 py-1.5 rounded-md text-sm font-medium shrink-0"
              :class="isSelected(track)
                ? 'bg-cyan-500/30 text-cyan-100 cursor-default'
                : 'bg-white/10 text-white hover:bg-white/20'"
              :disabled="isSelected(track)"
              @click="select(track)"
            >
              {{ isSelected(track) ? $t('player.otherSubs.selected') : $t('player.otherSubs.select') }}
            </button>
          </li>
        </ul>
      </section>

      <p v-if="providersDown.length > 0" class="text-warning/80 text-xs text-center pt-2 border-t border-white/10">
        {{ $t('player.otherSubs.providersDown', { providers: providersDown.join(', ') }) }}
      </p>
    </div>
  </Modal>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Modal from '@/components/ui/Modal.vue'
import Badge from '@/components/ui/Badge.vue'
import { Spinner } from '@/components/ui'
import { subtitlesApi } from '@/api/client'
import type { GroupedSubs, SubtitleTrack } from '@/types/raw'

const props = defineProps<{
  modelValue: boolean
  animeId: string
  episode: number
  currentTrackUrl: string | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  close: []
  'select-track': [track: SubtitleTrack]
}>()

const { t, locale } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<GroupedSubs | null>(null)

const providerFilter = ref<string>('all')
const langFilter = ref<string>('all')

// Preferred-order sort for language codes.
const orderLangs = (langs: string[]): string[] => {
  const preferred = [locale.value, 'ja', 'en', 'ru']
  return [...langs].sort((a, b) => {
    const ai = preferred.indexOf(a)
    const bi = preferred.indexOf(b)
    if (ai !== -1 && bi === -1) return -1
    if (bi !== -1 && ai === -1) return 1
    if (ai !== -1 && bi !== -1) return ai - bi
    return a.localeCompare(b)
  })
}

const languageGroups = computed(() => {
  if (!data.value) return []
  return orderLangs(Object.keys(data.value.languages)).map((lang) => ({
    lang,
    tracks: data.value!.languages[lang] ?? [],
  }))
})

// Provider chips: 'all' + whichever providers actually returned tracks.
const providerOptions = computed<string[]>(() => {
  if (!data.value) return ['all']
  const set = new Set<string>()
  for (const tracks of Object.values(data.value.languages)) {
    for (const t of tracks ?? []) set.add(t.provider)
  }
  return ['all', ...[...set].sort()]
})

// Language chips with counts, honoring the active provider filter.
const languageOptions = computed<{ lang: string; count: number }[]>(() => {
  if (!data.value) return []
  const counts: Record<string, number> = {}
  for (const [lang, tracks] of Object.entries(data.value.languages)) {
    for (const t of tracks ?? []) {
      if (providerFilter.value !== 'all' && t.provider !== providerFilter.value) continue
      counts[lang] = (counts[lang] ?? 0) + 1
    }
  }
  return orderLangs(Object.keys(counts)).map((lang) => ({ lang, count: counts[lang] }))
})

// Groups after BOTH filters are applied.
const filteredGroups = computed(() => {
  return languageGroups.value
    .map((g) => ({
      lang: g.lang,
      tracks: g.tracks.filter(
        (t) => providerFilter.value === 'all' || t.provider === providerFilter.value,
      ),
    }))
    .filter((g) => g.tracks.length > 0)
    .filter((g) => langFilter.value === 'all' || g.lang === langFilter.value)
})

// Reset the language pin only when the new provider no longer offers it —
// preserves a still-valid selection across provider switches.
watch(providerFilter, () => {
  if (langFilter.value !== 'all' && !languageOptions.value.some((l) => l.lang === langFilter.value)) {
    langFilter.value = 'all'
  }
})

const providersDown = computed(() => data.value?.providers_down ?? [])

const isSelected = (track: SubtitleTrack) =>
  props.currentTrackUrl !== null && track.url === props.currentTrackUrl

const select = (track: SubtitleTrack) => {
  emit('select-track', track)
  emit('update:modelValue', false)
}

const providerLabel = (provider: string) => {
  const key = `player.otherSubs.providerChip.${provider}`
  const label = t(key)
  return label === key ? provider : label
}

const providerVariant = (provider: string): 'primary' | 'secondary' | 'default' => {
  switch (provider) {
    case 'jimaku':
      return 'primary'
    case 'opensubtitles':
      return 'secondary'
    default:
      return 'default'
  }
}

const languageHeader = (lang: string) => {
  const key = `player.otherSubs.lang.${lang}`
  const label = t(key)
  return label === key ? lang.toUpperCase() : label
}

const fetchAll = async () => {
  if (!props.animeId || !props.episode) return
  loading.value = true
  error.value = null
  try {
    const resp = await subtitlesApi.all(props.animeId, props.episode)
    data.value = resp.data?.data ?? resp.data
    providerFilter.value = 'all'
    langFilter.value = 'all'
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    error.value = t('player.otherSubs.loadError', { error: msg })
  } finally {
    loading.value = false
  }
}

// Fetch when the panel opens and we don't have data yet (or the episode changed).
watch(
  () => [props.modelValue, props.animeId, props.episode] as const,
  ([open]) => {
    if (open) {
      fetchAll()
    }
  },
  { immediate: true },
)
</script>
