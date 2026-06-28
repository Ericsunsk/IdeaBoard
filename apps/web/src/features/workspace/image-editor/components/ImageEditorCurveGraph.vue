<template>
  <div
    ref="graphRef"
    class="image-editor-curves__graph"
    role="group"
    aria-label="曲线图"
    @pointerdown="onGraphPointerDown"
  >
    <div class="image-editor-curves__plot" aria-hidden="true">
      <svg viewBox="0 0 100 100" focusable="false">
        <defs>
          <linearGradient :id="gradientId" x1="0" y1="1" x2="1" y2="0">
            <stop offset="0" stop-color="var(--workspace-text-subtle)" stop-opacity="0.26" />
            <stop offset="1" stop-color="var(--workspace-accent)" stop-opacity="0.9" />
          </linearGradient>
        </defs>
        <path v-if="histogramPath" class="image-editor-curves__histogram" :d="histogramPath" />
        <path class="image-editor-curves__grid image-editor-curves__grid--fine" :d="fineGridPath" />
        <path
          class="image-editor-curves__grid image-editor-curves__grid--strong"
          :d="strongGridPath"
        />
        <path class="image-editor-curves__baseline" d="M 0 100 L 100 0" />
        <path
          v-for="ghost in ghostCurvePaths"
          :key="ghost.channel"
          class="image-editor-curves__ghost-curve"
          :d="ghost.path"
        />
        <path
          class="image-editor-curves__curve"
          :d="activeCurvePath"
          :stroke="`url(#${gradientId})`"
        />
      </svg>
    </div>
    <button
      v-for="point in points"
      :key="point.id"
      type="button"
      class="image-editor-curves__point"
      :class="{ active: selectedPointId === point.id }"
      :data-point-id="point.id"
      :style="resolvePointStyle(point)"
      :aria-label="`曲线点 输入 ${point.input} 输出 ${point.output}`"
      :aria-keyshortcuts="point.endpoint ? undefined : 'Delete Backspace'"
      @pointerdown.stop="onPointPointerDown(point.id, $event)"
      @keydown="onPointKeydown(point, $event)"
      @focus="emit('select-point', point.id)"
    />
  </div>
</template>

<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import type { ImageEditorCurveChannel } from '../types'
import type { NormalizedCurveChannelPoint } from '../utils/imageEditorCurves'

type PointerPosition = Pick<PointerEvent, 'clientX' | 'clientY'>

const props = defineProps<{
  activeCurvePath: string
  ghostCurvePaths: readonly { channel: ImageEditorCurveChannel; path: string }[]
  histogramPath: string
  points: readonly NormalizedCurveChannelPoint[]
  selectedPointId: string
  focusRequest?: number
}>()

const emit = defineEmits<{
  (e: 'add-point', value: { input: number; output: number }): void
  (e: 'move-point', pointId: string, value: { input: number; output: number }): void
  (e: 'remove-point', pointId: string): void
  (e: 'select-point', pointId: string): void
}>()

const CURVE_POINT_HIT_RADIUS_PX = 7
const USER_POINT_MIN_INPUT = 1
const USER_POINT_MAX_INPUT = 99

const graphRef = ref<HTMLElement | null>(null)
const activePointerId = ref<number | null>(null)
const activeDragPointId = ref<string | null>(null)
const pendingCreatedPointDrag = ref<{
  latestPosition: PointerPosition
  hasMoved: boolean
} | null>(null)
const gradientId = `image-editor-curves-gradient-${useId()}`
const fineGridPath = createGridPath([25, 75])
const strongGridPath = createGridPath([50])

function createGridPath(values: number[]) {
  return values
    .flatMap((value) => [`M ${value} 0 L ${value} 100`, `M 0 ${value} L 100 ${value}`])
    .join(' ')
}

function onGraphPointerDown(event: PointerEvent) {
  event.preventDefault()
  const graphPoint = resolveGraphPoint(event)
  if (!graphPoint) return
  const existingPoint = findHitPoint(event)
  if (existingPoint) {
    startPointDrag(existingPoint.id, event)
    return
  }

  startPendingCreatedPointDrag(event)
  emit('add-point', {
    input: clampUserPointInput(graphPoint.x),
    output: 100 - graphPoint.y,
  })
}

function onPointPointerDown(pointId: string, event: PointerEvent) {
  event.preventDefault()
  const target = event.currentTarget as HTMLElement | null
  target?.focus({ preventScroll: true })
  startPointDrag(pointId, event)
}

function startPointDrag(pointId: string, event: PointerEvent) {
  emit('select-point', pointId)
  focusPointButton(pointId)
  activePointerId.value = event.pointerId
  activeDragPointId.value = pointId
  pendingCreatedPointDrag.value = null
  graphRef.value?.setPointerCapture(event.pointerId)
  window.addEventListener('pointermove', onWindowPointerMove)
  window.addEventListener('pointerup', onWindowPointerUp)
  window.addEventListener('pointercancel', onWindowPointerCancel)
  updatePointFromPointerPosition(pointId, event)
}

function startPendingCreatedPointDrag(event: PointerEvent) {
  activePointerId.value = event.pointerId
  activeDragPointId.value = null
  pendingCreatedPointDrag.value = {
    latestPosition: event,
    hasMoved: false,
  }
  graphRef.value?.setPointerCapture(event.pointerId)
  window.addEventListener('pointermove', onWindowPointerMove)
  window.addEventListener('pointerup', onWindowPointerUp)
  window.addEventListener('pointercancel', onWindowPointerCancel)
}

function onWindowPointerMove(event: PointerEvent) {
  if (activePointerId.value !== event.pointerId) return
  if (activeDragPointId.value) {
    updatePointFromPointerPosition(activeDragPointId.value, event)
    return
  }
  if (!pendingCreatedPointDrag.value) return
  pendingCreatedPointDrag.value = {
    ...pendingCreatedPointDrag.value,
    latestPosition: event,
    hasMoved: true,
  }
}

function onWindowPointerUp(event: PointerEvent) {
  stopPointDrag(event.pointerId)
}

function onWindowPointerCancel(event: PointerEvent) {
  stopPointDrag(event.pointerId)
}

function stopPointDrag(pointerId: number) {
  if (activePointerId.value !== pointerId) return
  if (graphRef.value?.hasPointerCapture(pointerId)) graphRef.value.releasePointerCapture(pointerId)
  activePointerId.value = null
  activeDragPointId.value = null
  pendingCreatedPointDrag.value = null
  window.removeEventListener('pointermove', onWindowPointerMove)
  window.removeEventListener('pointerup', onWindowPointerUp)
  window.removeEventListener('pointercancel', onWindowPointerCancel)
}

function updatePointFromPointerPosition(pointId: string, position: PointerPosition) {
  const graphPoint = resolveGraphPoint(position)
  if (!graphPoint) return
  emit('move-point', pointId, {
    input: graphPoint.x,
    output: 100 - graphPoint.y,
  })
}

function resolveGraphPoint(position: PointerPosition) {
  const rect = graphRef.value?.getBoundingClientRect()
  if (!rect || rect.width <= 0 || rect.height <= 0) return null
  return {
    x: clampPercent(((position.clientX - rect.left) / rect.width) * 100),
    y: clampPercent(((position.clientY - rect.top) / rect.height) * 100),
  }
}

function findHitPoint(event: PointerEvent) {
  const rect = graphRef.value?.getBoundingClientRect()
  if (!rect || rect.width <= 0 || rect.height <= 0) return null
  return (
    props.points.find((point) => {
      const pointX = rect.left + (point.input / 100) * rect.width
      const pointY = rect.top + ((100 - point.output) / 100) * rect.height
      return Math.hypot(pointX - event.clientX, pointY - event.clientY) <= CURVE_POINT_HIT_RADIUS_PX
    }) ?? null
  )
}

function resolvePointStyle(point: NormalizedCurveChannelPoint) {
  return {
    left: `${point.input}%`,
    top: `${100 - point.output}%`,
  }
}

function onPointKeydown(point: NormalizedCurveChannelPoint, event: KeyboardEvent) {
  const step = event.shiftKey ? 10 : 1
  if (event.key === 'Delete' || event.key === 'Backspace') {
    event.preventDefault()
    event.stopPropagation()
    if (!point.endpoint) emit('remove-point', point.id)
    return
  }
  const delta = resolveKeyboardDelta(event.key, step)
  if (!delta) return
  event.preventDefault()
  event.stopPropagation()
  emit('move-point', point.id, {
    input: point.input + delta.input,
    output: point.output + delta.output,
  })
}

function resolveKeyboardDelta(key: string, step: number) {
  switch (key) {
    case 'ArrowLeft':
      return { input: -step, output: 0 }
    case 'ArrowRight':
      return { input: step, output: 0 }
    case 'ArrowDown':
      return { input: 0, output: -step }
    case 'ArrowUp':
      return { input: 0, output: step }
    default:
      return null
  }
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, Math.round(value)))
}

function clampUserPointInput(value: number) {
  return Math.min(USER_POINT_MAX_INPUT, Math.max(USER_POINT_MIN_INPUT, value))
}

function focusPointButton(pointId: string) {
  const buttons = graphRef.value?.querySelectorAll<HTMLButtonElement>(
    '.image-editor-curves__point',
  )
  const pointButton = Array.from(buttons ?? []).find((button) => button.dataset.pointId === pointId)
  pointButton?.focus({ preventScroll: true })
}

watch(
  () => [props.points, props.selectedPointId] as const,
  () => {
    const pendingDrag = pendingCreatedPointDrag.value
    if (!pendingDrag) return

    const point = props.points.find(
      (item) => !item.endpoint && item.id === props.selectedPointId,
    )
    if (!point) return

    activeDragPointId.value = point.id
    pendingCreatedPointDrag.value = null
    if (pendingDrag.hasMoved) updatePointFromPointerPosition(point.id, pendingDrag.latestPosition)
    void nextTick(() => focusPointButton(point.id))
  },
  { flush: 'post' },
)

watch(
  () => props.focusRequest,
  () => {
    void nextTick(() => focusPointButton(props.selectedPointId))
  },
  { flush: 'post' },
)

onBeforeUnmount(() => {
  if (activePointerId.value != null) stopPointDrag(activePointerId.value)
})
</script>
