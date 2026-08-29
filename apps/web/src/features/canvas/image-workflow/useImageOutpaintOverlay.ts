import { tryOnScopeDispose, useEventListener } from '@vueuse/core'
import { computed, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { CanvasInteractionKey } from '../workspace-ui/useFlowCanvasInteractionState'
import {
  containImageRect,
  createOutpaintRect,
  resizeOutpaintFrameFromDrag,
  resolveDefaultOutpaintFrame,
  type OutpaintRect,
} from '../utils/outpaintGeometry'
import { isEditableEventTarget } from '../utils/domEventTargets'
import { useCanvasCustomLayerOverlay } from '../overlay-lifecycle'
import { usePointerDragSession } from '../utils/interaction/usePointerDragSession'

const windowTarget = () => (typeof window === 'undefined' ? null : window)

type ImageDisplayPayload = {
  url: string
  ratio: number
}

type OutpaintOverlayState = {
  nodeId: string
  frameRect: OutpaintRect
  imageRect: OutpaintRect
}

type OutpaintCommitPayload = {
  nodeId: string
  frameRect: OutpaintRect
  originalImageRect: OutpaintRect
  prompt: string
}

type UseImageOutpaintOverlayOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  startCanvasInteraction?: (key: CanvasInteractionKey) => void
  stopCanvasInteraction?: (key: CanvasInteractionKey) => void
  onBeforePointerCancel?: () => void
  onCommit: (payload: OutpaintCommitPayload) => Promise<void> | void
}

export const OUTPAINT_DRAG_ANCHORS = ['n', 'e', 's', 'w', 'nw', 'ne', 'sw', 'se'] as const
export type OutpaintDragAnchor = (typeof OUTPAINT_DRAG_ANCHORS)[number]

type ResizeDragSession = {
  mode: 'resize'
  pointerId: number
  anchor: OutpaintDragAnchor
  startClientX: number
  startClientY: number
  initialFrame: OutpaintRect
  imageRect: OutpaintRect
}
type MoveDragSession = {
  mode: 'move'
  pointerId: number
  startClientX: number
  startClientY: number
  initialFrame: OutpaintRect
  imageRect: OutpaintRect
}
type DragSession = ResizeDragSession | MoveDragSession

const HANDLE_RADIUS = 4
const OUTPAINT_OVERLAY_INTERACTION_KEY: CanvasInteractionKey = 'outpaint-overlay'

export function useImageOutpaintOverlay(options: UseImageOutpaintOverlayOptions) {
  const state = ref<OutpaintOverlayState | null>(null)
  const dragSession = ref<DragSession | null>(null)
  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onHandlePointerMove(event),
    onEnd: clearDragSession,
    onCancel: clearDragSession,
  })

  const isOutpaintMode = computed(() => !!state.value)
  const activeOutpaintNodeKind = computed(() => {
    const nodeId = state.value?.nodeId
    if (!nodeId) return null
    return options.nodes.value.find((item) => item.id === nodeId)?.data?.kind ?? null
  })
  const outpaintOverlayStyle = computed<Record<string, string> | null>(() => {
    if (!state.value) return null
    const rect = state.value.frameRect
    return {
      left: `${rect.left}px`,
      top: `${rect.top}px`,
      width: `${rect.width}px`,
      height: `${rect.height}px`,
    }
  })

  function clearDragSession() {
    dragSession.value = null
  }

  function stopDragSession() {
    const stoppedDragSession = pointerDragSession.endDrag()
    if (!stoppedDragSession) clearDragSession()
  }

  function startOutpaintInteraction() {
    options.startCanvasInteraction?.(OUTPAINT_OVERLAY_INTERACTION_KEY)
  }

  function stopOutpaintInteraction() {
    options.stopCanvasInteraction?.(OUTPAINT_OVERLAY_INTERACTION_KEY)
  }

  function cancelOutpaintMode() {
    stopDragSession()
    if (state.value) {
      state.value = null
      stopOutpaintInteraction()
    }
  }

  function enterOutpaintMode(nodeId: string): boolean {
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false
    const stage = options.getImageStageElement(nodeId)
    if (!stage) return false
    const containRect = containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
    if (!containRect) return false
    const imageRect = containRect
    const frameRect = resolveDefaultOutpaintFrame(imageRect)
    const wasOutpaintActive = !!state.value
    state.value = { nodeId, frameRect, imageRect }
    if (!wasOutpaintActive) startOutpaintInteraction()
    return true
  }

  function beginHandleResize(event: PointerEvent, anchor: OutpaintDragAnchor) {
    if (!state.value) return
    if (!(event.target instanceof HTMLElement)) return
    event.preventDefault()
    event.stopPropagation()
    stopDragSession()
    pointerDragSession.beginDrag(event, undefined, { target: event.target })
    dragSession.value = {
      mode: 'resize',
      pointerId: event.pointerId,
      anchor,
      startClientX: event.clientX,
      startClientY: event.clientY,
      initialFrame: { ...state.value.frameRect },
      imageRect: { ...state.value.imageRect },
    }
  }

  function beginFrameDrag(event: PointerEvent) {
    if (!state.value) return
    if (!(event.target instanceof HTMLElement)) return
    event.preventDefault()
    event.stopPropagation()
    stopDragSession()
    pointerDragSession.beginDrag(event, undefined, { target: event.target })
    dragSession.value = {
      mode: 'move',
      pointerId: event.pointerId,
      startClientX: event.clientX,
      startClientY: event.clientY,
      initialFrame: { ...state.value.frameRect },
      imageRect: { ...state.value.imageRect },
    }
  }

  function resizeFromDrag(session: ResizeDragSession, clientX: number, clientY: number) {
    const deltaX = clientX - session.startClientX
    const deltaY = clientY - session.startClientY
    const frame = resizeOutpaintFrameFromDrag(
      session.initialFrame,
      session.imageRect,
      session.anchor,
      deltaX,
      deltaY,
    )
    if (!state.value) return
    state.value = {
      ...state.value,
      frameRect: frame,
    }
  }

  function moveFromDrag(session: MoveDragSession, clientX: number, clientY: number) {
    const deltaX = clientX - session.startClientX
    const deltaY = clientY - session.startClientY
    const minLeft = session.imageRect.left + session.imageRect.width - session.initialFrame.width
    const maxLeft = session.imageRect.left
    const minTop = session.imageRect.top + session.imageRect.height - session.initialFrame.height
    const maxTop = session.imageRect.top
    const nextLeft = Math.max(minLeft, Math.min(maxLeft, session.initialFrame.left + deltaX))
    const nextTop = Math.max(minTop, Math.min(maxTop, session.initialFrame.top + deltaY))

    if (!state.value) return
    state.value = {
      ...state.value,
      frameRect: createOutpaintRect(
        nextLeft,
        nextTop,
        session.initialFrame.width,
        session.initialFrame.height,
      ),
    }
  }

  function onHandlePointerMove(event: PointerEvent) {
    const session = dragSession.value
    if (!session || session.pointerId !== event.pointerId) return
    if (session.mode === 'resize') {
      resizeFromDrag(session, event.clientX, event.clientY)
      return
    }
    moveFromDrag(session, event.clientX, event.clientY)
  }

  function onHandlePointerUp(event: PointerEvent) {
    const session = dragSession.value
    if (!session || session.pointerId !== event.pointerId) return
    stopDragSession()
  }

  async function commitOutpaintMode(prompt = '') {
    if (!state.value) return
    const nodeId = state.value.nodeId
    const payload: OutpaintCommitPayload = {
      nodeId,
      frameRect: { ...state.value.frameRect },
      originalImageRect: { ...state.value.imageRect },
      prompt: `${prompt}`.trim(),
    }
    cancelOutpaintMode()
    await options.onCommit(payload)
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!state.value) return
    const target = event.target as HTMLElement | null
    if (target?.closest('[data-outpaint-composer]')) return
    if (isEditableEventTarget(event.target)) return
    if (event.key === 'Enter' && !event.isComposing) {
      event.preventDefault()
      void commitOutpaintMode()
    }
  }

  useCanvasCustomLayerOverlay({
    visible: isOutpaintMode,
    ignore: ['[data-outpaint-frame]', '[data-outpaint-composer]'],
    ignoreEscape: [
      '[data-outpaint-composer]',
      'textarea',
      'input:not([type="button"]):not([type="submit"]):not([type="reset"])',
      '[contenteditable=""]',
      '[contenteditable="true"]',
    ],
    ownerDocument: () => {
      const nodeId = state.value?.nodeId
      return (
        (nodeId ? options.getImageStageElement(nodeId)?.ownerDocument : null) ??
        (typeof document === 'undefined' ? null : document)
      )
    },
    shouldDismiss: (dismissal, event) =>
      dismissal !== 'outside-pointer' || !(event instanceof PointerEvent) || event.button === 0,
    onDismiss: (dismissal) => {
      if (dismissal === 'outside-pointer') options.onBeforePointerCancel?.()
      cancelOutpaintMode()
    },
  })

  watch(
    () => options.activeTool.value,
    () => {
      if (options.activeTool.value !== 'select' && state.value) {
        cancelOutpaintMode()
      }
    },
  )

  watch(activeOutpaintNodeKind, (kind) => {
    if (!state.value) return
    if (kind !== 'image') cancelOutpaintMode()
  })

  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onGlobalKeydown, true)
  tryOnScopeDispose(cancelOutpaintMode)

  return {
    isOutpaintMode,
    outpaintNodeId: computed(() => state.value?.nodeId ?? null),
    outpaintOverlayStyle,
    enterOutpaintMode,
    cancelOutpaintMode,
    beginHandleResize,
    beginFrameDrag,
    onHandlePointerMove,
    onHandlePointerUp,
    commitOutpaintMode,
    handleRadius: HANDLE_RADIUS,
  }
}
