<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import type { FavoriteAnimeConfig } from '@/types/showcase'
import { defaultVariant } from '@/types/showcase'
import { animeApi } from '@/api/client'
import { fromHomeAnime } from '@/utils/toCardModel'
import PosterCard from '@/components/anime/PosterCard.vue'
import type { AnimeCardModel } from '@/types/card'

const props = defineProps<{ config: FavoriteAnimeConfig; variant?: string; userId?: string }>()

// Raw API anime shape from getById
interface ApiAnime {
  id: string
  name?: string
  name_ru?: string
  name_jp?: string
  poster_url?: string
  score?: number
  status?: string
  episodes_count?: number
  episodes_aired?: number
  year?: number
  next_episode_at?: string
}

const items = ref<AnimeCardModel[]>([])

const v = computed(() => props.variant || defaultVariant('favorite_anime'))

onMounted(async () => {
  const ids = props.config.anime_ids ?? []
  if (!ids.length) return
  try {
    const results = await Promise.all(
      ids.map((id) =>
        animeApi
          .getById(id)
          .then((r) => {
            const raw = r.data as { data?: ApiAnime } & ApiAnime
            return ('data' in raw && raw.data ? raw.data : raw) as ApiAnime
          })
          .catch(() => null),
      ),
    )
    items.value = results
      .filter((a): a is ApiAnime => a !== null)
      .map((a) => fromHomeAnime(a))
  } catch {
    items.value = []
  }
})

// Podium order: 2nd (silver) | 1st (gold) | 3rd (bronze)
const podiumItems = computed(() => {
  const top = items.value.slice(0, 3)
  if (top.length < 2) return top.map((item, i) => ({ item, rank: i + 1 }))
  const [first, second, third] = top
  return [
    { item: second, rank: 2 },
    { item: first, rank: 1 },
    ...(third ? [{ item: third, rank: 3 }] : []),
  ]
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.favorite_anime') }}</h3>

    <!-- A: row (default) — horizontal scrolling strip with rank + score overlays -->
    <div v-if="v === 'row'">
      <div v-if="items.length" class="flex gap-3 overflow-x-auto pb-1 scrollbar-thin scrollbar-track-transparent scrollbar-thumb-border">
        <div
          v-for="(item, i) in items"
          :key="item.id"
          class="relative flex-none w-[120px] sm:w-[132px]"
        >
          <PosterCard :model="item" />
          <span class="absolute top-2 left-2 rounded-md bg-black/60 px-1.5 py-0.5 text-xs font-semibold text-foreground">
            {{ i + 1 }}
          </span>
          <span
            v-if="item.malScore"
            class="absolute top-2 right-2 rounded-md bg-black/60 px-1.5 py-0.5 text-xs font-semibold text-info"
          >
            ◆ {{ item.malScore.toFixed(1) }}
          </span>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- B: podium — olympic top-3, gold center + raised -->
    <div v-else-if="v === 'podium'">
      <div v-if="items.length" class="podium-grid mx-auto max-w-[580px] items-end gap-4">
        <div
          v-for="entry in podiumItems"
          :key="entry.item.id"
          class="flex flex-col"
          :data-rank="entry.rank"
        >
          <!-- crown only on 1st -->
          <div v-if="entry.rank === 1" class="mb-1.5 text-center text-2xl" style="filter:drop-shadow(0 0 8px var(--warning))">👑</div>
          <!-- poster with rank-specific ring -->
          <div class="relative w-full overflow-hidden rounded-xl" :class="{
            'ring-2 ring-warning shadow-[0_22px_54px_-18px_var(--warning)]': entry.rank === 1,
            'ring-[1.5px] ring-info/70 shadow-[0_18px_44px_-20px_var(--info)]': entry.rank === 2,
            'ring-[1.5px] ring-warning/50 shadow-[0_18px_44px_-20px_var(--warning)]': entry.rank === 3,
          }">
            <PosterCard :model="entry.item" />
          </div>
          <!-- pedestal -->
          <div
            class="podium-ped mt-2.5 flex items-center justify-center rounded-[10px_10px_5px_5px] font-semibold"
            :class="{
              'podium-ped-gold h-[60px]': entry.rank === 1,
              'podium-ped-silver h-[42px]': entry.rank === 2,
              'podium-ped-bronze h-[30px]': entry.rank === 3,
            }"
          >
            <span class="podium-ped-num text-2xl leading-none">{{ entry.rank }}</span>
          </div>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- C: grid — equal 6-col (3-col mobile) grid of posters -->
    <div v-else-if="v === 'grid'">
      <div v-if="items.length" class="grid grid-cols-3 gap-3 sm:grid-cols-4 md:grid-cols-6">
        <PosterCard v-for="item in items" :key="item.id" :model="item" />
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- D: list — mini poster + title + score + progress bar -->
    <div v-else-if="v === 'list'">
      <div v-if="items.length" class="flex flex-col gap-2.5">
        <div
          v-for="(item, i) in items"
          :key="item.id"
          class="group flex items-center gap-3.5 rounded-[14px] border border-border bg-white/[0.02] px-2.5 py-2 transition-transform duration-150 hover:translate-x-[3px] hover:bg-brand-cyan/[0.06]"
        >
          <!-- rank number -->
          <span class="w-[22px] shrink-0 text-center text-sm font-semibold text-brand-cyan">{{ i + 1 }}</span>
          <!-- mini poster -->
          <img
            :src="item.coverImage"
            :alt="item.title"
            class="h-16 w-[46px] shrink-0 rounded-[9px] object-cover"
          />
          <!-- info -->
          <div class="min-w-0 flex-1">
            <div class="truncate text-sm font-semibold text-foreground" data-testid="list-title">{{ item.title }}</div>
            <div class="mt-0.5 text-xs text-muted-foreground">
              <span v-if="item.episodes">{{ item.episodes }} {{ $t('showcase.ep') }}</span>
            </div>
            <!-- score bar -->
            <div class="mt-1.5 h-[5px] max-w-[200px] overflow-hidden rounded-full bg-white/[0.08]">
              <i
                v-if="item.malScore"
                class="block h-full rounded-full bg-gradient-to-r from-brand-cyan to-brand-violet"
                :style="{ width: `${Math.round((item.malScore / 10) * 100)}%` }"
              />
            </div>
          </div>
          <!-- score diamond -->
          <span v-if="item.malScore" class="shrink-0 text-sm font-semibold text-brand-cyan">
            ◆ {{ item.malScore.toFixed(1) }}
          </span>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>

    <!-- E: banner — wide cover strips with overlay gradient + zoom on hover -->
    <div v-else-if="v === 'banner'">
      <div v-if="items.length" class="flex flex-col gap-3">
        <div
          v-for="item in items"
          :key="item.id"
          class="group relative h-[120px] cursor-pointer overflow-hidden rounded-[14px] border border-border"
        >
          <!-- cover image with zoom -->
          <img
            :src="item.coverImage"
            :alt="item.title"
            class="h-full w-full object-cover object-[center_28%] transition-transform duration-[450ms] ease-out group-hover:scale-[1.06]"
          />
          <!-- dark overlay -->
          <div class="banner-overlay absolute inset-0" />
          <!-- text content -->
          <div class="absolute bottom-0 left-5 top-0 flex flex-col justify-center">
            <div class="text-xl font-semibold leading-tight text-foreground" data-testid="banner-title">{{ item.title }}</div>
            <div v-if="item.episodes" class="mt-0.5 text-xs text-muted-foreground">
              {{ item.episodes }} {{ $t('showcase.ep') }}
            </div>
          </div>
          <!-- score chip -->
          <span
            v-if="item.malScore"
            class="absolute right-4 top-3.5 rounded-lg border border-brand-cyan/30 bg-black/60 px-2.5 py-[3px] text-xs font-semibold text-brand-cyan"
          >
            ◆ {{ item.malScore.toFixed(1) }}
          </span>
        </div>
      </div>
      <p v-else class="text-sm text-muted-foreground">{{ $t('showcase.empty') }}</p>
    </div>
  </div>
</template>

<style scoped>
/* Podium: 3-col grid (silver | gold | bronze), gold center is taller */
.podium-grid {
  display: grid;
  grid-template-columns: 1fr 1.18fr 1fr;
  gap: 16px;
}
@media (max-width: 680px) {
  .podium-grid {
    grid-template-columns: 1fr 1fr;
  }
}

/* Medal pedestals — gold/silver/bronze gradients; no DS token within tolerance */
.podium-ped-gold {
  /* gold: warning token top → deep-gold bottom */
  background: linear-gradient(180deg, var(--warning), #ff9d00);
}
.podium-ped-silver {
  /* silver: light-blue periwinkle ramp; no token */
  background: linear-gradient(180deg, #eaf0ff, #9aa6c9);
}
.podium-ped-bronze {
  /* bronze: warm copper ramp; no token */
  background: linear-gradient(180deg, #ffc7a3, #e6703a);
}

/* rank number on medal pedestal — near-black for contrast on light medal surface */
.podium-ped-num {
  color: #06121a;
}

/* Banner left-to-right dark overlay */
.banner-overlay {
  background: linear-gradient(90deg, var(--scrim-bg-strong) 0%, var(--scrim-bg-soft) 55%, transparent);
}
</style>
