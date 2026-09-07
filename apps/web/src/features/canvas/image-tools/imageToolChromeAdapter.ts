import { computed } from 'vue'
import {
  resolveMediaAssetDerivativeURL,
  useMediaAssetDelivery,
} from '../../../shared/api/mediaAssetDelivery'
import { useImageRetouchRenderedPreview } from '../retouch/useImageRetouchRenderedPreview'
import { useRetouchHistogramSource } from '../retouch/useRetouchHistogramSource'
import type { ImageToolChromeCommands, ImageToolChromeModel } from './imageToolBindings'
import type { ImageToolSession } from './imageToolSession'

export function createImageToolChromeAdapter(
  session: ImageToolSession,
  commands: ImageToolChromeCommands,
) {
  const { board, execution } = session.capabilities
  const previewAssetId = computed(() => {
    const nodeId = session.retouchNodeId.value
    return nodeId ? (board.getImageNodeDisplayPayload(nodeId)?.mediaAssetId ?? null) : null
  })
  const previewDelivery = useMediaAssetDelivery(previewAssetId, 'preview')
  const { retouchRenderedPreview, retouchPreviewSource } = useImageRetouchRenderedPreview({
    isAdjustMode: session.isImageRetouchMode,
    layers: session.retouchSession.retouchLayers,
    sourceUrl: () => {
      const payload = session.retouchNodeId.value
        ? board.getImageNodeDisplayPayload(session.retouchNodeId.value)
        : null
      if (payload?.mediaAssetId) {
        if (previewDelivery.status.value === 'ready') {
          const previewVariant = previewDelivery.metadata.value?.derivatives.preview
          const format =
            previewVariant?.format === 'jpeg' || previewVariant?.format === 'png'
              ? previewVariant.format
              : 'webp'
          return resolveMediaAssetDerivativeURL(payload.mediaAssetId, 'preview', format)
        }
        // A derivative is the only source allowed during editing. Falling
        // back to the persisted URL here can silently load the original.
        return null
      }
      return payload?.url ?? null
    },
  })
  const { retouchHistogramSource } = useRetouchHistogramSource({
    isAdjustMode: session.isImageRetouchMode,
    layers: session.retouchSession.retouchLayers,
    selectedEffectLayer: session.retouchSession.selectedRetouchEffectLayer,
    previewSource: retouchPreviewSource,
  })

  const chromeModel = computed<ImageToolChromeModel>(() => {
    const nodeId = session.retouchNodeId.value
    return {
      nodeId,
      mode: session.retouchMode.value,
      anchorRect: session.coordinates.retouchChromeAnchorRect.value,
      isReady: session.coordinates.isRetouchChromeReady.value,
      mask: {
        drawMode: session.inpaint.drawMode.value,
        brushSize: session.inpaint.brushSize.value,
        layerOpacity: session.maskChromeState.inpaintLayerOpacity.value,
        directGenerating: nodeId ? execution.isNodeExecutionActive(nodeId) : false,
        canUndo: session.inpaint.canUndoMask.value,
        canRedo: session.inpaint.canRedoMask.value,
      },
      editor: {
        layers: session.retouchSession.retouchLayers.value,
        selectedLayerId: session.retouchSession.selectedRetouchLayerId.value,
        selectedEffectLayer: session.retouchSession.selectedRetouchEffectLayer.value,
        histogramSource: retouchHistogramSource.value,
        applyingRetouch: session.isApplyingRetouch.value,
      },
    }
  })

  return {
    chromeModel,
    chromeCommands: commands,
    retouchRenderedPreview,
  }
}
