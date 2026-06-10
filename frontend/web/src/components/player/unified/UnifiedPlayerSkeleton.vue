<template>
  <!-- Player-shaped placeholder shown while the UnifiedPlayer chunk loads, so
       selecting the AnimeEnigma tab never flashes a blank gap (the existing
       player card unmounts immediately on select). Matches the `.pl` frame:
       16:9, rounded, dark, bordered. -->
  <div
    class="relative w-full aspect-video rounded-[var(--r-xl,16px)] overflow-hidden bg-black border border-[var(--border)] grid place-items-center"
    role="status"
    :aria-label="$t('player.unified.loadingPlayer')"
    data-test="unified-player-skeleton"
  >
    <div class="absolute inset-0 pl-skeleton-shimmer" aria-hidden="true" />
    <div class="relative flex flex-col items-center gap-3">
      <Spinner size="lg" tone="signature" :label="$t('player.unified.loadingPlayer')" />
      <span class="text-sm font-medium text-[var(--muted-foreground)]">
        {{ $t('player.unified.loadingPlayer') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import Spinner from '@/components/ui/Spinner.vue'
</script>

<style scoped>
.pl-skeleton-shimmer {
  background: linear-gradient(
    100deg,
    rgba(255, 255, 255, 0) 30%,
    rgba(255, 255, 255, 0.04) 50%,
    rgba(255, 255, 255, 0) 70%
  );
  background-size: 200% 100%;
  animation: pl-skeleton-sweep 1.4s ease-in-out infinite;
}

@keyframes pl-skeleton-sweep {
  from {
    background-position: 200% 0;
  }
  to {
    background-position: -200% 0;
  }
}
</style>
