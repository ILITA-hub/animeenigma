<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-5xl">
      <!-- Header -->
      <div class="mb-6">
        <h1 class="text-3xl font-semibold text-white">{{ $t('admin.secretFeatures.title') }}</h1>
        <p class="text-white/60 text-sm mt-1">{{ $t('admin.secretFeatures.subtitle') }}</p>
      </div>

      <!-- Error -->
      <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.error403') }}</p>
      </div>
      <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <template v-else>
        <!-- Master switch -->
        <div class="glass-card p-4 md:p-6 mb-6 flex items-center justify-between gap-4">
          <div>
            <h2 class="text-base font-semibold text-white">{{ $t('admin.secretFeatures.rouletteLabel') }}</h2>
            <p class="text-white/60 text-sm mt-1">{{ $t('admin.secretFeatures.rouletteHint') }}</p>
          </div>
          <Switch
            :model-value="config.rouletteEnabled"
            :disabled="saving"
            :aria-label="$t('admin.secretFeatures.rouletteLabel')"
            @update:model-value="onToggleRoulette"
          />
        </div>

        <!-- Features table -->
        <div class="glass-card overflow-x-auto">
          <table class="w-full text-sm text-white">
            <thead class="bg-black/40 backdrop-blur">
              <tr class="text-white/70 text-xs uppercase">
                <th scope="col" class="px-4 py-3 text-left">{{ $t('admin.secretFeatures.colFeature') }}</th>
                <th scope="col" class="px-4 py-3 text-left">{{ $t('admin.secretFeatures.colLink') }}</th>
                <th scope="col" class="px-4 py-3 text-center">{{ $t('admin.secretFeatures.colEnabled') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="f in features"
                :key="f.key"
                class="border-t border-white/10 hover:bg-white/5"
                :class="{ 'opacity-50': !config.rouletteEnabled }"
              >
                <td class="px-4 py-3 font-medium">{{ $t(f.labelKey) }}</td>
                <td class="px-4 py-3">
                  <router-link
                    :to="f.to"
                    class="inline-flex items-center gap-1 text-brand-cyan hover:underline font-mono text-xs"
                  >
                    {{ displayPath(f) }}
                    <ExternalLink class="size-3" aria-hidden="true" />
                  </router-link>
                </td>
                <td class="px-4 py-3 text-center">
                  <Switch
                    :model-value="featureEnabled(f.key)"
                    :disabled="saving || !config.rouletteEnabled"
                    :aria-label="$t(f.labelKey)"
                    @update:model-value="(v: boolean) => onToggleFeature(f.key, v)"
                  />
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <p class="text-white/40 text-xs mt-4">{{ $t('admin.secretFeatures.note') }}</p>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ExternalLink } from 'lucide-vue-next'
import { Spinner, Switch } from '@/components/ui'
import { adminApi, type SecretFeatureConfig } from '@/api/client'
import { SECRET_FEATURES, secretFeatureDisplayPath, type SecretFeature } from '@/utils/secretFeatures'

// The feature roster is the frontend's source of truth (utils/secretFeatures.ts);
// the backend stores only admin on/off overrides. Here we render the roster and
// overlay the resolved state.
const features = SECRET_FEATURES

const config = ref<SecretFeatureConfig>({ rouletteEnabled: true, features: {} })
const isLoading = ref(true)
const saving = ref(false)
const error = ref<string | null>(null)

function displayPath(f: SecretFeature): string {
  return secretFeatureDisplayPath(f)
}

// Absent key ⇒ enabled (default true), matching the backend's fail-open model.
function featureEnabled(key: string): boolean {
  return config.value.features[key] ?? true
}

function handleError(e: unknown): void {
  const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  error.value = err.response?.status === 403
    ? '403'
    : (err.response?.data?.error?.message || err.message || 'Failed')
}

async function load(): Promise<void> {
  isLoading.value = true
  error.value = null
  try {
    const res = await adminApi.getSecretFeatures()
    config.value = res.data.data
  } catch (e) {
    handleError(e)
  } finally {
    isLoading.value = false
  }
}

async function onToggleRoulette(enabled: boolean): Promise<void> {
  saving.value = true
  error.value = null
  try {
    const res = await adminApi.setSecretRoulette(enabled)
    config.value = res.data.data
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

async function onToggleFeature(key: string, enabled: boolean): Promise<void> {
  saving.value = true
  error.value = null
  try {
    const res = await adminApi.setSecretFeature(key, enabled)
    config.value = res.data.data
  } catch (e) {
    handleError(e)
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>
