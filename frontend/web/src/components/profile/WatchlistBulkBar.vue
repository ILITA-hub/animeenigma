<template>
  <div class="fixed inset-x-0 bottom-4 z-50 flex justify-center px-4 pointer-events-none">
    <div class="pointer-events-auto flex items-center gap-3 rounded-full border border-white/10 bg-popover/95 backdrop-blur-xl px-4 py-2 shadow-xl shadow-black/30">
      <span class="text-sm font-medium text-white whitespace-nowrap">{{ $t('profile.bulk.selected', { n: count }) }}</span>

      <div class="w-40">
        <Select
          :model-value="''"
          :options="statusOptions"
          size="sm"
          :placeholder="$t('profile.bulk.setStatus')"
          @update:model-value="onStatus"
        />
      </div>

      <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive" @click="emit('remove')">
        <Trash2 class="size-4" />
        <span>{{ $t('profile.bulk.remove') }}</span>
      </Button>

      <Button variant="ghost" size="sm" class="text-muted-foreground" @click="emit('clear')">
        {{ $t('profile.bulk.clear') }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Trash2 } from 'lucide-vue-next'
import Button from '@/components/ui/Button.vue'
import Select from '@/components/ui/Select.vue'
import type { SelectOption } from '@/components/ui/Select.vue'

defineProps<{
  count: number
  statusOptions: SelectOption[]
}>()

const emit = defineEmits<{
  'set-status': [status: string]
  remove: []
  clear: []
}>()

function onStatus(v: string | number) {
  const s = String(v)
  if (s) emit('set-status', s)
}
</script>
