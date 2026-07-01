import { useEventListener } from '@vueuse/core'
import { computed, ref, watch, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import { clampImageNodeOutputCount } from '../../canvas-board/domain'
import {
  containImageRect,
  createCanvasExtendRect,
  resizeCanvasExtendFrameFromDrag,
  resolveDefaultCanvasExtendFrame,
  type CanvasExtendRect,
} from '../utils/canvasExtendGeometry'
import { isEditableEventTarget } from '../utils/domEventTargets'
import { usePointerDragSession } from './usePointerDragSession'

const windowTarget = () => (typeof window === 'undefined' ? null : window)

type ImageDisplayPayload = {
  url: string
  ratio: number
}

type ExtendOverlayState = {
  nodeId: string
  frameRect: CanvasExtendRect
  imageRect: CanvasExtendRect
}

type ExtendCommitPayload = {
  nodeId: string
  frameRect: CanvasExtendRect
  originalImageRect: CanvasExtendRect
  prompt: string
  outputCount: number
}

type UseImageCanvasExtendOverlayOptions = {
  nodes: Ref<Node[]>
  activeTool: Ref<string>
  getImageNodeDisplayPayload: (nodeId: string) => ImageDisplayPayload | null
  getImageStageElement: (nodeId: string) => HTMLElement | null
  onBeforePointerCancel?: () => void
  onCommit: (payload: ExtendCommitPayload) => Promise<void> | void
}

export const CANVAS_EXTEND_DRAG_ANCHORS = ['n', 'e', 's', 'w', 'nw', 'ne', 'sw', 'se'] as const
export type CanvasExtendDragAnchor = (typeof CANVAS_EXTEND_DRAG_ANCHORS)[number]

type ResizeDragSession = {
  mode: 'resize'
  pointerId: number
  anchor: CanvasExtendDragAnchor
  startClientX: number
  startClientY: number
  initialFrame: CanvasExtendRect
  imageRect: CanvasExtendRect
}
type MoveDragSession = {
  mode: 'move'
  pointerId: number
  startClientX: number
  startClientY: number
  initialFrame: CanvasExtendRect
  imageRect: CanvasExtendRect
}
type DragSession = ResizeDragSession | MoveDragSession

const HANDLE_RADIUS = 4

export function useImageCanvasExtendOverlay(options: UseImageCanvasExtendOverlayOptions) {
  const state = ref<ExtendOverlayState | null>(null)
  const dragSession = ref<DragSession | null>(null)
  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onHandlePointerMove(event),
    onEnd: clearDragSession,
    onCancel: clearDragSession,
  })

  const isExpandMode = computed(() => !!state.value)
  const activeExpandNodeKind = computed(() => {
    const nodeId = state.value?.nodeId
    if (!nodeId) return null
    return options.nodes.value.find((item) => item.id === nodeId)?.data?.kind ?? null
  })
  const expandOverlayStyle = computed<Record<string, string> | null>(() => {
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

  function cancelExpandMode() {
    stopDragSession()
    state.value = null
  }

  function enterExpandMode(nodeId: string): boolean {
    const display = options.getImageNodeDisplayPayload(nodeId)
    if (!display?.url) return false
    const stage = options.getImageStageElement(nodeId)
    if (!stage) return false
    const containRect = containImageRect(stage.clientWidth, stage.clientHeight, display.ratio)
    if (!containRect) return false
    const imageRect = containRect
    const frameRect = resolveDefaultCanvasExtendFrame(imageRect)
    state.value = { nodeId, frameRect, imageRect }
    return true
  }

  function beginHandleResize(event: PointerEvent, anchor: CanvasExtendDragAnchor) {
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
    const frame = resizeCanvasExtendFrameFromDrag(
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
      frameRect: createCanvasExtendRect(
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

  async function commitExpandMode(prompt = '', outputCount = 1) {
    if (!state.value) return
    const nodeId = state.value.nodeId
    const payload: ExtendCommitPayload = {
      nodeId,
      frameRect: { ...state.value.frameRect },
      originalImageRect: { ...state.value.imageRect },
      prompt: `${prompt}`.trim(),
      outputCount: clampImageNodeOutputCount(outputCount),
    }
    cancelExpandMode()
    await options.onCommit(payload)
  }

  function onGlobalKeydown(event: KeyboardEvent) {
    if (!state.value) return
    const target = event.target as HTMLElement | null
    if (target?.closest('[data-image-extend-composer]')) return
    if (isEditableEventTarget(event.target)) return
    if (event.key === 'Escape') {
      event.preventDefault()
      cancelExpandMode()
      return
    }
    if (event.key === 'Enter' && !event.isComposing) {
      event.preventDefault()
      void commitExpandMode()
    }
  }

  function onGlobalPointerDown(event: PointerEvent) {
    if (!state.value) return
    if (event.button !== 0) return
    const target = event.target as HTMLElement | null
    if (!target) return
    const stage = options.getImageStageElement(state.value.nodeId)
    const inImageStage = !!stage?.contains(target)
    const inOverlayFrame = !!target.closest('[data-image-extend-frame]')
    const inOverlayComposer = !!target.closest('[data-image-extend-composer]')
    if (inOverlayComposer) return
    if (inImageStage && !inOverlayFrame) {
      options.onBeforePointerCancel?.()
      cancelExpandMode()
      return
    }
    if (!inImageStage) {
      options.onBeforePointerCancel?.()
      cancelExpandMode()
    }
  }

  watch(
    () => options.activeTool.value,
    () => {
      if (options.activeTool.value !== 'select' && state.value) {
        cancelExpandMode()
      }
    },
  )

  watch(activeExpandNodeKind, (kind) => {
    if (!state.value) return
    if (kind !== 'image') cancelExpandMode()
  })

  useEventListener<KeyboardEvent>(windowTarget, 'keydown', onGlobalKeydown, true)
  useEventListener<PointerEvent>(windowTarget, 'pointerdown', onGlobalPointerDown, true)

  return {
    isExpandMode,
    expandNodeId: computed(() => state.value?.nodeId ?? null),
    expandOverlayStyle,
    enterExpandMode,
    cancelExpandMode,
    beginHandleResize,
    beginFrameDrag,
    onHandlePointerMove,
    onHandlePointerUp,
    commitExpandMode,
    handleRadius: HANDLE_RADIUS,
  }
}
