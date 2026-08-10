import type { Edge, Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import type { CanvasBoardSnapshot, CanvasNodeItem } from '../../canvas-board/domain'
import type { CanvasFeedback } from '../feedback/useCanvasFeedback'
import type { CanvasVueFlowGraphAdapter } from '../composables/useCanvasVueFlowGraphAdapter'
import { useFlowCanvasDrop } from '../composables/useFlowCanvasDrop'
import { useFlowCanvasNodeHandlers } from '../composables/useFlowCanvasNodeHandlers'
import { useFlowImageNodeSnapshots } from '../composables/useFlowImageNodeSnapshots'
import { useFlowImageUploadActions } from '../composables/useFlowImageUploadActions'
import { useFlowMediaInputImages } from '../composables/useFlowMediaInputImages'
import { useFlowNodeInternalsRefreshQueue } from '../composables/useFlowNodeInternalsRefreshQueue'
import { useFlowNodeMediaInputs } from '../composables/useFlowNodeMediaInputs'
import { useFlowNodeMutations } from '../composables/useFlowNodeMutations'
import type { FlowSnapshotParamPatch } from '../utils/flowSnapshotParamMutations'
import type { CanvasGraphEditingBrowserAdapter } from './browserAdapter'

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
  deleteNode: (nodeId: string) => boolean
  updateNodeInternals: (nodeIds: string[]) => void
  screenToFlowCoordinate: (point: { x: number; y: number }) => { x: number; y: number }
  browser: CanvasGraphEditingBrowserAdapter
  upload: CanvasGraphEditingUploadAdapter
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

  const { getImageNodeSnapshot, getImageNodeDisplayPayload } = useFlowImageNodeSnapshots({
    nodes: options.nodes,
  })
  const { uploadImageToBoard, applyImageFileToImageNode, createImageNodeFromFile } =
    useFlowImageUploadActions({
      currentBoardId: options.currentBoardId,
      createNodeAtPosition: nodeMutations.createNodeAtPosition,
      updateSnapshotNode,
      getImageNodeSnapshot,
      uploadApi: options.upload,
      onWarning: options.feedback.operationWarning,
      onSuccess: options.feedback.operationSucceeded,
      onNodeMutated: (nodeId) => nodeMutations.markFlowExecutionNodeDirty(nodeId),
    })
  const mediaInputs = useFlowMediaInputImages({
    browser: options.browser,
    nodes: options.nodes,
    edges: options.edges,
    uploadImageToBoard,
    updateSnapshotNodeBase: nodeMutations.updateSnapshotNode,
    graphAdapter: options.graphAdapter,
    markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
    getImageNodeDisplayPayload,
    openImageUrlPreview: options.preview.openImageUrl,
    onWarning: options.feedback.operationWarning,
  })
  const canvasDrop = useFlowCanvasDrop({
    browser: options.browser,
    screenToFlowCoordinate: options.screenToFlowCoordinate,
    onWarning: options.feedback.operationWarning,
    createImageNodeFromFile,
  })
  function createNodeInteractions(
    interactionOptions: CanvasGraphEditingNodeInteractionsOptions,
  ): CanvasGraphEditingNodeInteractions {
    async function applyImageFile(nodeId: string, file: File) {
      const updated = await applyImageFileToImageNode(nodeId, file)
      if (updated) interactionOptions.discardNodeEdits(nodeId)
      return updated
    }

    const media = useFlowNodeMediaInputs({
      pickImageFile: options.browser.pickImageFile,
      readDroppedFile: options.browser.readDroppedFile,
      applyImageFileToImageNode: applyImageFile,
      updateSnapshotNode,
      markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
    })
    const handlers = useFlowCanvasNodeHandlers({
      dismissTransientCanvasUi: interactionOptions.dismissTransientCanvasUi,
      discardFlowImageToolNodeEdits: interactionOptions.discardNodeEdits,
      deleteNode: options.deleteNode,
      updatePromptText: nodeMutations.onFlowPromptTextUpdate,
      updateNodeParams: nodeMutations.onFlowNodeParamUpdate,
      resetImage: media.onFlowImageReset,
      markFlowExecutionNodeDirty: nodeMutations.markFlowExecutionNodeDirty,
    })

    return {
      ...handlers,
      onFlowImageOpenFile: media.onFlowImageOpenFile,
      onFlowImageStageDrop: media.onFlowImageStageDrop,
    }
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
  }
}
