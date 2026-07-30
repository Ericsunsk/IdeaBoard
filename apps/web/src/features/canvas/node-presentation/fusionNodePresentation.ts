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
