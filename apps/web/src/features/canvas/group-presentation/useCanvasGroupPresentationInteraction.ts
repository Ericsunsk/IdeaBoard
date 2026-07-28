import type { Node } from '@vue-flow/core'
import { onScopeDispose, type ComputedRef, type Ref } from 'vue'
import type { CanvasGraphGroupId } from '../graph-runtime'
import type {
  CanvasFlowViewportTransform,
  CanvasGroupFrame,
  CanvasGroupResizeHandle,
  CanvasPoint,
} from '../utils/canvasGroupGeometry'
import type { CanvasSelectionGroupAction } from '../composables/useCanvasGroupRuntime'
import type { DismissTransientCanvasUi } from '../composables/useFlowTransientUiDismissal'
import { useCanvasGroupPointerInteraction } from './useCanvasGroupPointerInteraction'

export type CanvasGroupPresentationDragControls = {
  begin: (point: CanvasPoint) => boolean
  move: (point: CanvasPoint) => boolean
  commit: () => boolean
  cancel: () => boolean
}

export type CanvasGroupPresentationResizeControls = {
  begin: (groupId: CanvasGraphGroupId, handle: CanvasGroupResizeHandle, point: CanvasPoint) => boolean
  move: (point: CanvasPoint) => boolean
  commit: () => boolean
  cancel: () => boolean
}

export type CanvasGroupPresentationCanvas = {
  host: Ref<HTMLElement | null>
  nodes: Ref<Node[]>
  viewport: ComputedRef<CanvasFlowViewportTransform> | Ref<CanvasFlowViewportTransform>
  isInteractionEnabled: ComputedRef<boolean>
}

export type CanvasGroupPresentationGrouping = {
  frames: ComputedRef<CanvasGroupFrame[]>
  action: ComputedRef<CanvasSelectionGroupAction | null>
  selectAtPoint: (point: CanvasPoint) => boolean
  drag: CanvasGroupPresentationDragControls
  resize: CanvasGroupPresentationResizeControls
  onAction: () => void
}

export type CanvasGroupPresentationPane = {
  toFlowPoint: (point: CanvasPoint) => CanvasPoint
  onPointerDownCapture: (event: PointerEvent) => void
  dismiss: (options: Parameters<DismissTransientCanvasUi>[0]) => void
}

export type UseCanvasGroupPresentationInteractionOptions = {
  canvas: CanvasGroupPresentationCanvas
  grouping: CanvasGroupPresentationGrouping
  pane: CanvasGroupPresentationPane
}

export function useCanvasGroupPresentationInteraction(
  options: UseCanvasGroupPresentationInteractionOptions,
) {
  const interactionShell = useCanvasGroupPointerInteraction({
    canvasWrapRef: options.canvas.host,
    nodes: options.canvas.nodes,
    groupFrames: options.grouping.frames,
    viewport: options.canvas.viewport,
    selectionGroupAction: options.grouping.action,
    isEnabled: options.canvas.isInteractionEnabled,
    screenToFlowCoordinate: options.pane.toFlowPoint,
    paneSelectionPointerDownCapture: options.pane.onPointerDownCapture,
    selectGroupAtPoint: options.grouping.selectAtPoint,
    groupDrag: options.grouping.drag,
    groupResize: options.grouping.resize,
    dismissTransientCanvasUi: options.pane.dismiss,
  })
  const { onAction } = interactionShell.bindClosePolicyHandlers({
    onAction: options.grouping.onAction,
  })
  onScopeDispose(interactionShell.cleanup)
  const { cleanup: _cleanup, ...presentationInteraction } = interactionShell

  return {
    ...presentationInteraction,
    groupFrames: options.grouping.frames,
    selectionGroupAction: options.grouping.action,
    onSelectionGroupAction: onAction,
  }
}

export type CanvasGroupPresentationInteraction = ReturnType<
  typeof useCanvasGroupPresentationInteraction
>
