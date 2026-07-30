import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { ImageNodeAction } from '../config/imageActionRegistry'
import type { CanvasExtendDragAnchor } from '../composables/useImageCanvasExtendOverlay'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

export type CanvasNodePresentationIntent =
  | { type: 'text-edited'; nodeId: string; text: string }
  | { type: 'parameter-changed'; nodeId: string; patch: FlowSnapshotParamPatch }
  | { type: 'delete-requested'; nodeId: string }
  | { type: 'media-input-dropped'; nodeId: string; event: DragEvent }
  | { type: 'media-input-open-requested'; nodeId: string }
  | {
      type: 'media-input-preview-requested'
      nodeId: string
      image: { url: string; ratio: number }
    }
  | { type: 'media-input-remove-requested'; nodeId: string; imageId: string }
  | {
      type: 'prompt-generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
      aspectRatio: PromptAspectRatio
    }
  | { type: 'stage-dropped'; nodeId: string; event: DragEvent }
  | { type: 'file-open-requested'; nodeId: string }
  | { type: 'preview-requested'; nodeId: string }
  | { type: 'append-to-canvas-requested'; nodeId: string }
  | { type: 'reset-requested'; nodeId: string }
  | { type: 'image-context-menu-requested'; nodeId: string; event: MouseEvent }
  | {
      type: 'image-prompt-generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
    }
  | { type: 'canvas-extend-submitted'; nodeId: string; prompt: string; outputCount: number }
  | { type: 'action-requested'; nodeId: string; action: ImageNodeAction }
  | {
      type: 'canvas-extend-handle-pointerdown'
      nodeId: string
      anchor: CanvasExtendDragAnchor
      event: PointerEvent
    }
  | { type: 'canvas-extend-handle-pointermove'; nodeId: string; event: PointerEvent }
  | { type: 'canvas-extend-handle-pointerup'; nodeId: string; event: PointerEvent }
  | { type: 'canvas-extend-frame-pointerdown'; nodeId: string; event: PointerEvent }
  | { type: 'fusion-context-menu-requested'; nodeId: string; event: MouseEvent }
  | {
      type: 'fusion-generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
      aspectRatio: PromptAspectRatio
    }
