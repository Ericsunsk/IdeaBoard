import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import type { ImageActionBinding, ImageToolStageBinding } from '../image-tools'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

export type ImageNodePresentationBinding = {
  actionBinding: ImageActionBinding
  stageBinding: ImageToolStageBinding | null
}

export type ImageNodePresentationBindingSource = {
  actionBindingFor: (nodeId: string) => ImageActionBinding
  stageBindingFor: (nodeId: string) => ImageToolStageBinding | null
}

export type ImageNodePresentationResolver = {
  forNode: (nodeId: string) => ImageNodePresentationBinding
}

export function createImageNodePresentationResolver(
  source: ImageNodePresentationBindingSource,
): ImageNodePresentationResolver {
  const cache = new Map<
    string,
    {
      binding: ImageNodePresentationBinding
      actionBindingIdentity: ImageActionBinding
      stageBindingIdentity: ImageToolStageBinding | null
    }
  >()

  function forNode(nodeId: string): ImageNodePresentationBinding {
    const actionBinding = source.actionBindingFor(nodeId)
    const stageBinding = source.stageBindingFor(nodeId)
    const cached = cache.get(nodeId)
    if (
      cached?.actionBindingIdentity === actionBinding &&
      cached.stageBindingIdentity === stageBinding
    ) {
      return cached.binding
    }

    const binding: ImageNodePresentationBinding = { actionBinding, stageBinding }
    cache.set(nodeId, {
      binding,
      actionBindingIdentity: actionBinding,
      stageBindingIdentity: stageBinding,
    })
    return binding
  }

  return { forNode }
}

export type ImageNodePresentationIntent =
  | { type: 'parameter-changed'; nodeId: string; patch: FlowSnapshotParamPatch }
  | { type: 'delete-requested'; nodeId: string }
  | { type: 'stage-dropped'; nodeId: string; event: DragEvent }
  | { type: 'file-open-requested'; nodeId: string }
  | { type: 'preview-requested'; nodeId: string }
  | { type: 'append-to-canvas-requested'; nodeId: string }
  | { type: 'reset-requested'; nodeId: string }
  | { type: 'context-menu-requested'; nodeId: string; event: MouseEvent }
  | { type: 'prompt-generation-requested'; nodeId: string; prompt: string; outputCount: number }
  | { type: 'canvas-extend-submitted'; nodeId: string; prompt: string; outputCount: number }
  | { type: 'action-requested'; nodeId: string; action: ImageNodeAction }
  | {
      type: 'canvas-extend-handle-pointerdown'
      nodeId: string
      anchor: CanvasExtendDragAnchor
      event: PointerEvent
    }
  | { type: 'canvas-extend-handle-pointermove'; event: PointerEvent }
  | { type: 'canvas-extend-handle-pointerup'; event: PointerEvent }
  | { type: 'canvas-extend-frame-pointerdown'; nodeId: string; event: PointerEvent }

export type ImageNodePresentationIntentHandlers = {
  onParameterChanged: (nodeId: string, patch: FlowSnapshotParamPatch) => void
  onDeleteRequested: (nodeId: string) => void
  onStageDrop: (nodeId: string, event: DragEvent) => void
  onFileOpenRequested: (nodeId: string) => void
  onPreviewRequested: (nodeId: string) => void
  onAppendToCanvasRequested: (nodeId: string) => void
  onResetRequested: (nodeId: string) => void
  onContextMenuRequested: (nodeId: string, event: MouseEvent) => void
  onPromptGenerationRequested: (nodeId: string, prompt: string, outputCount: number) => void
  onCanvasExtendSubmitted: (nodeId: string, prompt: string, outputCount: number) => void
  onImageActionRequested: (nodeId: string, action: ImageNodeAction) => void
  onCanvasExtendHandlePointerDown: (
    nodeId: string,
    anchor: CanvasExtendDragAnchor,
    event: PointerEvent,
  ) => void
  onCanvasExtendHandlePointerMove: (event: PointerEvent) => void
  onCanvasExtendHandlePointerUp: (event: PointerEvent) => void
  onCanvasExtendFramePointerDown: (nodeId: string, event: PointerEvent) => void
}

export function createImageNodePresentationIntentHandler(
  handlers: ImageNodePresentationIntentHandlers,
) {
  return (intent: ImageNodePresentationIntent) => {
    switch (intent.type) {
      case 'parameter-changed':
        handlers.onParameterChanged(intent.nodeId, intent.patch)
        return
      case 'delete-requested':
        handlers.onDeleteRequested(intent.nodeId)
        return
      case 'stage-dropped':
        handlers.onStageDrop(intent.nodeId, intent.event)
        return
      case 'file-open-requested':
        handlers.onFileOpenRequested(intent.nodeId)
        return
      case 'preview-requested':
        handlers.onPreviewRequested(intent.nodeId)
        return
      case 'append-to-canvas-requested':
        handlers.onAppendToCanvasRequested(intent.nodeId)
        return
      case 'reset-requested':
        handlers.onResetRequested(intent.nodeId)
        return
      case 'context-menu-requested':
        handlers.onContextMenuRequested(intent.nodeId, intent.event)
        return
      case 'prompt-generation-requested':
        handlers.onPromptGenerationRequested(intent.nodeId, intent.prompt, intent.outputCount)
        return
      case 'canvas-extend-submitted':
        handlers.onCanvasExtendSubmitted(intent.nodeId, intent.prompt, intent.outputCount)
        return
      case 'action-requested':
        handlers.onImageActionRequested(intent.nodeId, intent.action)
        return
      case 'canvas-extend-handle-pointerdown':
        handlers.onCanvasExtendHandlePointerDown(intent.nodeId, intent.anchor, intent.event)
        return
      case 'canvas-extend-handle-pointermove':
        handlers.onCanvasExtendHandlePointerMove(intent.event)
        return
      case 'canvas-extend-handle-pointerup':
        handlers.onCanvasExtendHandlePointerUp(intent.event)
        return
      case 'canvas-extend-frame-pointerdown':
        handlers.onCanvasExtendFramePointerDown(intent.nodeId, intent.event)
    }
  }
}
