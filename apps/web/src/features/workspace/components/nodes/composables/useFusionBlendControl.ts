import { computed, nextTick, ref, type Ref } from 'vue'
import { useEventListener } from '@vueuse/core'
import {
  constrainFusionBlendPoint,
  getDefaultBlendPointForImageCount,
  type FusionBlendPoint,
} from '../../../../canvas-board/domain'

interface UseFusionBlendControlOptions {
  stageRef: Ref<HTMLElement | null>
  enabled: Ref<boolean>
  imageCount: Ref<number>
  blendPoint: Ref<unknown>
  nodeWidth: Ref<number | undefined>
  onCommit: (point: FusionBlendPoint) => void
}

export function useFusionBlendControl({
  stageRef,
  enabled,
  imageCount,
  blendPoint,
  nodeWidth,
  onCommit,
}: UseFusionBlendControlOptions) {
  const blendDragging = ref(false)
  const blendHandleHover = ref(false)
  const draftBlendPoint = ref<FusionBlendPoint | null>(null)
  const activeBlendPointerId = ref<number | null>(null)

  const constrainedBlendPoint = computed(() =>
    constrainFusionBlendPoint(blendPoint.value, imageCount.value, nodeWidth.value),
  )
  const activeBlendPoint = computed(() => draftBlendPoint.value ?? constrainedBlendPoint.value)
  const blendHandleStyle = computed(() => ({
    left: `${activeBlendPoint.value.x * 100}%`,
    top: `${activeBlendPoint.value.y * 100}%`,
  }))

  function constrainBlendPointForCurrentInputs(point: FusionBlendPoint): FusionBlendPoint {
    return constrainFusionBlendPoint(point, imageCount.value, nodeWidth.value)
  }

  function commitBlendPoint(point: FusionBlendPoint) {
    if (!enabled.value) return
    onCommit(constrainBlendPointForCurrentInputs(point))
  }

  function resolveBlendPointFromPointer(event: PointerEvent): FusionBlendPoint | null {
    const stage = stageRef.value
    if (!stage) return null
    const rect = stage.getBoundingClientRect()
    if (rect.width <= 0 || rect.height <= 0) return null
    return {
      x: (event.clientX - rect.left) / rect.width,
      y: (event.clientY - rect.top) / rect.height,
    }
  }

  function onBlendPointerMove(event: PointerEvent) {
    if (!blendDragging.value) return
    if (activeBlendPointerId.value !== null && event.pointerId !== activeBlendPointerId.value)
      return
    const point = resolveBlendPointFromPointer(event)
    if (!point) return
    draftBlendPoint.value = constrainBlendPointForCurrentInputs(point)
  }

  function stopBlendDrag(commit = false) {
    if (commit && draftBlendPoint.value) {
      commitBlendPoint(draftBlendPoint.value)
      void nextTick(() => {
        draftBlendPoint.value = null
      })
    } else {
      draftBlendPoint.value = null
    }
    blendDragging.value = false
    blendHandleHover.value = false
    activeBlendPointerId.value = null
  }

  function onBlendPointerUp(event: PointerEvent) {
    if (!blendDragging.value) return
    if (activeBlendPointerId.value !== null && event.pointerId !== activeBlendPointerId.value)
      return
    stopBlendDrag(true)
  }

  function onBlendHandlePointerDown(event: PointerEvent) {
    if (!enabled.value) return
    blendDragging.value = true
    blendHandleHover.value = true
    activeBlendPointerId.value = event.pointerId
    const handleElement = event.currentTarget as HTMLElement | null
    handleElement?.setPointerCapture?.(event.pointerId)
    const point = resolveBlendPointFromPointer(event)
    draftBlendPoint.value = point
      ? constrainBlendPointForCurrentInputs(point)
      : constrainedBlendPoint.value
  }

  function onBlendHandlePointerEnter() {
    blendHandleHover.value = true
  }

  function onBlendHandlePointerLeave() {
    if (!blendDragging.value) blendHandleHover.value = false
  }

  function onBlendHandleDoubleClick() {
    draftBlendPoint.value = null
    commitBlendPoint(getDefaultBlendPointForImageCount(imageCount.value, nodeWidth.value))
  }

  useEventListener(window, 'pointermove', onBlendPointerMove)
  useEventListener(window, 'pointerup', onBlendPointerUp)
  useEventListener(window, 'pointercancel', onBlendPointerUp)

  return {
    activeBlendPoint,
    blendDragging,
    blendHandleHover,
    blendHandleStyle,
    onBlendHandleDoubleClick,
    onBlendHandlePointerDown,
    onBlendHandlePointerEnter,
    onBlendHandlePointerLeave,
  }
}
