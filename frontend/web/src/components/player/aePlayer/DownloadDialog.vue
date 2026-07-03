<template>
  <div class="dl-dialog" role="dialog" :aria-label="$t('player.aePlayer.offline.quality')">
    <div class="dl-title text-sm font-semibold">{{ $t('player.aePlayer.offline.quality') }}</div>
    <div class="dl-est text-muted-foreground">{{ $t('player.aePlayer.offline.estimate', { size: SIZE_HINT[quality] }) }}</div>
    <div class="dl-opts">
      <button
        v-for="q in QUALITIES"
        :key="q"
        type="button"
        class="dl-opt"
        :class="{ 'dl-opt-active': q === quality }"
        @click="quality = q"
      >{{ q }}p</button>
    </div>
    <div class="dl-actions">
      <button type="button" class="dl-btn dl-btn-primary font-medium" @click="confirm">
        {{ $t('player.aePlayer.offline.start') }}
      </button>
      <button type="button" class="dl-btn font-medium" @click="emit('close')">
        {{ $t('player.aePlayer.offline.cancel') }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const QUALITIES = ['480', '720', '1080'] as const
const SIZE_HINT: Record<string, string> = { '480': '250 MB', '720': '450 MB', '1080': '900 MB' }
const LS_KEY = 'ae.downloadQuality'

const emit = defineEmits<{
  (e: 'confirm', quality: string): void
  (e: 'close'): void
}>()

const saved = localStorage.getItem(LS_KEY)
const quality = ref<string>(saved && (QUALITIES as readonly string[]).includes(saved) ? saved : '720')

function confirm() {
  localStorage.setItem(LS_KEY, quality.value)
  emit('confirm', quality.value)
}
</script>

<style scoped>
.dl-dialog {
  position: absolute;
  inset-inline: 0;
  bottom: 4rem;
  margin-inline: auto;
  width: 16rem;
  padding: 0.75rem;
  border-radius: 0.5rem;
  background: var(--scrim-bg-strong);
  border: 1px solid var(--white-a8);
  z-index: 30;
}
.dl-title { margin-bottom: 0.25rem; }
.dl-est { font-size: 0.75rem; margin-bottom: 0.5rem; }
.dl-opts { display: flex; gap: 0.5rem; margin-bottom: 0.75rem; }
.dl-opt {
  flex: 1;
  padding: 0.25rem 0;
  border-radius: 0.375rem;
  border: 1px solid var(--white-a8);
  color: var(--color-muted-foreground, currentColor);
}
.dl-opt-active { border-color: var(--brand-cyan); color: var(--brand-cyan); }
.dl-actions { display: flex; gap: 0.5rem; }
.dl-btn { flex: 1; padding: 0.375rem 0; border-radius: 0.375rem; border: 1px solid var(--white-a8); }
.dl-btn-primary { border-color: var(--brand-cyan); color: var(--brand-cyan); }
</style>
