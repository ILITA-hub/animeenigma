<template>
  <Modal
    :model-value="modelValue"
    :title="$t('player.otherSubs.title')"
    size="lg"
    @update:model-value="(v) => emit('update:modelValue', v)"
    @close="emit('close')"
  >
    <div v-if="loading" class="flex items-center justify-center py-12">
      <div class="w-8 h-8 border-2 accent-border border-t-transparent rounded-full animate-spin" />
    </div>

    <div v-else-if="error" class="py-8 text-center text-red-400 text-sm">
      {{ error }}
    </div>

    <div v-else-if="languageGroups.length === 0" class="py-12 text-center text-white/60 text-sm">
      {{ $t('player.otherSubs.empty') }}
    </div>

    <div v-else class="space-y-6">
      <section
        v-for="group in languageGroups"
        :key="group.lang"
        class="space-y-2"
      >
        <header class="flex items-center gap-2 sticky top-0 bg-black/40 backdrop-blur-sm py-1">
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
                {{ track.format?.toUpperCase() || $t('player.otherSubs.unknownFormat') }}
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

      <p v-if="providersDown.length > 0" class="text-amber-400/80 text-xs text-center pt-2 border-t border-white/10">
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

const languageGroups = computed(() => {
  if (!data.value) return []
  const out = Object.entries(data.value.languages).map(([lang, tracks]) => ({
    lang,
    tracks: tracks ?? [],
  }))
  // Preferred order: user's UI locale first, then ja, en, ru, then alphabetical.
  const preferred = [locale.value, 'ja', 'en', 'ru']
  return out.sort((a, b) => {
    const ai = preferred.indexOf(a.lang)
    const bi = preferred.indexOf(b.lang)
    if (ai !== -1 && bi === -1) return -1
    if (bi !== -1 && ai === -1) return 1
    if (ai !== -1 && bi !== -1) return ai - bi
    return a.lang.localeCompare(b.lang)
  })
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
