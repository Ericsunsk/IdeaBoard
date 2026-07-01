import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import { IMAGE_EDITOR_CHROME_LAYOUT, getImageEditorChromeOutset } from '../config/imageEditorChrome'
import { waitForNextAnimationFrame } from '../../utils/animationFrame'
import { useRafScheduler } from '../../composables/useRafScheduler'

type FlowViewport = { x: number; y: number; zoom: number }

type ImageEditorAnchorRect = {
  top: number
  left: number
  width: number
  height: number
}

type UseImageEditorViewportControllerOptions = {
  nodes: Ref<Node[]>
  canvasWrapRef: Ref<HTMLElement | null>
  imageEditorNodeId: Ref<string | null>
  getImageStageElement: (nodeId: string) => HTMLElement | null
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  getViewport: () => FlowViewport
  setViewport: (
    viewport: FlowViewport,
    options?: { duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
  setCenter: (
    x: number,
    y: number,
    options?: { zoom?: number; duration?: number; interpolate?: 'smooth' },
  ) => Promise<unknown> | unknown
}

const IMAGE_MASK_EDITOR_OVERLAY_SELECTOR = '[data-image-mask-editor]'
const MASK_EDITOR_CANVAS_SELECTOR = '.image-mask-editor-canvas'
const IMAGE_STAGE_IMAGE_SELECTOR = '.image-node-stage-image'
const IMAGE_EDITOR_FOCUS_MAX_ZOOM = 2
const IMAGE_EDITOR_FOCUS_MIN_ZOOM = 0.1
const CANVAS_OVERSCROLL_GUARD_CLASS = 'canvas-overscroll-guard'

function setCanvasOverscrollGuard(enabled: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
  document.body.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
}

export function useImageEditorViewportController(options: UseImageEditorViewportControllerOptions) {
  const imageEditorChromeAnchorRect = ref<ImageEditorAnchorRect | null>(null)
  const isImageEditorChromeReady = ref(false)
  const chromeAnchorScheduler = useRafScheduler()
  const windowTarget = typeof window === 'undefined' ? null : window
  const chromeResizeTarget = computed(() => {
    const nodeId = options.imageEditorNodeId.value
    const stage = nodeId ? options.getImageStageElement(nodeId) : null
    return stage ? resolveImageEditorChromeTarget(stage) : null
  })

  let imageEditorReturnViewport: FlowViewport | null = null
  let imageEditorFocusRequestId = 0

  function resolveFlowNodeCenter(nodeId: string): { x: number; y: number } | null {
    const stage = options.getImageStageElement(nodeId)
    if (stage) {
      const rect = stage.getBoundingClientRect()
      return options.screenToFlowCoordinate({
        x: rect.left + rect.width / 2,
        y: rect.top + rect.height / 2,
      })
    }

    const node = (
      options.nodes.value as Array<{
        id: string
        position: { x: number; y: number }
        data?: { snapshotNode?: unknown }
      }>
    ).find((item) => item.id === nodeId)
    if (!node) return null
    const snapshot = node.data?.snapshotNode as { width?: unknown } | undefined
    const width = Number(snapshot?.width)
    return {
      x: Number(node.position.x) + (Number.isFinite(width) && width > 0 ? width / 2 : 140),
      y: Number(node.position.y) + 140,
    }
  }

  function getViewportRect() {
    const viewportRect = options.canvasWrapRef.value?.getBoundingClientRect()
    return {
      width: viewportRect?.width ?? windowTarget?.innerWidth ?? 0,
      height: viewportRect?.height ?? windowTarget?.innerHeight ?? 0,
    }
  }

  function resolveImageEditorFocusTarget(nodeId: string): FlowViewport | null {
    const stage = options.getImageStageElement(nodeId)
    if (!stage) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: IMAGE_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const target = resolveImageEditorChromeTarget(stage)
    if (!target) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: IMAGE_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const rect = target.getBoundingClientRect()
    if (!rect || rect.width <= 0 || rect.height <= 0) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: IMAGE_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const chromeOutset = getImageEditorChromeOutset()
    const viewport = options.getViewport()
    const currentZoom = Number.isFinite(viewport.zoom) && viewport.zoom > 0 ? viewport.zoom : 1
    const flowImageWidth = rect.width / currentZoom
    const flowImageHeight = rect.height / currentZoom
    const viewportRect = getViewportRect()
    const availableWidth = Math.max(
      1,
      viewportRect.width - IMAGE_EDITOR_CHROME_LAYOUT.viewportPadding * 2,
    )
    const availableHeight = Math.max(
      1,
      viewportRect.height - IMAGE_EDITOR_CHROME_LAYOUT.viewportPadding * 2,
    )
    const availableImageWidth = Math.max(1, availableWidth - chromeOutset.left - chromeOutset.right)
    const fitZoom = Math.min(
      availableImageWidth / flowImageWidth,
      availableHeight / flowImageHeight,
    )
    const zoom = Math.max(
      IMAGE_EDITOR_FOCUS_MIN_ZOOM,
      Math.min(
        IMAGE_EDITOR_FOCUS_MAX_ZOOM,
        Number.isFinite(fitZoom) && fitZoom > 0 ? fitZoom : IMAGE_EDITOR_FOCUS_MAX_ZOOM,
      ),
    )
    const imageCenter = options.screenToFlowCoordinate({
      x: rect.left + rect.width / 2,
      y: rect.top + rect.height / 2,
    })

    return {
      x: imageCenter.x + (chromeOutset.right - chromeOutset.left) / (2 * zoom),
      y: imageCenter.y,
      zoom,
    }
  }

  function isImageEditorFocusRequestCurrent(nodeId: string, requestId: number) {
    return requestId === imageEditorFocusRequestId && options.imageEditorNodeId.value === nodeId
  }

  async function focusImageEditorStage(nodeId: string, requestId: number) {
    await nextTick()
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    const target = resolveImageEditorFocusTarget(nodeId)
    if (!target) return
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    await options.setCenter(target.x, target.y, {
      zoom: target.zoom,
      duration: 220,
      interpolate: 'smooth',
    })
  }

  function measureImageEditorChromeAnchor(nodeId: string): ImageEditorAnchorRect | null {
    const wrap = options.canvasWrapRef.value
    const stage = options.getImageStageElement(nodeId)
    if (!wrap || !stage) return null

    const target = resolveImageEditorChromeTarget(stage)
    if (!target) return null

    const wrapRect = wrap.getBoundingClientRect()
    const targetRect = target.getBoundingClientRect()
    if (targetRect.width <= 0 || targetRect.height <= 0) return null

    return {
      top: targetRect.top - wrapRect.top,
      left: targetRect.left - wrapRect.left,
      width: targetRect.width,
      height: targetRect.height,
    }
  }

  function updateImageEditorChromeAnchorRect() {
    const nodeId = options.imageEditorNodeId.value
    imageEditorChromeAnchorRect.value = nodeId ? measureImageEditorChromeAnchor(nodeId) : null
  }

  function resolveImageEditorChromeTarget(stage: HTMLElement): HTMLElement | null {
    const overlay = stage.querySelector<HTMLElement>(IMAGE_MASK_EDITOR_OVERLAY_SELECTOR)
    const overlayCanvas = overlay?.querySelector<HTMLElement>(MASK_EDITOR_CANVAS_SELECTOR)
    return overlayCanvas ?? overlay ?? stage.querySelector<HTMLElement>(IMAGE_STAGE_IMAGE_SELECTOR)
  }

  function scheduleImageEditorChromeAnchorUpdate() {
    chromeAnchorScheduler.schedule(updateImageEditorChromeAnchorRect)
  }

  function restoreImageEditorReturnViewport() {
    imageEditorFocusRequestId += 1
    const viewport = imageEditorReturnViewport
    if (!viewport) return
    imageEditorReturnViewport = null
    void options.setViewport({ ...viewport }, { duration: 220, interpolate: 'smooth' })
    scheduleImageEditorChromeAnchorUpdate()
  }

  async function focusImageEditorAndShowChrome(nodeId: string) {
    const requestId = ++imageEditorFocusRequestId
    imageEditorReturnViewport = { ...options.getViewport() }
    isImageEditorChromeReady.value = false
    imageEditorChromeAnchorRect.value = null
    await focusImageEditorStage(nodeId, requestId)
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    await waitForNextAnimationFrame()
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    updateImageEditorChromeAnchorRect()
    if (imageEditorChromeAnchorRect.value) {
      isImageEditorChromeReady.value = true
    }
  }

  function onCanvasWheel(event: WheelEvent) {
    const isImageEditing = !!options.imageEditorNodeId.value
    const isBrowserGestureCandidate = event.ctrlKey || Math.abs(event.deltaX) > 0
    if (!isImageEditing && !isBrowserGestureCandidate) return
    event.preventDefault()
    if (isImageEditing) event.stopPropagation()
  }

  watch(
    () => options.imageEditorNodeId.value,
    (nextNodeId, prevNodeId) => {
      if (nextNodeId) {
        isImageEditorChromeReady.value = false
        imageEditorChromeAnchorRect.value = null
        return
      }
      isImageEditorChromeReady.value = false
      imageEditorChromeAnchorRect.value = null
      if (prevNodeId) restoreImageEditorReturnViewport()
    },
  )

  onMounted(() => {
    setCanvasOverscrollGuard(true)
  })

  onBeforeUnmount(() => {
    setCanvasOverscrollGuard(false)
  })

  useEventListener(windowTarget, 'resize', scheduleImageEditorChromeAnchorUpdate)
  useResizeObserver(options.canvasWrapRef, scheduleImageEditorChromeAnchorUpdate)
  useResizeObserver(chromeResizeTarget, scheduleImageEditorChromeAnchorUpdate)

  return {
    imageEditorChromeAnchorRect,
    isImageEditorChromeReady,
    onCanvasWheel,
    focusImageEditorAndShowChrome,
    scheduleImageEditorChromeAnchorUpdate,
  }
}
