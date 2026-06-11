<template>
  <div class="flex gap-2.5 items-end min-w-0">
    <span
      class="tg-avatar w-7 h-7 rounded-full grid place-items-center flex-shrink-0 text-white"
      aria-hidden="true"
    >
      <Send class="w-3.5 h-3.5" />
    </span>
    <div
      class="bg-white/[0.07] border border-white/10 rounded-2xl rounded-bl-[4px] px-3.5 py-2.5 min-w-0"
    >
      <slot />
      <span
        v-if="time"
        class="float-right font-mono text-[10px] text-muted-foreground mt-1 ml-3"
        >{{ time }}</span
      >
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * Spotlight UI primitive (v4 D-4 lock, 2026-06-11) — a Telegram-style
 * chat bubble: channel avatar, bubble with tail (asymmetric bottom-left
 * radius), trailing time stamp. Used by TelegramNewsCard's post list.
 */
import { Send } from 'lucide-vue-next'

withDefaults(defineProps<{ time?: string }>(), { time: '' })
</script>

<style scoped>
/* Telegram brand-blue gradient — brand identity color, not a DS token
   (allowlisted in design-system-allowlist.txt). */
.tg-avatar {
  background: linear-gradient(135deg, #2aabee, #229ed9);
}
</style>
