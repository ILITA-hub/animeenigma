<template>
  <div
    class="ae-stepper inline-flex items-center gap-0.5 rounded-[var(--r-md)] p-0.5"
    data-test="stepper"
  >
    <button
      type="button"
      class="ae-stepper-btn"
      :aria-label="`Decrease ${label}`"
      data-test="stepper-dec"
      @click="bump(-1)"
      @mousedown.prevent
    >−</button>
    <div class="ae-stepper-field inline-flex items-baseline rounded-[var(--r-sm)] px-2">
      <input
        type="number"
        :value="modelValue"
        :step="step"
        :min="min"
        :max="max"
        class="ae-stepper-input text-right text-[14px] text-white border-0 bg-transparent py-[5px] focus:outline-none"
        :style="{ width: inputWidth }"
        :aria-label="label"
        data-test="stepper-input"
        @change="onChange"
      />
      <span v-if="suffix" class="text-[12px] text-[var(--muted-foreground)] ml-px">{{ suffix }}</span>
    </div>
    <button
      type="button"
      class="ae-stepper-btn"
      :aria-label="`Increase ${label}`"
      data-test="stepper-inc"
      @click="bump(1)"
      @mousedown.prevent
    >+</button>
  </div>
</template>

<script setup lang="ts">
/**
 * Stepper — numeric −/value/+ input (design-system primitive).
 * Values are rounded to the step's decimal precision and clamped to min/max.
 */
const props = withDefaults(
  defineProps<{
    modelValue: number
    step?: number
    min?: number
    max?: number
    /** unit shown after the number, e.g. "s" or "%" */
    suffix?: string
    /** accessible name for the input and ± buttons */
    label?: string
    inputWidth?: string
  }>(),
  { step: 1, min: undefined, max: undefined, suffix: undefined, label: 'value', inputWidth: '46px' },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: number): void
}>()

// Decimal places implied by the step (0.1 → 1, 0.25 → 2, 1 → 0)
function precision(): number {
  const s = String(props.step)
  const dot = s.indexOf('.')
  return dot === -1 ? 0 : s.length - dot - 1
}

function normalize(v: number): number {
  const p = 10 ** precision()
  let next = Math.round(v * p) / p
  if (props.min !== undefined) next = Math.max(props.min, next)
  if (props.max !== undefined) next = Math.min(props.max, next)
  return next
}

function bump(dir: 1 | -1) {
  emit('update:modelValue', normalize(props.modelValue + dir * props.step))
}

function onChange(e: Event) {
  const val = parseFloat((e.target as HTMLInputElement).value)
  if (!isNaN(val)) emit('update:modelValue', normalize(val))
}
</script>

<style scoped>
.ae-stepper {
  background: var(--line);
}

.ae-stepper-btn {
  width: 26px;
  height: 26px;
  border: 0;
  border-radius: var(--r-sm);
  background: var(--white-a8);
  color: white;
  font-size: 16px;
  line-height: 1;
  cursor: pointer;
  transition: color 0.15s;
}

.ae-stepper-btn:hover {
  color: var(--brand-cyan);
}

.ae-stepper-field {
  background: var(--black-a40);
}

.ae-stepper-input {
  -moz-appearance: textfield;
  appearance: textfield;
}

.ae-stepper-input::-webkit-outer-spin-button,
.ae-stepper-input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
</style>
