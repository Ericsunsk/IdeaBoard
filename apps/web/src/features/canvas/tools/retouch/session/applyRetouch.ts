import { hasRenderableRetouchLayerStack, renderRetouchLayerStackFile } from './stage/runtime/finalRender'
import { isProcessingCancelled, processingCancelledError } from './stage/runtime/processingCancelled'
import {
  MediaAssetDerivativeNotReadyError,
  resolveMediaAssetProcessingURL,
} from '../../../../../shared/api/mediaAssetDelivery'
import { isCanvasGraphNodeId, type CanvasGraphNodeId } from '../../../domain/graph'
import type { ImageNodeDisplayPayload } from '../../../infrastructure/vue-flow-adapter/flowImageNodeSelectors.ts'
import type { CanvasVueFlowConnectionAdapter } from '../../../infrastructure/vue-flow-adapter'
import type { CanvasNodeItem, ImageNodeItem } from '../../../../board/ports/canvas'
import { i18n } from '../../../../../shared/i18n'
import { buildImageRetouchPlan } from '../core/planPipeline.ts'
import { deserializeImageRetouchPlan, serializeImageRetouchPlan } from '../core/recipe.ts'
import { hasRetouchRasterEdits } from '../core/rasterTools.ts'
import type { RetouchLayer, RetouchMode } from '../core/model.ts'

type RetouchEffectLayers = Parameters<typeof hasRenderableRetouchLayerStack>[0]

/*
 * Structured error code -> existing copy key. Every entry reuses a key that
 * already exists in all four locales (canvas.tools.* for the apply gate,
 * canvas.retouch.errors.* for processing failures); no new copy is
 * introduced. Cancelled/stale resolve to null (silent, same as before).
 */
const IMAGE_PROCESSING_ERROR_KEY_BY_CODE: Record<string, string> = {
  'empty-plan': 'canvas.tools.noEditableLayers',
  'invalid-source': 'canvas.retouch.errors.readImageFailed',
  'invalid-url': 'canvas.retouch.errors.invalidImageUrl',
  'fetch-failed': 'canvas.retouch.errors.readImageFailed',
  'not-image': 'canvas.retouch.errors.notAnImage',
  'animated-image': 'canvas.retouch.errors.animatedUnsupported',
  'too-large': 'canvas.retouch.errors.tooLargeToEdit',
  'unsupported-canvas': 'canvas.retouch.errors.unsupportedCanvas',
  disposed: 'canvas.retouch.errors.cacheDisposed',
  'decode-failed': 'canvas.retouch.errors.decodeFailed',
  'backend-failed': 'canvas.retouch.errors.processingFailed',
  'encode-failed': 'canvas.retouch.errors.processingFailed',
  unknown: 'canvas.retouch.errors.processingFailed',
}

function resolveImageProcessingErrorKey(error: unknown): string | null {
  if (!error || typeof error !== 'object') return null
  const code = (error as { code?: unknown }).code
  if (typeof code !== 'string') return null
  if (code === 'cancelled' || code === 'stale') return null
  return IMAGE_PROCESSING_ERROR_KEY_BY_CODE[code] ?? null
}

export type RetouchApplySource = {
  nodeId: CanvasGraphNodeId
  boardId: string | null | undefined
  source: ImageNodeDisplayPayload
  sourceNode: ImageNodeItem
  layers: RetouchEffectLayers
  selectedLayerId: string
}

/**
 * Board capabilities the retouch apply pipeline needs. Structural subset of
 * the image-tools session board capabilities: the pipeline never touches the
 * stage element, so `getImageStageElement` stays on the image-tools side.
 */
export type RetouchApplyBoardCapabilities = {
  currentBoardId: { readonly value: string | null | undefined }
  getImageNodeDisplayPayload: (nodeId: string) => ImageNodeDisplayPayload | null
  getImageNodeSnapshot: (nodeId: string) => ImageNodeItem | null
  updateSnapshotNode?:
    ((nodeId: string, patcher: (previous: CanvasNodeItem) => CanvasNodeItem) => void) | undefined
  createPendingRetouchNode: (sourceNode: ImageNodeItem) => string | null
  applyImageFileToImageNode: (nodeId: string, file: File) => Promise<boolean>
  removeNode: (nodeId: string) => void
  connectIntent: CanvasVueFlowConnectionAdapter['connectIntent']
}

export type RetouchApplyFeedbackCapabilities = {
  warning: (message: string) => void
  succeeded: (message: string) => void
}

export type RetouchApplyScope = {
  readonly scopeId: string
  isCurrent(): boolean
  release(): void
}

export type RetouchApplyScopeHost = {
  readonly isApplying: { readonly value: boolean }
  begin(nodeId: string, boardId: string | null | undefined): RetouchApplyScope | null
  notePendingNode(nodeId: string): void
  clearPendingNode(nodeId: string): void
  onCleanup(cleanup: (() => void) | null): void
}

/**
 * Session surface the retouch apply pipeline reads. Structural: the
 * image-tools session satisfies this without retouch importing image-tools
 * types (the dependency points image-tools -> retouch only).
 */
export type RetouchApplySession = {
  capabilities: {
    board: RetouchApplyBoardCapabilities
    feedback: RetouchApplyFeedbackCapabilities
  }
  applyScope: RetouchApplyScopeHost
  retouchNodeId: { readonly value: string | null }
  retouchMode: { readonly value: RetouchMode | null }
  retouchSession: {
    retouchLayers: { readonly value: readonly RetouchLayer[] }
    selectedRetouchLayerId: { readonly value: string }
    resetRetouchSession: () => void
  }
  exitRetouchMode: (options?: { runBeforeExit?: boolean }) => void
}

export function createRetouchApplyCommands(session: RetouchApplySession) {
  const { board, feedback } = session.capabilities

  const reportGenerationFailure = () => {
    feedback.warning(i18n.global.t('canvas.feedback.imageGenerationFailed'))
  }

  function readApplySource(): RetouchApplySource | null {
    if (session.applyScope.isApplying.value) return null

    const nodeId = session.retouchNodeId.value
    if (!nodeId || session.retouchMode.value !== 'adjust') return null
    if (!isCanvasGraphNodeId(nodeId)) {
      feedback.warning(i18n.global.t('canvas.tools.nodeWriteFailed'))
      return null
    }

    const boardId = board.currentBoardId.value
    const source = board.getImageNodeDisplayPayload(nodeId)
    const sourceNode = board.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      feedback.warning(i18n.global.t('canvas.tools.imageNotEditable'))
      return null
    }

    const layers = [...session.retouchSession.retouchLayers.value]
    if (!hasRenderableRetouchLayerStack(layers)) {
      feedback.warning(i18n.global.t('canvas.tools.noEditableLayers'))
      return null
    }
    // Capture the selection now: apply exits and resets the session before
    // the async render completes, so late persistence must use this snapshot.
    const selectedLayerId = session.retouchSession.selectedRetouchLayerId.value
    return { nodeId, boardId, source, sourceNode, layers, selectedLayerId }
  }

  function createRetouchPendingNode(sourceNode: ImageNodeItem): CanvasGraphNodeId | null {
    const createdId = board.createPendingRetouchNode(sourceNode)
    if (!createdId || !isCanvasGraphNodeId(createdId)) {
      reportGenerationFailure()
      return null
    }
    return createdId
  }

  function connectRetouchNode(
    nodeId: CanvasGraphNodeId,
    pendingNodeId: CanvasGraphNodeId,
    cleanupPendingNode: () => void,
  ): boolean {
    session.applyScope.notePendingNode(pendingNodeId)
    const didConnect = board.connectIntent({
      sourceNodeId: nodeId,
      sourcePort: 'image',
      targetNodeId: pendingNodeId,
      targetPort: 'image',
    })
    if (!didConnect) {
      cleanupPendingNode()
      feedback.warning(i18n.global.t('canvas.tools.autoConnectFailed'))
      return false
    }
    return true
  }

  /**
   * Resolve the full-resolution export source. When assetId is present, the
   * processing derivative URL is required; failures throw directly.
   */
  async function resolveExportSourceUrl(
    retouch: RetouchApplySource,
    isCurrentScope: () => boolean,
  ): Promise<string> {
    // Keep image processing same-origin. The API endpoint resolves storage
    // access server-side, avoiding CORS failures from direct signed URLs.
    if (!retouch.source.assetId) return retouch.source.url
    const url = await resolveMediaAssetProcessingURL(retouch.source.assetId)
    if (!isCurrentScope()) throw processingCancelledError()
    return url
  }

  async function renderRetouchFile(
    retouch: RetouchApplySource,
    scopeId: string,
    isCurrentScope: () => boolean,
  ): Promise<File> {
    // Full-resolution export stays on the background network path (issue
    // #673): the processing derivative, not the interactive preview texture.
    const sourceUrl = await resolveExportSourceUrl(retouch, isCurrentScope)
    // A late URL response must not supersede the current latestOnly render.
    if (!isCurrentScope()) throw processingCancelledError()
    return renderRetouchLayerStackFile(
      { url: sourceUrl, title: retouch.sourceNode.title || 'Image' },
      retouch.layers,
      {
        scopeId,
        maxSide: 2560,
        maxPixels: 8_000_000,
        fitWithinMaxPixels: true,
      },
    )
  }

  function persistRetouchRecipe(
    resultNodeId: CanvasGraphNodeId,
    retouch: RetouchApplySource,
  ): void {
    const currentPlan = buildImageRetouchPlan(retouch.layers)
    const recipeSourceAssetId =
      retouch.sourceNode.retouchRecipe?.sourceAssetId ?? retouch.source.assetId
    if (!session.capabilities.board.updateSnapshotNode) return
    // Invariant: a result node stores baked pixels, so it must never carry the
    // same adjustment stack. Restoring that stack on re-entry would show stale
    // slider values and the zero-copy stage (DOM image as texture base) would
    // apply the same effects a second time. Only the recipe — re-renderable
    // from the original asset (issue #671) — is durable here.
    if (!recipeSourceAssetId || currentPlan.length === 0 || hasRetouchRasterEdits(retouch.layers)) {
      return
    }
    const previousPlan = retouch.sourceNode.retouchRecipe
      ? deserializeImageRetouchPlan(retouch.sourceNode.retouchRecipe)
      : []
    const recipe = serializeImageRetouchPlan(
      recipeSourceAssetId,
      [...previousPlan, ...currentPlan],
      retouch.sourceNode.retouchRecipe?.sourceMimeType,
    )
    session.capabilities.board.updateSnapshotNode(resultNodeId, (previous) =>
      previous.type === 'image-node' ? { ...previous, retouchRecipe: recipe } : previous,
    )
  }

  function reportRetouchError(error: unknown, isCurrentScope: () => boolean): void {
    if (isProcessingCancelled(error) || !isCurrentScope()) {
      return
    }
    if (error instanceof MediaAssetDerivativeNotReadyError) {
      if (error.status === 'pending' || error.status === 'processing') {
        feedback.warning(i18n.global.t('canvas.task.stillProcessing'))
      } else {
        reportGenerationFailure()
      }
      return
    }
    // Structured processing failures carry only a code (runtime stays
    // locale-free); localized copy resolves here via the existing canvas.*
    // keys so no new copy is introduced across the four locales.
    const errorKey = resolveImageProcessingErrorKey(error)
    if (errorKey) {
      feedback.warning(i18n.global.t(errorKey))
      return
    }
    reportGenerationFailure()
  }

  async function applyRetouch() {
    const retouch = readApplySource()
    if (!retouch) return
    const { nodeId, boardId } = retouch

    const scope = session.applyScope.begin(nodeId, boardId)
    if (!scope) return
    const scopeId = scope.scopeId

    // The editor is a configuration surface, not the processing surface. Exit
    // immediately so the derived node can reuse the standard AI generation UI.
    // Bypass the exit hook because it is intentionally wired to cancel active
    // work when the user dismisses the editor.
    session.exitRetouchMode({ runBeforeExit: false })
    session.retouchSession.resetRetouchSession()

    // Generation-scoped guard: once this cleanup has run (or the apply never
    // created a node), later invocations are inert. The host tracks the
    // cross-generation pending node for readers; this local makes a stale
    // closure unable to remove a newer apply's node. Do not merge the two.
    let pendingNodeId: CanvasGraphNodeId | null = null
    let pendingNodeMaterialized = false

    const isCurrentScope = () => scope.isCurrent()

    const cleanupPendingNode = () => {
      if (!pendingNodeId) return
      const nodeIdToCleanup = pendingNodeId
      if (!pendingNodeMaterialized) board.removeNode(nodeIdToCleanup)
      session.applyScope.clearPendingNode(nodeIdToCleanup)
      pendingNodeId = null
    }
    session.applyScope.onCleanup(cleanupPendingNode)

    try {
      const createdNodeId = createRetouchPendingNode(retouch.sourceNode)
      if (!createdNodeId) return
      pendingNodeId = createdNodeId
      if (!connectRetouchNode(nodeId, createdNodeId, cleanupPendingNode)) return

      const renderedFile = await renderRetouchFile(retouch, scopeId, isCurrentScope)
      if (!isCurrentScope()) {
        cleanupPendingNode()
        return
      }

      const materialized = await board.applyImageFileToImageNode(createdNodeId, renderedFile)
      if (!isCurrentScope()) {
        cleanupPendingNode()
        return
      }
      if (!materialized) {
        cleanupPendingNode()
        reportGenerationFailure()
        return
      }
      pendingNodeMaterialized = true
      if (!isCurrentScope()) {
        cleanupPendingNode()
        return
      }

      persistRetouchRecipe(createdNodeId, retouch)
      feedback.succeeded(i18n.global.t('canvas.feedback.imageGenerated'))
    } catch (error) {
      cleanupPendingNode()
      reportRetouchError(error, isCurrentScope)
    } finally {
      scope.release()
    }
  }

  return {
    applyRetouch,
  }
}
