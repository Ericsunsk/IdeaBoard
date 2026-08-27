import { computed } from 'vue'
import { resolveMediaAssetOriginalURL } from '../../../shared/api/mediaAssetDelivery'
import { useImageAdjustmentRenderedPreview } from '../image-editor/useImageAdjustmentRenderedPreview'
import { useImageEditorHistogramSource } from '../image-editor/useImageEditorHistogramSource'
import type { ImageToolChromeCommands, ImageToolChromeModel } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolChromeAdapter(
  session: ImageToolSession,
  commands: ImageToolChromeCommands,
) {
  const { board, execution } = session.capabilities
  const { imageEditorRenderedPreview, imageEditorPreviewSource } =
    useImageAdjustmentRenderedPreview({
      isAdjustMode: session.isImageAdjustmentMode,
      layers: session.imageEditorSession.imageEditorLayers,
      sourceUrl: () => {
        const payload = session.imageEditorNodeId.value
          ? board.getImageNodeDisplayPayload(session.imageEditorNodeId.value)
          : null
        return payload?.mediaAssetId
          ? resolveMediaAssetOriginalURL(payload.mediaAssetId)
          : (payload?.url ?? null)
      },
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
      anchorRect: session.coordinates.imageEditorChromeAnchorRect.value,
      isReady: session.coordinates.isImageEditorChromeReady.value,
      mask: {
        drawMode: session.maskEditor.drawMode.value,
        brushSize: session.maskEditor.brushSize.value,
        layerOpacity: session.maskChromeState.maskEditorLayerOpacity.value,
        directGenerating: nodeId ? execution.isNodeExecutionActive(nodeId) : false,
        canUndo: session.maskEditor.canUndoMask.value,
        canRedo: session.maskEditor.canRedoMask.value,
      },
      editor: {
        layers: session.imageEditorSession.imageEditorLayers.value,
        selectedLayerId: session.imageEditorSession.selectedImageEditorLayerId.value,
        selectedAdjustmentLayer:
          session.imageEditorSession.selectedImageEditorAdjustmentLayer.value,
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
