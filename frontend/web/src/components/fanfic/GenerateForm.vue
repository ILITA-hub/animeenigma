<!--
  GenerateForm — structured input for POST /api/fanfic/generate.

  Anime search (debounced animeApi.search) -> on pick, fetch characters via
  the shared useCharacters() composable -> chip multi-select (<=6) ->
  curated + custom tag chips (<=8) -> length/POV/rating/language Selects
  (Explicit rating gated behind a useConfirm() dialog) -> prompt textarea.

  Emits `generate` with a fully-built GenerateInput; the parent
  (FanficsView.vue) owns the actual SSE call.
-->
<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useDebounceFn } from '@vueuse/core'
import { Search, X, Plus, Sparkles } from 'lucide-vue-next'
import { animeApi } from '@/api/client'
import { fanficApi } from '@/api/fanfic'
import { useCharacters } from '@/composables/useCharacters'
import { useConfirm } from '@/composables/useConfirm'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import { Input, Select, Chip, Button, Switch } from '@/components/ui'
import type { CharacterCardModel } from '@/types/character'
import type {
  GenerateInput,
  FanficAnimeRef,
  FanficCharacterRef,
  FanficTag,
  FanficLength,
  FanficPOV,
  FanficRating,
  FanficLang,
} from '@/types/fanfic'

const MAX_CHARACTERS = 6
const MAX_TAGS = 8
const MAX_PROMPT = 2000

const props = withDefaults(defineProps<{ disabled?: boolean }>(), { disabled: false })
const emit = defineEmits<{ generate: [input: GenerateInput] }>()

const { t, locale } = useI18n()
const { confirm } = useConfirm()

// ── Anime search ─────────────────────────────────────────────────────────────

interface RawAnimeSearchItem {
  id: string
  shikimori_id?: string
  name: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
}

const animeQuery = ref('')
const animeResults = ref<RawAnimeSearchItem[]>([])
const searchingAnime = ref(false)
const selectedAnime = ref<FanficAnimeRef | null>(null)

let animeAbort: AbortController | null = null

const runAnimeSearch = useDebounceFn(async (q: string) => {
  if (q.trim().length < 2) {
    animeResults.value = []
    return
  }
  animeAbort?.abort()
  animeAbort = new AbortController()
  searchingAnime.value = true
  try {
    const res = await animeApi.search(q, undefined, 8, animeAbort.signal)
    const data = (res.data?.data ?? res.data) as RawAnimeSearchItem[]
    animeResults.value = Array.isArray(data) ? data : []
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') return
    animeResults.value = []
  } finally {
    searchingAnime.value = false
  }
}, 300)

watch(animeQuery, (q) => {
  void runAnimeSearch(q)
})

function selectAnime(item: RawAnimeSearchItem): void {
  selectedAnime.value = {
    id: item.id,
    shikimori_id: item.shikimori_id,
    title: getLocalizedTitle(item.name, item.name_ru, item.name_jp),
    japanese: item.name_jp || undefined,
    poster: item.poster_url,
  }
  animeQuery.value = ''
  animeResults.value = []
  selectedCharacters.value = []
  if (item.id) void loadCharacters(item.id)
}

function clearAnime(): void {
  selectedAnime.value = null
  selectedCharacters.value = []
  characterCandidates.value = []
}

// ── Characters ───────────────────────────────────────────────────────────────

const { characters: characterCandidates, fetchCharacters } = useCharacters()
const selectedCharacters = ref<FanficCharacterRef[]>([])
const loadingCharacters = ref(false)

async function loadCharacters(animeId: string): Promise<void> {
  loadingCharacters.value = true
  try {
    await fetchCharacters(animeId)
  } finally {
    loadingCharacters.value = false
  }
}

function isCharacterSelected(id: string): boolean {
  return selectedCharacters.value.some((c) => c.id === id)
}

function toggleCharacter(c: CharacterCardModel): void {
  if (isCharacterSelected(c.id)) {
    selectedCharacters.value = selectedCharacters.value.filter((x) => x.id !== c.id)
    return
  }
  if (selectedCharacters.value.length >= MAX_CHARACTERS) return
  selectedCharacters.value = [...selectedCharacters.value, { id: c.id, name: c.name }]
}

// ── Tags ─────────────────────────────────────────────────────────────────────

const tagCandidates = ref<FanficTag[]>([])
const selectedTags = ref<string[]>([])
const customTagInput = ref('')

onMounted(async () => {
  try {
    tagCandidates.value = await fanficApi.tags()
  } catch {
    tagCandidates.value = []
  }
})

function tagLabel(tag: FanficTag): string {
  return locale.value === 'en' ? tag.en : tag.ru
}

function isTagSelected(value: string): boolean {
  return selectedTags.value.includes(value)
}

function toggleCuratedTag(tag: FanficTag): void {
  if (isTagSelected(tag.slug)) {
    selectedTags.value = selectedTags.value.filter((s) => s !== tag.slug)
    return
  }
  if (selectedTags.value.length >= MAX_TAGS) return
  selectedTags.value = [...selectedTags.value, tag.slug]
}

function addCustomTag(): void {
  const value = customTagInput.value.trim()
  if (!value || selectedTags.value.length >= MAX_TAGS || selectedTags.value.includes(value)) {
    customTagInput.value = ''
    return
  }
  selectedTags.value = [...selectedTags.value, value]
  customTagInput.value = ''
}

function removeTag(value: string): void {
  selectedTags.value = selectedTags.value.filter((s) => s !== value)
}

function tagChipLabel(value: string): string {
  const curated = tagCandidates.value.find((tag) => tag.slug === value)
  return curated ? tagLabel(curated) : value
}

// ── Length / POV / rating / language ────────────────────────────────────────

const length = ref<FanficLength>('oneshot')
const pov = ref<FanficPOV>('third')
const rating = ref<FanficRating>('teen')
const language = ref<FanficLang>('ru')

const lengthOptions = computed(() => [
  { value: 'drabble', label: t('fanfic.length.drabble') },
  { value: 'oneshot', label: t('fanfic.length.oneshot') },
  { value: 'short', label: t('fanfic.length.short') },
])
const povOptions = computed(() => [
  { value: 'first', label: t('fanfic.pov.first') },
  { value: 'third', label: t('fanfic.pov.third') },
])
const ratingOptions = computed(() => [
  { value: 'teen', label: t('fanfic.rating.teen') },
  { value: 'mature', label: t('fanfic.rating.mature') },
  { value: 'explicit', label: t('fanfic.rating.explicit') },
])
const languageOptions = computed(() => [
  { value: 'ru', label: t('fanfic.lang.ru') },
  { value: 'en', label: t('fanfic.lang.en') },
])

function onLengthChange(value: string | number): void {
  length.value = String(value) as FanficLength
}
function onPovChange(value: string | number): void {
  pov.value = String(value) as FanficPOV
}
function onLanguageChange(value: string | number): void {
  language.value = String(value) as FanficLang
}

/**
 * Explicit is gated behind an 18+ confirm; any other pick applies immediately.
 *
 * The confirm() dialog MUST NOT open synchronously inside this reka `Select`
 * `update:model-value` handler. The Select is still closing when this fires, and
 * on close reka returns focus to its trigger. Our ConfirmDialog is a NON-modal
 * reka Dialog (Modal.vue, `modal=false`) that dismisses on any outside
 * interaction — and reka treats that focus-return as a focus-outside, so a
 * synchronously-opened dialog auto-dismisses before the user can answer and
 * confirm() resolves false (→ "18+ is not settable"). Yielding one macrotask
 * lets the Select fully close and settle focus first; the dialog then opens
 * cleanly and traps focus onto itself.
 */
async function onRatingChange(value: string | number): Promise<void> {
  const next = String(value) as FanficRating
  if (next === 'explicit' && rating.value !== 'explicit') {
    // Defer past the closing Select's focus-return (see note above).
    await new Promise((resolve) => {
      setTimeout(resolve, 0)
    })
    const ok = await confirm({
      title: t('fanfic.rating.explicit'),
      description: t('fanfic.rating.explicitConfirm'),
      confirmText: t('common.confirm'),
      cancelText: t('common.cancel'),
      variant: 'destructive',
    })
    if (!ok) return
  }
  rating.value = next
}

// ── Prompt + submit ──────────────────────────────────────────────────────────

const prompt = ref('')
const canon = ref(false)
const spotlightCredit = ref(false)

// Rune-ish count (spread iterates by code point) to track the backend's
// utf8.RuneCountInString(r.Prompt) > 2000 cap (services/fanfic/internal/
// domain/request.go) closely enough — plain JS `.length`/`maxlength` count
// UTF-16 code units, which over-count for astral-plane characters (emoji,
// some CJK). `maxlength="2000"` on the textarea below is a coarse extra
// guard; this is the authoritative one.
const promptLength = computed(() => [...prompt.value].length)
const promptOverLimit = computed(() => promptLength.value > MAX_PROMPT)

const canGenerate = computed(
  () =>
    !!selectedAnime.value &&
    (canon.value || prompt.value.trim().length > 0) &&
    !promptOverLimit.value &&
    !props.disabled,
)

function buildInput(): GenerateInput {
  return {
    anime: selectedAnime.value as FanficAnimeRef,
    characters: selectedCharacters.value,
    tags: selectedTags.value,
    length: length.value,
    pov: pov.value,
    rating: rating.value,
    language: language.value,
    prompt: prompt.value.trim(),
    canon: canon.value,
    spotlight_credit: spotlightCredit.value,
  }
}

function onSubmit(): void {
  if (!canGenerate.value) return
  emit('generate', buildInput())
}

defineExpose({
  selectedAnime,
  selectAnime,
  clearAnime,
  characterCandidates,
  selectedCharacters,
  toggleCharacter,
  MAX_CHARACTERS,
  tagCandidates,
  selectedTags,
  toggleCuratedTag,
  addCustomTag,
  removeTag,
  customTagInput,
  MAX_TAGS,
  length,
  pov,
  rating,
  language,
  onLengthChange,
  onPovChange,
  onRatingChange,
  onLanguageChange,
  prompt,
  canon,
  spotlightCredit,
  MAX_PROMPT,
  promptLength,
  promptOverLimit,
  canGenerate,
  buildInput,
  onSubmit,
})
</script>

<template>
  <div class="space-y-5">
    <!-- Anime picker -->
    <div class="relative">
      <label class="block text-sm font-medium text-white/70 mb-2">{{ t('fanfic.form.anime') }}</label>
      <div v-if="!selectedAnime" class="relative">
        <Input v-model="animeQuery" type="search" :placeholder="t('fanfic.form.animePlaceholder')" clearable>
          <template #prefix><Search class="size-4" aria-hidden="true" /></template>
        </Input>
        <div
          v-if="animeResults.length > 0"
          class="absolute top-full left-0 right-0 mt-1 z-20 max-h-72 overflow-y-auto rounded-xl border border-border bg-popover/95 backdrop-blur-xl shadow-xl"
        >
          <button
            v-for="item in animeResults"
            :key="item.id"
            type="button"
            class="w-full flex items-center gap-3 px-3 py-2 text-left hover:bg-white/10 transition-colors"
            @click="selectAnime(item)"
          >
            <img
              :src="getImageUrl(item.poster_url)"
              :alt="item.name"
              class="w-8 h-11 rounded object-cover flex-shrink-0"
            />
            <span class="text-sm text-foreground truncate">
              {{ getLocalizedTitle(item.name, item.name_ru, item.name_jp) }}
            </span>
          </button>
        </div>
      </div>
      <div v-else class="flex items-center gap-3 rounded-xl border border-border bg-card p-3">
        <img
          :src="getImageUrl(selectedAnime.poster)"
          :alt="selectedAnime.title"
          class="w-10 h-14 rounded object-cover flex-shrink-0"
        />
        <span class="flex-1 text-sm font-medium text-foreground truncate">{{ selectedAnime.title }}</span>
        <Button variant="ghost" size="sm" type="button" @click="clearAnime">
          <X class="size-4" aria-hidden="true" />
        </Button>
      </div>
    </div>

    <!-- Characters -->
    <div v-if="selectedAnime">
      <label class="block text-sm font-medium text-white/70 mb-2">
        {{ t('fanfic.form.characters') }} ({{ selectedCharacters.length }}/{{ MAX_CHARACTERS }})
      </label>
      <p v-if="loadingCharacters" class="text-sm text-muted-foreground">{{ t('common.loading') }}</p>
      <div v-else class="flex flex-wrap gap-2">
        <Chip
          v-for="c in characterCandidates"
          :key="c.id"
          :active="isCharacterSelected(c.id)"
          size="sm"
          @click="toggleCharacter(c)"
        >
          {{ c.name }}
        </Chip>
      </div>
    </div>

    <!-- Tags -->
    <div>
      <label class="block text-sm font-medium text-white/70 mb-2">
        {{ t('fanfic.form.tags') }} ({{ selectedTags.length }}/{{ MAX_TAGS }})
      </label>
      <div class="flex flex-wrap gap-2 mb-2">
        <Chip
          v-for="tag in tagCandidates"
          :key="tag.slug"
          :active="isTagSelected(tag.slug)"
          size="sm"
          @click="toggleCuratedTag(tag)"
        >
          {{ tagLabel(tag) }}
        </Chip>
      </div>
      <div v-if="selectedTags.length > 0" class="flex flex-wrap gap-2 mb-2">
        <Chip
          v-for="value in selectedTags"
          :key="value"
          removable
          size="sm"
          @remove="removeTag(value)"
        >
          {{ tagChipLabel(value) }}
        </Chip>
      </div>
      <div class="flex gap-2">
        <Input
          v-model="customTagInput"
          size="sm"
          class="flex-1"
          :placeholder="t('fanfic.form.addTag')"
          @keydown.enter.prevent="addCustomTag"
        />
        <Button variant="outline" size="sm" type="button" :aria-label="t('fanfic.form.addTag')" @click="addCustomTag">
          <Plus class="size-4" aria-hidden="true" />
        </Button>
      </div>
    </div>

    <!-- Length / POV / rating / language -->
    <div class="grid grid-cols-2 md:grid-cols-4 gap-4">
      <Select
        :label="t('fanfic.form.length')"
        :model-value="length"
        :options="lengthOptions"
        size="sm"
        @update:model-value="onLengthChange"
      />
      <Select
        :label="t('fanfic.form.pov')"
        :model-value="pov"
        :options="povOptions"
        size="sm"
        @update:model-value="onPovChange"
      />
      <Select
        :label="t('fanfic.form.rating')"
        :model-value="rating"
        :options="ratingOptions"
        size="sm"
        @update:model-value="onRatingChange"
      />
      <Select
        :label="t('fanfic.form.language')"
        :model-value="language"
        :options="languageOptions"
        size="sm"
        @update:model-value="onLanguageChange"
      />
    </div>

    <!-- Canon continuation -->
    <div class="flex items-center justify-between rounded-xl border border-border bg-card p-3">
      <div>
        <p class="text-sm font-medium text-white/80">{{ t('fanfic.canon.label') }}</p>
        <p class="text-xs text-muted-foreground">{{ t('fanfic.canon.hint') }}</p>
      </div>
      <Switch v-model="canon" :aria-label="t('fanfic.canon.label')" />
    </div>

    <!-- Spotlight credit opt-in -->
    <div class="flex items-center justify-between rounded-xl border border-border bg-card p-3">
      <div>
        <p class="text-sm font-medium text-white/80">{{ t('fanfic.spotlightCredit.label') }}</p>
        <p class="text-xs text-muted-foreground">{{ t('fanfic.spotlightCredit.desc') }}</p>
      </div>
      <Switch v-model="spotlightCredit" :aria-label="t('fanfic.spotlightCredit.label')" />
    </div>

    <!-- Prompt -->
    <div>
      <label class="block text-sm font-medium text-white/70 mb-2">
        {{ canon ? t('fanfic.canon.directionLabel') : t('fanfic.form.prompt') }}
      </label>
      <textarea
        v-model="prompt"
        rows="4"
        maxlength="2000"
        class="w-full rounded-lg bg-white/5 border border-border px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-cyan-500/50 resize-y"
        :placeholder="canon ? t('fanfic.canon.directionPlaceholder') : t('fanfic.form.promptPlaceholder')"
      ></textarea>
      <div
        class="mt-1 text-right text-xs"
        :class="promptOverLimit ? 'text-destructive' : 'text-muted-foreground'"
      >
        {{ promptLength }}/{{ MAX_PROMPT }}
      </div>
    </div>

    <div class="flex justify-end">
      <Button :disabled="!canGenerate" :loading="props.disabled" @click="onSubmit">
        <template #icon><Sparkles class="size-4" aria-hidden="true" /></template>
        {{ t('fanfic.form.generate') }}
      </Button>
    </div>
  </div>
</template>
