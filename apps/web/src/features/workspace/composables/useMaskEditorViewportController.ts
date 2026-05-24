import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { IMAGE_UPLOAD_ACCEPT } from '../../../shared/constants/media'
import { clampImageNodeOutputCount } from '../types/canvasNodes'
import { MASK_EDITOR_CHROME_LAYOUT, getMaskEditorChromeOutset } from '../config/maskEditorChrome'

type FlowViewport = { x: number; y: number; zoom: number }

type MaskEditorAnchorRect = {
  top: number
  left: number
  width: number
  height: number
}

type MaskEditorReferenceImage = {
  id: string
  url: string
  ratio: number
}

type UseMaskEditorViewportControllerOptions = {
  nodes: Ref<Node[]>
  canvasWrapRef: Ref<HTMLElement | null>
  flowMaskEditorNodeId: Ref<string | null>
  getImageNodeSnapshot: (nodeId: string) => { outputCount?: number } | null
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
  uploadImageToBoard: (file: File) => Promise<{ url: string; ratio: number }>
  onWarning: (msg: string) => void
  onReferenceImagePreview?: (image: { url: string; ratio: number }) => void
  onViewportTransform?: (viewport: FlowViewport) => void
}

const MASK_EDITOR_OVERLAY_SELECTOR = '[data-image-mask-editor]'
const MASK_EDITOR_CANVAS_SELECTOR = '.image-mask-editor-canvas'
const MASK_EDITOR_FOCUS_MAX_ZOOM = 2
const MASK_EDITOR_FOCUS_MIN_ZOOM = 0.1
const WORKSPACE_OVERSCROLL_GUARD_CLASS = 'workspace-overscroll-guard'

function setWorkspaceOverscrollGuard(enabled: boolean) {
  if (typeof document === 'undefined') return
  document.documentElement.classList.toggle(WORKSPACE_OVERSCROLL_GUARD_CLASS, enabled)
  document.body.classList.toggle(WORKSPACE_OVERSCROLL_GUARD_CLASS, enabled)
}

export function useMaskEditorViewportController(options: UseMaskEditorViewportControllerOptions) {
  const maskEditorChromeAnchorRect = ref<MaskEditorAnchorRect | null>(null)
  const isMaskEditorChromeReady = ref(false)
  const maskEditorLayerOpacity = ref(72)
  const maskEditorReferenceImages = ref<MaskEditorReferenceImage[]>([])

  let maskEditorReturnViewport: FlowViewport | null = null
  let maskEditorFocusRequestId = 0
  let maskEditorChromeAnchorFrame = 0

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

  function resolveMaskEditorFocusTarget(nodeId: string): FlowViewport | null {
    const stage = options.getImageStageElement(nodeId)
    if (!stage) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: MASK_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const overlay = stage.querySelector<HTMLElement>(MASK_EDITOR_OVERLAY_SELECTOR)
    if (!overlay) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: MASK_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const overlayCanvas = overlay.querySelector<HTMLElement>(MASK_EDITOR_CANVAS_SELECTOR)
    const rect = overlayCanvas?.getBoundingClientRect()
    if (!rect || rect.width <= 0 || rect.height <= 0) {
      const center = resolveFlowNodeCenter(nodeId)
      return center ? { ...center, zoom: MASK_EDITOR_FOCUS_MAX_ZOOM } : null
    }

    const chromeOutset = getMaskEditorChromeOutset()
    const viewport = options.getViewport()
    const currentZoom = Number.isFinite(viewport.zoom) && viewport.zoom > 0 ? viewport.zoom : 1
    const flowImageWidth = rect.width / currentZoom
    const flowImageHeight = rect.height / currentZoom
    const viewportRect = getViewportRect()
    const availableWidth = Math.max(
      1,
      viewportRect.width - MASK_EDITOR_CHROME_LAYOUT.viewportPadding * 2,
    )
    const availableHeight = Math.max(
      1,
      viewportRect.height - MASK_EDITOR_CHROME_LAYOUT.viewportPadding * 2,
    )
    const availableImageWidth = Math.max(1, availableWidth - chromeOutset.left - chromeOutset.right)
    const fitZoom = Math.min(availableImageWidth / flowImageWidth, availableHeight / flowImageHeight)
    const zoom = Math.max(
      MASK_EDITOR_FOCUS_MIN_ZOOM,
      Math.min(
        MASK_EDITOR_FOCUS_MAX_ZOOM,
        Number.isFinite(fitZoom) && fitZoom > 0 ? fitZoom : MASK_EDITOR_FOCUS_MAX_ZOOM,
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

  function isMaskEditorFocusRequestCurrent(nodeId: string, requestId: number) {
    return requestId === maskEditorFocusRequestId && options.flowMaskEditorNodeId.value === nodeId
  }

  function waitForAnimationFrame(): Promise<void> {
    if (typeof requestAnimationFrame === 'undefined') return Promise.resolve()
    return new Promise((resolve) => {
      requestAnimationFrame(() => resolve())
    })
  }

  async function focusMaskEditorStage(nodeId: string, requestId: number) {
    await nextTick()
    if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
    const target = resolveMaskEditorFocusTarget(nodeId)
    if (!target) return
    if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
    await options.setCenter(target.x, target.y, {
      zoom: target.zoom,
      duration: 220,
      interpolate: 'smooth',
    })
    if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
    options.onViewportTransform?.(options.getViewport())
  }

  function updateMaskEditorChromeAnchorRect() {
    maskEditorChromeAnchorFrame = 0
    const nodeId = options.flowMaskEditorNodeId.value
    maskEditorChromeAnchorRect.value = nodeId ? resolveMaskEditorChromeAnchorRect(nodeId) : null
  }

  function resolveMaskEditorChromeAnchorRect(nodeId: string): MaskEditorAnchorRect | null {
    const wrap = options.canvasWrapRef.value
    const stage = options.getImageStageElement(nodeId)
    if (!wrap || !stage) return null
    const overlay = stage.querySelector<HTMLElement>(MASK_EDITOR_OVERLAY_SELECTOR)
    const target = overlay?.querySelector<HTMLElement>(MASK_EDITOR_CANVAS_SELECTOR) ?? overlay
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

  function scheduleMaskEditorChromeAnchorUpdate() {
    if (typeof requestAnimationFrame === 'undefined') {
      updateMaskEditorChromeAnchorRect()
      return
    }
    if (maskEditorChromeAnchorFrame) cancelAnimationFrame(maskEditorChromeAnchorFrame)
    maskEditorChromeAnchorFrame = requestAnimationFrame(updateMaskEditorChromeAnchorRect)
  }

  function restoreMaskEditorReturnViewport() {
    maskEditorFocusRequestId += 1
    const viewport = maskEditorReturnViewport
    if (!viewport) return
    maskEditorReturnViewport = null
    void options.setViewport({ ...viewport }, { duration: 220, interpolate: 'smooth' })
    options.onViewportTransform?.(viewport)
    scheduleMaskEditorChromeAnchorUpdate()
  }

  async function focusMaskEditorAndShowChrome(nodeId: string) {
    const requestId = ++maskEditorFocusRequestId
    maskEditorReturnViewport = { ...options.getViewport() }
    isMaskEditorChromeReady.value = false
    maskEditorChromeAnchorRect.value = null
    await focusMaskEditorStage(nodeId, requestId)
    if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
    await waitForAnimationFrame()
    if (!isMaskEditorFocusRequestCurrent(nodeId, requestId)) return
    updateMaskEditorChromeAnchorRect()
    if (maskEditorChromeAnchorRect.value) {
      isMaskEditorChromeReady.value = true
    }
  }

  function setMaskEditorLayerOpacity(value: number) {
    const next = Math.max(15, Math.min(100, Number(value) || 72))
    maskEditorLayerOpacity.value = next
  }

  function onMaskEditorReferenceImageOpen() {
    if (maskEditorReferenceImages.value.length >= 4) {
      options.onWarning('局部重绘最多添加 4 张参考图')
      return
    }
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = IMAGE_UPLOAD_ACCEPT
    input.onchange = () => {
      const file = input.files?.[0]
      if (!file) return
      void addMaskEditorReferenceImage(file)
    }
    input.click()
  }

  async function addMaskEditorReferenceImage(file: File) {
    if (maskEditorReferenceImages.value.length >= 4) {
      options.onWarning('局部重绘最多添加 4 张参考图')
      return
    }
    try {
      const uploaded = await options.uploadImageToBoard(file)
      maskEditorReferenceImages.value = [
        ...maskEditorReferenceImages.value,
        {
          id: `mask-ref:${Date.now()}:${Math.random().toString(16).slice(2, 8)}`,
          url: uploaded.url,
          ratio: uploaded.ratio,
        },
      ].slice(0, 4)
    } catch (error) {
      const msg = error instanceof Error ? error.message : '上传失败'
      options.onWarning(msg)
    }
  }

  function onMaskEditorReferenceImageRemove(imageId: string) {
    maskEditorReferenceImages.value = maskEditorReferenceImages.value.filter(
      (image) => image.id !== imageId,
    )
  }

  function onMaskEditorReferenceImagePreview(image: { url: string; ratio?: number }) {
    options.onReferenceImagePreview?.({
      url: image.url,
      ratio: Number(image.ratio) || 1,
    })
  }

  function onCanvasWheel(event: WheelEvent) {
    const isMaskEditing = !!options.flowMaskEditorNodeId.value
    const isBrowserGestureCandidate = event.ctrlKey || Math.abs(event.deltaX) > 0
    if (!isMaskEditing && !isBrowserGestureCandidate) return
    event.preventDefault()
    if (isMaskEditing) event.stopPropagation()
  }

  const activeMaskEditorOutputCount = computed(() => {
    const nodeId = options.flowMaskEditorNodeId.value
    if (!nodeId) return 1
    const snapshot = options.getImageNodeSnapshot(nodeId)
    return snapshot ? clampImageNodeOutputCount(snapshot.outputCount) : 1
  })

  watch(
    () => options.flowMaskEditorNodeId.value,
    (nextNodeId, prevNodeId) => {
      if (nextNodeId) {
        isMaskEditorChromeReady.value = false
        maskEditorLayerOpacity.value = 72
        maskEditorReferenceImages.value = []
        maskEditorChromeAnchorRect.value = null
        return
      }
      isMaskEditorChromeReady.value = false
      maskEditorChromeAnchorRect.value = null
      maskEditorReferenceImages.value = []
      if (prevNodeId) restoreMaskEditorReturnViewport()
    },
  )

  onMounted(() => {
    if (typeof window === 'undefined') return
    setWorkspaceOverscrollGuard(true)
    window.addEventListener('resize', scheduleMaskEditorChromeAnchorUpdate)
  })

  onBeforeUnmount(() => {
    if (maskEditorChromeAnchorFrame) cancelAnimationFrame(maskEditorChromeAnchorFrame)
    if (typeof window !== 'undefined') {
      window.removeEventListener('resize', scheduleMaskEditorChromeAnchorUpdate)
      setWorkspaceOverscrollGuard(false)
    }
  })

  return {
    maskEditorChromeAnchorRect,
    isMaskEditorChromeReady,
    maskEditorLayerOpacity,
    maskEditorReferenceImages,
    activeMaskEditorOutputCount,
    setMaskEditorLayerOpacity,
    onMaskEditorReferenceImageOpen,
    onMaskEditorReferenceImageRemove,
    onMaskEditorReferenceImagePreview,
    onCanvasWheel,
    focusMaskEditorAndShowChrome,
    scheduleMaskEditorChromeAnchorUpdate,
  }
}
