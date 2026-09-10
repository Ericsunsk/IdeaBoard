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
import type { RetouchLevelsParams } from '../model.ts'
import { createRetouchHistogram, sampleHistogramPath } from '../processing/histogram.ts'
import {
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from '../processing/channelScale.ts'
import {
  clampLevelsHandles,
  createLevelsInputConstraints,
  createLevelsMarkers,
  createLevelsOutputConstraints,
  moveLevelsHandle,
  type RetouchLevelsInputHandleId,
} from '../processing/levels.ts'
import RetouchHistogramPlot, { type RetouchHistogramPlotMarker } from './HistogramPlot.vue'
import RetouchLevelsFields from './LevelsFields.vue'
import RetouchLevelsOutputControl from './LevelsOutputControl.vue'
import './styles/retouch-histogram-controls.css'

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
const histogramPath = computed(() =>
  sampleHistogramPath(createRetouchHistogram(props.histogramSource ?? null, 'rgb'), 80),
)
const plotMarkers = computed<readonly RetouchHistogramPlotMarker[]>(() =>
  createLevelsMarkers(clampedParams.value).map((marker) => ({
    id: marker.id,
    position: retouchChannelToPlotPercent(marker.position),
    ariaLabel: marker.ariaLabel,
    ariaValueMin: marker.ariaValueMin,
    ariaValueMax: marker.ariaValueMax,
    ariaValueNow: marker.position,
    keyboardStep: retouchChannelToPlotPercent(1),
    keyboardShiftStep: retouchChannelToPlotPercent(10),
    className: `is-${marker.id}`,
    guideLine: true,
    guideLineClassName: `is-${marker.id}`,
  })),
)
const plotAriaLabel = computed(() => t('canvas.retouch.levelsHistogramAria'))

function onMarkerUpdate(markerId: string, position: number) {
  if (!isLevelsHandleId(markerId)) return
  emit(
    'update:params',
    moveLevelsHandle(clampedParams.value, markerId, retouchPlotPercentToChannel(position)),
  )
}

function isLevelsHandleId(value: string): value is RetouchLevelsInputHandleId {
  return value === 'shadows' || value === 'midtones' || value === 'highlights'
}
</script>
