<template>
  <div class="kodik-player">
    <!-- Loading state for translations -->
    <div v-if="loadingTranslations" class="flex items-center justify-center py-20">
      <div class="w-10 h-10 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin" />
    </div>

    <!-- No translations available -->
    <div v-else-if="translations.length === 0 && !loadingTranslations" class="text-center py-20 text-white/60">
      <svg class="w-12 h-12 mx-auto mb-3 opacity-50" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M15 10l4.553-2.276A1 1 0 0121 8.618v6.764a1 1 0 01-1.447.894L15 14M5 18h8a2 2 0 002-2V8a2 2 0 00-2-2H5a2 2 0 00-2 2v8a2 2 0 002 2z" />
      </svg>
      {{ $t('player.noTranslations') || 'Нет доступных озвучек' }}
    </div>

    <!-- Main content when translations available -->
    <div v-else class="flex flex-col lg:flex-row gap-4">
      <!-- Left: Video Player -->
      <div class="flex-1 min-w-0">
        <div class="relative aspect-video bg-black rounded-xl overflow-hidden">
          <!-- Loading overlay -->
          <div
            v-if="loadingVideo"
            class="absolute inset-0 z-10 flex items-center justify-center bg-black/80"
          >
            <div class="text-center">
              <div class="w-10 h-10 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mx-auto mb-3" />
              <p class="text-white/60 text-sm">Загрузка серии {{ selectedEpisode }}...</p>
            </div>
          </div>

          <!-- Iframe player -->
          <iframe
            v-if="embedUrl"
            ref="playerFrame"
            :src="embedUrl"
            class="absolute inset-0 w-full h-full"
            frameborder="0"
            allowfullscreen
            allow="autoplay; fullscreen; encrypted-media"
          />

          <!-- Placeholder when no video loaded yet -->
          <div
            v-else-if="!loadingVideo"
            class="absolute inset-0 flex items-center justify-center"
          >
            <div class="text-center text-white/40">
              <svg class="w-16 h-16 mx-auto mb-3" fill="currentColor" viewBox="0 0 24 24">
                <path d="M8 5v14l11-7z" />
              </svg>
              <p>Выберите озвучку для начала просмотра</p>
            </div>
          </div>
        </div>

        <!-- Episodes below player -->
        <div class="mt-4">
          <h3 class="text-white/60 text-sm mb-3 flex items-center gap-2">
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 10h16M4 14h16M4 18h16" />
            </svg>
            {{ $t('anime.episodes') || 'Серии' }} ({{ episodeRange.length }})
          </h3>
          <div class="flex flex-wrap gap-2 max-h-32 overflow-y-auto custom-scrollbar p-1">
            <button
              v-for="ep in episodeRange"
              :key="ep"
              @click="selectEpisode(ep)"
              class="w-12 h-10 rounded-lg text-sm font-medium transition-all"
              :class="selectedEpisode === ep
                ? 'bg-cyan-500 text-black'
                : 'bg-white/10 text-white hover:bg-white/20'"
            >
              {{ ep }}
            </button>
          </div>
        </div>
      </div>

      <!-- Right: Translations list -->
      <div class="lg:w-72 flex-shrink-0">
        <!-- Tab buttons -->
        <div class="flex gap-2 mb-3">
          <button
            @click="translationType = 'voice'"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="translationType === 'voice'
              ? 'bg-green-500/20 text-green-400 border border-green-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11a7 7 0 01-7 7m0 0a7 7 0 01-7-7m7 7v4m0 0H8m4 0h4m-4-8a3 3 0 01-3-3V5a3 3 0 116 0v6a3 3 0 01-3 3z" />
            </svg>
            Озвучка
            <span class="text-xs opacity-70">({{ voiceTranslations.length }})</span>
          </button>
          <button
            @click="translationType = 'subtitles'"
            class="flex-1 flex items-center justify-center gap-2 px-3 py-2 rounded-lg text-sm font-medium transition-all"
            :class="translationType === 'subtitles'
              ? 'bg-blue-500/20 text-blue-400 border border-blue-500/50'
              : 'bg-white/5 text-white/60 border border-transparent hover:bg-white/10'"
          >
            <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M7 8h10M7 12h4m1 8l-4-4H5a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v8a2 2 0 01-2 2h-3l-4 4z" />
            </svg>
            Субтитры
            <span class="text-xs opacity-70">({{ subtitleTranslations.length }})</span>
          </button>
        </div>

        <!-- Translations list -->
        <div class="space-y-2 max-h-[350px] lg:max-h-[450px] overflow-y-auto custom-scrollbar pr-1">
          <template v-if="filteredTranslations.length > 0">
            <div
              v-for="t in filteredTranslations"
              :key="t.id"
              class="relative group"
            >
              <button
                @click="selectTranslation(t.id)"
                class="w-full text-left p-3 rounded-lg transition-all"
                :class="[
                  selectedTranslation === t.id
                    ? (translationType === 'voice' ? 'bg-green-500/20 border border-green-500/50' : 'bg-blue-500/20 border border-blue-500/50')
                    : 'bg-white/5 border border-transparent hover:bg-white/10',
                  t.pinned ? 'ring-1 ring-amber-500/30' : ''
                ]"
              >
                <div class="flex items-center justify-between gap-2">
                  <div class="flex-1 min-w-0">
                    <div class="flex items-center gap-2">
                      <!-- Pinned badge -->
                      <span
                        v-if="t.pinned"
                        class="inline-flex items-center gap-1 text-xs px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-400"
                        title="Рекомендуемая озвучка"
                      >
                        <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20">
                          <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
                        </svg>
                      </span>
                      <p class="text-white font-medium truncate" :title="t.title">{{ t.title }}</p>
                    </div>
                    <span class="text-white/40 text-xs">{{ t.episodes_count }} эп.</span>
                  </div>
                  <div
                    v-if="selectedTranslation === t.id"
                    class="w-6 h-6 rounded-full flex items-center justify-center flex-shrink-0"
                    :class="translationType === 'voice' ? 'bg-green-500' : 'bg-blue-500'"
                  >
                    <svg class="w-4 h-4 text-black" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
              </button>

              <!-- Pin/Unpin button -->
              <button
                @click.stop="togglePin(t)"
                class="absolute top-2 right-2 p-1.5 rounded-lg transition-all opacity-0 group-hover:opacity-100"
                :class="t.pinned
                  ? 'bg-amber-500/20 text-amber-400 hover:bg-amber-500/30'
                  : 'bg-white/10 text-white/40 hover:bg-white/20 hover:text-white'"
                :title="t.pinned ? 'Открепить' : 'Закрепить'"
              >
                <svg class="w-4 h-4" :fill="t.pinned ? 'currentColor' : 'none'" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M11.049 2.927c.3-.921 1.603-.921 1.902 0l1.519 4.674a1 1 0 00.95.69h4.915c.969 0 1.371 1.24.588 1.81l-3.976 2.888a1 1 0 00-.363 1.118l1.518 4.674c.3.922-.755 1.688-1.538 1.118l-3.976-2.888a1 1 0 00-1.176 0l-3.976 2.888c-.783.57-1.838-.197-1.538-1.118l1.518-4.674a1 1 0 00-.363-1.118l-3.976-2.888c-.784-.57-.38-1.81.588-1.81h4.914a1 1 0 00.951-.69l1.519-4.674z" />
                </svg>
              </button>
            </div>
          </template>
          <div v-else class="text-center py-8 text-white/40">
            <p>{{ translationType === 'voice' ? 'Нет доступных озвучек' : 'Нет доступных субтитров' }}</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Error message -->
    <div v-if="error" class="mt-4 p-4 bg-pink-500/20 border border-pink-500/30 rounded-lg text-pink-400">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { kodikApi } from '@/api/client'

interface KodikTranslation {
  id: number
  title: string
  type: string
  episodes_count: number
  pinned?: boolean
}

interface PinnedTranslation {
  anime_id: string
  translation_id: number
  translation_title: string
  translation_type: string
}

const props = defineProps<{
  animeId: string
  totalEpisodes?: number
  initialEpisode?: number
}>()

const translations = ref<KodikTranslation[]>([])
const pinnedIds = ref<Set<number>>(new Set())
const selectedTranslation = ref<number | null>(null)
const selectedEpisode = ref(1)
const embedUrl = ref<string | null>(null)
const loadingTranslations = ref(false)
const loadingVideo = ref(false)
const error = ref<string | null>(null)
const isInitialized = ref(false)
const translationType = ref<'voice' | 'subtitles'>('voice')

// Filtered and sorted translations by type (pinned first)
const voiceTranslations = computed(() => {
  const voices = translations.value.filter(t => t.type === 'voice')
  return sortByPinned(voices)
})

const subtitleTranslations = computed(() => {
  const subs = translations.value.filter(t => t.type !== 'voice')
  return sortByPinned(subs)
})

const filteredTranslations = computed(() =>
  translationType.value === 'voice' ? voiceTranslations.value : subtitleTranslations.value
)

// Sort translations: pinned first, then by title
function sortByPinned(list: KodikTranslation[]): KodikTranslation[] {
  return [...list].sort((a, b) => {
    if (a.pinned && !b.pinned) return -1
    if (!a.pinned && b.pinned) return 1
    return a.title.localeCompare(b.title)
  })
}

const episodeRange = computed(() => {
  // Use episode count from selected translation if available
  const selectedTrans = translations.value.find(t => t.id === selectedTranslation.value)
  const count = selectedTrans?.episodes_count || props.totalEpisodes || 12
  return Array.from({ length: count }, (_, i) => i + 1)
})

const fetchPinnedTranslations = async () => {
  try {
    const response = await kodikApi.getPinnedTranslations(props.animeId)
    const data = response.data?.data || response.data || []
    pinnedIds.value = new Set(data.map((p: PinnedTranslation) => p.translation_id))
  } catch {
    // Ignore errors, pinned translations are optional
    pinnedIds.value = new Set()
  }
}

const fetchTranslations = async () => {
  loadingTranslations.value = true
  error.value = null
  isInitialized.value = false

  try {
    // Fetch pinned translations first
    await fetchPinnedTranslations()

    const response = await kodikApi.getTranslations(props.animeId)
    const data = response.data?.data || response.data
    const rawTranslations: KodikTranslation[] = Array.isArray(data) ? data : []

    // Mark pinned translations
    translations.value = rawTranslations.map(t => ({
      ...t,
      pinned: pinnedIds.value.has(t.id)
    }))

    if (translations.value.length > 0) {
      // Prefer pinned voice translations, then any voice, then subtitles
      const voices = translations.value.filter(t => t.type === 'voice')
      const subs = translations.value.filter(t => t.type !== 'voice')
      const pinnedVoice = voices.find(t => t.pinned)
      const pinnedSub = subs.find(t => t.pinned)

      if (pinnedVoice) {
        translationType.value = 'voice'
        selectedTranslation.value = pinnedVoice.id
      } else if (voices.length > 0) {
        translationType.value = 'voice'
        selectedTranslation.value = voices[0].id
      } else if (pinnedSub) {
        translationType.value = 'subtitles'
        selectedTranslation.value = pinnedSub.id
      } else if (subs.length > 0) {
        translationType.value = 'subtitles'
        selectedTranslation.value = subs[0].id
      }

      // Auto-load first video after setting translation
      isInitialized.value = true
      await loadVideo()
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить список озвучек'
    translations.value = []
  } finally {
    loadingTranslations.value = false
  }
}

const loadVideo = async () => {
  if (!selectedTranslation.value) return

  loadingVideo.value = true
  error.value = null

  try {
    const response = await kodikApi.getVideo(props.animeId, selectedEpisode.value, selectedTranslation.value)
    const data = response.data?.data || response.data
    if (data?.embed_link) {
      // Add parameters to hide Kodik's built-in controls
      let url = data.embed_link
      const separator = url.includes('?') ? '&' : '?'
      // Hide translation selector and episode list in Kodik player
      url += `${separator}hide_selectors=true&only_season=true`
      embedUrl.value = url
    } else {
      error.value = 'Видео не найдено'
    }
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось загрузить видео'
  } finally {
    loadingVideo.value = false
  }
}

const selectTranslation = (translationId: number) => {
  if (selectedTranslation.value === translationId) return

  selectedTranslation.value = translationId

  // Check if current episode exceeds available episodes for this translation
  const trans = translations.value.find(t => t.id === translationId)
  if (trans?.episodes_count && selectedEpisode.value > trans.episodes_count) {
    selectedEpisode.value = 1
  }

  loadVideo()
}

const selectEpisode = (episode: number) => {
  if (selectedEpisode.value === episode) return
  selectedEpisode.value = episode
  if (selectedTranslation.value) {
    loadVideo()
  }
}

const togglePin = async (translation: KodikTranslation) => {
  try {
    if (translation.pinned) {
      await kodikApi.unpinTranslation(props.animeId, translation.id)
      pinnedIds.value.delete(translation.id)
    } else {
      await kodikApi.pinTranslation(props.animeId, translation.id, translation.title, translation.type)
      pinnedIds.value.add(translation.id)
    }

    // Update the pinned status in the translations list
    translations.value = translations.value.map(t => ({
      ...t,
      pinned: pinnedIds.value.has(t.id)
    }))
  } catch (err: any) {
    error.value = err.response?.data?.message || 'Не удалось изменить закрепление'
  }
}

// Reset when anime changes
watch(() => props.animeId, () => {
  embedUrl.value = null
  selectedEpisode.value = 1
  fetchTranslations()
})

onMounted(() => {
  if (props.initialEpisode) {
    selectedEpisode.value = props.initialEpisode
  }
  fetchTranslations()
})
</script>

<style scoped>
.kodik-player {
  width: 100%;
}

.custom-scrollbar::-webkit-scrollbar {
  width: 4px;
}

.custom-scrollbar::-webkit-scrollbar-track {
  background: transparent;
}

.custom-scrollbar::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.2);
  border-radius: 2px;
}

.custom-scrollbar::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.3);
}
</style>
