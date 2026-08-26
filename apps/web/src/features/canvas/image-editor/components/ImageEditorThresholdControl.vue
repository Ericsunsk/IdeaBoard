<template>
  <div class="editing-controls-stack image-editor-threshold-control">
    <ImageEditorHistogramPlot
      :aria-label="t('canvas.imageEditor.thresholdHistogramAria')"
      :histogram-path="histogramPath"
      marker-mode="range"
      :markers="plotMarkers"
      @update-marker="onMarkerUpdate"
    />
    <ImageEditorThresholdFields
      :threshold="normalizedParams.threshold"
      :invert="normalizedParams.invert"
      @update:threshold="updateThreshold"
      @update:invert="updateInvert"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import type { ImageEditorThresholdParams } from '../types'
import { createImageEditorHistogram, sampleHistogramPath } from '../utils/imageEditorHistogram'
import { imageEditorChannelToPlotPercent } from '../utils/imageEditorChannelScale'
import {
  IMAGE_EDITOR_THRESHOLD_MARKER_ID,
  createThresholdMarker,
  moveThresholdHandle,
  normalizeThresholdAdjustment,
} from '../utils/imageEditorThreshold'
import ImageEditorHistogramPlot, {
  type ImageEditorHistogramPlotMarker,
} from './ImageEditorHistogramPlot.vue'
import ImageEditorThresholdFields from './ImageEditorThresholdFields.vue'
import './styles/image-editor-histogram-controls.css'

const props = defineProps<{
  params: ImageEditorThresholdParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorThresholdParams): void
}>()

const { t } = useI18n()

const normalizedParams = computed(() => normalizeThresholdAdjustment(props.params))
const histogramPath = computed(() =>
  sampleHistogramPath(createImageEditorHistogram(props.histogramSource ?? null, 'rgb'), 80),
)
const plotMarkers = computed<readonly ImageEditorHistogramPlotMarker[]>(() => {
  const marker = createThresholdMarker(normalizedParams.value)
  return [
    {
      id: marker.id,
      position: marker.position,
      ariaLabel: marker.ariaLabel,
      ariaValueMin: 0,
      ariaValueMax: 255,
      ariaValueNow: marker.value,
      keyboardStep: imageEditorChannelToPlotPercent(1),
      keyboardShiftStep: imageEditorChannelToPlotPercent(10),
      className: 'is-threshold',
      guideLine: true,
      guideLineClassName: 'is-threshold',
    },
  ]
})

function onMarkerUpdate(markerId: string, position: number) {
  if (markerId !== IMAGE_EDITOR_THRESHOLD_MARKER_ID) return
  emit('update:params', moveThresholdHandle(normalizedParams.value, position))
}

function updateThreshold(threshold: number) {
  emit('update:params', normalizeThresholdAdjustment({ ...props.params, threshold }))
}

function updateInvert(invert: boolean) {
  emit('update:params', normalizeThresholdAdjustment({ ...props.params, invert }))
}
</script>
