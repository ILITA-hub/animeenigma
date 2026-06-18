<template>
  <!-- Phase 17 (UX-33) public detail page at /collections/:slug. -->
  <div class="min-h-screen bg-base">
    <!-- Loading -->
    <div v-if="isLoading" class="flex justify-center pt-32">
      <Spinner size="lg" />
    </div>

    <!-- 404 -->
    <div v-else-if="notFound" class="pt-32 max-w-3xl mx-auto px-4 text-center">
      <h1 class="text-3xl font-semibold text-white mb-4">{{ $t('collections.notFound') }}</h1>
      <router-link to="/" class="text-cyan-400 hover:underline">← {{ $t('nav.home') }}</router-link>
    </div>

    <!-- Detail -->
    <template v-else-if="collection">
      <!-- Hero -->
      <section
        class="relative w-full overflow-hidden"
        :style="heroStyle"
      >
        <div class="absolute inset-0 bg-gradient-to-b from-black/30 via-black/50 to-base"></div>
        <div class="relative max-w-7xl mx-auto px-4 lg:px-8 py-20 md:py-28">
          <h1 class="text-4xl md:text-5xl font-semibold text-white mb-4 drop-shadow-lg">
            {{ localizedTitle }}
          </h1>
          <p v-if="localizedDescription" class="text-white/80 text-lg max-w-3xl drop-shadow whitespace-pre-line">
            {{ localizedDescription }}
          </p>
          <p class="text-white/60 text-sm mt-3">
            {{ collection.item_count || (collection.items?.length ?? 0) }}
            <span class="ml-1">{{ $t('admin.collections.tableItems').toLowerCase() }}</span>
          </p>
        </div>
      </section>

      <!-- Grid -->
      <section class="max-w-7xl mx-auto px-4 lg:px-8 py-10">
        <EmptyState v-if="!collection.items || collection.items.length === 0" class="italic">
          {{ $t('collections.emptyItems') }}
        </EmptyState>
        <div
          v-else
          class="grid gap-4"
          style="grid-template-columns: repeat(auto-fill, minmax(160px, 1fr));"
        >
          <router-link
            v-for="item in sortedItems"
            :key="item.id"
            :to="`/anime/${item.anime?.id || item.anime_id}`"
            class="group block"
          >
            <PosterImage
              :src="item.anime?.poster_url || '/placeholder.svg'"
              :alt="cardTitle(item)"
              ratio="2/3"
              rounded="lg"
              :proxy-width="384"
              class="mb-2 group-hover:scale-105 transition-transform duration-300"
            />
            <h3 class="text-sm font-medium text-white truncate group-hover:text-cyan-400 transition-colors">
              {{ cardTitle(item) }}
            </h3>
          </router-link>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { animeApi, type Collection, type CollectionItem } from '@/api/client'
import { Spinner, EmptyState } from '@/components/ui'
import PosterImage from '@/components/anime/PosterImage.vue'
import { getLocalizedTitle } from '@/utils/title'

const route = useRoute()
const { locale } = useI18n()

const collection = ref<Collection | null>(null)
const isLoading = ref(true)
const notFound = ref(false)

function unwrap<T>(resp: { data: T | { data: T } }): T {
  const d = resp.data as unknown
  if (d && typeof d === 'object' && 'data' in (d as object)) {
    return (d as { data: T }).data
  }
  return d as T
}

async function load() {
  const slug = (route.params.slug as string) || ''
  if (!slug) {
    notFound.value = true
    isLoading.value = false
    return
  }
  isLoading.value = true
  notFound.value = false
  try {
    const resp = await animeApi.getCollection(slug)
    collection.value = unwrap<Collection>(resp)
  } catch (e: unknown) {
    const err = e as { response?: { status?: number } }
    if (err.response?.status === 404) {
      notFound.value = true
    } else {
      notFound.value = true
    }
  } finally {
    isLoading.value = false
  }
}

const localizedTitle = computed(() => {
  if (!collection.value) return ''
  void locale.value
  return getLocalizedTitle(collection.value.title, collection.value.title_ru, collection.value.title_jp)
})

const localizedDescription = computed(() => {
  if (!collection.value) return ''
  void locale.value
  const c = collection.value
  switch (locale.value) {
    case 'en':
      return c.description || c.description_ru || c.description_jp || ''
    case 'ja':
      return c.description_jp || c.description || c.description_ru || ''
    default: // 'ru'
      return c.description_ru || c.description || c.description_jp || ''
  }
})

const sortedItems = computed(() => {
  const list = collection.value?.items || []
  // Defensive copy + sort — backend already returns sorted, but never trust.
  return [...list].sort((a, b) => a.sort_order - b.sort_order)
})

const heroStyle = computed(() => {
  if (collection.value?.cover_image_url) {
    return {
      backgroundImage: `url("${collection.value.cover_image_url}")`,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
    }
  }
  // Fallback: a subtle cyan/purple gradient.
  return {
    background: 'linear-gradient(135deg, #0e7490 0%, #6b21a8 100%)',
  }
})

function cardTitle(item: CollectionItem): string {
  const a = item.anime
  if (!a) return item.anime_id
  return getLocalizedTitle(a.name, a.name_ru, a.name_jp)
}

watch(() => route.params.slug, () => {
  void load()
})

onMounted(load)
</script>
