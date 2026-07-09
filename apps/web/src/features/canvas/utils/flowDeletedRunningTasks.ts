type FlowNodeLike = {
  id: string
  data?: unknown
}

type PendingTaskSnapshotLike = {
  outputStatus?: unknown
  pendingTaskId?: unknown
}

function readPendingTaskSnapshot(node: FlowNodeLike): PendingTaskSnapshotLike | null {
  const data = node.data
  if (!data || typeof data !== 'object' || !('snapshotNode' in data)) return null
  const snapshotNode = (data as { snapshotNode?: unknown }).snapshotNode
  if (!snapshotNode || typeof snapshotNode !== 'object') return null
  return snapshotNode as PendingTaskSnapshotLike
}

export function collectDeletedRunningTaskIds(
  nodes: FlowNodeLike[],
  nodeIds: Iterable<string>,
): string[] {
  const deletedNodeIds = new Set(nodeIds)
  if (deletedNodeIds.size === 0) return []

  const taskIds = new Set<string>()
  for (const node of nodes) {
    if (!deletedNodeIds.has(node.id)) continue
    const snapshot = readPendingTaskSnapshot(node)
    if (!snapshot || snapshot.outputStatus !== 'running') continue
    const taskId = typeof snapshot.pendingTaskId === 'string' ? snapshot.pendingTaskId.trim() : ''
    if (taskId) taskIds.add(taskId)
  }
  return Array.from(taskIds)
}
