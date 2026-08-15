import {
  hasRenderableImageEditorLayerStack,
  renderImageEditorLayerStackFile,
} from '../image-editor/services/imageAdjustmentRenderer'
import { useMaskEditorChromeActions } from '../mask-editor/composables/useMaskEditorChromeActions'
import { isCanvasGraphNodeId } from '../graph-runtime'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolCommands(session: ImageToolSession) {
  const { options } = session

  async function applyAdjustments() {
    if (session.isApplyingImageEditorAdjustments.value) return

    const nodeId = session.imageEditorNodeId.value
    if (!nodeId || session.imageEditorMode.value !== 'adjust') return
    if (!isCanvasGraphNodeId(nodeId)) {
      options.feedback.warning('图片节点未能写入画布')
      return
    }

    const boardId = options.currentBoardId.value
    const source = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      options.feedback.warning('当前图片不可编辑')
      return
    }

    const layers = session.imageEditorSession.imageEditorLayers.value
    if (!hasRenderableImageEditorLayerStack(layers)) {
      options.feedback.warning('没有可应用的编辑图层')
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
      if (options.currentBoardId.value !== boardId || session.imageEditorNodeId.value !== nodeId) {
        return
      }
      const newNodeId = await options.createImageNodeFromFile(
        {
          x: Number(sourceNode.x ?? 0) + Math.max(240, Number(sourceNode.width ?? 300) + 64),
          y: Number(sourceNode.y ?? 0) + 18,
        },
        renderedFile,
      )
      if (!newNodeId) return
      const didConnect = options.connectionAdapter.connectIntent({
        sourceNodeId: nodeId,
        sourcePort: 'image',
        targetNodeId: newNodeId,
        targetPort: 'image',
      })
      if (!didConnect) {
        options.feedback.warning('图片已创建，但自动连线失败，请手动连接')
        return
      }
      session.imageEditorSession.clearImageEditorRasterHistory()
      session.exitImageEditorMode({ runBeforeExit: false })
      session.imageEditorSession.resetImageEditorSession()
      options.feedback.succeeded('已应用编辑并派生新节点')
    } catch (error) {
      if (
        isImageAdjustmentFinalApplyCancelled(error) ||
        options.currentBoardId.value !== boardId ||
        session.imageEditorNodeId.value !== nodeId ||
        session.activeImageAdjustmentFinalScopeId.value !== scopeId
      ) {
        return
      }
      options.feedback.warning(error instanceof Error ? error.message : '应用编辑失败')
    } finally {
      if (session.activeImageAdjustmentFinalScopeId.value === scopeId) {
        session.activeImageAdjustmentFinalScopeId.value = null
        session.isApplyingImageEditorAdjustments.value = false
        session.imageAdjustmentFinalApplyProgress.value = null
      }
    }
  }

  function runPromptAction(nodeId: string, prompt: string, outputCount: number) {
    const committedMaskData = session.maskEditor.getCommittedMaskData(nodeId)
    if (committedMaskData?.executionMaskDataUrl) {
      void options.runInpaintAction(nodeId, {
        prompt,
        outputCount,
        maskDataUrl: committedMaskData.executionMaskDataUrl,
      })
      return
    }
    options.feedback.warning('请先在图片上标记需要重绘的区域')
  }

  const maskChromeActions = useMaskEditorChromeActions({
    maskEditorNodeId: session.maskEditor.maskEditorNodeId,
    referenceImages: session.maskChromeState.maskEditorReferenceImages,
    commitMaskEditorMode: session.commitMask,
    getCommittedMaskData: session.maskEditor.getCommittedMaskData,
    clearActiveNodePanel: options.clearActiveNodePanel,
    hideStrengthControl: options.hideStrengthControl,
    runInpaintAction: options.runInpaintAction,
    updateOutputCount: (nodeId, outputCount) => {
      options.updateOutputCount(nodeId, {
        key: 'outputCount',
        value: outputCount,
      })
    },
    onWarning: options.feedback.warning,
  })

  const chromeCommands: ImageToolChromeCommands = {
    setMaskDrawMode: session.maskEditor.setDrawMode,
    setMaskBrushShape: session.maskEditor.setBrushShape,
    setMaskBrushSize: session.maskEditor.setBrushSize,
    setMaskBrushHardness: session.maskEditor.setBrushHardness,
    setMaskLayerOpacity: session.maskChromeState.setMaskEditorLayerOpacity,
    clearMask: session.maskEditor.clearMask,
    invertMask: session.maskEditor.invertMask,
    featherMask: session.maskEditor.featherMask,
    undoMask: session.maskEditor.undoMask,
    redoMask: session.maskEditor.redoMask,
    updateMaskOutputCount: maskChromeActions.onMaskEditorChromeOutputCountUpdate,
    openMaskReferenceImageInput: session.maskChromeState.onMaskEditorReferenceImageOpen,
    removeMaskReferenceImage: session.maskChromeState.onMaskEditorReferenceImageRemove,
    previewMaskReferenceImage: session.maskChromeState.onMaskEditorReferenceImagePreview,
    submitMaskPrompt: maskChromeActions.onMaskEditorChromeSubmit,
    selectEditorTool: session.imageEditorSession.selectImageEditorTool,
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
    updateEditorRasterToolSettings: session.imageEditorSession.updateImageEditorRasterToolSettings,
    undoEditorRasterStroke: session.imageEditorSession.undoImageEditorRasterStroke,
    redoEditorRasterStroke: session.imageEditorSession.redoImageEditorRasterStroke,
    applyAdjustments,
  }

  session.setImageEditorApplyRequest(() => {
    void applyAdjustments()
  })

  return {
    chromeCommands,
    applyAdjustments,
    runPromptAction,
  }
}

function isImageAdjustmentFinalApplyCancelled(error: unknown) {
  if (!(error instanceof Error)) return false
  const message = error.message.toLowerCase()
  return message === 'cancelled' || message.includes('已取消')
}
