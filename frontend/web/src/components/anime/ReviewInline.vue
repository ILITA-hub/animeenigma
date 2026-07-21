<!-- ReviewInline — one line of InlineTokens as text nodes. Spoiler tokens are
     <button> chips until revealed (per-token key handled by the parent). -->
<template>
  <template v-for="(t, ti) in tokens" :key="ti">
    <strong v-if="t.kind === 'bold'" class="font-semibold text-white/90">{{ t.text }}</strong>
    <em v-else-if="t.kind === 'italic'">{{ t.text }}</em>
    <s v-else-if="t.kind === 'strike'" class="text-white/50">{{ t.text }}</s>
    <span
      v-else-if="t.kind === 'spoiler' && revealed.has(`${block}:${ti}`)"
      class="rounded bg-white/10 px-1"
      >{{ t.text }}</span
    >
    <button
      v-else-if="t.kind === 'spoiler'"
      type="button"
      class="mx-0.5 rounded bg-white/10 px-2 py-0.5 text-xs text-white/60 hover:bg-white/15 hover:text-white/80 transition-colors align-baseline"
      :aria-label="$t('anime.reviewFmt.spoilerReveal')"
      @click="emit('reveal', `${block}:${ti}`)"
    >
      {{ $t('anime.reviewFmt.spoiler') }}
    </button>
    <template v-else>{{ t.text }}</template>
  </template>
</template>

<script setup lang="ts">
import type { InlineToken } from '@/utils/reviewMarkdown'

defineProps<{ tokens: InlineToken[]; revealed: Set<string>; block: string }>()
const emit = defineEmits<{ reveal: [key: string] }>()
</script>
