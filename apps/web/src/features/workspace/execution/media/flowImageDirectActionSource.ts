import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../../canvas-board/domain'
import { resolveImageNodeLayoutAnchor, type ImageNodeLayoutAnchor } from '../../utils/flowImageNodeSelectors'

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
