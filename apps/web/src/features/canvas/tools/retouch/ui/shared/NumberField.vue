<template>
  <input
    :name="name"
    class="editing-controls-control editing-controls-number-input"
    :class="inputClass"
    type="number"
    :min="min"
    :max="max"
    :step="step"
    :inputmode="inputMode"
    :aria-label="ariaLabel"
    :aria-valuemin="ariaValuemin"
    :aria-valuemax="ariaValuemax"
    :value="value"
    :disabled="disabled"
    @input="onInput"
  />
</template>

<script setup lang="ts">
import { readFiniteNumberInputValue } from './readFiniteNumberInputValue.ts'

/*
 * Shared number-input row for retouch controls.
 * Converges the LevelsFields / LevelsOutputControl / CurvesControl / Slider
 * number inputs (row markup plus the read/null-guard/emit handler) without
 * changing behavior or a11y: optional bindings stay omitted when undefined so
 * Curves keeps its endpoint `:disabled` and `:value ?? ''` clear-without-zero
 * semantics, and label/min/max/inputmode wiring renders exactly as before.
 */

withDefaults(
  defineProps<{
    name: string
    min?: number | undefined
    max?: number | undefined
    step?: number | string | undefined
    inputMode?:
      'none' | 'text' | 'decimal' | 'numeric' | 'tel' | 'search' | 'email' | 'url' | undefined
    ariaLabel?: string | undefined
    ariaValuemin?: number | undefined
    ariaValuemax?: number | undefined
    value: number | string
    disabled?: boolean | undefined
    inputClass?: string | undefined
  }>(),
  {
    min: undefined,
    max: undefined,
    step: 1,
    inputMode: undefined,
    ariaLabel: undefined,
    ariaValuemin: undefined,
    ariaValuemax: undefined,
    disabled: false,
    inputClass: undefined,
  },
)

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

function onInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:value', value)
}
</script>
