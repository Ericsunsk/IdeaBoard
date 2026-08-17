import {
  hasRenderableImageEditorLayerStack,
  renderImageEditorLayerStackFile,
} from '../image-editor/services/imageAdjustmentRenderer'
import { useMaskEditorChromeActions } from '../mask-editor/composables/useMaskEditorChromeActions'
import { isCanvasGraphNodeId } from '../graph-runtime'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolCommands(session: ImageToolSession) {
  const { board, execution, ui, feedback } = session.capabilities

  async function applyAdjustments() {
    if (session.isApplyingImageEditorAdjustments.value) return

    const nodeId = session.imageEditorNodeId.value
    if (!nodeId || session.imageEditorMode.value !== 'adjust') return
    if (!isCanvasGraphNodeId(nodeId)) {
      feedback.warning('图片节点未能写入画布')
      return
    }

    const boardId = board.currentBoardId.value
    const source = board.getImageNodeDisplayPayload(nodeId)
    const sourceNode = board.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      feedback.warning('当前图片不可编辑')
      return
    }

    const layers = session.imageEditorSession.imageEditorLayers.value
    if (!hasRenderableImageEditorLayerStack(layers)) {
      feedback.warning('没有可应用的编辑图层')
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
        feedback.warning('图片已创建，但自动连线失败，请手动连接')
        return
      }
      session.exitImageEditorMode({ runBeforeExit: false })
      session.imageEditorSession.resetImageEditorSession()
      feedback.succeeded('已应用编辑并派生新节点')
    } catch (error) {
      if (
        isImageAdjustmentFinalApplyCancelled(error) ||
        board.currentBoardId.value !== boardId ||
        session.imageEditorNodeId.value !== nodeId ||
        session.activeImageAdjustmentFinalScopeId.value !== scopeId
      ) {
        return
      }
      feedback.warning(error instanceof Error ? error.message : '应用编辑失败')
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
  if (!(error instanceof Error)) return false
  const message = error.message.toLowerCase()
  return message === 'cancelled' || message.includes('已取消')
}
