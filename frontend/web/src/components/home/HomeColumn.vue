<template>
  <div class="col">
    <!-- Column header -->
    <div class="col-head">
      <div class="icon" :class="iconTone">
        <!-- green: play-circle icon -->
        <template v-if="iconTone === 'green'">
          <PlayCircle class="size-[18px]" aria-hidden="true" />
        </template>
        <!-- gold: star icon -->
        <template v-else-if="iconTone === 'gold'">
          <Star class="size-[18px]" fill="currentColor" aria-hidden="true" />
        </template>
        <!-- blue: calendar icon -->
        <template v-else>
          <Calendar class="size-[18px]" aria-hidden="true" />
        </template>
      </div>

      <div class="col-head-text">
        <h3>{{ title }}</h3>
        <div v-if="sub" class="sub">{{ sub }}</div>
      </div>

      <router-link :to="seeAllTo" class="all" :aria-label="$t('home.seeAllFor', { title })">{{ $t('home.seeAll') }}</router-link>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading" class="col-list">
      <div v-for="i in 5" :key="i" class="skeleton-row">
        <div class="skeleton-poster"></div>
        <div class="skeleton-body">
          <div class="skeleton-line w-3/4"></div>
          <div class="skeleton-line w-1/2"></div>
        </div>
      </div>
    </div>

    <!-- Items slot -->
    <div v-else class="col-list">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { PlayCircle, Star, Calendar } from 'lucide-vue-next'

defineProps<{
  title: string
  sub?: string
  iconTone: 'green' | 'gold' | 'blue'
  seeAllTo: string
  loading: boolean
}>()
</script>

<style scoped>
.col {
  background: linear-gradient(180deg, var(--white-a4) 0%, var(--white-a4) 100%);
  border: 1px solid var(--line);
  border-radius: var(--r-xl); /* ~22px */
  padding: 18px;
}

.col-head {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.col-head .icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: grid;
  place-items: center;
  flex-shrink: 0;
}
.col-head .icon.green {
  background: var(--success-soft);
  color: var(--color-success);
  border: 1px solid var(--success-soft);
}
.col-head .icon.gold {
  background: var(--warning-soft);
  color: var(--color-warning);
  border: 1px solid var(--warning-soft);
}
.col-head .icon.blue {
  background: var(--accent-soft);
  color: var(--brand-cyan);
  border: 1px solid var(--accent-line);
}

.col-head-text {
  flex: 1;
  min-width: 0;
}

.col-head h3 {
  font-family: var(--font-display);
  font-size: 17px;
  font-weight: 700;
}

.col-head .sub {
  font-size: 11px;
  color: var(--ink-4);
  font-family: var(--font-mono);
  letter-spacing: 0.04em;
  margin-top: 1px;
}

.col-head .all {
  margin-left: auto;
  font-size: 12px;
  color: var(--muted-foreground);
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--line);
  transition: border 0.15s ease, color 0.15s ease;
  text-decoration: none;
  white-space: nowrap;
  flex-shrink: 0;
}
.col-head .all:hover {
  color: var(--brand-cyan);
  border-color: var(--accent-line);
}

.col-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 600px;
  overflow-y: auto;
}

/* scrollbar styling */
.col-list::-webkit-scrollbar { width: 4px; }
.col-list::-webkit-scrollbar-track { background: transparent; }
.col-list::-webkit-scrollbar-thumb { background: var(--border); border-radius: 2px; }
.col-list::-webkit-scrollbar-thumb:hover { background: var(--white-a20); }

/* Loading skeleton */
.skeleton-row {
  display: flex;
  gap: 12px;
  padding: 10px;
  animation: pulse 1.5s ease-in-out infinite;
  /* Same as .item: don't let the flex-column .col-list shrink rows. */
  flex-shrink: 0;
}
.skeleton-poster {
  width: 56px;
  aspect-ratio: 2 / 3;
  background: var(--border);
  border-radius: 8px;
  flex-shrink: 0;
}
.skeleton-body {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding-top: 4px;
}
.skeleton-line {
  height: 12px;
  background: var(--border);
  border-radius: 4px;
}
.skeleton-line.w-3\/4 { width: 75%; }
.skeleton-line.w-1\/2 { width: 50%; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
