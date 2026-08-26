<template>
  <div class="image-editor-levels-output" :aria-label="t('canvas.imageEditor.outputLevelsLabel')">
    <div class="editing-controls-label image-editor-levels-output__header">
      <span>{{ t("canvas.imageEditor.outputLevelsLabel") }}</span>
    </div>
    <EditingTwoHandleSlider
      class="image-editor-levels-output__slider"
      :aria-label="t('canvas.imageEditor.outputLevelsRangeAria')"
      :min="IMAGE_EDITOR_CHANNEL_MIN"
      :max="IMAGE_EDITOR_CHANNEL_MAX"
      :step="1"
      :shift-step="10"
      :handles="outputSliderHandles"
      @update:handle="onOutputSliderHandleUpdate"
    />
    <div class="editing-controls-fields image-editor-levels-output__fields">
      <label
        class="editing-controls-field editing-controls-field--compact image-editor-levels-output__field"
      >
        <input
          name="image-editor-output-black"
          class="editing-controls-control editing-controls-number-input"
          type="number"
          :min="constraints.outputBlack.min"
          :max="constraints.outputBlack.max"
          step="1"
          inputmode="numeric"
          :aria-label="t('canvas.imageEditor.params.outputBlack')"
          :aria-valuemin="constraints.outputBlack.min"
          :aria-valuemax="constraints.outputBlack.max"
          :value="params.outputBlack"
          @input="onOutputInput('outputBlack', $event)"
        />
      </label>
      <label
        class="editing-controls-field editing-controls-field--compact image-editor-levels-output__field"
      >
        <input
          name="image-editor-output-white"
          class="editing-controls-control editing-controls-number-input"
          type="number"
          :min="constraints.outputWhite.min"
          :max="constraints.outputWhite.max"
          step="1"
          inputmode="numeric"
          :aria-label="t('canvas.imageEditor.params.outputWhite')"
          :aria-valuemin="constraints.outputWhite.min"
          :aria-valuemax="constraints.outputWhite.max"
          :value="params.outputWhite"
          @input="onOutputInput('outputWhite', $event)"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import EditingTwoHandleSlider from '../../components/EditingTwoHandleSlider.vue'
import type { ImageEditorLevelsParams } from '../types'
import {
  moveLevelsOutputHandle,
  type ImageEditorLevelsConstraint,
  type ImageEditorLevelsOutputHandleId,
} from '../utils/imageEditorLevels'
import {
  IMAGE_EDITOR_CHANNEL_MAX,
  IMAGE_EDITOR_CHANNEL_MIN,
} from '../utils/imageEditorChannelScale'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  params: ImageEditorLevelsParams
  constraints: Record<ImageEditorLevelsOutputHandleId, ImageEditorLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorLevelsParams): void
}>()

const { t } = useI18n()

const outputSliderHandles = computed(() => [
  {
    id: 'outputBlack',
    value: props.params.outputBlack,
    min: props.constraints.outputBlack.min,
    max: props.constraints.outputBlack.max,
    ariaLabel: t('canvas.imageEditor.params.outputBlack'),
  },
  {
    id: 'outputWhite',
    value: props.params.outputWhite,
    min: props.constraints.outputWhite.min,
    max: props.constraints.outputWhite.max,
    ariaLabel: t('canvas.imageEditor.params.outputWhite'),
  },
])

function onOutputInput(handleId: ImageEditorLevelsOutputHandleId, event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:params', moveLevelsOutputHandle(props.params, handleId, value))
}

function onOutputSliderHandleUpdate(handleId: string, value: number) {
  if (!isOutputHandleId(handleId)) return
  emit('update:params', moveLevelsOutputHandle(props.params, handleId, value))
}

function isOutputHandleId(handleId: string): handleId is ImageEditorLevelsOutputHandleId {
  return handleId === 'outputBlack' || handleId === 'outputWhite'
}
</script>
