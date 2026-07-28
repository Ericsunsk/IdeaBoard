import type { PromptAspectRatio } from '../../canvas-board/domain'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'

export type FusionNodePresentationBinding = {
  promptComposerVisible: boolean
}

export type FusionNodePresentationBindingSource = {
  promptComposerVisibleFor: (nodeId: string) => boolean
}

export type FusionNodePresentationResolver = {
  forNode: (nodeId: string) => FusionNodePresentationBinding
}

export function createFusionNodePresentationResolver(
  source: FusionNodePresentationBindingSource,
): FusionNodePresentationResolver {
  const cache = new Map<string, FusionNodePresentationBinding>()

  function forNode(nodeId: string): FusionNodePresentationBinding {
    const promptComposerVisible = source.promptComposerVisibleFor(nodeId)
    const cached = cache.get(nodeId)
    if (cached?.promptComposerVisible === promptComposerVisible) return cached

    const binding = { promptComposerVisible }
    cache.set(nodeId, binding)
    return binding
  }

  return { forNode }
}

export type FusionNodePresentationIntent =
  | { type: 'parameter-changed'; nodeId: string; patch: FlowSnapshotParamPatch }
  | { type: 'delete-requested'; nodeId: string }
  | { type: 'media-input-dropped'; nodeId: string; event: DragEvent }
  | { type: 'media-input-remove-requested'; nodeId: string; imageId: string }
  | { type: 'context-menu-requested'; nodeId: string; event: MouseEvent }
  | {
      type: 'generation-requested'
      nodeId: string
      prompt: string
      outputCount: number
      aspectRatio: PromptAspectRatio
    }

export type FusionNodePresentationIntentHandlers = {
  onParameterChanged: (nodeId: string, patch: FlowSnapshotParamPatch) => void
  onDeleteRequested: (nodeId: string) => void
  onMediaInputDrop: (nodeId: string, event: DragEvent) => void
  onMediaInputRemove: (nodeId: string, imageId: string) => void
  onContextMenuRequested: (nodeId: string, event: MouseEvent) => void
  onGenerationRequested: (
    nodeId: string,
    prompt: string,
    outputCount: number,
    aspectRatio: PromptAspectRatio,
  ) => void
}

export function createFusionNodePresentationIntentHandler(
  handlers: FusionNodePresentationIntentHandlers,
) {
  return (intent: FusionNodePresentationIntent) => {
    switch (intent.type) {
      case 'parameter-changed':
        handlers.onParameterChanged(intent.nodeId, intent.patch)
        return
      case 'delete-requested':
        handlers.onDeleteRequested(intent.nodeId)
        return
      case 'media-input-dropped':
        handlers.onMediaInputDrop(intent.nodeId, intent.event)
        return
      case 'media-input-remove-requested':
        handlers.onMediaInputRemove(intent.nodeId, intent.imageId)
        return
      case 'context-menu-requested':
        handlers.onContextMenuRequested(intent.nodeId, intent.event)
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
