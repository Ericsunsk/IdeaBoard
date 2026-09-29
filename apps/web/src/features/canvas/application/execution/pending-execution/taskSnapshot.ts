import type { BoardSnapshot, ImageNodeItem } from '../../../../board/ports/canvas'

export type PendingImageTaskSnapshot = Readonly<{
  resultNodeId: `image-${number}`
  taskId: string
  resultIndex?: number | undefined
  fallbackRatio: number
  clientActionId: string
}>

function readPendingImageTask(node: ImageNodeItem): PendingImageTaskSnapshot | null {
  const taskId = `${node.pendingTaskId ?? ''}`.trim()
  const clientActionId = `${node.pendingClientActionId ?? ''}`.trim()
  if (!taskId || !clientActionId || node.outputStatus !== 'running') return null
  const ratio = Number(node.sourceImageRatio)
  return Object.freeze({
    resultNodeId: `image-${node.id}`,
    taskId,
    ...(Number.isInteger(node.pendingResultIndex) && (node.pendingResultIndex as number) >= 0
      ? { resultIndex: node.pendingResultIndex }
      : {}),
    fallbackRatio: Number.isFinite(ratio) && ratio > 0 ? ratio : 1,
    clientActionId,
  })
}

export function collectPendingImageTasks(snapshot: BoardSnapshot): PendingImageTaskSnapshot[] {
  return snapshot.imageNodes.flatMap((node) => {
    const pending = readPendingImageTask(node)
    return pending ? [pending] : []
  })
}
