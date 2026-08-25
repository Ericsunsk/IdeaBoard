import { computed, ref, watch, type ComputedRef, type Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { CanvasGraphGroupId } from '../graph-runtime'
import {
  resolveCanvasSelectionGroupActionBounds,
  resolveCanvasSelectionGroupActionButtonPosition,
  type CanvasFlowViewportTransform,
  type CanvasGroupFrame,
  type CanvasGroupResizeHandle,
  type CanvasPoint,
} from '../utils/canvasGroupGeometry'
import type { CanvasSelectionGroupAction } from '../graph-runtime/application/useCanvasGroupRuntime'
import type { DismissTransientCanvasUi } from '../workspace-ui/useFlowTransientUiDismissal'

export type CanvasGroupResizeStartPayload = {
  event: PointerEvent
  groupId: CanvasGraphGroupId
  handle: CanvasGroupResizeHandle
}

type CanvasGroupDragControls = {
  begin: (point: CanvasPoint) => boolean
  move: (point: CanvasPoint) => boolean
  commit: () => boolean
  cancel: () => boolean
}

type CanvasGroupResizeControls = {
  begin: (
    groupId: CanvasGraphGroupId,
    handle: CanvasGroupResizeHandle,
    point: CanvasPoint,
  ) => boolean
  move: (point: CanvasPoint) => boolean
  commit: () => boolean
  cancel: () => boolean
}

type CanvasGroupClosePolicyHandler = (...args: never[]) => unknown
type CanvasGroupClosePolicyHandlers = Record<string, CanvasGroupClosePolicyHandler>
type CanvasGroupInteractionViewport =
  ComputedRef<CanvasFlowViewportTransform> | Ref<CanvasFlowViewportTransform>

export type UseCanvasGroupPointerInteractionOptions = {
  canvasWrapRef: Ref<HTMLElement | null>
  nodes: Ref<Node[]>
  groupFrames: ComputedRef<CanvasGroupFrame[]>
  viewport: CanvasGroupInteractionViewport
  selectionGroupAction: ComputedRef<CanvasSelectionGroupAction | null>
  isEnabled: ComputedRef<boolean>
  screenToFlowCoordinate: (point: CanvasPoint) => CanvasPoint
  paneSelectionPointerDownCapture: (event: PointerEvent) => void
  selectGroupAtPoint: (point: CanvasPoint) => boolean
  groupDrag: CanvasGroupDragControls
  groupResize: CanvasGroupResizeControls
  dismissTransientCanvasUi: (options: Parameters<DismissTransientCanvasUi>[0]) => void
}

const ACTION_BUTTON_SELECTOR = '.canvas-selection-group-action-button'
const GROUP_FRAME_BODY_SELECTOR = '.canvas-group-frame-body'
const GROUP_FRAME_RESIZE_HANDLE_SELECTOR = '.canvas-group-frame-resize-handle'
const FLOW_PANE_CLASS = 'vue-flow__pane'
const ACTION_BUTTON_SELECTION_GAP_PX = 12

export function useCanvasGroupPointerInteraction(options: UseCanvasGroupPointerInteractionOptions) {
  const isSelectionGroupActionMenuOpen = ref(false)
  let activeGroupDragPointerId: number | null = null
  let activeGroupResizePointerId: number | null = null

  const isSelectionGroupActionVisible = computed(
    () =>
      isSelectionGroupActionMenuOpen.value &&
      !!options.selectionGroupAction.value &&
      options.isEnabled.value,
  )

  const selectionGroupActionButtonPosition = computed(() =>
    resolveCanvasSelectionGroupActionButtonPosition({
      action: options.selectionGroupAction.value,
      nodes: options.nodes.value,
      frames: options.groupFrames.value,
      viewport: options.viewport.value,
      gapPx: ACTION_BUTTON_SELECTION_GAP_PX,
    }),
  )

  function readOwnerWindow() {
    return options.canvasWrapRef.value?.ownerDocument.defaultView ?? window
  }

  function readPointerFlowPoint(event: PointerEvent) {
    return options.screenToFlowCoordinate({ x: event.clientX, y: event.clientY })
  }

  function closeSelectionGroupAction() {
    isSelectionGroupActionMenuOpen.value = false
  }

  function withSelectionGroupActionClosed<TArgs extends unknown[], TResult>(
    handler: (...args: TArgs) => TResult,
  ) {
    return (...args: TArgs) => {
      closeSelectionGroupAction()
      return handler(...args)
    }
  }

  function bindClosePolicyHandlers<THandlers extends CanvasGroupClosePolicyHandlers>(
    handlers: THandlers,
  ): THandlers {
    const wrappedHandlers = {} as THandlers
    for (const key of Object.keys(handlers) as Array<keyof THandlers>) {
      wrappedHandlers[key] = withSelectionGroupActionClosed(handlers[key]) as THandlers[typeof key]
    }
    return wrappedHandlers
  }

  function isSelectionGroupActionButtonTarget(target: EventTarget | null) {
    return target instanceof Element && !!target.closest(ACTION_BUTTON_SELECTOR)
  }

  function isVueFlowPointerTarget(target: EventTarget | null): boolean {
    return target instanceof Element && target.classList.contains(FLOW_PANE_CLASS)
  }

  function isCanvasGroupFrameBodyTarget(target: EventTarget | null): boolean {
    return target instanceof Element && !!target.closest(GROUP_FRAME_BODY_SELECTOR)
  }

  function isCanvasGroupResizeHandleTarget(target: EventTarget | null): boolean {
    return target instanceof Element && !!target.closest(GROUP_FRAME_RESIZE_HANDLE_SELECTOR)
  }

  function isVueFlowElementContextMenuTarget(target: EventTarget | null): boolean {
    if (!(target instanceof Element)) return false
    let current: Element | null = target
    while (current) {
      if (
        current.classList.contains('vue-flow__node') ||
        current.classList.contains('vue-flow__edge') ||
        current.classList.contains('canvas-node-context-menu')
      ) {
        return true
      }
      current = current.parentElement
    }
    return false
  }

  function bindGroupDragListeners() {
    const ownerWindow = readOwnerWindow()
    ownerWindow.addEventListener('pointermove', onGroupDragPointerMove)
    ownerWindow.addEventListener('pointerup', onGroupDragPointerUp)
    ownerWindow.addEventListener('pointercancel', onGroupDragPointerCancel)
  }

  function cleanupGroupDragListeners() {
    const ownerWindow = readOwnerWindow()
    ownerWindow.removeEventListener('pointermove', onGroupDragPointerMove)
    ownerWindow.removeEventListener('pointerup', onGroupDragPointerUp)
    ownerWindow.removeEventListener('pointercancel', onGroupDragPointerCancel)
    activeGroupDragPointerId = null
  }

  function bindGroupResizeListeners() {
    const ownerWindow = readOwnerWindow()
    ownerWindow.addEventListener('pointermove', onGroupResizePointerMove)
    ownerWindow.addEventListener('pointerup', onGroupResizePointerUp)
    ownerWindow.addEventListener('pointercancel', onGroupResizePointerCancel)
  }

  function cleanupGroupResizeListeners() {
    const ownerWindow = readOwnerWindow()
    ownerWindow.removeEventListener('pointermove', onGroupResizePointerMove)
    ownerWindow.removeEventListener('pointerup', onGroupResizePointerUp)
    ownerWindow.removeEventListener('pointercancel', onGroupResizePointerCancel)
    activeGroupResizePointerId = null
  }

  function onGroupDragPointerMove(event: PointerEvent) {
    if (activeGroupDragPointerId !== event.pointerId) return
    options.groupDrag.move(readPointerFlowPoint(event))
  }

  function onGroupDragPointerUp(event: PointerEvent) {
    if (activeGroupDragPointerId !== event.pointerId) return
    cleanupGroupDragListeners()
    options.groupDrag.commit()
  }

  function onGroupDragPointerCancel(event: PointerEvent) {
    if (activeGroupDragPointerId !== event.pointerId) return
    cleanupGroupDragListeners()
    options.groupDrag.cancel()
  }

  function onGroupResizePointerMove(event: PointerEvent) {
    if (activeGroupResizePointerId !== event.pointerId) return
    options.groupResize.move(readPointerFlowPoint(event))
  }

  function onGroupResizePointerUp(event: PointerEvent) {
    if (activeGroupResizePointerId !== event.pointerId) return
    cleanupGroupResizeListeners()
    options.groupResize.commit()
  }

  function onGroupResizePointerCancel(event: PointerEvent) {
    if (activeGroupResizePointerId !== event.pointerId) return
    cleanupGroupResizeListeners()
    options.groupResize.cancel()
  }

  function onCanvasContextMenuCapture(event: MouseEvent) {
    if (isSelectionGroupActionButtonTarget(event.target)) return
    if (isVueFlowElementContextMenuTarget(event.target)) return
    if (!options.isEnabled.value) {
      closeSelectionGroupAction()
      return
    }

    if (!options.selectionGroupAction.value) {
      closeSelectionGroupAction()
      return
    }

    const bounds = resolveCanvasSelectionGroupActionBounds({
      action: options.selectionGroupAction.value,
      nodes: options.nodes.value,
      frames: options.groupFrames.value,
      requireSelectedGroupFrame: true,
    })
    if (options.selectionGroupAction.value.kind === 'ungroup' && !bounds) {
      closeSelectionGroupAction()
      return
    }

    event.preventDefault()
    event.stopPropagation()
    options.dismissTransientCanvasUi({
      closeExpand: true,
      closeBoardHeader: true,
      closeContextMenu: true,
      clearNodePanel: false,
      hideStrength: false,
    })
    isSelectionGroupActionMenuOpen.value = true
  }

  function onGroupResizeStart(payload: CanvasGroupResizeStartPayload) {
    if (!options.isEnabled.value) return
    const event = payload.event
    if (event.button !== 0 || event.pointerType === 'touch') return
    closeSelectionGroupAction()
    const flowPoint = readPointerFlowPoint(event)
    if (!options.groupResize.begin(payload.groupId, payload.handle, flowPoint)) return
    activeGroupResizePointerId = event.pointerId
    bindGroupResizeListeners()
  }

  function tryBeginCanvasGroupPointerInteraction(event: PointerEvent): boolean {
    if (!options.isEnabled.value) return false
    if (event.button !== 0 || event.pointerType === 'touch') return false
    if (isSelectionGroupActionButtonTarget(event.target)) return false
    if (isCanvasGroupResizeHandleTarget(event.target)) return false
    if (!isVueFlowPointerTarget(event.target) && !isCanvasGroupFrameBodyTarget(event.target)) {
      return false
    }

    const flowPoint = readPointerFlowPoint(event)
    if (!options.selectGroupAtPoint(flowPoint)) return false
    event.preventDefault()
    event.stopPropagation()
    closeSelectionGroupAction()

    if (options.groupDrag.begin(flowPoint)) {
      activeGroupDragPointerId = event.pointerId
      bindGroupDragListeners()
    }
    return true
  }

  function onCanvasPointerDownCapture(event: PointerEvent) {
    if (tryBeginCanvasGroupPointerInteraction(event)) return
    options.paneSelectionPointerDownCapture(event)
  }

  function cleanup() {
    closeSelectionGroupAction()
    cleanupGroupDragListeners()
    cleanupGroupResizeListeners()
    options.groupDrag.cancel()
    options.groupResize.cancel()
  }

  watch(options.selectionGroupAction, () => {
    closeSelectionGroupAction()
  })

  watch(options.isEnabled, (enabled) => {
    if (!enabled) closeSelectionGroupAction()
  })

  return {
    isSelectionGroupActionMenuOpen,
    isSelectionGroupActionVisible,
    selectionGroupActionButtonPosition,
    closeSelectionGroupAction,
    bindClosePolicyHandlers,
    onCanvasContextMenuCapture,
    onCanvasPointerDownCapture,
    onGroupResizeStart,
    cleanup,
  }
}
