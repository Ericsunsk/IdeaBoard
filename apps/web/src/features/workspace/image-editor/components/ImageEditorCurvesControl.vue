<template>
  <div class="image-editor-curves">
    <div class="image-editor-curves__toolbar">
      <label class="image-editor-curves__select-label">
        <span>预设</span>
        <select value="custom" aria-label="曲线预设" disabled>
          <option value="custom">自定</option>
        </select>
      </label>
      <label class="image-editor-curves__select-label">
        <span>通道</span>
        <select value="rgb" aria-label="曲线通道" disabled>
          <option value="rgb">RGB</option>
        </select>
      </label>
    </div>

    <div
      ref="graphRef"
      class="image-editor-curves__graph"
      role="slider"
      tabindex="0"
      aria-label="曲线控制点"
      :aria-valuetext="`输入 ${inputValue}%，输出 ${outputValue}%`"
      :aria-valuemin="ranges.input.min"
      :aria-valuemax="ranges.input.max"
      :aria-valuenow="outputValue"
      @pointerdown="onGraphPointerDown"
      @keydown="onGraphKeydown"
    >
      <svg viewBox="0 0 100 100" aria-hidden="true" focusable="false">
        <defs>
          <linearGradient :id="gradientId" x1="0" y1="1" x2="1" y2="0">
            <stop offset="0" stop-color="var(--workspace-text-subtle)" stop-opacity="0.26" />
            <stop offset="1" stop-color="var(--workspace-accent)" stop-opacity="0.9" />
          </linearGradient>
        </defs>
        <path class="image-editor-curves__grid image-editor-curves__grid--fine" :d="fineGridPath" />
        <path class="image-editor-curves__grid image-editor-curves__grid--strong" :d="strongGridPath" />
        <path class="image-editor-curves__baseline" d="M 0 100 L 100 0" />
        <path class="image-editor-curves__curve-shadow" :d="curvePath" />
        <path class="image-editor-curves__curve" :d="curvePath" :stroke="`url(#${gradientId})`" />
        <circle
          class="image-editor-curves__point"
          :cx="pointPosition.x"
          :cy="pointPosition.y"
          r="3.5"
        />
      </svg>
    </div>

    <div class="image-editor-curves__fields">
      <label>
        <span>输入</span>
        <input
          type="number"
          :min="ranges.input.min"
          :max="ranges.input.max"
          step="1"
          :value="inputValue"
          @input="onInputValueChange"
        />
      </label>
      <label>
        <span>输出</span>
        <input
          type="number"
          :min="ranges.output.min"
          :max="ranges.output.max"
          step="1"
          :value="outputValue"
          @input="onOutputValueChange"
        />
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId } from 'vue'
import { IMAGE_ADJUSTMENT_DEFINITIONS } from '../utils/imageAdjustmentDefinitions'

const props = withDefaults(
  defineProps<{
    inputValue: number
    outputValue: number
    ranges?: typeof IMAGE_ADJUSTMENT_DEFINITIONS.curves.ranges
  }>(),
  {
    ranges: () => IMAGE_ADJUSTMENT_DEFINITIONS.curves.ranges,
  },
)

const emit = defineEmits<{
  (e: 'update:input-value', value: number): void
  (e: 'update:output-value', value: number): void
}>()

const graphRef = ref<HTMLElement | null>(null)
const gradientId = `image-editor-curves-gradient-${useId()}`
const fineGridPath = createGridPath([25, 75])
const strongGridPath = createGridPath([50])
const ranges = computed(() => props.ranges)
const inputValue = computed(() => clampToRange(props.inputValue, ranges.value.input))
const outputValue = computed(() => clampToRange(props.outputValue, ranges.value.output))
const pointPosition = computed(() => ({
  x: inputValue.value,
  y: 100 - outputValue.value,
}))
const curvePath = computed(
  () => `M 0 100 L ${pointPosition.value.x} ${pointPosition.value.y} L 100 0`,
)

function createGridPath(values: number[]) {
  return values
    .flatMap((value) => [`M ${value} 0 L ${value} 100`, `M 0 ${value} L 100 ${value}`])
    .join(' ')
}

function clampToRange(
  value: number,
  range: typeof IMAGE_ADJUSTMENT_DEFINITIONS.curves.ranges.input,
) {
  if (!Number.isFinite(value)) return range.defaultValue
  return Math.min(range.max, Math.max(range.min, Math.round(value)))
}

function updatePointFromEvent(event: PointerEvent) {
  const rect = graphRef.value?.getBoundingClientRect()
  if (!rect) return
  const x = ((event.clientX - rect.left) / rect.width) * 100
  const y = ((event.clientY - rect.top) / rect.height) * 100
  emit('update:input-value', clampToRange(x, ranges.value.input))
  emit('update:output-value', clampToRange(100 - y, ranges.value.output))
}

function stopGraphDrag(pointerId: number) {
  const graph = graphRef.value
  if (graph?.hasPointerCapture(pointerId)) graph.releasePointerCapture(pointerId)
  window.removeEventListener('pointermove', updatePointFromEvent)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerCancel)
}

function onGraphPointerDown(event: PointerEvent) {
  event.preventDefault()
  graphRef.value?.setPointerCapture(event.pointerId)
  updatePointFromEvent(event)
  window.addEventListener('pointermove', updatePointFromEvent)
  window.addEventListener('pointerup', onWindowPointerUp)
  window.addEventListener('pointercancel', onWindowPointerCancel)
}

function onWindowPointerUp(event: PointerEvent) {
  stopGraphDrag(event.pointerId)
}

function onWindowPointerCancel(event: PointerEvent) {
  stopGraphDrag(event.pointerId)
}

function onGraphKeydown(event: KeyboardEvent) {
  const step = event.shiftKey ? 10 : 1
  if (event.key === 'ArrowLeft') {
    event.preventDefault()
    emit('update:input-value', clampToRange(inputValue.value - step, ranges.value.input))
  } else if (event.key === 'ArrowRight') {
    event.preventDefault()
    emit('update:input-value', clampToRange(inputValue.value + step, ranges.value.input))
  } else if (event.key === 'ArrowDown') {
    event.preventDefault()
    emit('update:output-value', clampToRange(outputValue.value - step, ranges.value.output))
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    emit('update:output-value', clampToRange(outputValue.value + step, ranges.value.output))
  }
}

function onInputValueChange(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update:input-value', clampToRange(Number(target?.value), ranges.value.input))
}

function onOutputValueChange(event: Event) {
  const target = event.target as HTMLInputElement | null
  emit('update:output-value', clampToRange(Number(target?.value), ranges.value.output))
}

onBeforeUnmount(() => {
  window.removeEventListener('pointermove', updatePointFromEvent)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerCancel)
})
</script>

<style scoped>
.image-editor-curves {
  --image-editor-curves-frame-border-width: var(--image-editor-panel-border-width);
  --image-editor-curves-frame-border-color: var(--workspace-accent-outline);
  --image-editor-curves-frame-radius: 9px;
  display: flex;
  flex-direction: column;
  gap: 9px;
}

.image-editor-curves__toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 78px;
  gap: 8px;
}

.image-editor-curves__select-label,
.image-editor-curves__fields label {
  min-width: 0;
  display: grid;
  align-items: center;
  gap: 5px;
  color: var(--workspace-text-muted);
  font-size: var(--image-editor-panel-meta-text-size);
  font-weight: 600;
}

.image-editor-curves__select-label {
  grid-template-columns: auto minmax(0, 1fr);
}

.image-editor-curves__fields label {
  grid-template-columns: 28px minmax(0, 1fr);
}

.image-editor-curves__select-label select,
.image-editor-curves__fields input {
  height: 24px;
  min-width: 0;
  appearance: textfield;
  -moz-appearance: textfield;
  border: var(--image-editor-panel-border-width) solid var(--workspace-surface-border-soft);
  border-radius: 7px;
  background: var(--workspace-surface-muted);
  color: var(--workspace-text-body);
  font-size: var(--image-editor-panel-text-size);
  font-weight: 600;
  padding: 0 7px;
  text-align: right;
  font-variant-numeric: tabular-nums;
  outline: none;
}

.image-editor-curves__select-label select {
  padding: 0 8px;
}

.image-editor-curves__select-label select:disabled {
  cursor: not-allowed;
  opacity: 0.68;
}

.image-editor-curves__select-label select:focus-visible,
.image-editor-curves__fields input:focus-visible,
.image-editor-curves__graph:focus-visible {
  border-color: var(--workspace-accent);
  box-shadow: 0 0 0 2px var(--workspace-accent-soft);
}

.image-editor-curves__graph {
  position: relative;
  aspect-ratio: 1;
  width: 100%;
  border: var(--image-editor-curves-frame-border-width) solid
    var(--image-editor-curves-frame-border-color);
  border-radius: var(--image-editor-curves-frame-radius);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.78), rgba(255, 255, 255, 0.92)),
    var(--workspace-surface-muted);
  cursor: crosshair;
  overflow: hidden;
  outline: none;
  touch-action: none;
}

.image-editor-curves__graph svg {
  width: 100%;
  height: 100%;
  display: block;
}

.image-editor-curves__grid {
  fill: none;
  vector-effect: non-scaling-stroke;
}

.image-editor-curves__grid--fine {
  stroke: var(--workspace-surface-border-soft);
  stroke-width: 0.5;
}

.image-editor-curves__grid--strong {
  stroke: var(--workspace-accent-outline);
  stroke-width: 0.7;
}

.image-editor-curves__baseline {
  fill: none;
  stroke: var(--workspace-text-subtle);
  stroke-dasharray: 3 3;
  stroke-width: 0.9;
  vector-effect: non-scaling-stroke;
}

.image-editor-curves__curve-shadow,
.image-editor-curves__curve {
  fill: none;
  stroke-linecap: round;
  stroke-linejoin: round;
  vector-effect: non-scaling-stroke;
}

.image-editor-curves__curve-shadow {
  stroke: rgba(255, 255, 255, 0.88);
  stroke-width: 3.2;
}

.image-editor-curves__curve {
  stroke-width: 1.9;
}

.image-editor-curves__point {
  fill: #ffffff;
  stroke: var(--workspace-accent);
  stroke-width: 1.5;
  vector-effect: non-scaling-stroke;
}

.image-editor-curves__fields {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
}

.image-editor-curves__fields input::-webkit-outer-spin-button,
.image-editor-curves__fields input::-webkit-inner-spin-button {
  appearance: none;
  -webkit-appearance: none;
  margin: 0;
}
</style>
