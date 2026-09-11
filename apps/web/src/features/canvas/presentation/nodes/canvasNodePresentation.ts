import type { PromptAspectRatio } from '../../../board'
import type { ImageNodeAction } from '../../domain/nodes/imageActionRegistry.ts'
import type { OutpaintDragAnchor } from '../../application/image-workflow/useImageOutpaintOverlay.ts'
import type { FlowSnapshotParamPatch } from '../../infrastructure/vue-flow-adapter/flowSnapshotParamMutations.ts'

export type CanvasNodePresentationIntent =
  | { type: 'text-edited'; nodeId: string; text: string }
  | { type: 'parameter-changed'; nodeId: string; patch: FlowSnapshotParamPatch }
  | { type: 'delete-requested'; nodeId: string }
  | { type: 'image-import-retry-requested'; nodeId: string }
  | { type: 'media-input-dropped'; nodeId: string; event: DragEvent }
  | { type: 'media-input-open-requested'; nodeId: string }
  | {
      type: 'media-input-preview-requested'
      nodeId: string
      image: { url: string; ratio: number }
    }
  | { type: 'media-input-remove-requested'; nodeId: string; imageId: string }
  | {
      type: 'generation-requested'
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
  | { type: 'outpaint-submitted'; nodeId: string; prompt: string }
  | {
      type: 'action-requested'
      nodeId: string
      action: ImageNodeAction
      prompt: string
      outputCount: number
      strength?: number | undefined
    }
  | {
      type: 'outpaint-handle-pointerdown'
      nodeId: string
      anchor: OutpaintDragAnchor
      event: PointerEvent
    }
  | { type: 'outpaint-handle-pointermove'; nodeId: string; event: PointerEvent }
  | { type: 'outpaint-handle-pointerup'; nodeId: string; event: PointerEvent }
  | { type: 'outpaint-frame-pointerdown'; nodeId: string; event: PointerEvent }
  | { type: 'fusion-context-menu-requested'; nodeId: string; event: MouseEvent }
  | {
      type: 'fusion-generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
      aspectRatio: PromptAspectRatio
    }
