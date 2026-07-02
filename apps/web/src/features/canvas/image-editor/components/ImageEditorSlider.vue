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
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  label: string
  value: number
  min: number
  max: number
}>()

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

function onInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:value', Math.min(props.max, Math.max(props.min, Math.round(value))))
}
</script>

<style src="../../components/styles/editing-controls.css"></style>
