<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <h1 class="text-3xl font-semibold text-white mb-6">{{ $t('status.title') }}</h1>

      <!-- Loading -->
      <div v-if="initialLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <template v-else-if="status">
        <!-- Overall status banner -->
        <div class="glass-card p-4 mb-8 flex items-center gap-3" :class="overallBorderClass">
          <span class="w-3 h-3 rounded-full flex-shrink-0" :class="overallDotClass"></span>
          <span class="text-white font-medium">{{ $t(`status.${status.overall}`) }}</span>
        </div>

        <!-- Application Services -->
        <h2 class="text-lg font-semibold text-white/80 mb-4">{{ $t('status.appServices') }}</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 mb-8">
          <div
            v-for="svc in appServices"
            :key="svc.name"
            class="glass-card p-4 flex items-center gap-3"
          >
            <span
              class="w-2.5 h-2.5 rounded-full flex-shrink-0"
              :class="svc.status === 'up' ? 'bg-success' : 'bg-destructive'"
            ></span>
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm font-medium">{{ $t(`status.services.${svc.name}`) }}</p>
              <p v-if="svc.error" class="text-destructive text-xs truncate mt-0.5">{{ svc.error }}</p>
            </div>
            <span class="text-white/40 text-xs flex-shrink-0">
              {{ svc.status === 'up' ? `${svc.response_time_ms}ms` : $t('status.serviceDown') }}
            </span>
          </div>
        </div>

        <!-- Infrastructure -->
        <h2 class="text-lg font-semibold text-white/80 mb-4">{{ $t('status.infrastructure') }}</h2>
        <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3 mb-8">
          <div
            v-for="svc in infraServices"
            :key="svc.name"
            class="glass-card p-4 flex items-center gap-3"
          >
            <span
              class="w-2.5 h-2.5 rounded-full flex-shrink-0"
              :class="svc.status === 'up' ? 'bg-success' : 'bg-destructive'"
            ></span>
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm font-medium">{{ $t(`status.services.${svc.name}`) }}</p>
              <p v-if="svc.error" class="text-destructive text-xs truncate mt-0.5">{{ svc.error }}</p>
            </div>
            <span class="text-white/40 text-xs flex-shrink-0">
              {{ svc.status === 'up' ? `${svc.response_time_ms}ms` : $t('status.serviceDown') }}
            </span>
          </div>
        </div>

        <!-- Footer info -->
        <div class="flex items-center justify-between text-white/40 text-xs">
          <p>
            {{ $t('status.lastChecked', { time: checkedAtFormatted }) }}
            <span class="ml-2">{{ $t('status.autoRefresh') }}</span>
          </p>
          <Button
            variant="soft"
            size="sm"
            @click="fetchStatus"
          >
            {{ $t('status.refresh') }}
          </Button>
        </div>
      </template>

      <!-- Error -->
      <div v-else class="text-center py-12">
        <p class="text-destructive">{{ $t('status.down') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { statusApi } from '@/api/client'
import Button from '@/components/ui/Button.vue'
import { Spinner } from '@/components/ui'

const { locale } = useI18n()

interface ServiceStatus {
  name: string
  status: string
  response_time_ms: number
  category: string
  error?: string
}

interface StatusData {
  services: ServiceStatus[]
  overall: string
  checked_at: string
}

const status = ref<StatusData | null>(null)
const initialLoading = ref(true)
let refreshInterval: ReturnType<typeof setInterval> | null = null

const appServices = computed(() =>
  status.value?.services.filter(s => s.category === 'app') ?? []
)

const infraServices = computed(() =>
  status.value?.services.filter(s => s.category === 'infra') ?? []
)

const overallDotClass = computed(() => {
  if (!status.value) return 'bg-destructive'
  switch (status.value.overall) {
    case 'operational': return 'bg-success'
    case 'degraded': return 'bg-warning'
    default: return 'bg-destructive'
  }
})

const overallBorderClass = computed(() => {
  if (!status.value) return 'border-l-4 border-destructive'
  switch (status.value.overall) {
    case 'operational': return 'border-l-4 border-success'
    case 'degraded': return 'border-l-4 border-warning'
    default: return 'border-l-4 border-destructive'
  }
})

const checkedAtFormatted = computed(() => {
  if (!status.value?.checked_at) return ''
  return new Date(status.value.checked_at).toLocaleTimeString(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
})

async function fetchStatus() {
  try {
    const res = await statusApi.getStatus()
    status.value = res.data.data
  } catch {
    // Keep previous data on refresh failure
    if (initialLoading.value) {
      status.value = null
    }
  } finally {
    initialLoading.value = false
  }
}

onMounted(() => {
  fetchStatus()
  refreshInterval = setInterval(fetchStatus, 30000)
})

onUnmounted(() => {
  if (refreshInterval) clearInterval(refreshInterval)
})
</script>
