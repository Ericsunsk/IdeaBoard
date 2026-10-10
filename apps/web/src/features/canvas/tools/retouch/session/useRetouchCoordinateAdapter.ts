import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { useEventListener, useResizeObserver } from '@vueuse/core'
import { IMAGE_TOOL_CHROME_LAYOUT } from '../../image-tools/imageToolChrome.ts'
import { useRafScheduler } from '../../../infrastructure/browser/interaction/useRafScheduler.ts'
import { waitForNextAnimationFrame } from '../../../infrastructure/browser/animationFrame.ts'
import { containImageRect } from '../../../domain/graph/outpaintGeometry.ts'
import type { RetouchModeState } from './useRetouchMode'
import type { CanvasFlowViewportControls } from '../../../infrastructure/vue-flow-adapter'

type FlowViewport = { x: number; y: number; zoom: number }

type ImageDisplayPayload = {
  url: string
  ratio: number
}

export type RetouchImageRect = {
  top: number
  left: number
  width: number
  height: number
}

export type RetouchChromeAnchorRect = RetouchImageRect

type ScreenRect = RetouchImageRect

type RetouchGeometry = {
  imageRect: RetouchImageRect | null
  screenRect: ScreenRect | null
  anchorRect: RetouchChromeAnchorRect | null
}

type UseRetouchCoordinateAdapterOptions = {
  nodes: Ref<Node[]>
  canvasWrapRef: Ref<HTMLElement | null>
  retouchState: Ref<RetouchModeState | null>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
} & CanvasFlowViewportControls

const RETOUCH_FOCUS_MAX_ZOOM = 2
const RETOUCH_FOCUS_MIN_ZOOM = 0.1
const CANVAS_OVERSCROLL_GUARD_CLASS = 'canvas-overscroll-guard'

function setCanvasOverscrollGuard(enabled: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
  document.body.classList.toggle(CANVAS_OVERSCROLL_GUARD_CLASS, enabled)
}

export function useRetouchCoordinateAdapter(options: UseRetouchCoordinateAdapterOptions) {
  const retouchImageRect = ref<RetouchImageRect | null>(null)
  const retouchChromeAnchorRect = ref<RetouchChromeAnchorRect | null>(null)
  const isRetouchChromeReady = ref(false)
  const geometryScheduler = useRafScheduler()
  const windowTarget = typeof window === 'undefined' ? null : window
  const retouchNodeId = computed(() => options.retouchState.value?.nodeId ?? null)
  const stageResizeTarget = computed(() => {
    const nodeId = retouchNodeId.value
    return nodeId ? options.getImageStageElement(nodeId) : null
  })

  let retouchReturnViewport: FlowViewport | null = null
  let retouchFocusRequestId = 0

  function measureOnce(nodeId: string): RetouchGeometry {
    const empty: RetouchGeometry = { imageRect: null, screenRect: null, anchorRect: null }
    const stage = options.getImageStageElement(nodeId)
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!stage || !display?.url) return empty
    const imageRect = containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
    if (!imageRect || stage.clientWidth <= 0 || stage.clientHeight <= 0) {
      return { imageRect, screenRect: null, anchorRect: null }
    }

    const stageRect = stage.getBoundingClientRect()
    if (stageRect.width <= 0 || stageRect.height <= 0) {
      return { imageRect, screenRect: null, anchorRect: null }
    }

    const scaleX = stageRect.width / stage.clientWidth
    const scaleY = stageRect.height / stage.clientHeight
    const screenRect: ScreenRect = {
      top: stageRect.top + imageRect.top * scaleY,
      left: stageRect.left + imageRect.left * scaleX,
      width: imageRect.width * scaleX,
      height: imageRect.height * scaleY,
    }

    const wrap = options.canvasWrapRef.value
    if (!wrap) return { imageRect, screenRect, anchorRect: null }
    const wrapRect = wrap.getBoundingClientRect()
    return {
      imageRect,
      screenRect,
      anchorRect: {
        top: screenRect.top - wrapRect.top,
        left: screenRect.left - wrapRect.left,
        width: screenRect.width,
        height: screenRect.height,
      },
    }
  }

  function resolveFlowNodeCenter(nodeId: string): { x: number; y: number } | null {
    const screenRect = measureOnce(nodeId).screenRect
    if (screenRect) {
      return options.screenToFlowCoordinate({
        x: screenRect.left + screenRect.width / 2,
        y: screenRect.top + screenRect.height / 2,
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

  function resolveRetouchFocusTarget(nodeId: string): FlowViewport | null {
    const screenRect = measureOnce(nodeId).screenRect
    if (!screenRect) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: RETOUCH_FOCUS_MAX_ZOOM } : null
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
      RETOUCH_FOCUS_MIN_ZOOM,
      Math.min(
        RETOUCH_FOCUS_MAX_ZOOM,
        Number.isFinite(fitZoom) && fitZoom > 0 ? fitZoom : RETOUCH_FOCUS_MAX_ZOOM,
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

  function isRetouchFocusRequestCurrent(nodeId: string, requestId: number) {
    return requestId === retouchFocusRequestId && retouchNodeId.value === nodeId
  }

  async function focusRetouchStage(nodeId: string, requestId: number) {
    await nextTick()
    if (!isRetouchFocusRequestCurrent(nodeId, requestId)) return
    const target = resolveRetouchFocusTarget(nodeId)
    if (!target || !isRetouchFocusRequestCurrent(nodeId, requestId)) return
    await options.setCenter(target.x, target.y, {
      zoom: target.zoom,
      duration: 220,
      interpolate: 'smooth',
    })
  }

  function updateRetouchGeometry() {
    const nodeId = retouchNodeId.value
    const geometry = nodeId ? measureOnce(nodeId) : null
    retouchImageRect.value = geometry?.imageRect ?? null
    retouchChromeAnchorRect.value = geometry?.anchorRect ?? null
  }

  function scheduleRetouchChromeAnchorUpdate() {
    geometryScheduler.schedule(updateRetouchGeometry)
  }

  function restoreRetouchReturnViewport() {
    retouchFocusRequestId += 1
    const viewport = retouchReturnViewport
    if (!viewport) return
    retouchReturnViewport = null
    void options.setViewport({ ...viewport }, { duration: 220, interpolate: 'smooth' })
    scheduleRetouchChromeAnchorUpdate()
  }

  async function focusRetouchAndShowChrome(nodeId: string) {
    const requestId = ++retouchFocusRequestId
    retouchReturnViewport = { ...options.getViewport() }
    isRetouchChromeReady.value = false
    retouchChromeAnchorRect.value = null
    await focusRetouchStage(nodeId, requestId)
    if (!isRetouchFocusRequestCurrent(nodeId, requestId)) return
    await waitForNextAnimationFrame()
    if (!isRetouchFocusRequestCurrent(nodeId, requestId)) return
    updateRetouchGeometry()
    if (retouchChromeAnchorRect.value) isRetouchChromeReady.value = true
  }

  function onCanvasWheel(event: WheelEvent) {
    const isImageEditing = !!retouchNodeId.value
    const isBrowserGestureCandidate = event.ctrlKey || Math.abs(event.deltaX) > 0
    if (!isImageEditing && !isBrowserGestureCandidate) return
    event.preventDefault()
    if (isImageEditing) event.stopPropagation()
  }

  watch(
    retouchNodeId,
    (nextNodeId, previousNodeId) => {
      isRetouchChromeReady.value = false
      const geometry = nextNodeId ? measureOnce(nextNodeId) : null
      retouchImageRect.value = geometry?.imageRect ?? null
      retouchChromeAnchorRect.value = geometry?.anchorRect ?? null
      if (!nextNodeId && previousNodeId) restoreRetouchReturnViewport()
    },
    { flush: 'sync', immediate: true },
  )

  onMounted(() => {
    setCanvasOverscrollGuard(true)
  })

  onBeforeUnmount(() => {
    setCanvasOverscrollGuard(false)
  })

  useEventListener(windowTarget, 'resize', scheduleRetouchChromeAnchorUpdate)
  useResizeObserver(options.canvasWrapRef, scheduleRetouchChromeAnchorUpdate)
  useResizeObserver(stageResizeTarget, scheduleRetouchChromeAnchorUpdate)

  return {
    retouchImageRect,
    retouchChromeAnchorRect,
    isRetouchChromeReady,
    onCanvasWheel,
    focusRetouchAndShowChrome,
    scheduleRetouchChromeAnchorUpdate,
  }
}
