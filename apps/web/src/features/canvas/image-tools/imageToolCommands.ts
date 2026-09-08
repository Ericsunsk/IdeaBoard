import {
  hasRenderableRetouchLayerStack,
  renderRetouchLayerStackFile,
} from '../retouch/services/imageRetouchRenderer'
import {
  MediaAssetDerivativeNotReadyError,
  resolveMediaAssetProcessingURL,
} from '../../../shared/api/mediaAssetDelivery'
import { useInpaintChromeActions } from '../inpaint/useInpaintChromeActions'
import {
  isProcessingCancelled,
  processingCancelledError,
} from '../retouch/runtime/processingCancelled'
import { isCanvasGraphNodeId, type CanvasGraphNodeId } from '../graph-runtime'
import type { ImageNodeDisplayPayload } from '../utils/flowImageNodeSelectors'
import type { ImageNodeItem } from '../../canvas-board/domain'

type RetouchEffectLayers = Parameters<typeof hasRenderableRetouchLayerStack>[0]

type RetouchApplySource = {
  nodeId: CanvasGraphNodeId
  boardId: string | null | undefined
  source: ImageNodeDisplayPayload
  sourceNode: ImageNodeItem
  layers: RetouchEffectLayers
}
import { i18n } from '../../../shared/i18n'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'
import { buildImageRetouchPlan } from '../retouch/utils/imageRetouchPlanPipeline'
import {
  deserializeImageRetouchPlan,
  serializeImageRetouchPlan,
} from '../retouch/utils/imageRetouchRecipe'
import { hasRetouchRasterEdits } from '../retouch/utils/retouchRasterTools'

export function createImageToolCommands(session: ImageToolSession) {
  const { board, execution, ui, feedback } = session.capabilities

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
    return { nodeId, boardId, source, sourceNode, layers }
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

  async function renderRetouchFile(
    retouch: RetouchApplySource,
    scopeId: string,
    isCurrentScope: () => boolean,
  ): Promise<File> {
    // Keep image processing same-origin. The API endpoint resolves storage
    // access server-side, avoiding CORS failures from direct signed URLs.
    const sourceUrl = retouch.source.mediaAssetId
      ? await resolveMediaAssetProcessingURL(retouch.source.mediaAssetId)
      : retouch.source.url
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
    const recipeSourceMediaAssetId =
      retouch.sourceNode.retouchRecipe?.sourceMediaAssetId ?? retouch.source.mediaAssetId
    if (
      recipeSourceMediaAssetId &&
      currentPlan.length > 0 &&
      !hasRetouchRasterEdits(retouch.layers) &&
      session.capabilities.board.updateSnapshotNode
    ) {
      const previousPlan = retouch.sourceNode.retouchRecipe
        ? deserializeImageRetouchPlan(retouch.sourceNode.retouchRecipe)
        : []
      const recipe = serializeImageRetouchPlan(
        recipeSourceMediaAssetId,
        [...previousPlan, ...currentPlan],
        retouch.sourceNode.retouchRecipe?.sourceMimeType,
      )
      session.capabilities.board.updateSnapshotNode(resultNodeId, (previous) =>
        previous.type === 'image-node' ? { ...previous, retouchRecipe: recipe } : previous,
      )
    }
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

  const maskChromeActions = useInpaintChromeActions({
    inpaintNodeId: session.inpaint.inpaintNodeId,
    commitInpaintMode: session.commitMask,
    getCommittedMaskData: session.inpaint.getCommittedMaskData,
    clearActiveNodePanel: ui.clearActiveNodePanel,
    hideStrengthControl: ui.hideStrengthControl,
    runInpaintAction: execution.runInpaintAction,
    onWarning: feedback.warning,
  })

  const chromeCommands: ImageToolChromeCommands = {
    setMaskDrawMode: session.inpaint.setDrawMode,
    setMaskBrushSize: session.inpaint.setBrushSize,
    setMaskLayerOpacity: session.maskChromeState.setInpaintLayerOpacity,
    clearMask: session.inpaint.clearMask,
    undoMask: session.inpaint.undoMask,
    redoMask: session.inpaint.redoMask,
    submitMaskPrompt: maskChromeActions.onInpaintChromeSubmit,
    selectEffectLayer: session.retouchSession.selectImageEffectLayer,
    selectEditorLayer: session.retouchSession.selectRetouchLayer,
    setEditorLayerVisible: session.retouchSession.setRetouchLayerVisible,
    addEditorLayer: session.retouchSession.addRetouchLayer,
    copyEditorLayer: session.retouchSession.copyRetouchLayer,
    mergeEditorLayer: session.retouchSession.mergeRetouchLayer,
    removeEditorLayer: session.retouchSession.removeRetouchLayer,
    updateEditorParam: session.retouchSession.updateSelectedRetouchLayerParam,
    updateEditorRetouchParams: session.retouchSession.updateSelectedRetouchEffectParams,
    applyRetouch,
  }

  return {
    chromeCommands,
    applyRetouch,
  }
}
