<template>
  <div class="editing-controls-slider-row">
    <div class="editing-controls-slider-row__header">
      <span class="editing-controls-slider-row__label">{{ label }}</span>
      <input
        class="editing-controls-control editing-controls-number-input editing-controls-slider-row__value"
        type="number"
        :min="min"
        :max="max"
        step="1"
        inputmode="numeric"
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
      step="1"
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

function onInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:value', Math.min(props.max, Math.max(props.min, Math.round(value))))
}
</script>

<style src="../../components/styles/editing-controls.css"></style>
