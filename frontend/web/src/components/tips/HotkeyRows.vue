<template>
  <div
    v-for="row in rows"
    :key="row.descKey"
    class="flex items-center justify-between gap-4 py-2 border-b border-border last:border-0"
  >
    <span class="text-sm text-white/70">{{ t(row.descKey) }}</span>
    <span class="flex items-center gap-1 shrink-0">
      <template v-for="(alt, i) in row.keys" :key="i">
        <span v-if="i > 0" class="text-xs text-muted-foreground">/</span>
        <kbd
          class="inline-flex items-center justify-center min-w-7 h-7 px-2 rounded-md border border-border bg-muted font-mono text-xs text-white/90"
          >{{ alt }}</kbd
        >
      </template>
    </span>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

interface HotkeyRow {
  /** Alternative key labels rendered as <kbd>, joined by "/". */
  keys: string[]
  descKey: string
}

defineProps<{ rows: HotkeyRow[] }>()

const { t } = useI18n()
</script>
