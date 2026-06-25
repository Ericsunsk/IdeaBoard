import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'
import { resolveImageNodeLayoutAnchor, type ImageNodeLayoutAnchor } from './flowImageNodeSelectors'

export type ImageDirectActionSource = ImageNodeLayoutAnchor

type ResolveImageDirectActionSourceOptions = {
  requireUrl?: boolean
}

export function resolveImageDirectActionSource(
  node: Pick<Node, 'position'>,
  imageNode: ImageNodeItem,
  options: ResolveImageDirectActionSourceOptions = {},
): ImageDirectActionSource | null {
  return resolveImageNodeLayoutAnchor(node, imageNode, options)
}
