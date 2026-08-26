import {
  hasRenderableImageEditorLayerStack,
  renderImageEditorLayerStackFile,
} from '../image-editor/services/imageAdjustmentRenderer'
import { useMaskEditorChromeActions } from '../mask-editor/useMaskEditorChromeActions'
import { isProcessingCancelled } from '../image-editor/runtime/processingCancelled'
import { isCanvasGraphNodeId } from '../graph-runtime'
import { IMAGE_GENERATION_SUCCESS_MESSAGE } from '../../../shared/constants/media'
import { userFacingErrorCopy } from '../../../shared/errors/errorCatalog'
import { i18n } from '../../../shared/i18n'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolCommands(session: ImageToolSession) {
  const { board, execution, ui, feedback } = session.capabilities

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
      const renderedFile = await renderImageEditorLayerStackFile(
        { url: source.url, title: sourceNode.title || 'Image' },
        layers,
        {
          scopeId,
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
      const newNodeId = await board.createImageNodeFromFile(
        {
          x: Number(sourceNode.x ?? 0) + Math.max(240, Number(sourceNode.width ?? 300) + 64),
          y: Number(sourceNode.y ?? 0) + 18,
        },
        renderedFile,
      )
      if (!newNodeId) return
      const didConnect = board.connectIntent({
        sourceNodeId: nodeId,
        sourcePort: 'image',
        targetNodeId: newNodeId,
        targetPort: 'image',
      })
      if (!didConnect) {
        feedback.warning(i18n.global.t('canvas.tools.autoConnectFailed'))
        return
      }
      session.exitImageEditorMode({ runBeforeExit: false })
      session.imageEditorSession.resetImageEditorSession()
      feedback.succeeded(IMAGE_GENERATION_SUCCESS_MESSAGE)
    } catch (error) {
      if (
        isImageAdjustmentFinalApplyCancelled(error) ||
        board.currentBoardId.value !== boardId ||
        session.imageEditorNodeId.value !== nodeId ||
        session.activeImageAdjustmentFinalScopeId.value !== scopeId
      ) {
        return
      }
      feedback.warning(userFacingErrorCopy(error, i18n.global.t('canvas.tools.applyEditFailed')))
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
