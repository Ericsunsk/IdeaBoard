<template>
  <div class="editing-controls-slider-row">
    <div class="editing-controls-slider-row__header">
      <span class="editing-controls-slider-row__label">{{ label }}</span>
      <NumberField
        :name="`${label}-value`"
        :min="min"
        :max="max"
        :step="normalizedStep"
        :input-mode="inputMode"
        :aria-label="t('canvas.retouch.sliderValueAria', { label })"
        :value="value"
        input-class="editing-controls-slider-row__value"
        @update:value="onNumberValue"
      />
    </div>
    <input
      :name="`${label}-range`"
      class="canvas-range-input editing-controls-slider-row__range"
      :class="rangeClass"
      type="range"
      :min="min"
      :max="max"
      :step="normalizedStep"
      :value="value"
      :aria-label="t('canvas.retouch.sliderAria', { label })"
      @input="onRangeInput"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import NumberField from './NumberField.vue'
import { readFiniteNumberInputValue } from './readFiniteNumberInputValue.ts'
import { clamp } from '../../../../../../shared/utils/number'

const props = defineProps<{
  label: string
  value: number
  min: number
  max: number
  step?: number
  trackVariant?: RetouchSliderTrackVariant
}>()

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

const { t } = useI18n()

type RetouchSliderTrackVariant = 'hue' | 'tonal' | 'tonal-reverse'

const rangeClass = computed(() => ({
  'editing-controls-slider-row__range--hue': props.trackVariant === 'hue',
  'editing-controls-slider-row__range--tonal': props.trackVariant === 'tonal',
  'editing-controls-slider-row__range--tonal-reverse': props.trackVariant === 'tonal-reverse',
}))
const normalizedStep = computed(() => normalizeStep(props.step))
const inputMode = computed(() => (normalizedStep.value >= 1 ? 'numeric' : 'decimal'))

function onRangeInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:value', normalizeValue(value, props.min, props.max, normalizedStep.value))
}

function onNumberValue(value: number) {
  emit('update:value', normalizeValue(value, props.min, props.max, normalizedStep.value))
}

function normalizeStep(step: number | undefined) {
  if (step === undefined || !Number.isFinite(step) || step <= 0) return 1
  return step
}

function normalizeValue(value: number, min: number, max: number, step: number) {
  const clamped = clamp(value, min, max)
  if (step >= 1 && Number.isInteger(step)) return Math.round(clamped)

  const offsetSteps = Math.round((clamped - min) / step)
  const stepped = min + offsetSteps * step
  return Number(clamp(stepped, min, max).toFixed(resolveDecimalPlaces(step)))
}

function resolveDecimalPlaces(step: number) {
  const [, fraction = ''] = step.toString().split('.')
  return fraction.length
}
</script>
