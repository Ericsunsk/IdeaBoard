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
import type { RetouchThresholdParams } from '../../core/model.ts'
import {
  RETOUCH_THRESHOLD_MARKER_ID,
  createThresholdMarker,
  moveThresholdHandle,
  normalizeThresholdEffect,
} from '../../core/kinds/threshold.ts'
import RetouchHistogramPlot, {
  type RetouchHistogramPlotMarker,
} from '../histogram/HistogramPlot.vue'
import RetouchThresholdFields from './ThresholdFields.vue'
import {
  createHistogramMarkerBase,
  createHistogramMarkerUpdater,
  useRgbHistogramPath,
} from '../shared/histogramPlot.ts'
import '../shared/styles/retouch-histogram-controls.css'

const props = defineProps<{
  params: RetouchThresholdParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchThresholdParams): void
}>()

const { t } = useI18n()

const normalizedParams = computed(() => normalizeThresholdEffect(props.params))
const histogramPath = useRgbHistogramPath(() => props.histogramSource)
const plotMarkers = computed<readonly RetouchHistogramPlotMarker[]>(() => {
  const marker = createThresholdMarker(normalizedParams.value)
  return [
    {
      id: marker.id,
      position: marker.position,
      // Marker carries only a labelKey; t() runs here at call time so locale
      // switches re-translate (same precedent as definitions ranges / S8 rail).
      ariaLabel: t(marker.labelKey, { value: marker.value }),
      ariaValueMin: 0,
      ariaValueMax: 255,
      ariaValueNow: marker.value,
      ...createHistogramMarkerBase(),
      className: 'is-threshold',
      guideLineClassName: 'is-threshold',
    },
  ]
})

const onMarkerUpdate = createHistogramMarkerUpdater(
  (markerId) => markerId === RETOUCH_THRESHOLD_MARKER_ID,
  // Raw passthrough: the marker position is already channel-clamped by
  // retouchPlotPercentToChannel, and storage normalization lives in
  // layerStack (definitions.normalize), not in the emit path.
  (_markerId, position) => moveThresholdHandle(props.params, position),
  (nextParams) => emit('update:params', nextParams),
)

function updateThreshold(threshold: number) {
  emit('update:params', { ...props.params, threshold })
}

function updateInvert(invert: boolean) {
  emit('update:params', { ...props.params, invert })
}
</script>
