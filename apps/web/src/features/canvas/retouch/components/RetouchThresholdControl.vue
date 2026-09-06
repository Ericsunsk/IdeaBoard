<template>
  <div class="editing-controls-stack retouch-threshold-control">
    <RetouchHistogramPlot
      :aria-label="t('canvas.retouch.thresholdHistogramAria')"
      :histogram-path="histogramPath"
      marker-mode="range"
      :markers="plotMarkers"
      @update-marker="onMarkerUpdate"
    />
    <RetouchThresholdFields
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
import type { RetouchThresholdParams } from '../types'
import { createRetouchHistogram, sampleHistogramPath } from '../utils/retouchHistogram'
import { retouchChannelToPlotPercent } from '../utils/retouchChannelScale'
import {
  RETOUCH_THRESHOLD_MARKER_ID,
  createThresholdMarker,
  moveThresholdHandle,
  normalizeThresholdAdjustment,
} from '../utils/retouchThreshold'
import RetouchHistogramPlot, { type RetouchHistogramPlotMarker } from './RetouchHistogramPlot.vue'
import RetouchThresholdFields from './RetouchThresholdFields.vue'
import './styles/retouch-histogram-controls.css'

const props = defineProps<{
  params: RetouchThresholdParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchThresholdParams): void
}>()

const { t } = useI18n()

const normalizedParams = computed(() => normalizeThresholdAdjustment(props.params))
const histogramPath = computed(() =>
  sampleHistogramPath(createRetouchHistogram(props.histogramSource ?? null, 'rgb'), 80),
)
const plotMarkers = computed<readonly RetouchHistogramPlotMarker[]>(() => {
  const marker = createThresholdMarker(normalizedParams.value)
  return [
    {
      id: marker.id,
      position: marker.position,
      ariaLabel: marker.ariaLabel,
      ariaValueMin: 0,
      ariaValueMax: 255,
      ariaValueNow: marker.value,
      keyboardStep: retouchChannelToPlotPercent(1),
      keyboardShiftStep: retouchChannelToPlotPercent(10),
      className: 'is-threshold',
      guideLine: true,
      guideLineClassName: 'is-threshold',
    },
  ]
})

function onMarkerUpdate(markerId: string, position: number) {
  if (markerId !== RETOUCH_THRESHOLD_MARKER_ID) return
  emit('update:params', moveThresholdHandle(normalizedParams.value, position))
}

function updateThreshold(threshold: number) {
  emit('update:params', normalizeThresholdAdjustment({ ...props.params, threshold }))
}

function updateInvert(invert: boolean) {
  emit('update:params', normalizeThresholdAdjustment({ ...props.params, invert }))
}
</script>
