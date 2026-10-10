<template>
  <div class="editing-controls-stack retouch-levels-control">
    <RetouchHistogramPlot
      :aria-label="plotAriaLabel"
      :histogram-path="histogramPath"
      :markers="plotMarkers"
      @update-marker="onMarkerUpdate"
    />
    <RetouchLevelsFields
      :params="clampedParams"
      :constraints="inputConstraints"
      @update:params="emit('update:params', $event)"
    />
    <RetouchLevelsOutputControl
      :params="clampedParams"
      :constraints="outputConstraints"
      @update:params="emit('update:params', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { computed } from 'vue'
import type { RetouchLevelsParams } from '../../core/model.ts'
import {
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from '../../core/channelScale.ts'
import {
  clampLevelsHandles,
  createLevelsInputConstraints,
  createLevelsMarkers,
  createLevelsOutputConstraints,
  moveLevelsHandle,
  type RetouchLevelsInputHandleId,
} from '../../core/kinds/levels.ts'
import RetouchHistogramPlot, {
  type RetouchHistogramPlotMarker,
} from '../histogram/HistogramPlot.vue'
import RetouchLevelsFields from './LevelsFields.vue'
import RetouchLevelsOutputControl from './LevelsOutputControl.vue'
import {
  createHistogramMarkerBase,
  createHistogramMarkerUpdater,
  useRgbHistogramPath,
} from '../shared/histogramPlot.ts'
import '../shared/styles/retouch-histogram-controls.css'

const props = defineProps<{
  params: RetouchLevelsParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchLevelsParams): void
}>()

const { t } = useI18n()

const clampedParams = computed(() => clampLevelsHandles(props.params))
const inputConstraints = computed(() => createLevelsInputConstraints(clampedParams.value))
const outputConstraints = computed(() => createLevelsOutputConstraints(clampedParams.value))
const histogramPath = useRgbHistogramPath(() => props.histogramSource)
const plotMarkers = computed<readonly RetouchHistogramPlotMarker[]>(() =>
  createLevelsMarkers(clampedParams.value).map((marker) => ({
    id: marker.id,
    position: retouchChannelToPlotPercent(marker.position),
    // Markers carry only labelKeys; t() runs here at call time so locale
    // switches re-translate (same precedent as definitions ranges / S8 rail).
    ariaLabel: t(marker.labelKey, { value: marker.position }),
    ariaValueMin: marker.ariaValueMin,
    ariaValueMax: marker.ariaValueMax,
    ariaValueNow: marker.position,
    ...createHistogramMarkerBase(),
    className: `is-${marker.id}`,
    guideLineClassName: `is-${marker.id}`,
  })),
)
const plotAriaLabel = computed(() => t('canvas.retouch.levelsHistogramAria'))

const onMarkerUpdate = createHistogramMarkerUpdater(
  isLevelsHandleId,
  // Raw passthrough: moveLevelsHandle clamps through clampLevelsHandles
  // itself (the same function storage normalization uses), so feeding the
  // clamped display value here would normalize twice.
  (markerId, position) =>
    moveLevelsHandle(
      props.params,
      markerId as RetouchLevelsInputHandleId,
      retouchPlotPercentToChannel(position),
    ),
  (nextParams) => emit('update:params', nextParams),
)

function isLevelsHandleId(value: string): value is RetouchLevelsInputHandleId {
  return value === 'shadows' || value === 'midtones' || value === 'highlights'
}
</script>
