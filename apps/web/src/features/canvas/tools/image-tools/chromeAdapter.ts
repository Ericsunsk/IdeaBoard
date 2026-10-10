import { computed } from 'vue'
import { createRetouchChromeSource, hasRenderableRetouchLayerStack } from '../retouch'
import type { ImageToolChromeCommands, ImageToolChromeModel } from './bindings'
import type { ImageToolSession } from './session'

export function createImageToolChromeAdapter(
  session: ImageToolSession,
  commands: ImageToolChromeCommands,
) {
  const { board, execution } = session.capabilities

  // Retouch stage + histogram assembly lives in retouch (single owner):
  // this adapter only merges the mask and editor models for presentation.
  const { retouchStage, retouchHistogramSource } = createRetouchChromeSource({
    isAdjustMode: session.isImageRetouchMode,
    layers: session.retouchSession.retouchLayers,
    selectedEffectLayer: session.retouchSession.selectedRetouchEffectLayer,
    pendingEffectLayer: session.retouchSession.pendingEffectLayer,
    getStageElement: () => {
      const nodeId = session.retouchNodeId.value
      return nodeId ? board.getImageStageElement(nodeId) : null
    },
    sessionKey: computed(() => session.retouchNodeId.value ?? ''),
  })

  const maskModel = computed<ImageToolChromeModel['mask']>(() => {
    const nodeId = session.retouchNodeId.value
    return {
      drawMode: session.inpaint.drawMode.value,
      brushSize: session.inpaint.brushSize.value,
      layerOpacity: session.maskChromeState.inpaintLayerOpacity.value,
      directGenerating: nodeId ? execution.isNodeExecutionActive(nodeId) : false,
      canUndo: session.inpaint.canUndoMask.value,
      canRedo: session.inpaint.canRedoMask.value,
    }
  })
  const editorModel = computed<ImageToolChromeModel['editor']>(() => ({
    layers: session.retouchSession.retouchLayers.value,
    selectedLayerId: session.retouchSession.selectedRetouchLayerId.value,
    selectedEffectLayer: session.retouchSession.selectedRetouchEffectLayer.value,
    histogramSource: retouchHistogramSource.value,
    applyingRetouch: session.applyScope.isApplying.value,
    // Same gate as the retouch apply source: a renderable stack enables apply.
    canApply: hasRenderableRetouchLayerStack(session.retouchSession.retouchLayers.value),
  }))
  const chromeModel = computed<ImageToolChromeModel>(() => ({
    nodeId: session.retouchNodeId.value,
    mode: session.retouchMode.value,
    anchorRect: session.coordinates.retouchChromeAnchorRect.value,
    isReady: session.coordinates.isRetouchChromeReady.value,
    mask: maskModel.value,
    editor: editorModel.value,
  }))

  return {
    chromeModel,
    chromeCommands: commands,
    retouchStage,
  }
}
