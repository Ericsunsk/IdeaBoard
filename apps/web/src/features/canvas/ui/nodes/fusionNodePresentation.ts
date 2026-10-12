export type FusionNodePresentationBinding = {
  isPromptComposerVisible: boolean
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
    const isPromptComposerVisible = source.promptComposerVisibleFor(nodeId)
    const cached = cache.get(nodeId)
    if (cached?.isPromptComposerVisible === isPromptComposerVisible) return cached

    const binding = { isPromptComposerVisible }
    cache.set(nodeId, binding)
    return binding
  }

  return { forNode }
}
