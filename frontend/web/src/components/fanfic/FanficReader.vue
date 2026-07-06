<!--
  FanficReader — renders a Fanfic's markdown content as safe, selectable text.

  Consumes renderFanfic() blocks and maps them to real <h2>/<h3>/<p> elements
  via `{{ b.text }}` interpolation — NEVER `v-html`. This is the entire XSS
  defense for generated (or user-submitted) fanfic content: no HTML string
  ever reaches the DOM unescaped.

  Used both for the LIVE streaming view (GenerateForm tab, `streaming: true`
  shows a blinking caret at the end) and the saved-library reader (dialog).
-->
<script setup lang="ts">
import { computed } from 'vue'
import { renderFanfic } from './renderFanfic'

const props = defineProps<{
  title?: string
  content: string
  streaming?: boolean
}>()

const blocks = computed(() => renderFanfic(props.content))
</script>

<template>
  <article class="max-w-none select-text">
    <h1 v-if="title" class="text-2xl font-semibold text-foreground mb-4">{{ title }}</h1>
    <template v-for="(b, i) in blocks" :key="i">
      <h2 v-if="b.type === 'h2'" class="text-xl font-semibold text-foreground mt-6 mb-2">{{ b.text }}</h2>
      <h3 v-else-if="b.type === 'h3'" class="text-lg font-semibold text-foreground mt-4 mb-2">{{ b.text }}</h3>
      <p v-else class="text-muted-foreground leading-relaxed mb-3">{{ b.text }}</p>
    </template>
    <span
      v-if="streaming"
      class="inline-block w-2 h-4 bg-brand-cyan animate-pulse align-middle"
      aria-hidden="true"
    />
  </article>
</template>
