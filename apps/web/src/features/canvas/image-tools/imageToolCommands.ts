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
import { isCanvasGraphNodeId } from '../graph-runtime'
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

    const layers = session.imageEditorSession.imageEditorLayers.value
    if (!hasRenderableImageEditorLayerStack(layers)) {
      feedback.warning(i18n.global.t('canvas.tools.noEditableLayers'))
      return
    }

    session.isApplyingImageEditorAdjustments.value = true
    session.imageAdjustmentFinalApplyProgress.value = { phase: 'queued' }
    const scopeId = nodeId
    session.activeImageAdjustmentFinalScopeId.value = scopeId
    try {
      // Keep image processing same-origin. The API endpoint resolves storage
      // access server-side, avoiding CORS failures from direct signed URLs.
      const sourceUrl = source.mediaAssetId
        ? await resolveMediaAssetProcessingURL(source.mediaAssetId)
        : source.url
      const renderedFile = await renderImageEditorLayerStackFile(
        { url: sourceUrl, title: sourceNode.title || 'Image' },
        layers,
        {
          scopeId,
          maxSide: 2560,
          maxPixels: 8_000_000,
          fitWithinMaxPixels: true,
          onProgress: (progress) => {
            if (session.activeImageAdjustmentFinalScopeId.value !== scopeId) return
            session.imageAdjustmentFinalApplyProgress.value = {
              phase: progress.phase,
              ...(progress.loaded === undefined ? {} : { loaded: progress.loaded }),
              ...(progress.total === undefined ? {} : { total: progress.total }),
            }
          },
        },
      )
      if (board.currentBoardId.value !== boardId || session.imageEditorNodeId.value !== nodeId) {
        return
      }
      const creation = await board.createImageNodeFromFile(
        {
          x: Number(sourceNode.x ?? 0) + Math.max(240, Number(sourceNode.width ?? 300) + 64),
          y: Number(sourceNode.y ?? 0) + 18,
        },
        renderedFile,
      )
      if (!creation || creation.kind === 'rejected') {
        if (board.currentBoardId.value === boardId && session.imageEditorNodeId.value === nodeId) {
          reportGenerationFailure()
        }
        return
      }
      if (creation.kind === 'awaiting-retry') {
        board.connectIntent({
          sourceNodeId: nodeId,
          sourcePort: 'image',
          targetNodeId: creation.nodeId,
          targetPort: 'image',
        })
        reportGenerationFailure()
        return
      }
      const currentPlan = buildImageAdjustmentPlan(layers)
      const recipeSourceMediaAssetId =
        sourceNode.adjustmentRecipe?.sourceMediaAssetId ?? source.mediaAssetId
      if (
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
        session.capabilities.board.updateSnapshotNode(creation.nodeId, (previous) =>
          previous.type === 'image-node' ? { ...previous, adjustmentRecipe: recipe } : previous,
        )
      }
      const didConnect = board.connectIntent({
        sourceNodeId: nodeId,
        sourcePort: 'image',
        targetNodeId: creation.nodeId,
        targetPort: 'image',
      })
      if (!didConnect) {
        feedback.warning(i18n.global.t('canvas.tools.autoConnectFailed'))
        return
      }
      session.exitImageEditorMode({ runBeforeExit: false })
      session.imageEditorSession.resetImageEditorSession()
      feedback.succeeded(i18n.global.t('canvas.feedback.imageGenerated'))
    } catch (error) {
      if (
        isImageAdjustmentFinalApplyCancelled(error) ||
        board.currentBoardId.value !== boardId ||
        session.imageEditorNodeId.value !== nodeId ||
        session.activeImageAdjustmentFinalScopeId.value !== scopeId
      ) {
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
        session.activeImageAdjustmentFinalScopeId.value = null
        session.isApplyingImageEditorAdjustments.value = false
        session.imageAdjustmentFinalApplyProgress.value = null
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
