<template>
  <div class="col">
    <!-- Column header -->
    <div class="col-head">
      <div class="icon" :class="iconTone">
        <!-- green: flame / play icon -->
        <template v-if="iconTone === 'green'">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M14.752 11.168l-3.197-2.132A1 1 0 0010 9.87v4.263a1 1 0 001.555.832l3.197-2.132a1 1 0 000-1.664z" />
            <path d="M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
          </svg>
        </template>
        <!-- gold: star/trophy icon -->
        <template v-else-if="iconTone === 'gold'">
          <svg width="18" height="18" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
            <path d="M9.049 2.927c.3-.921 1.603-.921 1.902 0l1.07 3.292a1 1 0 00.95.69h3.462c.969 0 1.371 1.24.588 1.81l-2.8 2.034a1 1 0 00-.364 1.118l1.07 3.292c.3.921-.755 1.688-1.54 1.118l-2.8-2.034a1 1 0 00-1.175 0l-2.8 2.034c-.784.57-1.838-.197-1.539-1.118l1.07-3.292a1 1 0 00-.364-1.118L2.98 8.72c-.783-.57-.38-1.81.588-1.81h3.461a1 1 0 00.951-.69l1.07-3.292z" />
          </svg>
        </template>
        <!-- blue: calendar/announcement icon -->
        <template v-else>
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z" />
          </svg>
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
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.025) 0%, rgba(255, 255, 255, 0.01) 100%);
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
  background: rgba(0, 255, 157, 0.12);
  color: var(--color-success);
  border: 1px solid rgba(0, 255, 157, 0.2);
}
.col-head .icon.gold {
  background: rgba(255, 214, 0, 0.12);
  color: var(--color-warning);
  border: 1px solid rgba(255, 214, 0, 0.2);
}
.col-head .icon.blue {
  background: rgba(0, 212, 255, 0.12);
  color: var(--accent);
  border: 1px solid var(--accent-line);
}

.col-head-text {
  flex: 1;
  min-width: 0;
}

.col-head h3 {
  font-family: var(--f-display);
  font-size: 17px;
  font-weight: 700;
}

.col-head .sub {
  font-size: 11px;
  color: var(--ink-4);
  font-family: var(--f-mono);
  letter-spacing: 0.04em;
  margin-top: 1px;
}

.col-head .all {
  margin-left: auto;
  font-size: 12px;
  color: var(--ink-3);
  padding: 6px 10px;
  border-radius: 8px;
  border: 1px solid var(--line);
  transition: border 0.15s ease, color 0.15s ease;
  text-decoration: none;
  white-space: nowrap;
  flex-shrink: 0;
}
.col-head .all:hover {
  color: var(--accent);
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
.col-list::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.1); border-radius: 2px; }
.col-list::-webkit-scrollbar-thumb:hover { background: rgba(255, 255, 255, 0.2); }

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
  background: rgba(255, 255, 255, 0.1);
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
  background: rgba(255, 255, 255, 0.1);
  border-radius: 4px;
}
.skeleton-line.w-3\/4 { width: 75%; }
.skeleton-line.w-1\/2 { width: 50%; }

@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}
</style>
