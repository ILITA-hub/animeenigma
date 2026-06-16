<template>
  <Tabs
    :model-value="modelValue"
    :tabs="tabs"
    :aria-label="$t('anidle.mode_tabs_label')"
    @update:model-value="emit('update:modelValue', $event as 'daily' | 'endless')"
  />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Tabs from '@/components/ui/Tabs.vue'

defineProps<{
  modelValue: 'daily' | 'endless'
}>()

const emit = defineEmits<{
  'update:modelValue': [value: 'daily' | 'endless']
}>()

const { t } = useI18n()

const tabs = computed(() => [
  { value: 'daily', label: t('anidle.mode_daily') },
  { value: 'endless', label: t('anidle.mode_endless') },
])
</script>
