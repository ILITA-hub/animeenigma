<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { showcaseApi } from '@/api/client'

const props = defineProps<{
  userId: string
  isOwner?: boolean
  variant?: string
}>()

interface CompatData {
  percent: number
  shared_count: number
  shared_sample: string[]
  self?: boolean
}

const compat = ref<CompatData | null>(null)

function unwrap(resp: unknown): CompatData | null {
  if (!resp || typeof resp !== 'object') return null
  const r = resp as Record<string, unknown>
  if ('data' in r && r.data && typeof r.data === 'object') {
    const d = r.data as Record<string, unknown>
    if ('percent' in d) return d as unknown as CompatData
  }
  if ('percent' in r) return r as unknown as CompatData
  return null
}

onMounted(async () => {
  if (props.isOwner) return
  try {
    const resp = await showcaseApi.getCompatibility(props.userId)
    const data = unwrap(resp.data)
    if (!data || data.self === true) return
    compat.value = data
  } catch {
    // network error — render nothing
  }
})
</script>

<template>
  <div v-if="!isOwner && compat" class="h-full space-y-3">
    <h3 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider">
      {{ $t('showcase.block.compatibility') }}
    </h3>
    <div class="flex items-center gap-5 flex-wrap">
      <!-- Ring (conic gradient injected via CSS custom property to avoid inline color literals) -->
      <div
        class="compat-ring relative flex-shrink-0 grid place-items-center rounded-full"
        style="width: 128px; height: 128px"
        :style="{ '--compat-pct': `${compat.percent}%` }"
        role="img"
        :aria-label="`${compat.percent}% ${$t('showcase.compat.match')}`"
      >
        <!-- inner surface cut-out -->
        <div class="absolute rounded-full bg-background" style="inset: 11px" />
        <!-- value -->
        <div class="relative text-center">
          <span class="compat-pct-label font-semibold text-[30px] leading-none">
            {{ compat.percent }}%
          </span>
          <span class="block text-[10px] text-muted-foreground uppercase tracking-widest mt-0.5">
            {{ $t('showcase.compat.match') }}
          </span>
        </div>
      </div>
      <!-- Side text -->
      <div class="flex-1 min-w-48">
        <h4 class="font-semibold text-[15px] mb-1">{{ $t('showcase.compat.heading') }}</h4>
        <p class="text-muted-foreground text-sm">
          {{ compat.shared_count }} {{ $t('showcase.compat.in_common') }}
        </p>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Conic ring uses CSS custom property injected by :style binding (not a color literal) */
.compat-ring {
  background: conic-gradient(
    var(--success) var(--compat-pct, 0%),
    var(--white-a8) 0
  );
}

/* Gradient text: #fff → success — allowed in scoped <style> (Rule 8 only flags inline style="") */
.compat-pct-label {
  background: linear-gradient(180deg, #fff, var(--success));
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
}
</style>
