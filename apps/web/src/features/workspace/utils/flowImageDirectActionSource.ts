import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'

export type ImageDirectActionSource = {
  url: string
  ratio: number
  x: number
  y: number
  width: number
}

type ResolveImageDirectActionSourceOptions = {
  requireUrl?: boolean
}

const DEFAULT_IMAGE_DIRECT_ACTION_NODE_WIDTH = 280

export function resolveImageDirectActionSource(
  node: Pick<Node, 'position'>,
  imageNode: ImageNodeItem,
  options: ResolveImageDirectActionSourceOptions = {},
): ImageDirectActionSource | null {
  const outputs = Array.isArray(imageNode.outputImages) ? imageNode.outputImages : []
  const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
  const url = `${latestOutput?.url || imageNode.sourceImageUrl || ''}`.trim()
  if (options.requireUrl && !url) return null

  const outputRatio = Number(latestOutput?.ratio)
  const sourceRatio = Number(imageNode.sourceImageRatio)
  const ratio =
    Number.isFinite(outputRatio) && outputRatio > 0
      ? outputRatio
      : Number.isFinite(sourceRatio) && sourceRatio > 0
        ? sourceRatio
        : 1
  const width = Number(imageNode.width)

  return {
    url,
    ratio,
    x: Number(node.position?.x ?? imageNode.x ?? 0),
    y: Number(node.position?.y ?? imageNode.y ?? 0),
    width:
      Number.isFinite(width) && width > 0 ? width : DEFAULT_IMAGE_DIRECT_ACTION_NODE_WIDTH,
  }
}
