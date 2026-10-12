import { computed, nextTick, onScopeDispose, ref, watch, type Ref } from 'vue'
import {
  constrainFusionBlendPoint,
  getDefaultBlendPointForImageCount,
  type FusionBlendPoint,
} from '../../../../board/ports/canvas'

interface UseFusionBlendControlOptions {
  stageRef: Ref<HTMLElement | null>
  isEnabled: Ref<boolean>
  imageCount: Ref<number>
  blendPoint: Ref<unknown>
  nodeWidth: Ref<number | undefined>
  onCommit: (point: FusionBlendPoint) => void
}

export function useFusionBlendControl({
  stageRef,
  isEnabled,
  imageCount,
  blendPoint,
  nodeWidth,
  onCommit,
}: UseFusionBlendControlOptions) {
  const isBlendDragging = ref(false)
  const isBlendHandleHover = ref(false)
  const draftBlendPoint = ref<FusionBlendPoint | null>(null)
  const activeBlendPointerId = ref<number | null>(null)
  // Stage rect cached at pointerdown so per-pointermove resolution avoids
  // getBoundingClientRect (forced reflow in the hot path).
  let cachedStageRect: DOMRect | null = null
  let dragWindow: Window | null = null
  let blendHandleElement: HTMLElement | null = null

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
    if (!isEnabled.value) return
    onCommit(constrainBlendPointForCurrentInputs(point))
  }

  function readStageRect(): DOMRect | null {
    const stage = stageRef.value
    if (!stage) return null
    try {
      return stage.getBoundingClientRect()
    } catch {
      return null
    }
  }

  function refreshCachedStageRect() {
    // Re-resolve on scroll/resize mid-drag; cheap (once per scroll frame, not per move).
    cachedStageRect = readStageRect()
  }

  function resolveBlendPointFromPointer(event: PointerEvent): FusionBlendPoint | null {
    const rect = isBlendDragging.value && cachedStageRect ? cachedStageRect : readStageRect()
    if (!rect || rect.width <= 0 || rect.height <= 0) return null
    return {
      x: (event.clientX - rect.left) / rect.width,
      y: (event.clientY - rect.top) / rect.height,
    }
  }

  function onBlendPointerMove(event: PointerEvent) {
    if (!isBlendDragging.value) return
    if (activeBlendPointerId.value !== null && event.pointerId !== activeBlendPointerId.value)
      return
    const point = resolveBlendPointFromPointer(event)
    if (!point) return
    draftBlendPoint.value = constrainBlendPointForCurrentInputs(point)
  }

  function tryReleaseBlendCapture() {
    if (blendHandleElement && activeBlendPointerId.value !== null) {
      try {
        if (blendHandleElement.hasPointerCapture?.(activeBlendPointerId.value) !== false) {
          blendHandleElement.releasePointerCapture?.(activeBlendPointerId.value)
        }
      } catch {
        // Capture already lost; drag state rolls back below regardless.
      }
    }
    blendHandleElement = null
  }

  function cleanupBlendDragListeners() {
    // Remove from the captured window instance: the stage may have moved
    // across documents between bind and cleanup.
    if (!dragWindow) return
    const ownerWindow = dragWindow
    dragWindow = null
    ownerWindow.removeEventListener('pointermove', onBlendPointerMove, true)
    ownerWindow.removeEventListener('pointerup', onBlendPointerUp, true)
    ownerWindow.removeEventListener('pointercancel', onBlendPointerCancel, true)
    ownerWindow.removeEventListener('scroll', refreshCachedStageRect, true)
    ownerWindow.removeEventListener('resize', refreshCachedStageRect)
  }

  function bindBlendDragListeners() {
    cleanupBlendDragListeners()
    const ownerWindow =
      stageRef.value?.ownerDocument.defaultView ?? (typeof window === 'undefined' ? null : window)
    if (!ownerWindow) return
    dragWindow = ownerWindow
    ownerWindow.addEventListener('pointermove', onBlendPointerMove, true)
    ownerWindow.addEventListener('pointerup', onBlendPointerUp, true)
    ownerWindow.addEventListener('pointercancel', onBlendPointerCancel, true)
    ownerWindow.addEventListener('scroll', refreshCachedStageRect, true)
    ownerWindow.addEventListener('resize', refreshCachedStageRect)
  }

  function stopBlendDrag(shouldCommit = false) {
    if (shouldCommit && draftBlendPoint.value) {
      commitBlendPoint(draftBlendPoint.value)
      void nextTick(() => {
        draftBlendPoint.value = null
      })
    } else {
      draftBlendPoint.value = null
    }
    isBlendDragging.value = false
    isBlendHandleHover.value = false
    activeBlendPointerId.value = null
    cachedStageRect = null
    tryReleaseBlendCapture()
    cleanupBlendDragListeners()
  }

  function onBlendPointerUp(event: PointerEvent) {
    if (!isBlendDragging.value) return
    if (activeBlendPointerId.value !== null && event.pointerId !== activeBlendPointerId.value)
      return
    stopBlendDrag(true)
  }

  function onBlendPointerCancel(event: PointerEvent) {
    if (!isBlendDragging.value) return
    if (activeBlendPointerId.value !== null && event.pointerId !== activeBlendPointerId.value)
      return
    // A cancelled pointer was never released by the user: revert the draft
    // instead of persisting a blend point.
    stopBlendDrag(false)
  }

  function onBlendHandlePointerDown(event: PointerEvent) {
    if (!isEnabled.value) return
    isBlendDragging.value = true
    isBlendHandleHover.value = true
    activeBlendPointerId.value = event.pointerId
    cachedStageRect = readStageRect()
    const handleElement = event.currentTarget as HTMLElement | null
    blendHandleElement = handleElement
    if (handleElement) {
      try {
        handleElement.setPointerCapture?.(event.pointerId)
      } catch {
        // Proceed captureless: window-level listeners still drive the drag.
      }
    }
    bindBlendDragListeners()
    const point = resolveBlendPointFromPointer(event)
    draftBlendPoint.value = point
      ? constrainBlendPointForCurrentInputs(point)
      : constrainedBlendPoint.value
  }

  function onBlendHandlePointerEnter() {
    isBlendHandleHover.value = true
  }

  function onBlendHandlePointerLeave() {
    if (!isBlendDragging.value) isBlendHandleHover.value = false
  }

  function onBlendHandleDoubleClick() {
    draftBlendPoint.value = null
    commitBlendPoint(getDefaultBlendPointForImageCount(imageCount.value, nodeWidth.value))
  }

  watch(isEnabled, (isEnabled) => {
    if (!isEnabled && isBlendDragging.value) stopBlendDrag(false)
  })

  onScopeDispose(() => {
    if (isBlendDragging.value) stopBlendDrag(false)
    else cleanupBlendDragListeners()
  })

  return {
    activeBlendPoint,
    isBlendDragging,
    isBlendHandleHover,
    blendHandleStyle,
    onBlendHandleDoubleClick,
    onBlendHandlePointerDown,
    onBlendHandlePointerEnter,
    onBlendHandlePointerLeave,
  }
}
