import type { Ref } from 'vue'
import { computed, nextTick, onBeforeUnmount, onMounted, onUpdated, reactive, ref } from 'vue'

const HANDLE_FRAME_SELECTOR = '[data-flow-handle-frame]'
const WORKFLOW_NODE_SELECTOR = '.workflow-node'
const WORKFLOW_NODE_HEADER_CLASS = 'workflow-node-header'

/**
 * Manages the floating handle rail that overlays a canvas node.
 * Tracks the bounding box of the node's draggable frame element via
 * ResizeObserver + RAF, and exposes reactive style + visibility state.
 *
 * Extracted from FlowCanvasNode.vue to keep the host component focused
 * on template routing and event delegation.
 */
export function useFlowCanvasNodeHandleRail(flowNodeRef: Ref<HTMLElement | null>) {
  const isPointerOverNode = ref(false)
  const isKeyboardFocusWithinNode = ref(false)
  const isTouchHandleHintActive = ref(false)

  const handleRailBounds = reactive({ left: 0, top: 0, width: 0, height: 0 })

  let measureFrame = 0
  let touchHintTimer: ReturnType<typeof setTimeout> | null = null
  let resizeObserver: ResizeObserver | null = null
  let observedRoot: HTMLElement | null = null
  let observedFrame: HTMLElement | null = null
  let cachedRoot: HTMLElement | null = null
  let cachedFrame: HTMLElement | null = null
  let isHandleFrameCacheDirty = true

  // Style and visibility.

  const handleRailStyle = computed(() => ({
    left: `${handleRailBounds.left}px`,
    top: `${handleRailBounds.top}px`,
    width: `${handleRailBounds.width}px`,
    height: `${handleRailBounds.height}px`,
  }))

  const showHandleControls = computed(
    () =>
      isPointerOverNode.value || isKeyboardFocusWithinNode.value || isTouchHandleHintActive.value,
  )

  // DOM helpers.

  function findHandleFrameElement(root: HTMLElement): HTMLElement | null {
    const explicit = root.querySelector<HTMLElement>(HANDLE_FRAME_SELECTOR)
    if (explicit) return explicit

    const workflowNode = root.querySelector<HTMLElement>(WORKFLOW_NODE_SELECTOR)
    if (!workflowNode) return null

    const { children } = workflowNode
    for (let index = 0; index < children.length; index += 1) {
      const child = children.item(index)
      if (!(child instanceof HTMLElement)) continue
      if (child.classList.contains(WORKFLOW_NODE_HEADER_CLASS)) continue
      return child
    }
    return null
  }

  function getHandleFrameElement(root: HTMLElement | null = flowNodeRef.value): HTMLElement | null {
    if (!root) {
      cachedRoot = null
      cachedFrame = null
      isHandleFrameCacheDirty = true
      return null
    }

    const canUseCachedFrame =
      cachedRoot === root && !isHandleFrameCacheDirty && !!cachedFrame && root.contains(cachedFrame)

    if (canUseCachedFrame) {
      return cachedFrame
    }

    const frame = findHandleFrameElement(root)
    if (!frame) {
      cachedRoot = null
      cachedFrame = null
      isHandleFrameCacheDirty = true
      return null
    }

    cachedRoot = root
    cachedFrame = frame
    isHandleFrameCacheDirty = false
    return frame
  }

  function invalidateHandleFrameCache() {
    isHandleFrameCacheDirty = true
  }

  function getLayoutOffsetWithinRoot(
    element: HTMLElement,
    root: HTMLElement,
  ): { left: number; top: number } | null {
    let left = 0
    let top = 0
    let current: HTMLElement | null = element

    while (current && current !== root) {
      left += current.offsetLeft
      top += current.offsetTop
      current = current.offsetParent as HTMLElement | null
    }

    return current === root ? { left, top } : null
  }

  // Measurement.

  function setHandleRailBounds(next: { left: number; top: number; width: number; height: number }) {
    const unchanged =
      handleRailBounds.left === next.left &&
      handleRailBounds.top === next.top &&
      handleRailBounds.width === next.width &&
      handleRailBounds.height === next.height
    if (unchanged) return
    Object.assign(handleRailBounds, next)
  }

  function updateHandleRailBounds() {
    measureFrame = 0
    const root = flowNodeRef.value

    if (!root) return

    const frame = getHandleFrameElement(root)

    if (!frame) {
      setHandleRailBounds({
        left: 0,
        top: 0,
        width: Math.max(1, root.offsetWidth),
        height: Math.max(1, root.offsetHeight),
      })
      return
    }

    const offset = getLayoutOffsetWithinRoot(frame, root)
    if (!offset) return

    setHandleRailBounds({
      left: Math.round(offset.left),
      top: Math.round(offset.top),
      width: Math.max(1, Math.round(frame.offsetWidth)),
      height: Math.max(1, Math.round(frame.offsetHeight)),
    })
  }

  function refreshResizeObserver() {
    if (typeof ResizeObserver === 'undefined') return

    if (!resizeObserver) {
      resizeObserver = new ResizeObserver(() => requestMeasure())
    }

    const root = flowNodeRef.value
    const frame = getHandleFrameElement(root)

    if (observedRoot !== root) {
      if (observedRoot) resizeObserver.unobserve(observedRoot)
      observedRoot = root
      if (observedRoot) resizeObserver.observe(observedRoot)
    }

    if (observedFrame !== frame) {
      if (observedFrame) resizeObserver.unobserve(observedFrame)
      observedFrame = frame
      if (observedFrame) resizeObserver.observe(observedFrame)
    }
  }

  function requestMeasure() {
    if (typeof requestAnimationFrame === 'undefined') {
      refreshResizeObserver()
      updateHandleRailBounds()
      return
    }
    if (measureFrame) cancelAnimationFrame(measureFrame)
    measureFrame = requestAnimationFrame(() => {
      refreshResizeObserver()
      updateHandleRailBounds()
    })
  }

  // Focus and interaction helpers.

  function syncKeyboardFocus() {
    const root = flowNodeRef.value
    isKeyboardFocusWithinNode.value = !!root?.querySelector(':focus-visible')
  }

  function scheduleKeyboardFocusSync() {
    requestAnimationFrame(() => syncKeyboardFocus())
  }

  function clearTouchHint() {
    isTouchHandleHintActive.value = false
    if (!touchHintTimer) return
    clearTimeout(touchHintTimer)
    touchHintTimer = null
  }

  function refreshTransientHandleVisibility() {
    clearTouchHint()

    const activeEl = document.activeElement
    if (
      activeEl instanceof HTMLElement &&
      activeEl.classList.contains('flow-canvas-node__handle-aggregate')
    ) {
      activeEl.blur()
    }

    requestAnimationFrame(() => {
      const root = flowNodeRef.value
      if (!root) return
      isPointerOverNode.value = root.matches(':hover')
      syncKeyboardFocus()
    })
  }

  // Event handlers wired by the host component.

  function onNodeMouseEnter() {
    isPointerOverNode.value = true
  }

  function onNodeMouseLeave() {
    isPointerOverNode.value = false
  }

  function onNodeFocusIn() {
    scheduleKeyboardFocusSync()
  }

  function onNodeFocusOut(event: FocusEvent) {
    const nextTarget = event.relatedTarget
    if (nextTarget instanceof Node && flowNodeRef.value?.contains(nextTarget)) {
      scheduleKeyboardFocusSync()
      return
    }
    isKeyboardFocusWithinNode.value = false
  }

  function onNodePointerDown(event: PointerEvent) {
    if (event.pointerType === 'mouse') return
    isTouchHandleHintActive.value = true
    if (touchHintTimer) clearTimeout(touchHintTimer)
    touchHintTimer = setTimeout(() => {
      isTouchHandleHintActive.value = false
      touchHintTimer = null
    }, 2400)
  }

  // Lifecycle.

  onMounted(() => {
    void nextTick(requestMeasure)
  })

  onUpdated(() => {
    invalidateHandleFrameCache()
    requestMeasure()
  })

  onBeforeUnmount(() => {
    if (measureFrame) cancelAnimationFrame(measureFrame)
    if (touchHintTimer) clearTimeout(touchHintTimer)
    resizeObserver?.disconnect()
    resizeObserver = null
    observedRoot = null
    observedFrame = null
    cachedRoot = null
    cachedFrame = null
    isHandleFrameCacheDirty = true
  })

  return {
    handleRailStyle,
    showHandleControls,
    refreshTransientHandleVisibility,
    onNodeMouseEnter,
    onNodeMouseLeave,
    onNodeFocusIn,
    onNodeFocusOut,
    onNodePointerDown,
    requestMeasure,
  }
}
