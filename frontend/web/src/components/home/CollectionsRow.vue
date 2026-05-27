<template>
  <!-- Phase 17 (UX-33) admin-curated editorial collections row.
       Hidden when no published collections exist, per CONTEXT.md
       "specifics": empty state hides the row entirely. -->
  <div v-if="items.length > 0" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="flex items-center justify-between mb-4">
      <h2 class="collections-heading">
        {{ $t('collections.homeRowLabel') }}
      </h2>
    </div>
    <div class="collections-scroll">
      <router-link
        v-for="c in items"
        :key="c.id"
        :to="`/collections/${c.slug}`"
        class="collection-card-link"
      >
        <div class="collection-card">
          <!-- Cover image -->
          <img
            v-if="c.cover_image_url"
            :src="c.cover_image_url"
            :alt="localizedTitle(c)"
            class="collection-card-img"
            loading="lazy"
          />
          <!-- Fallback gradient (preserved from original) -->
          <div
            v-else
            class="collection-card-fallback"
          />
          <!-- Title overlay (Top-10 visual mode, per CONTEXT.md specifics). -->
          <div class="collection-card-overlay">
            <h3 class="collection-card-title">{{ localizedTitle(c) }}</h3>
            <p class="collection-card-count">{{ c.item_count }}</p>
          </div>
        </div>
      </router-link>
    </div>
  </div>
  <!-- Loading skeleton — mirrors ContinueWatchingRow.vue's pattern. -->
  <div v-else-if="isLoading" class="px-4 lg:px-8 max-w-7xl mx-auto mb-8">
    <div class="h-8 w-48 rounded animate-pulse mb-4" style="background: var(--line-strong);" />
    <div class="flex gap-3 overflow-hidden">
      <div
        v-for="i in 6"
        :key="i"
        class="collections-skeleton-card animate-pulse"
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

<style scoped>
/* Section heading */
.collections-heading {
  font-family: var(--f-display);
  font-size: 22px;
  font-weight: 700;
  letter-spacing: -0.01em;
  color: var(--ink);
}

/* Horizontal scroll row */
.collections-scroll {
  display: flex;
  gap: 12px;
  overflow-x: auto;
  padding-bottom: 8px;
  /* hide scrollbar on webkit */
  scrollbar-width: thin;
  scrollbar-color: rgba(255,255,255,0.08) transparent;
}
.collections-scroll::-webkit-scrollbar { height: 4px; }
.collections-scroll::-webkit-scrollbar-track { background: transparent; }
.collections-scroll::-webkit-scrollbar-thumb { background: rgba(255,255,255,0.08); border-radius: 999px; }

/* Card link — no text-decoration, fixed widths */
.collection-card-link {
  flex-shrink: 0;
  width: 160px;
  text-decoration: none;
  color: inherit;
}

@media (min-width: 768px) {
  .collection-card-link { width: 192px; }
}
@media (min-width: 1024px) {
  .collection-card-link { width: 224px; }
}

/* Card shell — Neon Tokyo token: --color-surface, --line, --r-lg */
.collection-card {
  position: relative;
  aspect-ratio: 2 / 3;
  background: var(--color-surface, #11111c);
  border: 1px solid var(--line);
  border-radius: var(--r-lg);
  overflow: hidden;
  transition: border-color 0.2s ease, box-shadow 0.2s ease, transform 0.2s ease;
}

.collection-card-link:hover .collection-card {
  border-color: var(--accent-line);
  box-shadow: var(--accent-glow);
  transform: translateY(-2px);
}

/* Cover image — scale on hover via parent hover */
.collection-card-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
  transition: transform 0.3s ease;
}

.collection-card-link:hover .collection-card-img {
  transform: scale(1.05);
}

/* Fallback gradient (preserved from original) */
.collection-card-fallback {
  width: 100%;
  height: 100%;
  background: linear-gradient(135deg, #0e7490 0%, #6b21a8 100%);
}

/* Title overlay */
.collection-card-overlay {
  position: absolute;
  inset-inline: 0;
  bottom: 0;
  padding: 12px;
  background: linear-gradient(to top, rgba(8,8,15,0.85) 0%, rgba(8,8,15,0.4) 60%, transparent 100%);
}

.collection-card-title {
  color: var(--ink);
  font-weight: 600;
  font-size: 13px;
  line-height: 1.3;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  text-shadow: 0 1px 4px rgba(0,0,0,0.6);
  margin-bottom: 2px;
}

@media (min-width: 768px) {
  .collection-card-title { font-size: 15px; }
}

/* Item count meta */
.collection-card-count {
  color: rgba(255,255,255,0.7);
  font-size: 11px;
  margin-top: 2px;
  font-family: var(--f-mono);
}

/* Loading skeleton cards */
.collections-skeleton-card {
  flex-shrink: 0;
  width: 160px;
  aspect-ratio: 2 / 3;
  border-radius: var(--r-lg);
  background: var(--line-strong);
}

@media (min-width: 768px) {
  .collections-skeleton-card { width: 192px; }
}
@media (min-width: 1024px) {
  .collections-skeleton-card { width: 224px; }
}
</style>
