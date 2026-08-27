import type { Edge, Node } from '@vue-flow/core'
import { onScopeDispose, watch, type Ref } from 'vue'
import type { CanvasBoardSnapshot, CanvasNodeItem } from '../../canvas-board/domain'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { CanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { CanvasGraphStore } from '../graph-runtime'
import { useFlowCanvasDrop } from './useFlowCanvasDrop'
import { useFlowImageNodeSnapshots } from './useFlowImageNodeSnapshots'
import { useFlowImageUploadActions } from './useFlowImageUploadActions'
import { useFlowMediaInputImages } from './useFlowMediaInputImages'
import { useFlowNodeInternalsRefreshQueue } from '../vue-flow-adapter/useFlowNodeInternalsRefreshQueue'
import { useFlowNodeMutations } from './useFlowNodeMutations'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import type { CanvasGraphEditingBrowserAdapter } from './browserAdapter'
import { useCanvasLibraryImportRuntime } from './useCanvasLibraryImportRuntime'
import type { CanvasBoardAttachment } from '../board-attachment'
import type { LibraryAssetMaterializer } from '../../library/libraryResource'
import type { FlowLocalUploadState, FlowNodeData } from '../types/flowTypes'
import { createFlowGraphCommandOptions } from '../vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import { createCanvasGraphEditingNodeRuntime } from './createCanvasGraphEditingNodeRuntime'

type ValueRef<T> = { readonly value: T }
type SnapshotNodeMutator = (snapshotNode: CanvasNodeItem) => CanvasNodeItem

export type CanvasGraphEditingUploadAdapter = {
  upload: (
    file: File,
    boardId: string,
  ) => Promise<{
    url: string
    media_asset_id?: number
    width?: number
    height?: number
    mime?: string
    ext?: string
  }>
}

export type CanvasGraphEditingNodeInteractionsOptions = {
  dismissTransientCanvasUi: (options?: { closeContextMenu?: boolean }) => void
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
  boardBaseSnapshot: Ref<CanvasBoardSnapshot>
  currentBoardId: ValueRef<string | null | undefined>
  graphAdapter: CanvasVueFlowGraphAdapter
  readCanvasGraphStore: () => CanvasGraphStore
  deleteNode: (nodeId: string) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  browser: CanvasGraphEditingBrowserAdapter
  upload: CanvasGraphEditingUploadAdapter
  materializeLibraryAsset: LibraryAssetMaterializer
  readLibraryImportAttachment: () => CanvasBoardAttachment | null
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
    updateOptions?: { history?: boolean },
  ) {
    nodeMutations.updateSnapshotNode(nodeId, mutate, updateOptions)
    scheduleFlowNodeInternalsRefresh([nodeId])
  }

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
      createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
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
    onWarning: options.feedback.operationWarning,
    onNodeMutated: (nodeId) => nodeMutations.markFlowExecutionNodeDirty(nodeId),
    setLocalUploadState,
  })
  const { uploadImageToBoard, applyImageFileToImageNode, createImageNodeFromFile } = uploadActions

  watch(
    () => ({
      boardId: options.currentBoardId.value,
      liveNodeIds: new Set(options.nodes.value.map((node) => node.id)),
    }),
    ({ boardId, liveNodeIds }) => uploadActions.releaseDetachedLocalUploads(liveNodeIds, boardId),
    { flush: 'sync' },
  )
  onScopeDispose(() => uploadActions.disposeLocalUploads())
  const mediaInputs = useFlowMediaInputImages({
    browser: options.browser,
    nodes: options.nodes,
    edges: options.edges,
    uploadImageToBoard,
    updateSnapshotNodeBase: nodeMutations.updateSnapshotNode,
    readCanvasGraphStore: options.readCanvasGraphStore,
    writeCanvasGraphStore: options.readCanvasGraphStore,
    graphAdapter: options.graphAdapter,
    markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload,
    openImageUrlPreview: options.preview.openImageUrl,
    onWarning: options.feedback.operationWarning,
  })
  const libraryImport = useCanvasLibraryImportRuntime({
    nodes: options.nodes,
    readAttachment: options.readLibraryImportAttachment,
    graphAdapter: options.graphAdapter,
    updateImageSnapshot: (nodeId, mutate) =>
      nodeMutations.updateSnapshotNode(
        nodeId,
        (snapshot) => (snapshot.type === 'image-node' ? mutate(snapshot) : snapshot),
        { history: false },
      ),
    createImageNodeAtPosition: (x, y, initialSnapshotPatch) =>
      nodeMutations.createNodeAtPosition('image', x, y, initialSnapshotPatch),
    refreshNodeInternals: scheduleFlowNodeInternalsRefresh,
    selectOnlyNode: options.selectOnlyNode,
    materialize: options.materializeLibraryAsset,
    onWarning: options.feedback.operationWarning,
  })

  async function retryImageImport(nodeId: string): Promise<boolean> {
    if (libraryImport.retry(nodeId)) return true
    return uploadActions.retryLocalUpload(nodeId)
  }
  const canvasDrop = useFlowCanvasDrop({
    browser: options.browser,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    onWarning: options.feedback.operationWarning,
    createImageNodeFromFile,
    addLibraryAsset: libraryImport.start,
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
      createImageNodeFromFile,
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
