<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { publicApi } from '@/api/client'
import { defaultVariant } from '@/types/showcase'

const props = defineProps<{ userId: string; variant?: string }>()

interface WatchlistStats {
  total_entries?: number
  avg_score?: number
  total_episodes?: number
  completed?: number
}

const stats = ref<WatchlistStats | null>(null)

onMounted(async () => {
  try {
    const res = await publicApi.getPublicWatchlistStats(props.userId)
    const data = (res.data as { data?: WatchlistStats } & WatchlistStats)
    stats.value = ('data' in data && data.data ? data.data : data) as WatchlistStats
  } catch {
    stats.value = null
  }
})

const v = computed(() => props.variant || defaultVariant('stats'))

// Rings: compute fill percentage for each stat ring
const ringPct = computed(() => {
  const s = stats.value
  const total = s?.total_entries ?? 0
  const score = s?.avg_score ?? 0
  const eps = s?.total_episodes ?? 0
  const done = s?.completed ?? 0
  return {
    total: 100,
    score: score > 0 ? Math.min(100, (score / 10) * 100) : 0,
    eps: eps > 0 ? Math.min(100, (eps / 5000) * 100) : 0,
    completed: total > 0 ? Math.min(100, (done / total) * 100) : 0,
  }
})

// Bars: headline stats as bars with proportional fill
const barItems = computed(() => {
  const s = stats.value
  const total = s?.total_entries ?? 0
  const score = s?.avg_score ?? 0
  const eps = s?.total_episodes ?? 0
  const done = s?.completed ?? 0

  const items = [
    { labelKey: 'profile.stats.totalAnime', value: total, max: Math.max(total, 1), color: 'var(--brand-cyan)', color2: 'var(--brand-cyan)' },
    { labelKey: 'profile.stats.avgScore', value: score > 0 ? score : 0, displayValue: score > 0 ? score.toFixed(1) : '-', max: 10, color: 'var(--brand-pink)', color2: 'var(--brand-pink)' },
    { labelKey: 'profile.stats.episodesWatched', value: eps, max: Math.max(eps, 1), color: 'var(--brand-violet)', color2: 'var(--brand-violet)' },
    { labelKey: 'profile.stats.completed', value: done, max: Math.max(total, done, 1), color: 'var(--success)', color2: 'var(--success)' },
  ]
  const maxVal = Math.max(...items.map(i => i.value))
  return items.map(i => ({
    ...i,
    pct: maxVal > 0 ? Math.min(100, (i.value / maxVal) * 100) : 0,
    displayValue: i.displayValue ?? String(i.value),
  }))
})

// Completed percentage for rings label
const completedPct = computed(() => {
  const s = stats.value
  const total = s?.total_entries ?? 0
  const done = s?.completed ?? 0
  if (!total) return '0%'
  return `${Math.round((done / total) * 100)}%`
})
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 class="mb-3 text-lg font-semibold text-foreground">{{ $t('showcase.block.stats') }}</h3>

    <!-- A: tiles (default) -->
    <div v-if="v === 'tiles'" class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <div class="rounded-2xl border border-border p-4 text-center" style="background:radial-gradient(120% 120% at 50% 0%,var(--cyan-a08),var(--white-a4))">
        <div class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-cyan));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.total_entries ?? 0 }}
        </div>
        <div class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.totalAnime') }}</div>
      </div>
      <div class="rounded-2xl border border-border p-4 text-center" style="background:radial-gradient(120% 120% at 50% 0%,var(--cyan-a08),var(--white-a4))">
        <div class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-pink));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.avg_score && stats.avg_score > 0 ? stats.avg_score.toFixed(1) : '-' }}
        </div>
        <div class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.avgScore') }}</div>
      </div>
      <div class="rounded-2xl border border-border p-4 text-center" style="background:radial-gradient(120% 120% at 50% 0%,var(--cyan-a08),var(--white-a4))">
        <div class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-violet));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.total_episodes ?? 0 }}
        </div>
        <div class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.episodesWatched') }}</div>
      </div>
      <div class="rounded-2xl border border-border p-4 text-center" style="background:radial-gradient(120% 120% at 50% 0%,var(--cyan-a08),var(--white-a4))">
        <div class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--success));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.completed ?? 0 }}
        </div>
        <div class="mt-1 text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.completed') }}</div>
      </div>
    </div>

    <!-- B: rings -->
    <div v-else-if="v === 'rings'" class="grid grid-cols-2 gap-3 sm:grid-cols-4">
      <div class="stats-ring flex flex-col items-center gap-2 p-2">
        <div
          class="stats-ring-circ relative grid h-[104px] w-[104px] place-items-center rounded-full"
          :style="`background:conic-gradient(var(--brand-cyan) ${ringPct.total}%,var(--white-a8) 0)`"
        >
          <div class="absolute inset-[9px] rounded-full bg-card"></div>
          <span class="relative text-xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-cyan));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
            {{ stats?.total_entries ?? 0 }}
          </span>
        </div>
        <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.totalAnime') }}</span>
      </div>
      <div class="stats-ring flex flex-col items-center gap-2 p-2">
        <div
          class="stats-ring-circ relative grid h-[104px] w-[104px] place-items-center rounded-full"
          :style="`background:conic-gradient(var(--brand-pink) ${ringPct.score}%,var(--white-a8) 0)`"
        >
          <div class="absolute inset-[9px] rounded-full bg-card"></div>
          <span class="relative text-xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-pink));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
            {{ stats?.avg_score && stats.avg_score > 0 ? stats.avg_score.toFixed(1) : '-' }}
          </span>
        </div>
        <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.avgScore') }}</span>
      </div>
      <div class="stats-ring flex flex-col items-center gap-2 p-2">
        <div
          class="stats-ring-circ relative grid h-[104px] w-[104px] place-items-center rounded-full"
          :style="`background:conic-gradient(var(--brand-violet) ${ringPct.eps}%,var(--white-a8) 0)`"
        >
          <div class="absolute inset-[9px] rounded-full bg-card"></div>
          <span class="relative text-xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-violet));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
            {{ stats?.total_episodes ?? 0 }}
          </span>
        </div>
        <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.episodesWatched') }}</span>
      </div>
      <div class="stats-ring flex flex-col items-center gap-2 p-2">
        <div
          class="stats-ring-circ relative grid h-[104px] w-[104px] place-items-center rounded-full"
          :style="`background:conic-gradient(var(--success) ${ringPct.completed}%,var(--white-a8) 0)`"
        >
          <div class="absolute inset-[9px] rounded-full bg-card"></div>
          <span class="relative text-xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--success));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
            {{ completedPct }}
          </span>
        </div>
        <span class="text-xs font-medium uppercase tracking-wide text-muted-foreground">{{ $t('profile.stats.completed') }}</span>
      </div>
    </div>

    <!-- C: bars -->
    <div v-else-if="v === 'bars'" class="flex flex-col gap-3">
      <div
        v-for="item in barItems"
        :key="item.labelKey"
        class="grid items-center gap-3"
        style="grid-template-columns:120px 1fr 46px"
      >
        <span class="text-sm font-medium text-muted-foreground">{{ $t(item.labelKey) }}</span>
        <div class="h-[10px] overflow-hidden rounded-[6px]" style="background:var(--white-a8)">
          <i
            class="block h-full rounded-[6px]"
            :style="`width:${item.pct}%;background:linear-gradient(90deg,${item.color},${item.color2})`"
          ></i>
        </div>
        <span class="text-right text-sm font-semibold text-foreground">{{ item.displayValue }}</span>
      </div>
    </div>

    <!-- D: strip -->
    <div v-else-if="v === 'strip'" class="flex flex-wrap items-center justify-between gap-3 px-1 py-1">
      <div class="stats-strip-item flex items-baseline gap-2">
        <span class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-cyan));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.total_entries ?? 0 }}
        </span>
        <span class="text-xs text-muted-foreground">{{ $t('profile.stats.totalAnime') }}</span>
      </div>
      <div class="h-[30px] w-px bg-border"></div>
      <div class="stats-strip-item flex items-baseline gap-2">
        <span class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-pink));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.avg_score && stats.avg_score > 0 ? stats.avg_score.toFixed(1) : '-' }}
        </span>
        <span class="text-xs text-muted-foreground">{{ $t('profile.stats.avgScore') }}</span>
      </div>
      <div class="h-[30px] w-px bg-border"></div>
      <div class="stats-strip-item flex items-baseline gap-2">
        <span class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--brand-violet));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.total_episodes ?? 0 }}
        </span>
        <span class="text-xs text-muted-foreground">{{ $t('profile.stats.episodesWatched') }}</span>
      </div>
      <div class="h-[30px] w-px bg-border"></div>
      <div class="stats-strip-item flex items-baseline gap-2">
        <span class="text-2xl font-semibold" style="background:linear-gradient(180deg,var(--foreground),var(--success));-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent">
          {{ stats?.completed ?? 0 }}
        </span>
        <span class="text-xs text-muted-foreground">{{ $t('profile.stats.completed') }}</span>
      </div>
    </div>
  </div>
</template>
