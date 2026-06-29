<template>
  <div class="image-editor-levels-output" aria-label="输出色阶">
    <div class="image-editor-levels-output__header">
      <span>输出色阶</span>
    </div>
    <div
      ref="sliderRef"
      class="image-editor-levels-output__slider"
      @pointerdown="onSliderPointerDown"
    >
      <input
        class="canvas-range-input image-editor-levels-output__range"
        type="range"
        :min="constraints.outputBlack.min"
        :max="constraints.outputBlack.max"
        step="1"
        aria-label="输出黑场"
        :aria-valuemin="constraints.outputBlack.min"
        :aria-valuemax="constraints.outputBlack.max"
        :value="params.outputBlack"
        @input="onOutputInput('outputBlack', $event)"
      />
      <input
        class="canvas-range-input image-editor-levels-output__range"
        type="range"
        :min="constraints.outputWhite.min"
        :max="constraints.outputWhite.max"
        step="1"
        aria-label="输出白场"
        :aria-valuemin="constraints.outputWhite.min"
        :aria-valuemax="constraints.outputWhite.max"
        :value="params.outputWhite"
        @input="onOutputInput('outputWhite', $event)"
      />
    </div>
    <div class="image-editor-levels-output__fields">
      <label class="image-editor-histogram-control__field image-editor-levels-output__field">
        <input
          type="number"
          :min="constraints.outputBlack.min"
          :max="constraints.outputBlack.max"
          step="1"
          inputmode="numeric"
          aria-label="输出黑场"
          :aria-valuemin="constraints.outputBlack.min"
          :aria-valuemax="constraints.outputBlack.max"
          :value="params.outputBlack"
          @input="onOutputInput('outputBlack', $event)"
        />
      </label>
      <label class="image-editor-histogram-control__field image-editor-levels-output__field">
        <input
          type="number"
          :min="constraints.outputWhite.min"
          :max="constraints.outputWhite.max"
          step="1"
          inputmode="numeric"
          aria-label="输出白场"
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
import { ref } from 'vue'
import type { ImageEditorLevelsParams } from '../types'
import {
  moveLevelsOutputHandle,
  type ImageEditorLevelsConstraint,
  type ImageEditorLevelsOutputHandleId,
} from '../utils/imageEditorLevels'
import { imageEditorPlotPercentToChannel } from '../utils/imageEditorChannelScale'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'

const props = defineProps<{
  params: ImageEditorLevelsParams
  constraints: Record<ImageEditorLevelsOutputHandleId, ImageEditorLevelsConstraint>
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorLevelsParams): void
}>()

const sliderRef = ref<HTMLElement | null>(null)

function onOutputInput(handleId: ImageEditorLevelsOutputHandleId, event: Event) {
  const value = readFiniteNumberInputValue(event)
  if (value == null) return
  emit('update:params', moveLevelsOutputHandle(props.params, handleId, value))
}

function onSliderPointerDown(event: PointerEvent) {
  const value = resolveSliderValue(event)
  if (value == null) return
  const nearestHandle = resolveNearestOutputHandle(value)
  emit('update:params', moveLevelsOutputHandle(props.params, nearestHandle, value))
}

function resolveSliderValue(event: PointerEvent) {
  const rect = sliderRef.value?.getBoundingClientRect()
  if (!rect || rect.width <= 0) return null
  const position = ((event.clientX - rect.left) / rect.width) * 100
  return imageEditorPlotPercentToChannel(position)
}

function resolveNearestOutputHandle(value: number): ImageEditorLevelsOutputHandleId {
  return Math.abs(value - props.params.outputBlack) <= Math.abs(value - props.params.outputWhite)
    ? 'outputBlack'
    : 'outputWhite'
}
</script>
