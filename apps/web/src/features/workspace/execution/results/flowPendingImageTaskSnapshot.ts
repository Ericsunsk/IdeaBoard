import type { Node } from '@vue-flow/core'
import type { ImageNodeItem } from '../../../canvas-board/domain'

export type PendingImageTaskSnapshot = {
  nodeId: string
  taskId: string
  fallbackRatio: number
}

export function readPendingImageSnapshot(nodes: Node[], nodeId: string): ImageNodeItem | null {
  const node = nodes.find((item) => item.id === nodeId)
  if (!node || node.data?.kind !== 'image') return null
  const snapshot = node.data.snapshotNode
  if (!snapshot || typeof snapshot !== 'object') return null
  return snapshot as ImageNodeItem
}

export function hasRenderableImageResult(snapshot: ImageNodeItem): boolean {
  const sourceUrl = `${snapshot.sourceImageUrl ?? ''}`.trim()
  if (sourceUrl) return true
  const outputImages = Array.isArray(snapshot.outputImages) ? snapshot.outputImages : []
  return outputImages.some((item) => `${item?.url ?? ''}`.trim().length > 0)
}

export function isPendingImageTaskSnapshot(snapshot: ImageNodeItem, taskId?: string): boolean {
  const pendingTaskId = `${snapshot.pendingTaskId ?? ''}`.trim()
  if (!pendingTaskId) return false
  if (taskId && pendingTaskId !== taskId) return false
  return snapshot.outputStatus === 'running' && !hasRenderableImageResult(snapshot)
}

export function collectPendingImageTasks(nodes: Node[]): PendingImageTaskSnapshot[] {
  return nodes.flatMap((node) => {
    if (node.data?.kind !== 'image') return []
    const snapshot = node.data.snapshotNode
    if (!snapshot || typeof snapshot !== 'object') return []
    const imageSnapshot = snapshot as ImageNodeItem
    if (!isPendingImageTaskSnapshot(imageSnapshot)) return []
    const fallbackRatio = Number(imageSnapshot.sourceImageRatio)
    return [
      {
        nodeId: node.id,
        taskId: imageSnapshot.pendingTaskId!.trim(),
        fallbackRatio: Number.isFinite(fallbackRatio) && fallbackRatio > 0 ? fallbackRatio : 1,
      },
    ]
  })
}
