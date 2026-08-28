import {
  hasRenderableImageEditorLayerStack,
  renderImageEditorLayerStackFile,
} from '../image-editor/services/imageAdjustmentRenderer'
import {
  MediaAssetDerivativeNotReadyError,
  resolveMediaAssetProcessingURL,
} from '../../../shared/api/mediaAssetDelivery'
import { useMaskEditorChromeActions } from '../mask-editor/useMaskEditorChromeActions'
import { isProcessingCancelled } from '../image-editor/runtime/processingCancelled'
import { isCanvasGraphNodeId, type CanvasGraphNodeId } from '../graph-runtime'
import { i18n } from '../../../shared/i18n'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'
import { buildImageAdjustmentPlan } from '../image-editor/utils/imageAdjustmentPlanPipeline'
import {
  deserializeImageAdjustmentPlan,
  serializeImageAdjustmentPlan,
} from '../image-editor/utils/imageAdjustmentRecipe'
import { hasImageEditorRasterEdits } from '../image-editor/utils/imageEditorRasterTools'

export function createImageToolCommands(session: ImageToolSession) {
  const { board, execution, ui, feedback } = session.capabilities

  const reportGenerationFailure = () => {
    feedback.warning(i18n.global.t('canvas.feedback.imageGenerationFailed'))
  }

  async function applyAdjustments() {
    if (session.isApplyingImageEditorAdjustments.value) return

    const nodeId = session.imageEditorNodeId.value
    if (!nodeId || session.imageEditorMode.value !== 'adjust') return
    if (!isCanvasGraphNodeId(nodeId)) {
      feedback.warning(i18n.global.t('canvas.tools.nodeWriteFailed'))
      return
    }

    const boardId = board.currentBoardId.value
    const source = board.getImageNodeDisplayPayload(nodeId)
    const sourceNode = board.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      feedback.warning(i18n.global.t('canvas.tools.imageNotEditable'))
      return
    }

    const layers = [...session.imageEditorSession.imageEditorLayers.value]
    if (!hasRenderableImageEditorLayerStack(layers)) {
      feedback.warning(i18n.global.t('canvas.tools.noEditableLayers'))
      return
    }

    session.isApplyingImageEditorAdjustments.value = true
    const scopeId = nodeId
    session.activeImageAdjustmentFinalScopeId.value = scopeId

    // The editor is a configuration surface, not the processing surface. Exit
    // immediately so the derived node can reuse the standard AI generation UI.
    // Bypass the exit hook because it is intentionally wired to cancel active
    // work when the user dismisses the editor.
    session.exitImageEditorMode({ runBeforeExit: false })
    session.imageEditorSession.resetImageEditorSession()

    const targetPoint = {
      x: Number(sourceNode.x ?? 0) + Math.max(240, Number(sourceNode.width ?? 300) + 64),
      y: Number(sourceNode.y ?? 0),
    }

    let pendingNodeId: CanvasGraphNodeId | null = null
    let pendingNodeMaterialized = false

    const isCurrentScope = () =>
      board.currentBoardId.value === boardId &&
      session.activeImageAdjustmentFinalScopeId.value === scopeId &&
      !!board.getImageNodeSnapshot(nodeId)

    const cleanupPendingNode = () => {
      if (!pendingNodeId) return
      const nodeIdToCleanup = pendingNodeId
      if (!pendingNodeMaterialized) board.removeNode(nodeIdToCleanup)
      if (session.activeImageAdjustmentPendingNodeId.value === nodeIdToCleanup) {
        session.activeImageAdjustmentPendingNodeId.value = null
      }
      pendingNodeId = null
    }
    session.registerImageAdjustmentFinalApplyCleanup(cleanupPendingNode)

    try {
      const createdId = board.createPendingAdjustmentNode(targetPoint, sourceNode)
      if (!createdId || !isCanvasGraphNodeId(createdId)) {
        reportGenerationFailure()
        return
      }
      pendingNodeId = createdId
      session.activeImageAdjustmentPendingNodeId.value = createdId
      const didConnect = board.connectIntent({
        sourceNodeId: nodeId,
        sourcePort: 'image',
        targetNodeId: pendingNodeId,
        targetPort: 'image',
      })
      if (!didConnect) {
        cleanupPendingNode()
        feedback.warning(i18n.global.t('canvas.tools.autoConnectFailed'))
        return
      }

      // Keep image processing same-origin. The API endpoint resolves storage
      // access server-side, avoiding CORS failures from direct signed URLs.
      const sourceUrl = source.mediaAssetId
        ? await resolveMediaAssetProcessingURL(source.mediaAssetId)
        : source.url
      if (!isCurrentScope()) {
        cleanupPendingNode()
        return
      }
      const renderedFile = await renderImageEditorLayerStackFile(
        { url: sourceUrl, title: sourceNode.title || 'Image' },
        layers,
        {
          scopeId,
          maxSide: 2560,
          maxPixels: 8_000_000,
          fitWithinMaxPixels: true,
        },
      )
      if (!isCurrentScope()) {
        cleanupPendingNode()
        return
      }

      const resultNodeId = pendingNodeId
      if (!resultNodeId) {
        reportGenerationFailure()
        return
      }
      const materialized = await board.applyImageFileToImageNode(resultNodeId, renderedFile)
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

      const currentPlan = buildImageAdjustmentPlan(layers)
      const recipeSourceMediaAssetId =
        sourceNode.adjustmentRecipe?.sourceMediaAssetId ?? source.mediaAssetId
      if (
        resultNodeId &&
        recipeSourceMediaAssetId &&
        currentPlan.length > 0 &&
        !hasImageEditorRasterEdits(layers) &&
        session.capabilities.board.updateSnapshotNode
      ) {
        const previousPlan = sourceNode.adjustmentRecipe
          ? deserializeImageAdjustmentPlan(sourceNode.adjustmentRecipe)
          : []
        const recipe = serializeImageAdjustmentPlan(
          recipeSourceMediaAssetId,
          [...previousPlan, ...currentPlan],
          sourceNode.adjustmentRecipe?.sourceMimeType,
        )
        session.capabilities.board.updateSnapshotNode(resultNodeId, (previous) =>
          previous.type === 'image-node' ? { ...previous, adjustmentRecipe: recipe } : previous,
        )
      }
      feedback.succeeded(i18n.global.t('canvas.feedback.imageGenerated'))
    } catch (error) {
      cleanupPendingNode()
      if (isImageAdjustmentFinalApplyCancelled(error) || !isCurrentScope()) {
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
    } finally {
      if (session.activeImageAdjustmentFinalScopeId.value === scopeId) {
        session.registerImageAdjustmentFinalApplyCleanup(null)
        session.activeImageAdjustmentFinalScopeId.value = null
        session.isApplyingImageEditorAdjustments.value = false
      }
    }
  }

  const maskChromeActions = useMaskEditorChromeActions({
    maskEditorNodeId: session.maskEditor.maskEditorNodeId,
    commitMaskEditorMode: session.commitMask,
    getCommittedMaskData: session.maskEditor.getCommittedMaskData,
    clearActiveNodePanel: ui.clearActiveNodePanel,
    hideStrengthControl: ui.hideStrengthControl,
    runInpaintAction: execution.runInpaintAction,
    onWarning: feedback.warning,
  })

  const chromeCommands: ImageToolChromeCommands = {
    setMaskDrawMode: session.maskEditor.setDrawMode,
    setMaskBrushSize: session.maskEditor.setBrushSize,
    setMaskLayerOpacity: session.maskChromeState.setMaskEditorLayerOpacity,
    clearMask: session.maskEditor.clearMask,
    undoMask: session.maskEditor.undoMask,
    redoMask: session.maskEditor.redoMask,
    submitMaskPrompt: maskChromeActions.onMaskEditorChromeSubmit,
    selectAdjustmentLayer: session.imageEditorSession.selectImageAdjustmentLayer,
    selectEditorLayer: session.imageEditorSession.selectImageEditorLayer,
    setEditorLayerVisible: session.imageEditorSession.setImageEditorLayerVisible,
    addEditorLayer: session.imageEditorSession.addImageEditorLayer,
    copyEditorLayer: session.imageEditorSession.copyImageEditorLayer,
    mergeEditorLayer: session.imageEditorSession.mergeImageEditorLayer,
    removeEditorLayer: session.imageEditorSession.removeImageEditorLayer,
    updateEditorParam: session.imageEditorSession.updateSelectedImageEditorLayerParam,
    updateEditorAdjustmentParams:
      session.imageEditorSession.updateSelectedImageEditorAdjustmentParams,
    applyAdjustments,
  }

  return {
    chromeCommands,
    applyAdjustments,
  }
}

function isImageAdjustmentFinalApplyCancelled(error: unknown) {
  return isProcessingCancelled(error)
}
