<template>
  <!-- Player-shaped placeholder shown while the AePlayer chunk loads, so
       selecting the AnimeEnigma tab never flashes a blank gap (the existing
       player card unmounts immediately on select). Matches the `.pl` frame:
       16:9, rounded, dark, bordered. -->
  <div
    class="relative w-full aspect-video rounded-[var(--r-xl,16px)] overflow-hidden bg-black border border-[var(--border)] grid place-items-center"
    role="status"
    :aria-label="$t('player.aePlayer.loadingPlayer')"
    data-test="ae-player-skeleton"
  >
    <div class="absolute inset-0 pl-skeleton-shimmer" aria-hidden="true" />
    <div class="relative flex flex-col items-center gap-3">
      <Spinner size="lg" tone="signature" :label="$t('player.aePlayer.loadingPlayer')" />
      <span class="text-sm font-medium text-[var(--muted-foreground)]">
        {{ $t('player.aePlayer.loadingPlayer') }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import Spinner from '@/components/ui/Spinner.vue'
</script>

<style scoped>
/* Compositor-driven sweep (transform on ::after, not background-position —
   see .sk-drift / .skeleton-shimmer in main.css). The host is
   `absolute inset-0` inside the overflow-hidden rounded frame above,
   which clips the oversized strip. */
.pl-skeleton-shimmer::after {
  content: '';
  position: absolute;
  inset: 0 auto 0 0;
  width: 300%;
  background: linear-gradient(
    100deg,
    transparent 45%,
    var(--white-a4) 50%,
    transparent 55%
  );
  animation: pl-skeleton-sweep 1.4s ease-in-out infinite;
}

@keyframes pl-skeleton-sweep {
  from {
    transform: translateX(-66.667%);
  }
  to {
    transform: translateX(0);
  }
}
</style>
