<template>
  <div class="editing-controls-stack retouch-curves">
    <div class="editing-controls-toolbar retouch-curves__toolbar">
      <label
        v-for="toolbarSelect in toolbarSelects"
        :key="toolbarSelect.name"
        class="editing-controls-field retouch-curves__select-label"
      >
        <span class="editing-controls-label">{{ toolbarSelect.label }}</span>
        <select
          :name="toolbarSelect.name"
          class="editing-controls-control editing-controls-select"
          :value="toolbarSelect.value"
          :aria-label="toolbarSelect.ariaLabel"
          @change="toolbarSelect.onChange"
        >
          <option v-if="toolbarSelect.customOption" :value="toolbarSelect.customOption.value">
            {{ toolbarSelect.customOption.label }}
          </option>
          <option v-for="option in toolbarSelect.options" :key="option.value" :value="option.value">
            {{ option.label }}
          </option>
        </select>
      </label>
    </div>

    <RetouchCurveGraph
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

    <div class="editing-controls-fields retouch-curves__fields">
      <label class="editing-controls-field retouch-curves__field">
        <span class="editing-controls-label">{{ t('canvas.retouch.inputLabel') }}</span>
        <NumberField
          name="retouch-curve-input"
          :min="0"
          :max="100"
          :disabled="selectedPoint?.endpoint"
          :value="selectedPoint?.input ?? ''"
          @update:value="onSelectedPointAxisChange('input', $event)"
        />
      </label>
      <label class="editing-controls-field retouch-curves__field">
        <span class="editing-controls-label">{{ t('canvas.retouch.outputLabel') }}</span>
        <NumberField
          name="retouch-curve-output"
          :min="0"
          :max="100"
          :value="selectedPoint?.output ?? ''"
          @update:value="onSelectedPointAxisChange('output', $event)"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import '../shared/styles/retouch-curves.css'
import { computed, ref, watch } from 'vue'
import type { RetouchCurveChannel, RetouchCurvesParams } from '../../core/model.ts'
import {
  addCurvePoint,
  applyCurvePreset,
  createIdentityCurvePoints,
  RETOUCH_CURVE_CHANNELS,
  RETOUCH_CURVE_ENDPOINT_BLACK_ID,
  RETOUCH_CURVE_PRESETS,
  moveCurvePoint,
  normalizeCurveChannelPoints,
  normalizeCurvesParams,
  removeCurvePoint,
  resolveCurvePresetId,
  sampleCurvePointsPath,
} from '../../core/kinds/curves.ts'
import { createRetouchHistogram, sampleHistogramPath } from '../../core/histogram.ts'
import RetouchCurveGraph from './CurveGraph.vue'
import NumberField from '../shared/NumberField.vue'

const props = defineProps<{
  params: RetouchCurvesParams
  histogramSource?: ImageData | null
}>()

const emit = defineEmits<{
  (e: 'update:params', value: RetouchCurvesParams): void
}>()

const { t } = useI18n()

const curveChannels = computed<Array<{ id: RetouchCurveChannel; label: string }>>(() => [
  { id: 'rgb', label: 'RGB' },
  { id: 'red', label: t('canvas.retouch.channels.red') },
  { id: 'green', label: t('canvas.retouch.channels.green') },
  { id: 'blue', label: t('canvas.retouch.channels.blue') },
])

// Presets carry only labelKeys; t() runs here at call time so runtime locale
// switches stay reactive (same precedent as definitions ranges / S8 rail).
const curvePresets = computed(() =>
  RETOUCH_CURVE_PRESETS.map((preset) => ({
    ...preset,
    label: t(preset.labelKey),
  })),
)

const activeChannel = ref<RetouchCurveChannel>('rgb')
const selectedPointId = ref<string>(RETOUCH_CURVE_ENDPOINT_BLACK_ID)
const focusRequest = ref(0)
let pointIdSeed = 0

type CurvesToolbarSelect = {
  name: string
  label: string
  ariaLabel: string
  value: string
  customOption?: { value: string; label: string } | undefined
  options: readonly { value: string; label: string }[]
  onChange: (event: Event) => void
}

const normalizedParams = computed(() => normalizeCurvesParams(props.params))
const activePoints = computed(() => getChannelPoints(activeChannel.value))
const selectedPoint = computed(
  () => activePoints.value.find((point) => point.id === selectedPointId.value) ?? null,
)
const selectedPresetId = computed(() => resolveCurvePresetId(props.params))
const activeCurvePath = computed(() => sampleCurvePointsPath(activePoints.value))
const ghostCurvePaths = computed(() =>
  RETOUCH_CURVE_CHANNELS.filter((channel) => channel !== activeChannel.value)
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
  const histogram = createRetouchHistogram(props.histogramSource, activeChannel.value)
  return sampleHistogramPath(histogram, 80)
})
const toolbarSelects = computed<readonly CurvesToolbarSelect[]>(() => [
  {
    name: 'retouch-curve-preset',
    label: t('canvas.retouch.presetLabel'),
    ariaLabel: t('canvas.retouch.curvesPresetAria'),
    value: selectedPresetId.value,
    customOption: { value: 'custom', label: t('canvas.retouch.customPresetOption') },
    options: curvePresets.value.map((preset) => ({ value: preset.id, label: preset.label })),
    onChange: onPresetChange,
  },
  {
    name: 'retouch-curve-channel',
    label: t('canvas.retouch.channelLabel'),
    ariaLabel: t('canvas.retouch.curvesChannelAria'),
    value: activeChannel.value,
    options: curveChannels.value.map((channel) => ({ value: channel.id, label: channel.label })),
    onChange: onChannelChange,
  },
])

watch(
  activePoints,
  (points) => {
    if (points.some((point) => point.id === selectedPointId.value)) return
    selectedPointId.value = resolveSelectablePointId(points)
  },
  { immediate: true },
)

function getChannelPoints(channel: RetouchCurveChannel) {
  return normalizedParams.value.channels[channel] ?? createIdentityCurvePoints()
}

function selectChannel(channel: RetouchCurveChannel) {
  activeChannel.value = channel
  selectedPointId.value = RETOUCH_CURVE_ENDPOINT_BLACK_ID
}

function onChannelChange(event: Event) {
  const target = event.target as HTMLSelectElement | null
  const channel = target?.value as RetouchCurveChannel | undefined
  if (!channel) return
  selectChannel(channel)
}

function selectPoint(pointId: string) {
  selectedPointId.value = pointId
}

function applyPreset(presetId: string) {
  emit('update:params', applyCurvePreset(presetId))
  selectedPointId.value = RETOUCH_CURVE_ENDPOINT_BLACK_ID
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

function onSelectedPointAxisChange(axis: 'input' | 'output', value: number) {
  const point = selectedPoint.value
  if (!point) return
  if (axis === 'input' && point.endpoint) return
  moveSelectedPoint(
    axis === 'input'
      ? { input: value, output: point.output }
      : { input: point.input, output: value },
  )
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
  return points.find((point) => !point.endpoint)?.id ?? RETOUCH_CURVE_ENDPOINT_BLACK_ID
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
    RETOUCH_CURVE_ENDPOINT_BLACK_ID
  )
}
</script>
