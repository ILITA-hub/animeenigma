<template>
  <div
    v-if="visible"
    class="pl-buffering"
    data-test="buffering-overlay"
  >
    <Spinner size="xl" label="Buffering" />
  </div>
</template>

<script setup lang="ts">
import Spinner from '@/components/ui/Spinner.vue'

defineProps<{ visible: boolean }>()
</script>

<style scoped>
.pl-buffering {
  position: absolute;
  inset: 0;
  z-index: 4;
  display: grid;
  place-items: center;
  pointer-events: none;
}
</style>
