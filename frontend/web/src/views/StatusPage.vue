<template>
  <div class="min-h-screen bg-base pt-20">
    <div class="container mx-auto px-4 py-8 max-w-4xl">
      <h1 class="text-3xl font-bold text-white mb-6">{{ $t('status.title') }}</h1>

      <!-- Loading -->
      <div v-if="initialLoading" class="flex justify-center py-12">
        <div class="w-8 h-8 border-2 border-cyan-500 border-t-transparent rounded-full animate-spin"></div>
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
              :class="svc.status === 'up' ? 'bg-emerald-500' : 'bg-red-500'"
            ></span>
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm font-medium">{{ $t(`status.services.${svc.name}`) }}</p>
              <p v-if="svc.error" class="text-red-400 text-xs truncate mt-0.5">{{ svc.error }}</p>
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
              :class="svc.status === 'up' ? 'bg-emerald-500' : 'bg-red-500'"
            ></span>
            <div class="flex-1 min-w-0">
              <p class="text-white text-sm font-medium">{{ $t(`status.services.${svc.name}`) }}</p>
              <p v-if="svc.error" class="text-red-400 text-xs truncate mt-0.5">{{ svc.error }}</p>
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
          <button
            class="px-3 py-1 rounded bg-white/10 hover:bg-white/20 text-white/60 hover:text-white transition-colors"
            @click="fetchStatus"
          >
            {{ $t('status.refresh') }}
          </button>
        </div>
      </template>

      <!-- Error -->
      <div v-else class="text-center py-12">
        <p class="text-red-400">{{ $t('status.down') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { statusApi } from '@/api/client'

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
  if (!status.value) return 'bg-red-500'
  switch (status.value.overall) {
    case 'operational': return 'bg-emerald-500'
    case 'degraded': return 'bg-amber-500'
    default: return 'bg-red-500'
  }
})

const overallBorderClass = computed(() => {
  if (!status.value) return 'border-l-4 border-red-500'
  switch (status.value.overall) {
    case 'operational': return 'border-l-4 border-emerald-500'
    case 'degraded': return 'border-l-4 border-amber-500'
    default: return 'border-l-4 border-red-500'
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
