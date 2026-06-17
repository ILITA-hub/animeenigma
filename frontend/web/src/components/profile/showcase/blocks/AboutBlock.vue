<script setup lang="ts">
import type { AboutConfig } from '@/types/showcase'

defineProps<{ config: AboutConfig }>()
</script>

<template>
  <div class="rounded-xl border border-border bg-card p-4 md:p-6">
    <h3 v-if="config.title" class="mb-2 text-lg font-semibold text-foreground">
      {{ config.title }}
    </h3>
    <p class="whitespace-pre-wrap text-sm text-muted-foreground">
      {{ config.text }}
    </p>
  </div>
</template>
