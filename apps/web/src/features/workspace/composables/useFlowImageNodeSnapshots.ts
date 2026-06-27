import type { Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../canvas-board/domain'
import {
  resolveImageNodeDisplayPayload,
  type ImageNodeDisplayPayload,
} from '../utils/flowImageNodeSelectors'

type UseFlowImageNodeSnapshotsOptions = {
  nodes: Ref<Node[]>
}

export function useFlowImageNodeSnapshots(options: UseFlowImageNodeSnapshotsOptions) {
  function getImageNodeSnapshot(nodeId: string): ImageNodeItem | null {
    const node = (
      options.nodes.value as Array<{ id: string; data?: { kind?: string; snapshotNode?: unknown } }>
    ).find((item) => item.id === nodeId)
    if (
      !node ||
      node.data?.kind !== 'image' ||
      !node.data.snapshotNode ||
      typeof node.data.snapshotNode !== 'object'
    )
      return null
    return node.data.snapshotNode as ImageNodeItem
  }

  function getImageNodeDisplayPayload(nodeId: string): ImageNodeDisplayPayload | null {
    const snapshot = getImageNodeSnapshot(nodeId)
    if (!snapshot) return null
    return resolveImageNodeDisplayPayload(snapshot)
  }

  return {
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
  }
}
