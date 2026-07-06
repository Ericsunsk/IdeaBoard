<template>
  <div class="editing-controls-slider-row">
    <div class="editing-controls-slider-row__header">
      <span class="editing-controls-slider-row__label">{{ label }}</span>
      <input
        class="editing-controls-control editing-controls-number-input editing-controls-slider-row__value"
        type="number"
        :min="min"
        :max="max"
        :step="normalizedStep"
        :inputmode="inputMode"
        :aria-label="`${label}数值`"
        :value="value"
        @input="onInput"
      />
    </div>
    <input
      class="canvas-range-input editing-controls-slider-row__range"
      :class="rangeClass"
      type="range"
      :min="min"
      :max="max"
      :step="normalizedStep"
      :value="value"
      :aria-label="`${label}滑块`"
      @input="onInput"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

type ImageEditorSliderTrackVariant = 'hue' | 'tonal' | 'tonal-reverse'

const props = defineProps<{
  label: string
  value: number
  min: number
  max: number
  step?: number
  trackVariant?: ImageEditorSliderTrackVariant
}>()

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

const rangeClass = computed(() => ({
  'editing-controls-slider-row__range--hue': props.trackVariant === 'hue',
  'editing-controls-slider-row__range--tonal': props.trackVariant === 'tonal',
  'editing-controls-slider-row__range--tonal-reverse': props.trackVariant === 'tonal-reverse',
}))
const normalizedStep = computed(() => normalizeStep(props.step))
const inputMode = computed(() => (normalizedStep.value >= 1 ? 'numeric' : 'decimal'))

function onInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:value', normalizeValue(value, props.min, props.max, normalizedStep.value))
}

function normalizeStep(step: number | undefined) {
  if (step === undefined || !Number.isFinite(step) || step <= 0) return 1
  return step
}

function normalizeValue(value: number, min: number, max: number, step: number) {
  const clamped = Math.min(max, Math.max(min, value))
  if (step >= 1 && Number.isInteger(step)) return Math.round(clamped)

  const offsetSteps = Math.round((clamped - min) / step)
  const stepped = min + offsetSteps * step
  return Number(Math.min(max, Math.max(min, stepped)).toFixed(resolveDecimalPlaces(step)))
}

function resolveDecimalPlaces(step: number) {
  const [, fraction = ''] = step.toString().split('.')
  return fraction.length
}
</script>
