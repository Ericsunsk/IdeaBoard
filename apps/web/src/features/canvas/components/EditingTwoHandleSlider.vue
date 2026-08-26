<template>
  <div
    ref="sliderRef"
    class="editing-two-handle-slider"
    role="group"
    :aria-label="resolvedAriaLabel"
    @pointerdown="onTrackPointerDown"
  >
    <div class="editing-two-handle-slider__track" aria-hidden="true" />
    <button
      v-for="handle in normalizedHandles"
      :key="handle.id"
      :ref="(element) => setHandleRef(handle.id, element)"
      class="editing-two-handle-slider__handle"
      type="button"
      role="slider"
      :class="{ 'is-active': isHandleActive(handle.id) }"
      :data-handle-id="handle.id"
      :style="{ left: `${resolveHandlePercent(handle)}%` }"
      :aria-label="handle.ariaLabel"
      aria-orientation="horizontal"
      :aria-valuemin="handle.min"
      :aria-valuemax="handle.max"
      :aria-valuenow="handle.value"
      :aria-valuetext="handle.ariaValueText"
      @pointerdown.stop="onHandlePointerDown(handle.id, $event)"
      @keydown="onHandleKeydown(handle, $event)"
      @focus="selectHandle(handle.id)"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePointerDragSession } from '../utils/interaction/usePointerDragSession'

type TwoHandleSliderHandle = {
  id: string
  value: number
  min: number
  max: number
  ariaLabel: string
  ariaValueText?: string
}

type PointerPosition = Pick<PointerEvent, 'clientX'>

const props = withDefaults(
  defineProps<{
    handles: readonly TwoHandleSliderHandle[]
    min: number
    max: number
    step?: number
    shiftStep?: number
    ariaLabel?: string
  }>(),
  {
    step: 1,
    shiftStep: 10,
    ariaLabel: undefined,
  },
)

const emit = defineEmits<{
  (e: 'update:handle', handleId: string, value: number): void
}>()

const { t } = useI18n()

const resolvedAriaLabel = computed(() => props.ariaLabel ?? t('canvas.controls.rangeControl'))

const sliderRef = ref<HTMLElement | null>(null)
const activeHandleId = ref<string | null>(null)
const selectedHandleId = ref<string | null>(null)
const handleRefs = new Map<string, HTMLButtonElement>()
const dragSession = usePointerDragSession<string>({
  onMove: ({ event, payload }) => updateHandleFromPointer(payload, event),
  onEnd: clearActiveHandle,
  onCancel: clearActiveHandle,
})

const normalizedHandles = computed(() =>
  props.handles.slice(0, 2).map((handle) => {
    const min = clampValue(handle.min, props.min, props.max)
    const max = clampValue(handle.max, props.min, props.max)
    const lower = Math.min(min, max)
    const upper = Math.max(min, max)
    return {
      ...handle,
      min: snapValue(lower),
      max: snapValue(upper),
      value: snapValue(clampValue(handle.value, lower, upper)),
    }
  }),
)

function onTrackPointerDown(event: PointerEvent) {
  const handleId = resolveNearestHandleId(event)
  if (!handleId) return
  startHandleDrag(handleId, event, { syncToPointer: true })
}

function onHandlePointerDown(handleId: string, event: PointerEvent) {
  startHandleDrag(handleId, event, { syncToPointer: false })
}

function startHandleDrag(
  handleId: string,
  event: PointerEvent,
  options: { syncToPointer: boolean },
) {
  event.preventDefault()
  selectHandle(handleId)
  focusHandle(handleId)
  activeHandleId.value = handleId
  dragSession.beginDrag(event, handleId, { target: sliderRef.value })
  if (options.syncToPointer) updateHandleFromPointer(handleId, event)
}

function updateHandleFromPointer(handleId: string, position: PointerPosition) {
  const value = resolvePointerValue(position)
  if (value == null) return
  emitHandleValue(handleId, value)
}

function onHandleKeydown(handle: TwoHandleSliderHandle, event: KeyboardEvent) {
  const nextValue = resolveKeyboardValue(handle, event)
  if (nextValue == null) return
  event.preventDefault()
  event.stopPropagation()
  selectHandle(handle.id)
  emitHandleValue(handle.id, nextValue)
}

function resolveKeyboardValue(handle: TwoHandleSliderHandle, event: KeyboardEvent) {
  switch (event.key) {
    case 'Home':
      return handle.min
    case 'End':
      return handle.max
    default: {
      const step = event.shiftKey ? props.shiftStep : props.step
      const delta = resolveKeyboardDelta(event.key, step)
      return delta == null ? null : handle.value + delta
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

function emitHandleValue(handleId: string, value: number) {
  const handle = normalizedHandles.value.find((item) => item.id === handleId)
  if (!handle) return
  emit('update:handle', handleId, snapValue(clampValue(value, handle.min, handle.max)))
}

function resolvePointerValue(position: PointerPosition) {
  const rect = sliderRef.value?.getBoundingClientRect()
  if (!rect || rect.width <= 0) return null
  const ratio = clampUnit((position.clientX - rect.left) / rect.width)
  return snapValue(props.min + ratio * (props.max - props.min))
}

function resolveNearestHandleId(position: PointerPosition) {
  const value = resolvePointerValue(position)
  if (value == null || normalizedHandles.value.length === 0) return null
  return normalizedHandles.value.reduce((nearest, handle) =>
    Math.abs(handle.value - value) < Math.abs(nearest.value - value) ? handle : nearest,
  ).id
}

function resolveHandlePercent(handle: TwoHandleSliderHandle) {
  const range = props.max - props.min
  if (range <= 0) return 0
  return clampUnit((handle.value - props.min) / range) * 100
}

function setHandleRef(handleId: string, element: unknown) {
  if (element instanceof HTMLButtonElement) {
    handleRefs.set(handleId, element)
    return
  }
  handleRefs.delete(handleId)
}

function focusHandle(handleId: string) {
  void nextTick(() => {
    handleRefs.get(handleId)?.focus()
  })
}

function selectHandle(handleId: string) {
  selectedHandleId.value = handleId
}

function isHandleActive(handleId: string) {
  return handleId === selectedHandleId.value || handleId === activeHandleId.value
}

function clearActiveHandle() {
  activeHandleId.value = null
}

function snapValue(value: number) {
  if (!Number.isFinite(value)) return props.min
  if (props.step <= 0) return value
  return Math.round(value / props.step) * props.step
}

function clampValue(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

function clampUnit(value: number) {
  return clampValue(value, 0, 1)
}
</script>

<style scoped>
.editing-two-handle-slider {
  --editing-two-handle-slider-track-background-default: var(
    --canvas-range-track-background,
    var(--canvas-range-track-color, var(--canvas-surface-muted))
  );
  --editing-two-handle-slider-track-height: var(--canvas-range-track-height, 4px);
  --editing-two-handle-slider-handle-size: var(--canvas-range-thumb-size, 9px);
  --editing-two-handle-slider-handle-color-default: var(
    --canvas-range-thumb-color,
    var(--canvas-range-fill-color, var(--canvas-accent))
  );
  position: relative;
  height: var(--canvas-range-input-height, 22px);
  min-width: 0;
  touch-action: none;
}

.editing-two-handle-slider__track {
  position: absolute;
  inset-inline: 0;
  top: 50%;
  height: var(--editing-two-handle-slider-track-height);
  border-radius: 999px;
  background: var(
    --editing-two-handle-slider-track-background,
    var(--editing-two-handle-slider-track-background-default)
  );
  transform: translateY(-50%);
  pointer-events: none;
}

.editing-two-handle-slider__handle {
  position: absolute;
  top: 50%;
  z-index: 1;
  width: var(--editing-two-handle-slider-handle-size);
  height: var(--editing-two-handle-slider-handle-size);
  border: 0;
  border-radius: 999px;
  padding: 0;
  background: var(
    --editing-two-handle-slider-handle-color,
    var(--editing-two-handle-slider-handle-color-default)
  );
  cursor: grab;
  transform: translate(-50%, -50%);
}

.editing-two-handle-slider__handle.is-active {
  z-index: 2;
}

.editing-two-handle-slider__handle:active {
  cursor: grabbing;
}

.editing-two-handle-slider__handle:focus-visible {
  outline: var(--ib-focus-outline-width) solid var(--canvas-range-focus-color, var(--canvas-accent));
  outline-offset: var(--ib-focus-outline-offset);
}
</style>
