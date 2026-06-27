import { nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { IMAGE_EDITOR_CHROME_LAYOUT, getImageEditorChromeOutset } from '../config/imageEditorChrome'
import { waitForNextAnimationFrame } from '../../utils/animationFrame'

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
  onViewportTransform?: (viewport: FlowViewport) => void
}

const IMAGE_MASK_EDITOR_OVERLAY_SELECTOR = '[data-image-mask-editor]'
const MASK_EDITOR_CANVAS_SELECTOR = '.image-mask-editor-canvas'
const IMAGE_STAGE_IMAGE_SELECTOR = '.image-node-stage-image'
const IMAGE_EDITOR_FOCUS_MAX_ZOOM = 2
const IMAGE_EDITOR_FOCUS_MIN_ZOOM = 0.1
const WORKSPACE_OVERSCROLL_GUARD_CLASS = 'workspace-overscroll-guard'

function setWorkspaceOverscrollGuard(enabled: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(WORKSPACE_OVERSCROLL_GUARD_CLASS, enabled)
  document.body.classList.toggle(WORKSPACE_OVERSCROLL_GUARD_CLASS, enabled)
}

export function useImageEditorViewportController(options: UseImageEditorViewportControllerOptions) {
  const imageEditorChromeAnchorRect = ref<ImageEditorAnchorRect | null>(null)
  const isImageEditorChromeReady = ref(false)

  let imageEditorReturnViewport: FlowViewport | null = null
  let imageEditorFocusRequestId = 0
  let imageEditorChromeAnchorFrame = 0

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
      width: viewportRect?.width ?? window.innerWidth,
      height: viewportRect?.height ?? window.innerHeight,
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
    if (!isImageEditorFocusRequestCurrent(nodeId, requestId)) return
    options.onViewportTransform?.(options.getViewport())
  }

  function updateImageEditorChromeAnchorRect() {
    imageEditorChromeAnchorFrame = 0
    const nodeId = options.imageEditorNodeId.value
    imageEditorChromeAnchorRect.value = nodeId ? resolveImageEditorChromeAnchorRect(nodeId) : null
  }

  function resolveImageEditorChromeAnchorRect(nodeId: string): ImageEditorAnchorRect | null {
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

  function resolveImageEditorChromeTarget(stage: HTMLElement): HTMLElement | null {
    const overlay = stage.querySelector<HTMLElement>(IMAGE_MASK_EDITOR_OVERLAY_SELECTOR)
    const overlayCanvas = overlay?.querySelector<HTMLElement>(MASK_EDITOR_CANVAS_SELECTOR)
    return overlayCanvas ?? overlay ?? stage.querySelector<HTMLElement>(IMAGE_STAGE_IMAGE_SELECTOR)
  }

  function scheduleImageEditorChromeAnchorUpdate() {
    if (typeof requestAnimationFrame === 'undefined') {
      updateImageEditorChromeAnchorRect()
      return
    }
    if (imageEditorChromeAnchorFrame) cancelAnimationFrame(imageEditorChromeAnchorFrame)
    imageEditorChromeAnchorFrame = requestAnimationFrame(updateImageEditorChromeAnchorRect)
  }

  function restoreImageEditorReturnViewport() {
    imageEditorFocusRequestId += 1
    const viewport = imageEditorReturnViewport
    if (!viewport) return
    imageEditorReturnViewport = null
    void options.setViewport({ ...viewport }, { duration: 220, interpolate: 'smooth' })
    options.onViewportTransform?.(viewport)
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
    if (typeof window === 'undefined') return
    setWorkspaceOverscrollGuard(true)
    window.addEventListener('resize', scheduleImageEditorChromeAnchorUpdate)
  })

  onBeforeUnmount(() => {
    if (imageEditorChromeAnchorFrame) cancelAnimationFrame(imageEditorChromeAnchorFrame)
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', scheduleImageEditorChromeAnchorUpdate)
      setWorkspaceOverscrollGuard(false)
    }
  })

  return {
    imageEditorChromeAnchorRect,
    isImageEditorChromeReady,
    onCanvasWheel,
    focusImageEditorAndShowChrome,
    scheduleImageEditorChromeAnchorUpdate,
  }
}
