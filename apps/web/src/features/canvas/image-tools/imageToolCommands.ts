import {
  flattenVisibleImageEditorAdjustmentLayers,
  hasEffectiveImageEditorAdjustmentLayers,
  renderImageAdjustmentFile,
} from '../image-editor'
import { useMaskEditorChromeActions } from '../mask-editor'
import type { ImageToolChromeCommands } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolCommands(session: ImageToolSession) {
  const { options } = session

  async function applyAdjustments() {
    if (session.isApplyingImageEditorAdjustments.value) return

    const nodeId = session.imageEditorNodeId.value
    if (!nodeId || session.imageEditorMode.value !== 'adjust') return

    const boardId = options.currentBoardId.value
    const source = options.getImageNodeDisplayPayload(nodeId)
    const sourceNode = options.getImageNodeSnapshot(nodeId)
    if (!source || !sourceNode) {
      options.feedback.warning('当前图片不可编辑')
      return
    }

    const visibleAdjustmentLayers = flattenVisibleImageEditorAdjustmentLayers(
      session.imageEditorSession.imageEditorLayers.value,
    )
    if (
      !hasEffectiveImageEditorAdjustmentLayers(session.imageEditorSession.imageEditorLayers.value)
    ) {
      options.feedback.warning('没有可应用的编辑图层')
      return
    }

    session.isApplyingImageEditorAdjustments.value = true
    session.imageAdjustmentFinalApplyProgress.value = { phase: 'queued' }
    const scopeId = nodeId
    session.activeImageAdjustmentFinalScopeId.value = scopeId
    try {
      const renderedFile = await renderImageAdjustmentFile(
        { url: source.url, title: sourceNode.title || 'Image' },
        visibleAdjustmentLayers,
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
      options.connectImageNodes({
        source: nodeId,
        sourceHandle: 'out:image',
        target: newNodeId,
        targetHandle: 'in:image',
      })
      session.exitImageEditorMode()
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
    void options.runPromptGeneration(nodeId, prompt, outputCount)
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
    applyAdjustments,
  }

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
