import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import { IMAGE_TOOL_CHROME_LAYOUT } from '../config/imageToolChrome'
import { useRafScheduler } from '../utils/interaction/useRafScheduler'
import { waitForNextAnimationFrame } from '../utils/animationFrame'
import { containImageRect } from '../utils/canvasExtendGeometry'
import type { ImageEditorModeState } from './useImageEditorMode'

type FlowViewport = { x: number; y: number; zoom: number }

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type ImageEditorImageRect = {
  top: number
  left: number
  width: number
  height: number
}

export type ImageEditorChromeAnchorRect = ImageEditorImageRect

type ScreenRect = ImageEditorImageRect

type UseImageEditorCoordinateAdapterOptions = {
  nodes: Ref<Node[]>
  canvasWrapRef: Ref<HTMLElement | null>
  imageEditorState: Ref<ImageEditorModeState | null>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
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

const IMAGE_EDITOR_FOCUS_MAX_ZOOM = 2
const IMAGE_EDITOR_FOCUS_MIN_ZOOM = 0.1
const CANVAS_OVERSCROLL_GUARD_CLASS = 'canvas-overscroll-guard'

function setCanvasOverscrollGuard(enabled: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
  document.body.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
}

export function useImageEditorCoordinateAdapter(options: UseImageEditorCoordinateAdapterOptions) {
  const imageEditorImageRect = ref<ImageEditorImageRect | null>(null)
  const imageEditorChromeAnchorRect = ref<ImageEditorChromeAnchorRect | null>(null)
  const isImageEditorChromeReady = ref(false)
  const geometryScheduler = useRafScheduler()
  const windowTarget = typeof window === 'undefined' ? null : window
  const imageEditorNodeId = computed(() => options.imageEditorState.value?.nodeId ?? null)
  const stageResizeTarget = computed(() => {
    const nodeId = imageEditorNodeId.value
    return nodeId ? options.getImageStageElement(nodeId) : null
  })

  let imageEditorReturnViewport: FlowViewport | null = null
  let imageEditorFocusRequestId = 0

  function measureImageRect(nodeId: string): ImageEditorImageRect | null {
    const stage = options.getImageStageElement(nodeId)
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!stage || !display?.url) return null
    return containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
  }

  function measureImageScreenRect(nodeId: string): ScreenRect | null {
    const stage = options.getImageStageElement(nodeId)
    const imageRect = measureImageRect(nodeId)
    if (!stage || !imageRect || stage.clientWidth <= 0 || stage.clientHeight <= 0) return null

    const stageRect = stage.getBoundingClientRect()
    if (stageRect.width <= 0 || stageRect.height <= 0) return null

    const scaleX = stageRect.width / stage.clientWidth
    const scaleY = stageRect.height / stage.clientHeight
    return {
      top: stageRect.top + imageRect.top * scaleY,
      left: stageRect.left + imageRect.left * scaleX,
      width: imageRect.width * scaleX,
      height: imageRect.height * scaleY,
    }
  }

  function resolveFlowNodeCenter(nodeId: string): { x: number; y: number } | null {
    const imageRect = measureImageScreenRect(nodeId)
    if (imageRect) {
      return options.screenToFlowCoordinate({
        x: imageRect.left + imageRect.width / 2,
        y: imageRect.top + imageRect.height / 2,
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
    const screenRect = measureImageScreenRect(nodeId)
    if (!screenRect) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: IMAGE_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const chromeOutset = {
      left: IMAGE_TOOL_CHROME_LAYOUT.toolButtonSize + IMAGE_TOOL_CHROME_LAYOUT.imageGap,
      right: IMAGE_TOOL_CHROME_LAYOUT.rightPanelWidth + IMAGE_TOOL_CHROME_LAYOUT.imageGap,
    }
    const viewport = options.getViewport()
    const currentZoom = Number.isFinite(viewport.zoom) && viewport.zoom > 0 ? viewport.zoom : 1
    const flowImageWidth = screenRect.width / currentZoom
    const flowImageHeight = screenRect.height / currentZoom
    const viewportRect = getViewportRect()
    const availableWidth = Math.max(
      1,
      viewportRect.width - IMAGE_TOOL_CHROME_LAYOUT.viewportPadding * 2,
    )
    const availableHeight = Math.max(
      1,
      viewportRect.height - IMAGE_TOOL_CHROME_LAYOUT.viewportPadding * 2,
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
      x: screenRect.left + screenRect.width / 2,
      y: screenRect.top + screenRect.height / 2,
    })

    return {
      x: imageCenter.x + (chromeOutset.right - chromeOutset.left) / (2 * zoom),
      y: imageCenter.y,
      zoom,
    }
  }

  function isImageEditorFocusRequestCurrent(nodeId: string, requestId: number) {
    return requestId === imageEditorFocusRequestId && imageEditorNodeId.value === nodeId
  }

  async function focusImageEditorStage(nodeId: string, requestId: number) {
    await nextTick()
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    const target = resolveImageEditorFocusTarget(nodeId)
    if (!target || !isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    await options.setCenter(target.x, target.y, {
      zoom: target.zoom,
      duration: 220,
      interpolate: 'smooth',
    })
  }

  function measureImageEditorChromeAnchor(nodeId: string): ImageEditorChromeAnchorRect | null {
    const wrap = options.canvasWrapRef.value
    const screenRect = measureImageScreenRect(nodeId)
    if (!wrap || !screenRect) return null

    const wrapRect = wrap.getBoundingClientRect()
    return {
      top: screenRect.top - wrapRect.top,
      left: screenRect.left - wrapRect.left,
      width: screenRect.width,
      height: screenRect.height,
    }
  }

  function updateImageEditorGeometry() {
    const nodeId = imageEditorNodeId.value
    imageEditorImageRect.value = nodeId ? measureImageRect(nodeId) : null
    imageEditorChromeAnchorRect.value = nodeId ? measureImageEditorChromeAnchor(nodeId) : null
  }

  function scheduleImageEditorChromeAnchorUpdate() {
    geometryScheduler.schedule(updateImageEditorGeometry)
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
    updateImageEditorGeometry()
    if (imageEditorChromeAnchorRect.value) isImageEditorChromeReady.value = true
  }

  function onCanvasWheel(event: WheelEvent) {
    const isImageEditing = !!imageEditorNodeId.value
    const isBrowserGestureCandidate = event.ctrlKey || Math.abs(event.deltaX) > 0
    if (!isImageEditing && !isBrowserGestureCandidate) return
    event.preventDefault()
    if (isImageEditing) event.stopPropagation()
  }

  watch(
    imageEditorNodeId,
    (nextNodeId, previousNodeId) => {
      isImageEditorChromeReady.value = false
      imageEditorChromeAnchorRect.value = null
      imageEditorImageRect.value = nextNodeId ? measureImageRect(nextNodeId) : null
      if (nextNodeId) {
        imageEditorChromeAnchorRect.value = measureImageEditorChromeAnchor(nextNodeId)
      }
      if (!nextNodeId && previousNodeId) restoreImageEditorReturnViewport()
    },
    { flush: 'sync', immediate: true },
  )

  onMounted(() => {
    setCanvasOverscrollGuard(true)
  })

  onBeforeUnmount(() => {
    setCanvasOverscrollGuard(false)
  })

  useEventListener(windowTarget, 'resize', scheduleImageEditorChromeAnchorUpdate)
  useResizeObserver(options.canvasWrapRef, scheduleImageEditorChromeAnchorUpdate)
  useResizeObserver(stageResizeTarget, scheduleImageEditorChromeAnchorUpdate)

  return {
    imageEditorImageRect,
    imageEditorChromeAnchorRect,
    isImageEditorChromeReady,
    onCanvasWheel,
    focusImageEditorAndShowChrome,
    scheduleImageEditorChromeAnchorUpdate,
  }
}
