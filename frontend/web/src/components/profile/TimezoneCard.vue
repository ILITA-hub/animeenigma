<!-- frontend/web/src/components/profile/TimezoneCard.vue -->
<!-- Account timezone: auto-set at sign-up, changed ONLY here. Affects the
     next-episode time on the anime page and the home rail chip. The schedule
     page keeps its own independent selector (useTimezonePref). -->
<template>
  <div class="glass-card p-6">
    <h2 class="text-lg font-semibold text-white mb-4">{{ $t('profile.settings.timezone.title') }}</h2>
    <p class="text-white/60 text-sm mb-4">{{ $t('profile.settings.timezone.description') }}</p>
    <div class="max-w-xs">
      <Select
        size="sm"
        :model-value="current"
        :options="options"
        :disabled="saving"
        @update:model-value="save(String($event))"
      />
    </div>
    <p v-if="saved" class="text-success text-xs mt-2">{{ $t('profile.settings.timezone.saved') }}</p>
    <p v-if="error" class="text-destructive text-xs mt-2">{{ error }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/ui/Select.vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import { TIMEZONE_CHOICES, browserTimezone, isValidTz } from '@/composables/useTimezonePref'
import { formatUtcOffset, tzOffsetMinutes } from '@/composables/schedule/timezone'

const { t } = useI18n()
const auth = useAuthStore()

const saving = ref(false)
const saved = ref(false)
const error = ref('')

const current = computed(() => {
  const tz = auth.user?.timezone
  return tz && isValidTz(tz) ? tz : browserTimezone
})

const options = computed(() => {
  const curated = [...TIMEZONE_CHOICES]
    .sort((a, b) => tzOffsetMinutes(a.value) - tzOffsetMinutes(b.value))
    .map((c) => ({ value: c.value, label: `${t('schedule.tz.cities.' + c.cityKey)} (${formatUtcOffset(c.value)})` }))
  // The auto-detected zone may not be in the curated list (e.g. Europe/Paris)
  // — surface it so the current value always has a visible option.
  if (!curated.some((o) => o.value === current.value)) {
    const city = current.value.split('/').pop()?.replace(/_/g, ' ') ?? current.value
    curated.unshift({ value: current.value, label: `${city} (${formatUtcOffset(current.value)})` })
  }
  return curated
})

async function save(tz: string) {
  if (tz === auth.user?.timezone) return
  saving.value = true
  saved.value = false
  error.value = ''
  try {
    await userApi.updateTimezone(tz)
    if (auth.user) auth.setUser({ ...auth.user, timezone: tz })
    saved.value = true
  } catch {
    error.value = t('profile.settings.timezone.error')
  } finally {
    saving.value = false
  }
}
</script>
