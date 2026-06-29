<template>
  <div class="pl-next" data-test="next-episode-card">
    <p class="pl-next-label">{{ $t('player.aePlayer.upNext') }}</p>

    <div class="pl-next-body">
      <!-- Thumbnail -->
      <div
        class="pl-next-thumb"
        :style="stillUrl ? { backgroundImage: `url(${stillUrl})` } : {}"
        aria-hidden="true"
      />

      <div class="min-w-0">
        <p class="pl-next-ep">{{ $t('player.aePlayer.epAbbrev') }} {{ nextEp }}</p>
        <p class="pl-next-title">{{ title }}</p>
      </div>
    </div>

    <!-- Countdown indicator -->
    <div v-if="countdown !== undefined && countdown > 0" class="pl-next-countdown" aria-live="polite">
      {{ $t('player.aePlayer.playingIn', { n: countdown }) }}
    </div>

    <div class="pl-next-actions">
      <button class="pl-next-play-btn" data-test="next-play" @click="emit('play')">
        <!-- Play icon -->
        <Play class="size-[14px]" aria-hidden="true" />
        {{ $t('player.aePlayer.playNow') }}
      </button>
      <button class="pl-next-cancel" data-test="next-cancel" @click="emit('cancel')">
        {{ $t('common.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Play } from 'lucide-vue-next'

defineProps<{
  nextEp: number
  title: string
  stillUrl?: string
  countdown?: number
}>()

const emit = defineEmits<{
  (e: 'play'): void
  (e: 'cancel'): void
}>()
</script>

<style scoped>
.pl-next {
  position: absolute;
  right: 22px;
  bottom: 96px;
  z-index: 6;
  width: 300px;
  padding: 14px;
  border-radius: var(--r-lg);
  background: var(--scrim-bg-strong);
  border: 1px solid var(--border);
  backdrop-filter: blur(10px);
  box-shadow: 0 16px 40px var(--black-a60);
}

.pl-next-label {
  font-size: 12px;
  font-weight: 600;
  color: var(--brand-cyan);
  margin: 0 0 10px;
}

.pl-next-body {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.pl-next-thumb {
  width: 76px;
  height: 44px;
  border-radius: 8px;
  flex-shrink: 0;
  background: var(--white-a8);
  background-size: cover;
  background-position: center;
}

.pl-next-ep {
  font-size: 12px;
  color: var(--muted-foreground);
  margin: 0 0 2px;
}

.pl-next-title {
  font-size: 14px;
  font-weight: 600;
  color: #fff;
  margin: 0;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.pl-next-countdown {
  font-size: 12px;
  color: var(--muted-foreground);
  margin-bottom: 10px;
}

.pl-next-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.pl-next-play-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 14px;
  border-radius: var(--r-sm);
  background: var(--brand-cyan);
  border: 0;
  color: var(--color-base, #08080f);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  transition: opacity 0.15s;
}

.pl-next-play-btn:hover {
  opacity: 0.88;
}

.pl-next-cancel {
  background: transparent;
  border: 0;
  color: var(--muted-foreground);
  font-size: 13px;
  cursor: pointer;
  transition: color 0.15s;
}

.pl-next-cancel:hover {
  color: #fff;
}
</style>
