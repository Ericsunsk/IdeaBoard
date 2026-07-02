<template>
  <div class="editing-controls-stack image-editor-curves">
    <div class="editing-controls-toolbar image-editor-curves__toolbar">
      <label class="editing-controls-field image-editor-curves__select-label">
        <span class="editing-controls-label">预设</span>
        <select
          class="editing-controls-control editing-controls-select"
          :value="selectedPresetId"
          aria-label="曲线预设"
          @change="onPresetChange"
        >
          <option value="custom">自定</option>
          <option v-for="preset in curvePresets" :key="preset.id" :value="preset.id">
            {{ preset.label }}
          </option>
        </select>
      </label>
      <label class="editing-controls-field image-editor-curves__select-label">
        <span class="editing-controls-label">通道</span>
        <select
          class="editing-controls-control editing-controls-select"
          :value="activeChannel"
          aria-label="曲线通道"
          @change="onChannelChange"
        >
          <option v-for="channel in curveChannels" :key="channel.id" :value="channel.id">
            {{ channel.label }}
          </option>
        </select>
      </label>
    </div>

    <ImageEditorCurveGraph
      :active-curve-path="activeCurvePath"
      :ghost-curve-paths="ghostCurvePaths"
      :histogram-path="histogramPath"
      :points="activePoints"
      :selected-point-id="selectedPointId"
      :focus-request="focusRequest"
      @add-point="addPoint"
      @move-point="movePoint"
      @remove-point="removePoint"
      @select-point="selectPoint"
    />

    <div class="editing-controls-fields image-editor-curves__fields">
      <label class="editing-controls-field image-editor-curves__field">
        <span class="editing-controls-label">输入</span>
        <input
          class="editing-controls-control editing-controls-number-input"
          type="number"
          min="0"
          max="100"
          step="1"
          :disabled="selectedPoint?.endpoint"
          :value="selectedPoint?.input ?? ''"
          @input="onSelectedPointInputChange"
        />
      </label>
      <label class="editing-controls-field image-editor-curves__field">
        <span class="editing-controls-label">输出</span>
        <input
          class="editing-controls-control editing-controls-number-input"
          type="number"
          min="0"
          max="100"
          step="1"
          :value="selectedPoint?.output ?? ''"
          @input="onSelectedPointOutputChange"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ImageEditorCurveChannel, ImageEditorCurvesParams } from '../types'
import {
  addCurvePoint,
  applyCurvePreset,
  createIdentityCurvePoints,
  IMAGE_EDITOR_CURVE_CHANNELS,
  IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID,
  IMAGE_EDITOR_CURVE_PRESETS,
  moveCurvePoint,
  normalizeCurveChannelPoints,
  normalizeCurvesParams,
  removeCurvePoint,
  resolveCurvePresetId,
  sampleCurvePointsPath,
} from '../utils/imageEditorCurves'
import { createImageEditorHistogram, sampleHistogramPath } from '../utils/imageEditorHistogram'
import { readFiniteNumberInputValue } from '../utils/readFiniteNumberInputValue'
import ImageEditorCurveGraph from './ImageEditorCurveGraph.vue'

const props = defineProps<{
  params: ImageEditorCurvesParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: ImageEditorCurvesParams): void
}>()

const curveChannels: Array<{ id: ImageEditorCurveChannel; label: string }> = [
  { id: 'rgb', label: 'RGB' },
  { id: 'red', label: '红' },
  { id: 'green', label: '绿' },
  { id: 'blue', label: '蓝' },
]

const activeChannel = ref<ImageEditorCurveChannel>('rgb')
const selectedPointId = ref<string>(IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID)
const focusRequest = ref(0)
let pointIdSeed = 0
const curvePresets = IMAGE_EDITOR_CURVE_PRESETS

const normalizedParams = computed(() => normalizeCurvesParams(props.params))
const activePoints = computed(() => getChannelPoints(activeChannel.value))
const selectedPoint = computed(
  () => activePoints.value.find((point) => point.id === selectedPointId.value) ?? null,
)
const selectedPresetId = computed(() => resolveCurvePresetId(props.params))
const activeCurvePath = computed(() => sampleCurvePointsPath(activePoints.value))
const ghostCurvePaths = computed(() =>
  IMAGE_EDITOR_CURVE_CHANNELS.filter((channel) => channel !== activeChannel.value)
    .map((channel) => ({
      channel,
      points: normalizedParams.value.channels[channel],
    }))
    .filter((item) => item.points && item.points.length > 0)
    .map((item) => ({
      channel: item.channel,
      path: sampleCurvePointsPath(item.points),
    })),
)
const histogramPath = computed(() => {
  const histogram = createImageEditorHistogram(props.histogramSource, activeChannel.value)
  return sampleHistogramPath(histogram, 80)
})

watch(
  activePoints,
  (points) => {
    if (points.some((point) => point.id === selectedPointId.value)) return
    selectedPointId.value = resolveSelectablePointId(points)
  },
  { immediate: true },
)

function getChannelPoints(channel: ImageEditorCurveChannel) {
  return normalizedParams.value.channels[channel] ?? createIdentityCurvePoints()
}

function selectChannel(channel: ImageEditorCurveChannel) {
  activeChannel.value = channel
  selectedPointId.value = IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID
}

function onChannelChange(event: Event) {
  const target = event.target as HTMLSelectElement | null
  const channel = target?.value as ImageEditorCurveChannel | undefined
  if (!channel) return
  selectChannel(channel)
}

function selectPoint(pointId: string) {
  selectedPointId.value = pointId
}

function applyPreset(presetId: string) {
  emit('update:params', applyCurvePreset(presetId))
  selectedPointId.value = IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID
}

function onPresetChange(event: Event) {
  const target = event.target as HTMLSelectElement | null
  const presetId = target?.value
  if (!presetId || presetId === 'custom') return
  applyPreset(presetId)
}

function addPoint(point: { input: number; output: number }) {
  const pointId = createCurvePointId()
  emit(
    'update:params',
    addCurvePoint(props.params, activeChannel.value, {
      id: pointId,
      input: point.input,
      output: point.output,
    }),
  )
  selectedPointId.value = pointId
}

function movePoint(pointId: string, point: { input: number; output: number }) {
  emit('update:params', moveCurvePoint(props.params, activeChannel.value, pointId, point))
}

function moveSelectedPoint(point: { input: number; output: number }) {
  if (!selectedPoint.value) return
  movePoint(selectedPoint.value.id, point)
}

function onSelectedPointInputChange(event: Event) {
  if (!selectedPoint.value || selectedPoint.value.endpoint) return
  const input = readFiniteNumberInputValue(event)
  if (input == null) return
  moveSelectedPoint({
    input,
    output: selectedPoint.value.output,
  })
}

function onSelectedPointOutputChange(event: Event) {
  if (!selectedPoint.value) return
  const output = readFiniteNumberInputValue(event)
  if (output == null) return
  moveSelectedPoint({
    input: selectedPoint.value.input,
    output,
  })
}

function removePoint(pointId: string) {
  const previousPoints = activePoints.value
  const nextParams = removeCurvePoint(props.params, activeChannel.value, pointId)
  emit('update:params', nextParams)

  const nextPoints = normalizeCurveChannelPoints(nextParams.channels[activeChannel.value] ?? [])
  selectedPointId.value = resolveNextSelectedPointId(previousPoints, nextPoints, pointId)
  focusRequest.value += 1
}

function createCurvePointId() {
  pointIdSeed += 1
  return `point:${Date.now().toString(36)}:${pointIdSeed}`
}

function resolveSelectablePointId(points: readonly { id: string; endpoint: boolean }[]) {
  return points.find((point) => !point.endpoint)?.id ?? IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID
}

function resolveNextSelectedPointId(
  previousPoints: readonly { id: string; endpoint: boolean }[],
  nextPoints: readonly { id: string; endpoint: boolean }[],
  removedPointId: string,
) {
  const previousUserPoints = previousPoints.filter((point) => !point.endpoint)
  const removedIndex = previousUserPoints.findIndex((point) => point.id === removedPointId)
  if (removedIndex < 0) return resolveSelectablePointId(nextPoints)

  const nextUserPoints = nextPoints.filter((point) => !point.endpoint)
  return (
    nextUserPoints[removedIndex]?.id ??
    nextUserPoints[removedIndex - 1]?.id ??
    IMAGE_EDITOR_CURVE_ENDPOINT_BLACK_ID
  )
}
</script>

<style src="./styles/image-editor-curves.css"></style>
