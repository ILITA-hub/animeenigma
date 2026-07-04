<template>
  <!-- Phase 17 (UX-33) admin create/edit form. -->
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <!-- Header + actions -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <div>
          <h1 class="text-3xl font-semibold text-white">
            {{ isNew ? $t('admin.collections.createNew') : $t('admin.collections.editTitle') }}
          </h1>
          <p v-if="!isNew && form.slug" class="text-white/40 text-xs font-mono mt-1">
            /collections/{{ form.slug }}
          </p>
        </div>
        <div class="flex items-center gap-3">
          <a
            v-if="!isNew && form.published && form.slug"
            :href="`/collections/${form.slug}`"
            target="_blank"
            rel="noopener"
            class="px-3 py-2 rounded bg-white/10 hover:bg-white/20 text-white/80 text-sm"
          >{{ $t('admin.collections.preview') }}</a>
          <Button
            variant="default"
            size="sm"
            :disabled="isSaving || !form.title"
            @click="onSave"
          >
            {{ isSaving ? '…' : $t('admin.collections.save') }}
          </Button>
        </div>
      </div>

      <!-- Error -->
      <div v-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <template v-else>
        <!-- Main form -->
        <form class="glass-card p-6 space-y-4 mb-6" @submit.prevent="onSave">
          <!-- Slug -->
          <div>
            <label class="block text-white/70 text-xs uppercase mb-1">
              {{ $t('admin.collections.fieldSlug') }}
            </label>
            <Input v-model="form.slug" type="text" size="sm" class="bg-black/40" :placeholder="$t('admin.collections.slugPlaceholder')" />
          </div>

          <!-- Titles -->
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldTitleEn') }} *
              </label>
              <Input v-model="form.title" type="text" size="sm" class="bg-black/40" required />
            </div>
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldTitleRu') }}
              </label>
              <Input v-model="form.title_ru" type="text" size="sm" class="bg-black/40" />
            </div>
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldTitleJp') }}
              </label>
              <Input v-model="form.title_jp" type="text" size="sm" class="bg-black/40" />
            </div>
          </div>

          <!-- Cover URL + preview -->
          <div>
            <label class="block text-white/70 text-xs uppercase mb-1">
              {{ $t('admin.collections.fieldCover') }}
            </label>
            <Input v-model="form.cover_image_url" type="url" size="sm" class="bg-black/40" placeholder="https://…" />
            <div v-if="form.cover_image_url" class="mt-2">
              <img
                :src="form.cover_image_url"
                alt=""
                class="w-[100px] h-[140px] object-cover rounded border border-white/10"
                @error="onImgError"
              />
            </div>
          </div>

          <!-- Descriptions -->
          <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldDescriptionEn') }}
              </label>
              <textarea
                v-model="form.description"
                rows="4"
                class="w-full px-3 py-2 rounded bg-black/40 border border-white/10 text-white text-sm"
              ></textarea>
            </div>
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldDescriptionRu') }}
              </label>
              <textarea
                v-model="form.description_ru"
                rows="4"
                class="w-full px-3 py-2 rounded bg-black/40 border border-white/10 text-white text-sm"
              ></textarea>
            </div>
            <div>
              <label class="block text-white/70 text-xs uppercase mb-1">
                {{ $t('admin.collections.fieldDescriptionJp') }}
              </label>
              <textarea
                v-model="form.description_jp"
                rows="4"
                class="w-full px-3 py-2 rounded bg-black/40 border border-white/10 text-white text-sm"
              ></textarea>
            </div>
          </div>

          <!-- Published toggle -->
          <div class="flex items-center gap-3">
            <label class="inline-flex items-center cursor-pointer">
              <Checkbox v-model="form.published" class="mr-2" />
              <span class="text-white text-sm">{{ $t('admin.collections.fieldPublished') }}</span>
            </label>
          </div>

          <p v-if="savedAt" class="text-success text-xs">{{ $t('admin.collections.saved') }} ✓</p>
        </form>

        <!-- Items section (only after save) -->
        <div v-if="!isNew" class="glass-card p-6">
          <h2 class="text-xl font-semibold text-white mb-4">
            {{ $t('admin.collections.itemsSection') }}
            <span class="text-white/40 text-sm font-normal ml-2">({{ items.length }})</span>
          </h2>

          <!-- Picker -->
          <div class="mb-4">
            <Input v-model="searchQuery" type="text" size="sm" class="bg-black/40" :placeholder="$t('admin.collections.itemSearchPlaceholder')" @input="onSearch" />
            <div v-if="searchResults.length > 0" class="mt-2 max-h-60 overflow-y-auto rounded border border-white/10 bg-black/60">
              <button
                v-for="r in searchResults"
                :key="r.id"
                type="button"
                class="flex items-center gap-3 w-full px-3 py-2 text-left hover:bg-white/10 border-t border-white/5 first:border-t-0"
                @click="onAddItem(r)"
              >
                <img
                  v-if="r.poster_url"
                  :src="r.poster_url"
                  alt=""
                  class="w-8 h-11 object-cover rounded flex-shrink-0"
                />
                <span class="text-white text-sm truncate">{{ r.name_ru || r.name || r.id }}</span>
              </button>
            </div>
          </div>

          <!-- Existing items -->
          <ul v-if="items.length > 0" class="space-y-2">
            <li
              v-for="item in items"
              :key="item.id"
              class="flex items-center gap-3 p-2 rounded bg-black/30 border border-white/10"
            >
              <img
                v-if="item.anime?.poster_url"
                :src="item.anime.poster_url"
                alt=""
                class="w-12 h-16 object-cover rounded flex-shrink-0"
              />
              <div class="flex-1 min-w-0">
                <p class="text-white text-sm truncate">
                  {{ item.anime?.name_ru || item.anime?.name || item.anime_id }}
                </p>
              </div>
              <div class="flex items-center gap-2 flex-shrink-0">
                <label class="text-white/50 text-xs">{{ $t('admin.collections.itemSortOrder') }}</label>
                <div class="w-16">
                  <Input :model-value="String(item.sort_order ?? 0)" type="number" size="sm" min="0" class="bg-black/40 text-right" @change="(e: Event) => onUpdateSort(item, (e.target as HTMLInputElement).valueAsNumber)" />
                </div>
                <button
                  type="button"
                  class="px-3 py-1 rounded bg-destructive/30 hover:bg-destructive/50 text-xs text-destructive"
                  @click="onRemoveItem(item)"
                >{{ $t('admin.collections.itemRemove') }}</button>
              </div>
            </li>
          </ul>
          <p v-else class="text-white/50 italic text-sm">
            {{ $t('admin.collections.itemsEmpty') }}
          </p>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'
import Checkbox from '@/components/ui/Checkbox.vue'
import Input from '@/components/ui/Input.vue'
import {
  adminApi,
  animeApi,
  type Collection,
  type CollectionItem,
  type CreateCollectionRequest,
  type UpdateCollectionRequest,
} from '@/api/client'
import { useConfirm } from '@/composables/useConfirm'

const { t } = useI18n()
const { confirm } = useConfirm()
const route = useRoute()
const router = useRouter()

const isNew = computed(() => route.params.id === 'new')
const id = computed(() => (route.params.id as string) || '')

interface FormState {
  slug: string
  title: string
  title_ru: string
  title_jp: string
  description: string
  description_ru: string
  description_jp: string
  cover_image_url: string
  published: boolean
}

const form = reactive<FormState>({
  slug: '',
  title: '',
  title_ru: '',
  title_jp: '',
  description: '',
  description_ru: '',
  description_jp: '',
  cover_image_url: '',
  published: false,
})

const items = ref<CollectionItem[]>([])
const isLoading = ref(false)
const isSaving = ref(false)
const error = ref<string | null>(null)
const savedAt = ref<number | null>(null)

const searchQuery = ref('')
interface AnimeSearchResult {
  id: string
  name?: string
  name_ru?: string
  poster_url?: string
}
const searchResults = ref<AnimeSearchResult[]>([])
let searchTimer: ReturnType<typeof setTimeout> | null = null

function unwrap<T>(resp: { data: T | { data: T } }): T {
  const d = resp.data as unknown
  if (d && typeof d === 'object' && 'data' in (d as object)) {
    return (d as { data: T }).data
  }
  return d as T
}

async function load() {
  if (isNew.value) return
  isLoading.value = true
  error.value = null
  try {
    const resp = await adminApi.getCollection(id.value)
    const c = unwrap<Collection>(resp)
    form.slug = c.slug || ''
    form.title = c.title || ''
    form.title_ru = c.title_ru || ''
    form.title_jp = c.title_jp || ''
    form.description = c.description || ''
    form.description_ru = c.description_ru || ''
    form.description_jp = c.description_jp || ''
    form.cover_image_url = c.cover_image_url || ''
    form.published = !!c.published
    items.value = (c.items || []).slice().sort((a, b) => a.sort_order - b.sort_order)
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    error.value = err.response?.data?.error?.message || err.message || 'Failed to load'
  } finally {
    isLoading.value = false
  }
}

async function onSave() {
  if (!form.title) return
  isSaving.value = true
  error.value = null
  try {
    if (isNew.value) {
      const body: CreateCollectionRequest = { ...form }
      const resp = await adminApi.createCollection(body)
      const created = unwrap<Collection>(resp)
      savedAt.value = Date.now()
      // Re-route to the edit view so the items picker is available.
      await router.replace(`/admin/collections/${created.id}`)
      return
    }
    const body: UpdateCollectionRequest = { ...form }
    const resp = await adminApi.updateCollection(id.value, body)
    const c = unwrap<Collection>(resp)
    form.slug = c.slug || form.slug
    savedAt.value = Date.now()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    error.value = err.response?.data?.error?.message || err.message || 'Save failed'
  } finally {
    isSaving.value = false
  }
}

function onSearch() {
  if (searchTimer) clearTimeout(searchTimer)
  const q = searchQuery.value.trim()
  if (!q) {
    searchResults.value = []
    return
  }
  searchTimer = setTimeout(async () => {
    try {
      const resp = await animeApi.search(q, undefined, 8)
      const raw = unwrap<AnimeSearchResult[] | { animes?: AnimeSearchResult[]; results?: AnimeSearchResult[] }>(
        resp as { data: AnimeSearchResult[] | { animes?: AnimeSearchResult[]; results?: AnimeSearchResult[] } },
      )
      // Backend search payload may be either a flat array or an envelope
      // {animes: [...], total: N}. Handle both shapes defensively.
      if (Array.isArray(raw)) {
        searchResults.value = raw
      } else if (raw && Array.isArray(raw.animes)) {
        searchResults.value = raw.animes
      } else if (raw && Array.isArray(raw.results)) {
        searchResults.value = raw.results
      } else {
        searchResults.value = []
      }
    } catch {
      searchResults.value = []
    }
  }, 300)
}

async function onAddItem(r: AnimeSearchResult) {
  try {
    const nextSort = items.value.length > 0
      ? Math.max(...items.value.map(i => i.sort_order)) + 1
      : 0
    await adminApi.addCollectionItem(id.value, { anime_id: r.id, sort_order: nextSort })
    searchQuery.value = ''
    searchResults.value = []
    await load()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    alert(err.response?.data?.error?.message || err.message || 'Add failed')
  }
}

async function onRemoveItem(item: CollectionItem) {
  if (!(await confirm({
    title: t('common.confirmTitle'),
    description: t('admin.collections.itemRemove') + '?',
    confirmText: t('common.delete'),
    cancelText: t('common.cancel'),
    variant: 'destructive',
  }))) return
  try {
    await adminApi.removeCollectionItem(id.value, item.anime_id)
    await load()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    alert(err.response?.data?.error?.message || err.message || 'Remove failed')
  }
}

async function onUpdateSort(item: CollectionItem, newSort: number) {
  if (Number.isNaN(newSort)) return
  if (newSort === item.sort_order) return
  try {
    // AddItem is idempotent on (collection_id, anime_id) — calling it
    // again with the same anime_id upserts the sort_order in-place.
    await adminApi.addCollectionItem(id.value, { anime_id: item.anime_id, sort_order: newSort })
    await load()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    alert(err.response?.data?.error?.message || err.message || 'Update failed')
  }
}

function onImgError(e: Event) {
  const img = e.target as HTMLImageElement
  img.style.opacity = '0.3'
}

onMounted(load)
</script>
