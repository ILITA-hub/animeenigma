<template>
  <!-- Spin dock under the slider: pity bar + Drops + Pull ×1/×10.
       Ported from the v21 .dock mock. -->
  <div class="dock">
    <div class="pity">
      <div class="pity-row">
        <span class="pity-label">{{ $t('gacha.dock_pity_label') }}</span>
        <b>{{ pity }} / {{ pityThreshold }}</b>
      </div>
      <div class="bar">
        <div
          class="fill"
          :style="{ width: `${pityPercent}%` }"
          role="progressbar"
          :aria-valuenow="pity"
          :aria-valuemax="pityThreshold"
        />
      </div>
    </div>

    <div class="actions">
      <Button variant="outline" data-testid="dock-drops" @click="$emit('drops')">
        <Layers class="size-4 mr-1.5" aria-hidden="true" />
        {{ $t('gacha.dock_drops_button') }}
      </Button>
      <Button
        variant="outline"
        :disabled="loading || balance < costX1"
        data-testid="dock-pull-x1"
        @click="$emit('pull', 'x1')"
      >
        <Gem class="size-4 mr-1.5 text-orange-400" aria-hidden="true" />
        {{ balance < costX1
          ? $t('gacha.spin_insufficient', { n: costX1 })
          : `${$t('gacha.dock_pull_x1')} · ${costX1}` }}
      </Button>
      <Button
        :disabled="loading || balance < costX10"
        data-testid="dock-pull-x10"
        @click="$emit('pull', 'x10')"
      >
        <Gem class="size-4 mr-1.5" aria-hidden="true" />
        {{ balance < costX10
          ? $t('gacha.spin_insufficient', { n: costX10 })
          : `${$t('gacha.dock_pull_x10')} · ${costX10}` }}
      </Button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Gem, Layers } from 'lucide-vue-next'
import Button from '@/components/ui/Button.vue'

const props = defineProps<{
  pity: number
  pityThreshold: number
  balance: number
  costX1: number
  costX10: number
  loading?: boolean
}>()

defineEmits<{
  drops: []
  pull: [mode: 'x1' | 'x10']
}>()

const pityPercent = computed(() => {
  if (props.pityThreshold <= 0) return 0
  return Math.min(Math.round((props.pity / props.pityThreshold) * 100), 100)
})
</script>

<style scoped>
.dock {
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-top: none;
  border-radius: 0 0 1.25rem 1.25rem;
  background: rgba(22, 22, 35, 0.65);
  backdrop-filter: blur(12px);
  padding: 1.1rem 1.5rem;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 1.2rem;
  justify-content: space-between;
}
.pity {
  flex: 1;
  min-width: 230px;
}
.pity-row {
  display: flex;
  justify-content: space-between;
  font-size: 0.8rem;
}
.pity-label {
  color: var(--ink-2);
}
.bar {
  height: 6px;
  border-radius: 3px;
  background: rgba(255, 255, 255, 0.1);
  margin-top: 0.45rem;
  overflow: hidden;
}
.fill {
  height: 100%;
  background: linear-gradient(90deg, var(--brand-cyan), rgb(124, 58, 237));
  transition: width 0.4s;
}
.actions {
  display: flex;
  gap: 0.7rem;
  flex-wrap: wrap;
}
</style>
