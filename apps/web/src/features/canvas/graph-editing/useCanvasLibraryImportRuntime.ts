import type { Node } from '@vue-flow/core'
import type { Ref } from 'vue'
import { resolveImageNodeFrameSize, type ImageNodeItem } from '../../canvas-board/domain'
import { i18n } from '../../../shared/i18n'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import type { CanvasBoardAttachment } from '../board-attachment'
import {
  createPendingLibraryImportFact,
  failLibraryImport,
  hasPendingLibraryImport,
  materializeLibraryImport,
  retryLibraryImport,
  type CanvasGraphNodeIdForKind,
} from '../graph-runtime'
import { createFlowGraphCommandOptions } from '../vue-flow-adapter/flowGraphAdapterCommandSideEffects'
import type { CanvasVueFlowGraphAdapter } from '../vue-flow-adapter/useCanvasVueFlowGraphAdapter'
import type { FlowNodeData } from '../types/flowTypes'
import type { CanvasLibraryImportDescriptor } from '../../library'
import type { LibraryAssetMaterializer } from '../../library'
import { preloadImage } from '../../../shared/media/preloadImage'

type UseCanvasLibraryImportRuntimeOptions = {
  nodes: Ref<Node[]>
  readAttachment: () => CanvasBoardAttachment | null
  graphAdapter: CanvasVueFlowGraphAdapter
  updateImageSnapshot: (nodeId: string, mutate: (snapshot: ImageNodeItem) => ImageNodeItem) => void
  createImageNodeAtPosition: (
    x: number,
    y: number,
    initialSnapshotPatch: Pick<
      ImageNodeItem,
      | 'width'
      | 'sourceImageUrl'
      | 'sourceImageRatio'
      | 'outputStatus'
      | 'outputImages'
      | 'libraryImport'
    > &
      Partial<Pick<ImageNodeItem, 'title'>>,
  ) => CanvasGraphNodeIdForKind<'image'> | null
  refreshNodeInternals: (nodeIds: string[]) => void
  selectOnlyNode: (nodeId: string) => boolean
  materialize: LibraryAssetMaterializer
  onWarning: (message: string) => void
  preloadImage?: (url: string) => Promise<boolean>
}

type LibraryImportNode = { node: Node; data: FlowNodeData; snapshot: ImageNodeItem }

const IMPORT_OFFSET = 24
const IMPORT_OFFSET_CYCLE = 6

function resolveRatio(candidate: CanvasLibraryImportDescriptor): number {
  const width = Number(candidate.width)
  const height = Number(candidate.height)
  return Number.isFinite(width) && Number.isFinite(height) && width > 0 && height > 0
    ? height / width
    : 1
}

function resolveErrorMessage(error: unknown): string {
  return userFacingErrorCopy(error, i18n.global.t('canvas.session.libraryImportFailed'))
}

function readLibraryImportNode(nodes: Node[], nodeId: string): LibraryImportNode | null {
  const node = nodes.find((item) => item.id === nodeId)
  const data = node?.data as FlowNodeData | undefined
  const snapshot = data?.snapshotNode as ImageNodeItem | undefined
  if (!node || !data || !snapshot || snapshot.type !== 'image-node') return null
  return { node, data, snapshot }
}

function isValidMaterialization(value: {
  mediaAssetId: number
  url: string
  width: number
  height: number
}): boolean {
  return (
    Number.isInteger(value.mediaAssetId) && value.mediaAssetId > 0 && value.url.trim().length > 0
  )
}

/**
 * Owns Canvas graph facts for Library materialization. Vue Flow is only the
 * projection adapter; pending and failed facts live in the persisted snapshot.
 */
export function useCanvasLibraryImportRuntime(options: UseCanvasLibraryImportRuntimeOptions) {
  let addSequence = 0
  const activeNodeIdsByAttachment = new WeakMap<CanvasBoardAttachment, Set<string>>()

  function isCurrentAttachment(attachment: CanvasBoardAttachment): boolean {
    return options.readAttachment() === attachment
  }

  function updateNode(
    nodeId: string,
    mutate: (current: LibraryImportNode) => Partial<Node>,
    commandOptions = createFlowGraphCommandOptions(),
  ) {
    options.graphAdapter.updateNode(
      nodeId,
      (currentNode) => {
        const current = readLibraryImportNode([currentNode], nodeId)
        return current ? mutate(current) : {}
      },
      commandOptions,
    )
  }

  function setTransientPreview(nodeId: string, previewUrl: string | undefined) {
    updateNode(
      nodeId,
      (current) => {
        const { libraryImportPreviewUrl: _previousPreview, ...data } = current.data
        return {
          data: {
            ...data,
            ...(previewUrl?.trim() ? { libraryImportPreviewUrl: previewUrl.trim() } : {}),
          },
        }
      },
      createFlowGraphCommandOptions({ recordHistory: false, markDirty: false }),
    )
  }

  function clearFactWithMaterializedMedia(
    nodeId: string,
    libraryAssetId: number,
    attachment: CanvasBoardAttachment,
    materialized: Awaited<ReturnType<LibraryAssetMaterializer>>,
    fallbackRatio: number,
  ) {
    if (!isCurrentAttachment(attachment)) return
    if (
      !hasPendingLibraryImport(
        readLibraryImportNode(options.nodes.value, nodeId)?.snapshot,
        libraryAssetId,
      )
    )
      return
    if (!isValidMaterialization(materialized)) {
      throw new Error(i18n.global.t('canvas.session.libraryImportInvalidAsset'))
    }

    options.updateImageSnapshot(nodeId, (current) => {
      if (!isCurrentAttachment(attachment)) return current
      const nextSnapshot = materializeLibraryImport(
        current,
        libraryAssetId,
        materialized,
        fallbackRatio,
      )
      return nextSnapshot ?? current
    })
    options.refreshNodeInternals([nodeId])
  }

  function failImport(
    nodeId: string,
    libraryAssetId: number,
    attachment: CanvasBoardAttachment,
    error: unknown,
    notify: boolean,
  ) {
    if (!isCurrentAttachment(attachment)) return
    if (
      !hasPendingLibraryImport(
        readLibraryImportNode(options.nodes.value, nodeId)?.snapshot,
        libraryAssetId,
      )
    )
      return
    const message = resolveErrorMessage(error)
    options.updateImageSnapshot(
      nodeId,
      (current) => failLibraryImport(current, libraryAssetId, message) ?? current,
    )
    options.refreshNodeInternals([nodeId])
    if (notify) options.onWarning(message)
  }

  function materializePending(
    nodeId: string,
    libraryAssetId: number,
    fallbackRatio: number,
    attachment: CanvasBoardAttachment,
    notifyFailure: boolean,
  ): boolean {
    const activeNodeIds = activeNodeIdsByAttachment.get(attachment) ?? new Set<string>()
    if (activeNodeIds.has(nodeId)) return false
    if (
      !hasPendingLibraryImport(
        readLibraryImportNode(options.nodes.value, nodeId)?.snapshot,
        libraryAssetId,
      )
    )
      return false
    activeNodeIds.add(nodeId)
    activeNodeIdsByAttachment.set(attachment, activeNodeIds)

    void options
      .materialize(libraryAssetId)
      .then((materialized) =>
        clearFactWithMaterializedMedia(
          nodeId,
          libraryAssetId,
          attachment,
          materialized,
          fallbackRatio,
        ),
      )
      .catch((error: unknown) =>
        failImport(nodeId, libraryAssetId, attachment, error, notifyFailure),
      )
      .finally(() => {
        activeNodeIds.delete(nodeId)
      })
    return true
  }

  async function start(
    candidate: CanvasLibraryImportDescriptor,
    center: { x: number; y: number },
  ): Promise<CanvasGraphNodeIdForKind<'image'> | null> {
    const previewReady = await (options.preloadImage ?? preloadImage)(candidate.previewUrl).catch(
      () => false,
    )
    if (!previewReady) {
      options.onWarning(i18n.global.t('canvas.session.libraryImportFailed'))
      return null
    }
    const sourceRatio = resolveRatio(candidate)
    const offset = (addSequence++ % IMPORT_OFFSET_CYCLE) * IMPORT_OFFSET
    const frame = resolveImageNodeFrameSize(sourceRatio, true)
    const nodeId = options.createImageNodeAtPosition(center.x + offset, center.y + offset, {
      ...(candidate.title.trim() ? { title: candidate.title.trim() } : {}),
      width: frame.width,
      sourceImageUrl: '',
      sourceImageRatio: sourceRatio,
      outputStatus: 'idle',
      outputImages: [],
      libraryImport: createPendingLibraryImportFact(candidate.assetId),
    })
    if (!nodeId) return null

    setTransientPreview(nodeId, candidate.previewUrl)
    options.selectOnlyNode(nodeId)
    options.refreshNodeInternals([nodeId])
    const attachment = options.readAttachment()
    if (attachment) materializePending(nodeId, candidate.assetId, sourceRatio, attachment, true)
    return nodeId
  }

  function resumePending() {
    const attachment = options.readAttachment()
    if (!attachment) return 0
    let resumed = 0
    options.nodes.value.forEach((node) => {
      const current = readLibraryImportNode([node], node.id)
      if (!current) return
      const fact = current.snapshot.libraryImport
      if (fact?.state !== 'pending') return
      if (
        materializePending(
          node.id,
          fact.libraryAssetId,
          current.snapshot.sourceImageRatio,
          attachment,
          false,
        )
      ) {
        resumed += 1
      }
    })
    return resumed
  }

  function retry(nodeId: string): boolean {
    const current = readLibraryImportNode(options.nodes.value, nodeId)
    const fact = current?.snapshot.libraryImport
    const attachment = options.readAttachment()
    if (!current || fact?.state !== 'failed' || !attachment) return false
    options.updateImageSnapshot(nodeId, (snapshot) => retryLibraryImport(snapshot) ?? snapshot)
    options.refreshNodeInternals([nodeId])
    return materializePending(
      nodeId,
      fact.libraryAssetId,
      current.snapshot.sourceImageRatio,
      attachment,
      true,
    )
  }

  return { start, resumePending, retry }
}
