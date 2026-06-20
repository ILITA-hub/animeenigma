<template>
  <div class="pl-wt-btn" data-test="wt-launch">
    <PlayerIconButton
      :disabled="disabled || loading"
      :title="t('watch_together.invite_button_label')"
      :aria-label="t('watch_together.invite_button_label')"
      @click="emit('launch')"
    >
      <Spinner v-if="loading" size="sm" tone="mono" aria-hidden="true" />
      <!-- Users icon -->
      <Users v-else class="size-5" aria-hidden="true" />
    </PlayerIconButton>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Users } from 'lucide-vue-next'
import { Spinner } from '@/components/ui'
import PlayerIconButton from '../PlayerIconButton.vue'

// In-player "Watch Together" launcher. Presentational only: emits `launch` and
// lets AePlayer create the room from the live source. AePlayer hides this button
// entirely when already inside a room or for anonymous users, and disables it
// until a usable source has resolved (`disabled`) / while the create-room
// request is in flight (`loading`).
defineProps<{
  disabled?: boolean
  loading?: boolean
}>()

const emit = defineEmits<{ (e: 'launch'): void }>()

const { t } = useI18n()
</script>

<style scoped>
.pl-wt-btn {
  position: relative;
  display: inline-flex;
}
</style>
