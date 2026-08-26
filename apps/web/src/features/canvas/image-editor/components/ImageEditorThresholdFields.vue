<template>
  <div class="editing-controls-fields image-editor-threshold-fields">
    <label class="editing-controls-field">
      <span class="editing-controls-label">{{ t("canvas.imageEditor.params.threshold") }}</span>
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
      :aria-label="t('canvas.imageEditor.invertThresholdOutput')"
      @click="emit('update:invert', !invert)"
    >
      <FlipReverse aria-hidden="true" />
      <span class="node-tool-tooltip" aria-hidden="true">{{ t("canvas.imageEditor.invertTooltip") }}</span>
    </button>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
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

const { t } = useI18n()

function onThresholdInput(event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:threshold', clampThresholdValue(value))
}
</script>
