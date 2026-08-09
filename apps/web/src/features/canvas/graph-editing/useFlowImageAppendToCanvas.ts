import { createCanvasNodeAt } from '../utils/canvasNodeFactory'
import { resolveImageNodeFrameSize, type ImageNodeItem } from '../../canvas-board/domain'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import { createImageFlowNode } from '../utils/flowImageNodeFactory'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import { createFlowGraphCommandOptions } from '../composables/flowGraphAdapterCommandSideEffects'

type UseFlowImageAppendToCanvasOptions = {
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  graphAdapter: CanvasVueFlowGraphAdapter
}

export function useFlowImageAppendToCanvas(options: UseFlowImageAppendToCanvasOptions) {
  function appendImageToCanvas(nodeId: string) {
    const payload = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!payload || !sourceNode) return
    const imageNode = createCanvasNodeAt('image', {
      x: sourceNode.x + Math.max(240, sourceNode.width + 64),
      y: sourceNode.y + 18,
    })
    imageNode.sourceImageUrl = payload.url
    imageNode.sourceMediaAssetId = payload.mediaAssetId
    imageNode.sourceImageRatio = payload.ratio
    imageNode.outputCount = sourceNode.outputCount ?? imageNode.outputCount
    imageNode.width = resolveImageNodeFrameSize(payload.ratio, true).width
    imageNode.outputStatus = 'ready'
    imageNode.outputImages = []
    options.graphAdapter.addNodes(createImageFlowNode(imageNode), createFlowGraphCommandOptions())
  }

  return { appendImageToCanvas }
}
