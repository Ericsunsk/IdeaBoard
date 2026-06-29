import { computed } from 'vue'
import {
  useImageAdjustmentRenderedPreview,
  useImageEditorHistogramSource,
} from '../image-editor'
import type {
  ImageToolChromeCommands,
  ImageToolChromeModel,
} from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolChromeAdapter(
  session: ImageToolSession,
  commands: ImageToolChromeCommands,
) {
  const { options } = session
  const { imageEditorRenderedPreview, imageEditorPreviewSource } =
    useImageAdjustmentRenderedPreview({
      isAdjustMode: session.isImageAdjustmentMode,
      layers: session.imageEditorSession.imageEditorLayers,
      sourceUrl: () =>
        session.imageEditorNodeId.value
          ? options.getImageNodeDisplayPayload(session.imageEditorNodeId.value)?.url
          : null,
    })
  const { imageEditorHistogramSource } = useImageEditorHistogramSource({
    isAdjustMode: session.isImageAdjustmentMode,
    layers: session.imageEditorSession.imageEditorLayers,
    selectedAdjustmentLayer: session.imageEditorSession.selectedImageEditorAdjustmentLayer,
    previewSource: imageEditorPreviewSource,
  })

  const chromeModel = computed<ImageToolChromeModel>(() => {
    const nodeId = session.imageEditorNodeId.value
    return {
      nodeId,
      mode: session.imageEditorMode.value,
      anchorRect: session.viewport.imageEditorChromeAnchorRect.value,
      isReady: session.viewport.isImageEditorChromeReady.value,
      mask: {
        drawMode: session.maskEditor.drawMode.value,
        brushShape: session.maskEditor.brushShape.value,
        brushSize: session.maskEditor.brushSize.value,
        brushHardness: session.maskEditor.brushHardness.value,
        layerOpacity: session.maskChromeState.maskEditorLayerOpacity.value,
        outputCount: session.maskChromeState.activeMaskEditorOutputCount.value,
        referenceImages: session.maskChromeState.maskEditorReferenceImages.value,
        directGenerating: nodeId ? options.isDirectActionRunning(nodeId) : false,
        canUndo: session.maskEditor.canUndoMask.value,
        canRedo: session.maskEditor.canRedoMask.value,
      },
      editor: {
        activeTool: session.imageEditorSession.activeImageEditorTool.value,
        layers: session.imageEditorSession.imageEditorLayers.value,
        selectedLayerId: session.imageEditorSession.selectedImageEditorLayerId.value,
        selectedAdjustmentLayer: session.imageEditorSession.selectedImageEditorAdjustmentLayer.value,
        histogramSource: imageEditorHistogramSource.value,
        applyingAdjustments: session.isApplyingImageEditorAdjustments.value,
      },
    }
  })

  return {
    chromeModel,
    chromeCommands: commands,
    imageEditorRenderedPreview,
  }
}
