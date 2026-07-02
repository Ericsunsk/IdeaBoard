<template>
  <div class="editing-controls-fields image-editor-levels-fields">
    <label
      v-for="field in levelFields"
      :key="field.key"
      class="editing-controls-field editing-controls-field--compact image-editor-levels-fields__field"
    >
      <input
        class="editing-controls-control editing-controls-number-input"
        type="number"
        :min="constraints[field.key].min"
        :max="constraints[field.key].max"
        step="1"
        inputmode="numeric"
        :aria-label="field.ariaLabel"
        :aria-valuemin="constraints[field.key].min"
        :aria-valuemax="constraints[field.key].max"
        :value="params[field.key]"
        @input="onLevelInput(field.key, $event)"
      />
    </label>
  </div>
</template>

<script setup lang="ts">
import type { ImageEditorLevelsParams } from '../types'
import {
  moveLevelsHandle,
  type ImageEditorLevelsConstraint,
  type ImageEditorLevelsInputHandleId,
} from '../utils/imageEditorLevels'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  params: ImageEditorLevelsParams
  constraints: Record<ImageEditorLevelsInputHandleId, ImageEditorLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorLevelsParams): void
}>()

const levelFields = [
  { key: 'shadows', ariaLabel: '黑场' },
  { key: 'midtones', ariaLabel: '中点' },
  { key: 'highlights', ariaLabel: '白场' },
] satisfies Array<{
  key: ImageEditorLevelsInputHandleId
  ariaLabel: string
}>

function onLevelInput(key: ImageEditorLevelsInputHandleId, event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:params', moveLevelsHandle(props.params, key, value))
}
</script>
