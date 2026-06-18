<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { FavoriteCharacterConfig } from '@/types/showcase'
import { defaultVariant } from '@/types/showcase'
import { charactersApi } from '@/api/client'
import { getLocalizedTitle } from '@/utils/title'
import { getImageUrl } from '@/composables/useImageProxy'
import CharacterCard from '@/components/anime/CharacterCard.vue'
import type { CharacterCardModel } from '@/types/character'

const props = defineProps<{ config: FavoriteCharacterConfig; variant?: string }>()

interface ApiCharacter {
  shikimori_id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
}

const items = ref<CharacterCardModel[]>([])

const v = computed(() => props.variant || defaultVariant('favorite_character'))

onMounted(async () => {
  const ids = props.config.character_ids ?? []
  if (!ids.length) return
  const results = await Promise.all(
    ids.map((id) =>
      charactersApi
        .getCharacter(String(id))
        .then((r): CharacterCardModel => {
          const raw = r.data as { data?: ApiCharacter } & ApiCharacter
          const c: ApiCharacter = 'data' in raw && raw.data ? raw.data : raw
          return {
            id: c.shikimori_id,
            name: getLocalizedTitle(c.name, c.name_ru, c.name_jp),
            image: getImageUrl(c.poster_url),
            role: 'supporting',
          }
        })
        .catch(() => null),
    ),
  )
  items.value = results.filter((c): c is CharacterCardModel => c !== null)
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_character') }}</h3>

    <!-- A: circles (default) — avatar ring row using CharacterCard -->
    <div v-if="v === 'circles'">
      <div v-if="items.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
        <CharacterCard v-for="c in items" :key="c.id" :model="c" />
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- B: portraits — vertical cards with gradient overlay + name -->
    <div v-else-if="v === 'portraits'">
      <div v-if="items.length" class="flex flex-wrap gap-3">
        <div
          v-for="(c, i) in items"
          :key="c.id"
          class="portrait-card relative w-[124px] flex-none overflow-hidden rounded-xl border border-border"
        >
          <!-- "★ #1" badge on first item -->
          <span
            v-if="i === 0"
            class="absolute left-2 top-2 z-10 rounded-md bg-black/70 px-1.5 py-0.5 text-xs font-semibold text-pink-400"
          >
            ★ #1
          </span>
          <img
            :src="c.image"
            :alt="c.name"
            class="h-full w-full object-cover"
            style="aspect-ratio: 3/4"
          />
          <!-- gradient overlay -->
          <div class="absolute inset-0 bg-gradient-to-t from-black/90 to-transparent" />
          <!-- name + role -->
          <div class="absolute bottom-0 left-0 right-0 p-2">
            <div class="truncate text-xs font-semibold text-foreground" data-testid="portrait-name">{{ c.name }}</div>
            <div class="truncate text-xs text-muted-foreground">{{ c.role }}</div>
          </div>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- C: hero — big featured card left + ranked list right -->
    <div v-else-if="v === 'hero'">
      <div v-if="items.length" class="hero-layout gap-4">
        <!-- big card (first character) -->
        <div
          class="relative overflow-hidden rounded-xl border border-border"
          style="aspect-ratio: 3/4"
          data-testid="hero-big-card"
        >
          <img
            :src="items[0].image"
            :alt="items[0].name"
            class="h-full w-full object-cover"
          />
          <!-- overlay -->
          <div class="absolute inset-0 bg-gradient-to-b from-black/15 via-transparent to-black/90" />
          <!-- ♥ #1 badge -->
          <span class="absolute left-2 top-2 rounded-md bg-black/70 px-1.5 py-0.5 text-xs font-semibold text-pink-400">
            ♥ #1
          </span>
          <!-- info bottom -->
          <div class="absolute bottom-0 left-0 right-0 p-3">
            <div class="truncate text-sm font-semibold text-foreground">{{ items[0].name }}</div>
            <div class="text-xs text-muted-foreground">{{ items[0].role }}</div>
          </div>
        </div>

        <!-- ranked list (remaining characters) -->
        <div class="flex flex-col gap-2">
          <div
            v-for="(c, i) in items.slice(1)"
            :key="c.id"
            class="flex items-center gap-3 rounded-xl border border-border bg-white/[0.02] px-3 py-2"
            data-testid="hero-list-item"
          >
            <!-- rank number -->
            <span class="w-5 shrink-0 text-center text-sm font-semibold text-pink-400">{{ i + 2 }}</span>
            <!-- avatar with conic ring -->
            <div class="hero-ring relative h-[42px] w-[42px] shrink-0 rounded-full">
              <img
                :src="c.image"
                :alt="c.name"
                class="h-full w-full rounded-full object-cover"
              />
            </div>
            <!-- name + role -->
            <div class="min-w-0 flex-1">
              <div class="truncate text-sm font-semibold text-foreground">{{ c.name }}</div>
              <div class="text-xs text-muted-foreground">{{ c.role }}</div>
            </div>
            <!-- heart icon -->
            <span class="shrink-0 text-xs text-pink-400">♥</span>
          </div>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- D: hex — clipped hexagon avatars -->
    <div v-else-if="v === 'hex'">
      <div v-if="items.length" class="flex flex-wrap gap-4">
        <div
          v-for="c in items"
          :key="c.id"
          class="hex-item flex w-[92px] flex-col items-center gap-1.5"
          data-testid="hex-item"
        >
          <!-- hexagon container -->
          <div class="hex-shape relative h-[92px] w-[92px]">
            <!-- gradient ring layer -->
            <div class="hex-ring absolute inset-0" />
            <!-- image layer clipped to hex -->
            <div class="hex-img absolute inset-[3px]">
              <img
                :src="c.image"
                :alt="c.name"
                class="h-full w-full object-cover"
              />
            </div>
          </div>
          <span class="w-full truncate text-center text-xs font-medium text-foreground">{{ c.name }}</span>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>
  </div>
</template>

<style scoped>
/* B: portraits card — fixed 3:4 aspect ratio */
.portrait-card {
  aspect-ratio: 3/4;
}

/* C: hero layout — big card left (236px) + list right (1fr) */
.hero-layout {
  display: grid;
  grid-template-columns: 236px 1fr;
}
@media (max-width: 600px) {
  .hero-layout {
    grid-template-columns: 1fr;
  }
}

/* C: conic-gradient ring for hero list avatars */
.hero-ring::before {
  content: '';
  position: absolute;
  inset: -2px;
  border-radius: 9999px;
  background: conic-gradient(from 150deg, var(--brand-pink), var(--brand-violet), var(--brand-cyan), var(--brand-pink));
  z-index: -1;
}

/* D: hexagon clip-path */
.hex-shape {
  position: relative;
}
.hex-ring {
  clip-path: polygon(50% 0%, 100% 25%, 100% 75%, 50% 100%, 0% 75%, 0% 25%);
  background: conic-gradient(from 150deg, var(--brand-pink), var(--brand-violet), var(--brand-cyan), var(--brand-pink));
}
.hex-img {
  clip-path: polygon(50% 0%, 100% 25%, 100% 75%, 50% 100%, 0% 75%, 0% 25%);
  overflow: hidden;
  border-radius: 0;
}
</style>
