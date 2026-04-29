import type { Ref } from 'vue'
import { isCanvasExecutionOutputNodeType } from '../types/canvasNodes'
import type {
  CanvasConnectionNodeType,
  CanvasExecutionOutputNodeType,
} from '../types/canvasNodes'

interface NodeOutputImageLike {
  url?: string | null
}

interface WorkspaceUploadUrlNodeLike {
  sourceImageUrl?: string | null
  outputImages?: NodeOutputImageLike[] | null
}

interface UseWorkspaceNodeLifecycleOptions {
  queueWorkspaceUploadUrlsForRecycle: (urls: string[]) => void
  clearNodeConnectionError: (nodeType: CanvasConnectionNodeType, id: number) => void
  setExecutionNodeLastRunFailed: (
    nodeType: CanvasExecutionOutputNodeType,
    nodeId: number,
    failed: boolean,
  ) => void
  removeConnectionsForNode: (nodeType: CanvasConnectionNodeType, id: number) => void
  removeSelectedNodeKey: (nodeType: CanvasConnectionNodeType, id: number) => void
  markCanvasLayoutChanged: () => void
}

function collectWorkspaceUploadUrlsFromNodeLike(node: WorkspaceUploadUrlNodeLike): string[] {
  const urls = new Set<string>()
  const sourceImageUrl = `${node.sourceImageUrl ?? ''}`.trim()
  if (sourceImageUrl) {
    urls.add(sourceImageUrl)
  }
  node.outputImages?.forEach((output) => {
    const outputUrl = `${output.url ?? ''}`.trim()
    if (outputUrl) {
      urls.add(outputUrl)
    }
  })
  return Array.from(urls)
}

export function useWorkspaceNodeLifecycle(options: UseWorkspaceNodeLifecycleOptions) {
  const {
    queueWorkspaceUploadUrlsForRecycle,
    clearNodeConnectionError,
    setExecutionNodeLastRunFailed,
    removeConnectionsForNode,
    removeSelectedNodeKey,
    markCanvasLayoutChanged,
  } = options

  function deleteNodeFromCollection<T extends { id: number }>(
    collection: Ref<T[]>,
    nodeType: CanvasConnectionNodeType,
    id: number,
    afterDelete?: () => void,
  ) {
    const removedNode = collection.value.find((node) => node.id === id) as
      | (T & WorkspaceUploadUrlNodeLike)
      | undefined
    if (removedNode) {
      queueWorkspaceUploadUrlsForRecycle(collectWorkspaceUploadUrlsFromNodeLike(removedNode))
    }
    collection.value = collection.value.filter((node) => node.id !== id)
    clearNodeConnectionError(nodeType, id)
    if (isCanvasExecutionOutputNodeType(nodeType)) {
      setExecutionNodeLastRunFailed(nodeType, id, false)
    }
    removeConnectionsForNode(nodeType, id)
    removeSelectedNodeKey(nodeType, id)
    afterDelete?.()
    markCanvasLayoutChanged()
  }

  return {
    deleteNodeFromCollection,
  }
}
