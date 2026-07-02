<template>
  <label class="editing-controls-slider-row">
    <span class="editing-controls-slider-row__label">{{ label }}</span>
    <input
      class="canvas-range-input"
      type="range"
      :min="min"
      :max="max"
      step="1"
      :value="value"
      @input="onInput"
    />
    <span class="editing-controls-slider-row__value">{{ value }}{{ unit }}</span>
  </label>
</template>

<script setup lang="ts">
defineProps<{
  label: string
  value: number
  min: number
  max: number
  unit?: string
}>()

const emit = defineEmits<{
  (e: 'update:value', value: number): void
}>()

function onInput(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update:value', Number(target?.value) || 0)
}
</script>

<style src="../../components/styles/editing-controls.css"></style>
