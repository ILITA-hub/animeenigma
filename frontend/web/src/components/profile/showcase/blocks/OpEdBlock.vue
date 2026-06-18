<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { themesApi } from '@/api/client'

export interface OpEdConfig {
  theme_ids: string[]
}

interface ThemeData {
  id: string
  poster_url?: string
  anime_name?: string
  song_title?: string
  artist_name?: string
  theme_type?: string
  slug?: string
}

const props = defineProps<{ config: OpEdConfig; variant?: string }>()

const themes = ref<ThemeData[]>([])

function unwrap(resp: unknown): ThemeData | null {
  if (!resp || typeof resp !== 'object') return null
  const r = resp as Record<string, unknown>
  if ('data' in r && r.data && typeof r.data === 'object') {
    const d = r.data as Record<string, unknown>
    if ('id' in d) return d as unknown as ThemeData
    // might be wrapped further
    if ('data' in d) return d.data as unknown as ThemeData
  }
  if ('id' in r) return r as unknown as ThemeData
  return null
}

onMounted(async () => {
  if (!props.config?.theme_ids?.length) return
  const results = await Promise.allSettled(
    props.config.theme_ids.map((id) => themesApi.get(id).then((r) => unwrap(r.data)))
  )
  themes.value = results
    .filter((r): r is PromiseFulfilledResult<ThemeData | null> => r.status === 'fulfilled' && r.value !== null)
    .map((r) => r.value!)
})
</script>

<template>
  <div v-if="themes.length" class="h-full space-y-3">
    <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
      {{ $t('showcase.block.op_ed') }}
    </h3>
    <div class="grid grid-cols-3 gap-3 max-[680px]:grid-cols-2">
      <div
        v-for="theme in themes"
        :key="theme.id"
        class="rounded-2xl overflow-hidden border border-border cursor-pointer transition-transform duration-150 hover:-translate-y-1"
      >
        <!-- Cover -->
        <div class="relative aspect-square">
          <img
            v-if="theme.poster_url"
            :src="theme.poster_url"
            :alt="theme.anime_name"
            class="w-full h-full object-cover"
          />
          <div v-else class="w-full h-full bg-muted" />
          <!-- gradient overlay -->
          <div class="absolute inset-0 bg-gradient-to-b from-transparent via-transparent to-black/85" />
          <!-- OP / ED badge -->
          <span
            class="absolute top-2 left-2 text-[10px] font-semibold px-2 py-0.5 rounded-md bg-black/70 border border-border"
            :class="theme.theme_type === 'ED' || theme.theme_type === 'ending' ? 'text-pink' : 'text-cyan'"
          >
            {{ theme.theme_type === 'ending' || theme.theme_type === 'ED' ? 'ED' : 'OP' }}
          </span>
          <!-- equalizer animation (decorative) -->
          <div class="absolute left-2.5 bottom-3 flex gap-0.5 items-end h-4" aria-hidden="true">
            <i
              v-for="n in 4"
              :key="n"
              class="w-0.5 rounded-sm bg-cyan"
              style="animation: oped-eq 900ms ease-in-out infinite"
              :style="{ animationDelay: `${(n - 1) * 150}ms` }"
            />
          </div>
          <!-- play button -->
          <div
            class="absolute right-2.5 bottom-2.5 w-9 h-9 rounded-full flex items-center justify-center shadow-lg oped-play-btn"
          >
            <svg width="14" height="14" viewBox="0 0 14 14" fill="currentColor" aria-hidden="true">
              <polygon points="3,1 13,7 3,13" />
            </svg>
          </div>
        </div>
        <!-- meta -->
        <div class="px-3 py-2.5">
          <div v-if="theme.song_title" class="text-sm font-semibold truncate">{{ theme.song_title }}</div>
          <div class="text-[11px] text-muted-foreground mt-0.5 truncate">
            <span v-if="theme.anime_name">{{ theme.anime_name }}</span>
            <span v-if="theme.artist_name"> · {{ theme.artist_name }}</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
@keyframes oped-eq {
  0%, 100% { height: 4px; }
  50%       { height: 16px; }
}
/* Play button: brand-cyan gradient + near-base-ink icon color */
.oped-play-btn {
  background: linear-gradient(135deg, var(--brand-cyan), var(--brand-cyan));
  color: var(--background);
  box-shadow: 0 6px 20px var(--cyan-a40);
}
</style>
