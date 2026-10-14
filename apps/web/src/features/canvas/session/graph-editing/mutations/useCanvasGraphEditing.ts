import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, watch, type Ref } from 'vue'
import {
  resolveImageNodeDisplayRatio,
  resolveImageNodeFrameSize,
  type BoardSnapshot,
  type CanvasNodeItem,
  type ImageNodeItem,
} from '../../../../board/ports/canvas'
import type { CanvasFeedback } from '../../workspace/useCanvasFeedback.ts'
import type { CanvasVueFlowGraphAdapter } from '../../../session/flow-adapter'
import type { CanvasGraphStore } from '../../../core/graph'
import type { CanvasRendererMetricsPort } from '../../../session/flow-adapter'
import { resolvePendingImageResultPosition, type PendingImageResultLookup } from '../../execution'
import { useFlowCanvasDrop } from '../inputs/useFlowCanvasDrop'
import { useFlowImageNodeSnapshots } from './useFlowImageNodeSnapshots'
import { useFlowImageUploadActions } from '../inputs/useFlowImageUploadActions'
import { useFlowMediaInputImages } from '../inputs/useFlowMediaInputImages'
import { useFlowNodeInternalsRefreshQueue } from '../../../session/flow-adapter'
import { useFlowNodeMutations } from './useFlowNodeMutations'
import type { FlowSnapshotParamPatch } from '../../../session/flow-adapter'
import type { CanvasGraphEditingBrowserAdapter } from '../inputs/browserAdapter'
import { useCanvasLibraryImportRuntime } from '../inputs/useCanvasLibraryImportRuntime'
import type { BoardAttachment } from '../../execution/board-attachment'
import type { LibraryAssetMaterializer } from '../../../../library'
import type { FlowLocalUploadState, FlowNodeData } from '../../../core/nodes/flowTypes.ts'
import { createFlowGraphCommandOptions } from '../../../session/flow-adapter'
import { createCanvasGraphEditingNodeRuntime } from './createCanvasGraphEditingNodeRuntime'

type ValueRef<T> = { readonly value: T }
type SnapshotNodeMutator = (snapshotNode: CanvasNodeItem) => CanvasNodeItem

export type CanvasGraphEditingUploadAdapter = {
  upload: (
    file: File,
    boardId: string,
  ) => Promise<{
    url: string
    asset_id: string
    width?: number
    height?: number
    mime?: string
    ext?: string
  }>
}

export type CanvasGraphEditingNodeInteractionsOptions = {
  dismissTransientCanvasUi: (options?: { shouldCloseContextMenu?: boolean }) => void
  discardNodeEdits: (nodeId: string) => void
}

export type CanvasGraphEditingNodeInteractions = {
  onFlowDeleteNode: (nodeId: string) => void
  onFlowPromptTextUpdate: (nodeId: string, value: string) => void
  onFlowNodeParamUpdate: (nodeId: string, payload: FlowSnapshotParamPatch) => void
  onFlowImageOpenFile: (nodeId: string) => Promise<void>
  onFlowImageReset: (nodeId: string) => void
  onFlowImageStageDrop: (nodeId: string, event: DragEvent) => void
}

type UseCanvasGraphEditingOptions = {
  nodes: Ref<Node[]>
  edges: Ref<Edge[]>
  boardBaseSnapshot: Ref<BoardSnapshot>
  currentBoardId: ValueRef<string | null | undefined>
  graphAdapter: CanvasVueFlowGraphAdapter
  readCanvasGraphStore: () => CanvasGraphStore
  deleteNode: (nodeId: string) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  browser: CanvasGraphEditingBrowserAdapter
  upload: CanvasGraphEditingUploadAdapter
  getWorkspaceId?: (() => string) | undefined
  materializeLibraryAsset: LibraryAssetMaterializer
  readLibraryImportAttachment: () => BoardAttachment | null
  selectOnlyNode: (nodeId: string) => boolean
  preview: {
    openImageUrl: (src: string, ratio?: number) => Promise<void>
  }
  feedback: CanvasFeedback['graph']
}

export function useCanvasGraphEditing(options: UseCanvasGraphEditingOptions) {
  const { scheduleFlowNodeInternalsRefresh } = useFlowNodeInternalsRefreshQueue({
    updateNodeInternals: options.updateNodeInternals,
  })
  const nodeMutations = useFlowNodeMutations({
    nodes: options.nodes,
    edges: options.edges,
    boardBaseSnapshot: options.boardBaseSnapshot,
    readCanvasGraphStore: options.readCanvasGraphStore,
    graphAdapter: options.graphAdapter,
    onWarning: options.feedback.operationWarning,
  })

  function updateSnapshotNode(
    nodeId: string,
    mutate: SnapshotNodeMutator,
    updateOptions?: { shouldRecordHistory?: boolean },
  ) {
    nodeMutations.updateSnapshotNode(nodeId, mutate, updateOptions)
    scheduleFlowNodeInternalsRefresh([nodeId])
  }

  const uploadActions = useGraphEditingImageUploads(options, nodeMutations, updateSnapshotNode)
  const {
    uploadImageToBoard,
    applyImageFileToImageNode,
    createImageNodeFromFile,
    getImageNodeSnapshot,
    getImageNodeDisplayPayload,
  } = uploadActions
  const mediaInputs = useFlowMediaInputImages({
    browser: options.browser,
    nodes: options.nodes,
    edges: options.edges,
    uploadImageToBoard,
    updateSnapshotNodeBase: nodeMutations.updateSnapshotNode,
    readCanvasGraphStore: options.readCanvasGraphStore,
    writeCanvasGraphStore: options.readCanvasGraphStore,
    markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload,
    openImageUrlPreview: options.preview.openImageUrl,
    onWarning: options.feedback.operationWarning,
  })
  const libraryImport = useGraphEditingLibraryImport(
    options,
    nodeMutations,
    scheduleFlowNodeInternalsRefresh,
  )

  async function retryImageImport(nodeId: string): Promise<boolean> {
    if (libraryImport.retry(nodeId)) return true
    return uploadActions.retryLocalUpload(nodeId)
  }
  const canvasDrop = useFlowCanvasDrop({
    browser: options.browser,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    onWarning: options.feedback.operationWarning,
    createImageNodeFromFile,
  })
  function createNodeInteractions(
    interactionOptions: CanvasGraphEditingNodeInteractionsOptions,
  ): CanvasGraphEditingNodeInteractions {
    return createCanvasGraphEditingNodeRuntime({
      browser: options.browser,
      applyImageFileToImageNode,
      updateSnapshotNode,
      markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
      deleteNode: options.deleteNode,
      updatePromptText: nodeMutations.onFlowPromptTextUpdate,
      updateNodeParams: nodeMutations.onFlowNodeParamUpdate,
      dismissTransientCanvasUi: interactionOptions.dismissTransientCanvasUi,
      discardNodeEdits: interactionOptions.discardNodeEdits,
    })
  }

  function createPendingRetouchNode(sourceNode: ImageNodeItem): string | null {
    const ratio = resolveImageNodeDisplayRatio(sourceNode)
    const sourceWidth = Number(sourceNode.width)
    const width =
      Number.isFinite(sourceWidth) && sourceWidth > 0
        ? sourceWidth
        : resolveImageNodeFrameSize(ratio, true).width
    const store = options.readCanvasGraphStore()
    const lookup: PendingImageResultLookup = {
      nodeById: new Map(
        store
          .getNodeIds()
          .map((nodeId) => store.getNode(nodeId))
          .filter((node): node is NonNullable<typeof node> => !!node)
          .map((node) => [node.id, node]),
      ),
      imageChildIdsBySource: new Map(),
    }
    const rendererMetrics: CanvasRendererMetricsPort = {
      readNodeMetrics: () => null,
    }
    const position = resolvePendingImageResultPosition(
      {
        x: Number(sourceNode.x ?? 0),
        y: Number(sourceNode.y ?? 0),
        width,
        ratio,
      },
      lookup,
      rendererMetrics,
      { width, ratio },
    )

    return nodeMutations.createNodeAtPosition('image', position.x, position.y, {
      title: sourceNode.title || 'Image',
      width,
      sourceImageRatio: ratio,
      outputStatus: 'running',
      outputImages: [],
    })
  }

  return {
    flowPane: {
      connectionAdapter: nodeMutations.connectionAdapter,
    },
    commands: {
      scheduleNodeInternalsRefresh: scheduleFlowNodeInternalsRefresh,
      createNodeAtPosition: nodeMutations.createNodeAtPosition,
      createNodeInteractions,
      mediaInputs: {
        drop: mediaInputs.onFlowMediaInputImageDrop,
        open: mediaInputs.onFlowMediaInputImageOpen,
        preview: mediaInputs.onFlowMediaInputImagePreview,
        remove: mediaInputs.onFlowMediaInputImageRemove,
      },
      canvasFiles: {
        drop: canvasDrop.onCanvasDrop,
        dragOver: canvasDrop.onCanvasDragOver,
      },
      libraryImport: {
        add: libraryImport.start,
      },
      imageImport: {
        retry: retryImageImport,
      },
    },
    workflowPort: {
      getImageNodeDisplayPayload,
      getImageNodeSnapshot,
      updateSnapshotNode,
      createImageNodeFromFile,
      createPendingRetouchNode,
      applyImageFileToImageNode,
      deleteNode: options.deleteNode,
      uploadImageToBoard,
      getMediaInputImages: mediaInputs.getMediaInputImages,
      resolvePromptInputImageNodeIds: mediaInputs.resolvePromptInputImageNodeIds,
      updateNodeParams: nodeMutations.onFlowNodeParamUpdate,
      markNodeExecutionDirty: nodeMutations.markFlowExecutionNodeDirty,
    },
    recoveryPort: {
      resumePendingLibraryImports: libraryImport.resumePending,
    },
  }
}

function useGraphEditingLibraryImport(
  options: UseCanvasGraphEditingOptions,
  nodeMutations: ReturnType<typeof useFlowNodeMutations>,
  scheduleFlowNodeInternalsRefresh: ReturnType<
    typeof useFlowNodeInternalsRefreshQueue
  >['scheduleFlowNodeInternalsRefresh'],
) {
  return useCanvasLibraryImportRuntime({
    nodes: options.nodes,
    readAttachment: options.readLibraryImportAttachment,
    graphAdapter: options.graphAdapter,
    updateImageSnapshot: (nodeId, mutate) =>
      nodeMutations.updateSnapshotNode(
        nodeId,
        (snapshot) => (snapshot.type === 'image-node' ? mutate(snapshot) : snapshot),
        { shouldRecordHistory: false },
      ),
    createImageNodeAtPosition: (x, y, initialSnapshotPatch) =>
      nodeMutations.createNodeAtPosition('image', x, y, initialSnapshotPatch),
    refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
    selectOnlyNode: options.selectOnlyNode,
    materialize: options.materializeLibraryAsset,
    onWarning: options.feedback.operationWarning,
  })
}
function useGraphEditingImageUploads(
  options: UseCanvasGraphEditingOptions,
  nodeMutations: ReturnType<typeof useFlowNodeMutations>,
  updateSnapshotNode: ReturnType<typeof useFlowNodeMutations>['updateSnapshotNode'],
) {
  function setLocalUploadState(nodeId: string, state: FlowLocalUploadState | null) {
    const node = options.nodes.value.find((item) => item.id === nodeId)
    const data = node?.data as FlowNodeData | undefined
    if (!node || data?.kind !== 'image') return
    options.graphAdapter.updateNode(
      nodeId,
      {
        data: {
          ...data,
          ...(state ? { localUpload: state } : { localUpload: undefined }),
        },
      },
      createFlowGraphCommandOptions({ shouldRecordHistory: false, shouldMarkDirty: false }),
    )
  }

  const { getImageNodeSnapshot, getImageNodeDisplayPayload } = useFlowImageNodeSnapshots({
    nodes: options.nodes,
  })
  const uploadActions = useFlowImageUploadActions({
    currentBoardId: options.currentBoardId,
    createNodeAtPosition: nodeMutations.createNodeAtPosition,
    updateSnapshotNode,
    getImageNodeSnapshot,
    uploadApi: options.upload,
    workspaceId: options.getWorkspaceId,
    onWarning: options.feedback.operationWarning,
    onNodeMutated: (nodeId) => nodeMutations.markFlowExecutionNodeDirty(nodeId),
    setLocalUploadState,
  })

  // The getter previously returned a fresh object (with a new Set) on every
  // evaluation, so the watcher fired on every graph commit — including
  // unrelated position/text edits. Watch the board id plus a cheap node
  // id-join instead, and bail before allocating anything when no local
  // uploads are in flight (the common case).
  watch(
    [
      () => options.currentBoardId.value,
      () => options.nodes.value.map((node) => node.id).join('|'),
    ],
    ([boardId]) => {
      if (!uploadActions.hasLocalUploads()) return
      uploadActions.releaseDetachedLocalUploads(
        new Set(options.nodes.value.map((node) => node.id)),
        boardId,
      )
    },
    { flush: 'sync' },
  )
  onScopeDispose(() => uploadActions.disposeLocalUploads())
  return { ...uploadActions, getImageNodeSnapshot, getImageNodeDisplayPayload }
}
