<template>
  <div
    ref="plotRef"
    class="image-editor-histogram-plot"
    role="group"
    :aria-label="ariaLabel || t('canvas.imageEditor.histogramControlsAria')"
    @pointerdown="onPlotPointerDown"
  >
    <div class="image-editor-histogram-plot__canvas" aria-hidden="true">
      <svg viewBox="0 0 100 100" focusable="false">
        <path
          v-if="histogramPath"
          class="image-editor-histogram-plot__histogram"
          :d="histogramPath"
        />
        <path
          class="image-editor-histogram-plot__grid image-editor-histogram-plot__grid--fine"
          :d="fineGridPath"
        />
        <path
          class="image-editor-histogram-plot__grid image-editor-histogram-plot__grid--strong"
          :d="strongGridPath"
        />
        <line
          v-for="marker in guideLineMarkers"
          :key="`${marker.id}-guide-line`"
          class="image-editor-histogram-plot__guide-line"
          :class="marker.guideLineClassName"
          :x1="resolveMarkerPosition(marker)"
          y1="0"
          :x2="resolveMarkerPosition(marker)"
          y2="100"
        />
      </svg>
    </div>

    <template v-if="rangeMarker">
      <input
        name="image-editor-histogram-range"
        class="image-editor-histogram-plot__range-input"
        type="range"
        :min="resolveRangeMin(rangeMarker)"
        :max="resolveRangeMax(rangeMarker)"
        :step="resolveRangeStep(rangeMarker)"
        :value="resolveRangeValue(rangeMarker)"
        :aria-label="rangeMarker.ariaLabel"
        :aria-valuemin="resolveRangeMin(rangeMarker)"
        :aria-valuemax="resolveRangeMax(rangeMarker)"
        :aria-valuenow="resolveRangeValue(rangeMarker)"
        :aria-valuetext="rangeMarker.ariaValueText"
        @input="onRangeMarkerInput(rangeMarker, $event)"
        @focus="selectMarker(rangeMarker.id)"
        @pointerdown.stop="selectMarker(rangeMarker.id)"
      />
      <span
        class="image-editor-histogram-plot__marker image-editor-histogram-plot__marker--range"
        :class="[rangeMarker.className, { active: isMarkerActive(rangeMarker) }]"
        :style="{ left: `${resolveMarkerPosition(rangeMarker)}%` }"
        aria-hidden="true"
      />
    </template>

    <button
      v-for="marker in buttonMarkers"
      :key="marker.id"
      :ref="(element) => setMarkerHandleRef(marker.id, element)"
      type="button"
      class="image-editor-histogram-plot__marker"
      role="slider"
      :class="[marker.className, { active: isMarkerActive(marker) }]"
      :data-marker-id="marker.id"
      :style="{ left: `${resolveMarkerPosition(marker)}%` }"
      :aria-label="marker.ariaLabel"
      aria-orientation="horizontal"
      :aria-valuemin="marker.ariaValueMin ?? 0"
      :aria-valuemax="marker.ariaValueMax ?? 100"
      :aria-valuenow="resolveMarkerAriaValue(marker)"
      :aria-valuetext="marker.ariaValueText"
      @pointerdown.stop="onMarkerPointerDown(marker.id, $event)"
      @keydown="onMarkerKeydown(marker, $event)"
      @focus="selectMarker(marker.id)"
    />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import './styles/image-editor-histogram-plot.css'
import { computed, nextTick, ref } from 'vue'
import { usePointerDragSession } from '../../utils/interaction/usePointerDragSession'

const props = withDefaults(
  defineProps<{
    histogramPath: string
    markers: readonly ImageEditorHistogramPlotMarker[]
    markerMode?: MarkerMode
    ariaLabel?: string
  }>(),
  {
    markerMode: 'button',
    ariaLabel: '',
  },
)

const emit = defineEmits<{
  (e: 'update-marker', markerId: string, position: number): void
}>()

const { t } = useI18n()

export type ImageEditorHistogramPlotMarker = {
  id: string
  position: number
  ariaLabel: string
  className?: string
  ariaValueMin?: number
  ariaValueMax?: number
  ariaValueNow?: number
  ariaValueText?: string
  keyboardStep?: number
  keyboardShiftStep?: number
  guideLine?: boolean
  guideLineClassName?: string
}

type MarkerMode = 'button' | 'range'
type PointerPosition = Pick<PointerEvent, 'clientX'>

const plotRef = ref<HTMLElement | null>(null)
const activeMarkerId = ref<string | null>(null)
const selectedMarkerId = ref<string | null>(null)
const markerHandleRefs = new Map<string, HTMLButtonElement>()
const pointerDragSession = usePointerDragSession<void>({
  onMove: ({ event }) => onDragPointerMove(event),
  onEnd: clearMarkerDrag,
  onCancel: clearMarkerDrag,
})
const rangeMarker = computed(() =>
  props.markerMode === 'range' && props.markers.length === 1 ? props.markers[0] : null,
)
const buttonMarkers = computed(() => (rangeMarker.value ? [] : props.markers))
const guideLineMarkers = computed(() => props.markers.filter((marker) => marker.guideLine === true))
const fineGridPath = createGridPath([25, 75])
const strongGridPath = createGridPath([50])

function createGridPath(values: number[]) {
  return values
    .flatMap((value) => [`M ${value} 0 L ${value} 100`, `M 0 ${value} L 100 ${value}`])
    .join(' ')
}

function onPlotPointerDown(event: PointerEvent) {
  if (rangeMarker.value) return
  const nearestMarker = resolveNearestMarker(event)
  if (!nearestMarker) return
  startMarkerDrag(nearestMarker.id, event)
}

function onMarkerPointerDown(markerId: string, event: PointerEvent) {
  startMarkerDrag(markerId, event)
}

function startMarkerDrag(markerId: string, event: PointerEvent) {
  event.preventDefault()
  selectMarker(markerId)
  focusMarkerHandle(markerId)
  pointerDragSession.beginDrag(event, undefined, { target: plotRef.value })
  activeMarkerId.value = markerId
  updateMarkerFromPointerPosition(markerId, event)
}

function onDragPointerMove(event: PointerEvent) {
  if (!activeMarkerId.value) return
  updateMarkerFromPointerPosition(activeMarkerId.value, event)
}

function clearMarkerDrag() {
  activeMarkerId.value = null
}

function updateMarkerFromPointerPosition(markerId: string, position: PointerPosition) {
  const plotPosition = resolvePlotPosition(position)
  if (plotPosition == null) return
  emit('update-marker', markerId, plotPosition)
}

function onMarkerKeydown(marker: ImageEditorHistogramPlotMarker, event: KeyboardEvent) {
  const nextPosition = resolveKeyboardPosition(marker, event)
  if (nextPosition == null) return
  event.preventDefault()
  event.stopPropagation()
  selectMarker(marker.id)
  emit('update-marker', marker.id, nextPosition)
}

function onRangeMarkerInput(marker: ImageEditorHistogramPlotMarker, event: Event) {
  const input = event.currentTarget
  if (!(input instanceof HTMLInputElement)) return
  const rangeValue = Number(input.value)
  if (!Number.isFinite(rangeValue)) return
  selectMarker(marker.id)
  emit('update-marker', marker.id, resolveRangeValuePosition(marker, rangeValue))
}

function setMarkerHandleRef(markerId: string, element: unknown) {
  if (element instanceof HTMLButtonElement) {
    markerHandleRefs.set(markerId, element)
    return
  }
  markerHandleRefs.delete(markerId)
}

function focusMarkerHandle(markerId: string) {
  void nextTick(() => {
    markerHandleRefs.get(markerId)?.focus()
  })
}

function selectMarker(markerId: string) {
  selectedMarkerId.value = markerId
}

function isMarkerActive(marker: ImageEditorHistogramPlotMarker) {
  return marker.id === selectedMarkerId.value || marker.id === activeMarkerId.value
}

function resolveMarkerPosition(marker: ImageEditorHistogramPlotMarker) {
  return clampPercent(marker.position)
}

function resolveMarkerAriaValue(marker: ImageEditorHistogramPlotMarker) {
  return marker.ariaValueNow ?? resolveMarkerPosition(marker)
}

function resolveRangeMin(marker: ImageEditorHistogramPlotMarker) {
  return marker.ariaValueMin ?? 0
}

function resolveRangeMax(marker: ImageEditorHistogramPlotMarker) {
  return marker.ariaValueMax ?? 100
}

function resolveRangeValue(marker: ImageEditorHistogramPlotMarker) {
  return resolveMarkerAriaValue(marker)
}

function resolveRangeStep(marker: ImageEditorHistogramPlotMarker) {
  if (
    isFiniteNumber(marker.ariaValueMin) &&
    isFiniteNumber(marker.ariaValueMax) &&
    isFiniteNumber(marker.ariaValueNow)
  ) {
    return 1
  }

  return marker.keyboardStep ?? 1
}

function resolveRangeValuePosition(marker: ImageEditorHistogramPlotMarker, value: number) {
  const currentValue = marker.ariaValueNow
  const unitStep = marker.keyboardStep
  if (isFiniteNumber(currentValue) && isFiniteNumber(unitStep)) {
    return clampNumber(
      marker.position + (value - currentValue) * unitStep,
      resolveMarkerPositionBounds(marker).min,
      resolveMarkerPositionBounds(marker).max,
    )
  }

  const minValue = marker.ariaValueMin
  const maxValue = marker.ariaValueMax
  if (isFiniteNumber(minValue) && isFiniteNumber(maxValue) && maxValue > minValue) {
    return clampPercent(((value - minValue) / (maxValue - minValue)) * 100)
  }

  return clampPercent(value)
}

function resolveKeyboardPosition(marker: ImageEditorHistogramPlotMarker, event: KeyboardEvent) {
  const bounds = resolveMarkerPositionBounds(marker)

  switch (event.key) {
    case 'Home':
      return bounds.min
    case 'End':
      return bounds.max
    default: {
      const step = event.shiftKey ? (marker.keyboardShiftStep ?? 10) : (marker.keyboardStep ?? 1)
      const delta = resolveKeyboardDelta(event.key, step)
      if (delta == null) return null
      return clampNumber(marker.position + delta, bounds.min, bounds.max)
    }
  }
}

function resolveKeyboardDelta(key: string, step: number) {
  switch (key) {
    case 'ArrowLeft':
    case 'ArrowDown':
      return -step
    case 'ArrowRight':
    case 'ArrowUp':
      return step
    default:
      return null
  }
}

function resolveMarkerPositionBounds(marker: ImageEditorHistogramPlotMarker) {
  const min = resolveMarkerBoundPosition(marker, marker.ariaValueMin, 0)
  const max = resolveMarkerBoundPosition(marker, marker.ariaValueMax, 100)

  return {
    min: Math.min(min, max),
    max: Math.max(min, max),
  }
}

function resolveMarkerBoundPosition(
  marker: ImageEditorHistogramPlotMarker,
  value: number | undefined,
  fallback: number,
) {
  const currentValue = marker.ariaValueNow
  const unitStep = marker.keyboardStep
  if (isFiniteNumber(value) && isFiniteNumber(currentValue) && isFiniteNumber(unitStep)) {
    return clampPercent(marker.position + (value - currentValue) * unitStep)
  }

  const minValue = marker.ariaValueMin
  const maxValue = marker.ariaValueMax
  if (
    isFiniteNumber(value) &&
    isFiniteNumber(minValue) &&
    isFiniteNumber(maxValue) &&
    maxValue > minValue
  ) {
    return clampPercent(((value - minValue) / (maxValue - minValue)) * 100)
  }

  return clampPercent(fallback)
}

function resolvePlotPosition(position: PointerPosition) {
  const rect = plotRef.value?.getBoundingClientRect()
  if (!rect || rect.width <= 0) return null
  return clampPercent(((position.clientX - rect.left) / rect.width) * 100)
}

function resolveNearestMarker(position: PointerPosition) {
  const plotPosition = resolvePlotPosition(position)
  if (plotPosition == null || props.markers.length === 0) return null
  return props.markers.reduce((nearest, marker) =>
    Math.abs(marker.position - plotPosition) < Math.abs(nearest.position - plotPosition)
      ? marker
      : nearest,
  )
}

function clampPercent(value: number) {
  return clampNumber(value, 0, 100)
}

function clampNumber(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

function isFiniteNumber(value: unknown): value is number {
  return typeof value === 'number' && Number.isFinite(value)
}
</script>
