<template>
  <!-- Phase 17 (UX-33) admin-curated editorial collections row.
       Hidden when no published collections exist, per CONTEXT.md
       "specifics": empty state hides the row entirely. -->
  <div v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="flex items-center justify-between mb-4">
      <h2 class="text-xl md:text-2xl font-bold text-white">
        {{ $t('collections.homeRowLabel') }}
      </h2>
    </div>
    <div class="flex gap-3 overflow-x-auto scrollbar-hide pb-2 -mx-1 px-1">
      <router-link
        v-for="c in items"
        :key="c.id"
        :to="`/collections/${c.slug}`"
        class="flex-shrink-0 w-40 md:w-48 lg:w-56 group"
      >
        <div class="relative rounded-xl overflow-hidden bg-white/5 aspect-[2/3] mb-2">
          <img
            v-if="c.cover_image_url"
            :src="c.cover_image_url"
            :alt="localizedTitle(c)"
            class="w-full h-full object-cover group-hover:scale-105 transition-transform duration-300"
            loading="lazy"
          />
          <div
            v-else
            class="w-full h-full"
            style="background: linear-gradient(135deg, #0e7490 0%, #6b21a8 100%);"
          ></div>
          <!-- Title overlay (Top-10 visual mode, per CONTEXT.md specifics). -->
          <div
            class="absolute inset-x-0 bottom-0 p-3 bg-gradient-to-t from-black/85 via-black/40 to-transparent"
          >
            <h3 class="text-white font-semibold text-sm md:text-base line-clamp-2 drop-shadow">
              {{ localizedTitle(c) }}
            </h3>
            <p class="text-white/70 text-xs mt-0.5">{{ c.item_count }}</p>
          </div>
        </div>
      </router-link>
    </div>
  </div>
  <!-- Loading skeleton — mirrors ContinueWatchingRow.vue's pattern. -->
  <div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="h-8 w-48 bg-white/10 rounded animate-pulse mb-4" />
    <div class="flex gap-3 overflow-hidden">
      <div
        v-for="i in 6"
        :key="i"
        class="flex-shrink-0 w-40 md:w-48 lg:w-56 aspect-[2/3] bg-white/10 rounded-xl animate-pulse"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { animeApi, type Collection } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'

const { locale } = useI18n()

const items = ref<Collection[]>([])
const isLoading = ref(true)

function unwrap<T>(resp: { data: T | { data: T } }): T {
  const d = resp.data as unknown
  if (d && typeof d === 'object' && 'data' in (d as object)) {
    return (d as { data: T }).data
  }
  return d as T
}

async function load() {
  isLoading.value = true
  try {
    const resp = await animeApi.listCollections(12)
    const data = unwrap<Collection[]>(resp)
    items.value = Array.isArray(data) ? data : []
  } catch {
    items.value = []
  } finally {
    isLoading.value = false
  }
}

function localizedTitle(c: Collection): string {
  void locale.value
  return getLocalizedTitle(c.title, c.title_ru, c.title_jp)
}

onMounted(load)
</script>
