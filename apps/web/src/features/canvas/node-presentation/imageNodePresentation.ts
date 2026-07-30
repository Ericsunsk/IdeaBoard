import type { ImageActionBinding, ImageToolStageBinding } from '../image-tools'

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
