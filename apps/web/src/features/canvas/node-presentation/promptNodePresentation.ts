import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

export type PromptNodePresentationIntent =
  | { type: 'text-edited'; nodeId: string; text: string }
  | {
      type: 'parameter-changed'
      nodeId: string
      patch: FlowSnapshotParamPatch
    }
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
      type: 'generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
      aspectRatio: PromptAspectRatio
    }

export type PromptNodePresentationIntentHandlers = {
  onTextEdited: (nodeId: string, text: string) => void
  onParameterChanged: (nodeId: string, patch: FlowSnapshotParamPatch) => void
  onDeleteRequested: (nodeId: string) => void
  onMediaInputDrop: (nodeId: string, event: DragEvent) => void
  onMediaInputOpen: (nodeId: string) => void
  onMediaInputPreview: (image: { url: string; ratio: number }) => void
  onMediaInputRemove: (nodeId: string, imageId: string) => void
  onGenerationRequested: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => void
}

export function createPromptNodePresentationIntentHandler(
  handlers: PromptNodePresentationIntentHandlers,
) {
  return (intent: PromptNodePresentationIntent) => {
    switch (intent.type) {
      case 'text-edited':
        handlers.onTextEdited(intent.nodeId, intent.text)
        return
      case 'parameter-changed':
        handlers.onParameterChanged(intent.nodeId, intent.patch)
        return
      case 'delete-requested':
        handlers.onDeleteRequested(intent.nodeId)
        return
      case 'media-input-dropped':
        handlers.onMediaInputDrop(intent.nodeId, intent.event)
        return
      case 'media-input-open-requested':
        handlers.onMediaInputOpen(intent.nodeId)
        return
      case 'media-input-preview-requested':
        handlers.onMediaInputPreview(intent.image)
        return
      case 'media-input-remove-requested':
        handlers.onMediaInputRemove(intent.nodeId, intent.imageId)
        return
      case 'generation-requested':
        handlers.onGenerationRequested(
          intent.nodeId,
          intent.prompt,
          intent.outputCount,
          intent.aspectRatio,
        )
    }
  }
}
