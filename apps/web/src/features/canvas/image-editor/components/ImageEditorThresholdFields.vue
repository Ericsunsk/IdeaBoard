<template>
  <div class="editing-controls-fields image-editor-threshold-fields">
    <label class="editing-controls-field">
      <span class="editing-controls-label">阈值</span>
      <input
        name="image-editor-threshold"
        class="editing-controls-control editing-controls-number-input"
        type="number"
        min="0"
        max="255"
        step="1"
        inputmode="numeric"
        :value="threshold"
        @input="onThresholdInput"
      />
    </label>
    <button
      class="editing-controls-toggle-button node-tool-tooltip-trigger"
      type="button"
      :aria-pressed="invert ? 'true' : 'false'"
      aria-label="反转阈值输出"
      @click="emit('update:invert', !invert)"
    >
      <FlipReverse aria-hidden="true" />
      <span class="node-tool-tooltip" aria-hidden="true">反转</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { FlipReverse } from '@iconoir/vue'
import { clampThresholdValue } from '../utils/imageEditorThreshold'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

defineProps<{
  threshold: number
  invert: boolean
}>()

const emit = defineEmits<{
  (e: 'update:threshold', value: number): void
  (e: 'update:invert', value: boolean): void
}>()

function onThresholdInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:threshold', clampThresholdValue(value))
}
</script>
