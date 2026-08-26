<template>
  <div class="editing-controls-stack image-editor-levels-control">
    <ImageEditorHistogramPlot
      :aria-label="plotAriaLabel"
      :histogram-path="histogramPath"
      :markers="plotMarkers"
      @update-marker="onMarkerUpdate"
    />
    <ImageEditorLevelsFields
      :params="clampedParams"
      :constraints="inputConstraints"
      @update:params="emit('update:params', $event)"
    />
    <ImageEditorLevelsOutputControl
      :params="clampedParams"
      :constraints="outputConstraints"
      @update:params="emit('update:params', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import type { ImageEditorLevelsParams } from '../types'
import { createImageEditorHistogram, sampleHistogramPath } from '../utils/imageEditorHistogram'
import {
  imageEditorChannelToPlotPercent,
  imageEditorPlotPercentToChannel,
} from '../utils/imageEditorChannelScale'
import {
  clampLevelsHandles,
  createLevelsInputConstraints,
  createLevelsMarkers,
  createLevelsOutputConstraints,
  moveLevelsHandle,
  type ImageEditorLevelsInputHandleId,
} from '../utils/imageEditorLevels'
import ImageEditorHistogramPlot, {
  type ImageEditorHistogramPlotMarker,
} from './ImageEditorHistogramPlot.vue'
import ImageEditorLevelsFields from './ImageEditorLevelsFields.vue'
import ImageEditorLevelsOutputControl from './ImageEditorLevelsOutputControl.vue'
import './styles/image-editor-histogram-controls.css'

const props = defineProps<{
  params: ImageEditorLevelsParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorLevelsParams): void
}>()

const { t } = useI18n()

const clampedParams = computed(() => clampLevelsHandles(props.params))
const inputConstraints = computed(() => createLevelsInputConstraints(clampedParams.value))
const outputConstraints = computed(() => createLevelsOutputConstraints(clampedParams.value))
const histogramPath = computed(() =>
  sampleHistogramPath(createImageEditorHistogram(props.histogramSource ?? null, 'rgb'), 80),
)
const plotMarkers = computed<readonly ImageEditorHistogramPlotMarker[]>(() =>
  createLevelsMarkers(clampedParams.value).map((marker) => ({
    id: marker.id,
    position: imageEditorChannelToPlotPercent(marker.position),
    ariaLabel: marker.ariaLabel,
    ariaValueMin: marker.ariaValueMin,
    ariaValueMax: marker.ariaValueMax,
    ariaValueNow: marker.position,
    keyboardStep: imageEditorChannelToPlotPercent(1),
    keyboardShiftStep: imageEditorChannelToPlotPercent(10),
    className: `is-${marker.id}`,
    guideLine: true,
    guideLineClassName: `is-${marker.id}`,
  })),
)
const plotAriaLabel = computed(() => t('canvas.imageEditor.levelsHistogramAria'))

function onMarkerUpdate(markerId: string, position: number) {
  if (!isLevelsHandleId(markerId)) return
  emit(
    'update:params',
    moveLevelsHandle(clampedParams.value, markerId, imageEditorPlotPercentToChannel(position)),
  )
}

function isLevelsHandleId(value: string): value is ImageEditorLevelsInputHandleId {
  return value === 'shadows' || value === 'midtones' || value === 'highlights'
}
</script>
