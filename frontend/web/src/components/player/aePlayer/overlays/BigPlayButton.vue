<template>
  <button
    v-if="visible"
    class="pl-bigplay"
    :aria-label="$t('player.aePlayer.play')"
    data-test="bigplay"
    @click="emit('play')"
  >
    <!-- Play triangle -->
    <Play class="size-9" aria-hidden="true" />
  </button>
</template>

<script setup lang="ts">
import { Play } from 'lucide-vue-next'

defineProps<{
  visible: boolean
}>()

const emit = defineEmits<{
  (e: 'play'): void
}>()
</script>

<style scoped>
.pl-bigplay {
  position: absolute;
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  z-index: 5;
  width: 88px;
  height: 88px;
  border-radius: 50%;
  background: var(--cyan-a20);
  backdrop-filter: blur(8px);
  border: 1px solid var(--accent-line);
  color: #fff;
  display: grid;
  place-items: center;
  padding-left: 5px;
  box-shadow: var(--glow-cyan);
  transition: transform 0.2s;
  cursor: pointer;
}

.pl-bigplay:hover {
  transform: translate(-50%, -50%) scale(1.08);
}
</style>
