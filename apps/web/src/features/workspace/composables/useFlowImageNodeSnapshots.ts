import type { Ref } from 'vue'
import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../types/canvasNodes'

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

  function getImageNodeDisplayPayload(nodeId: string): { url: string; ratio: number } | null {
    const snapshot = getImageNodeSnapshot(nodeId)
    if (!snapshot) return null
    const outputs = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
    const latestOutput = outputs.length > 0 ? outputs[outputs.length - 1] : null
    const url = `${latestOutput?.url || snapshot.sourceImageUrl || ''}`.trim()
    if (!url) return null
    const outputRatio = Number(latestOutput?.ratio)
    const sourceRatio = Number(snapshot.sourceImageRatio)
    return {
      url,
      ratio:
        Number.isFinite(outputRatio) && outputRatio > 0
          ? outputRatio
          : Number.isFinite(sourceRatio) && sourceRatio > 0
            ? sourceRatio
            : 1,
    }
  }

  return {
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
  }
}
