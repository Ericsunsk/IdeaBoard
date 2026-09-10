import { computed } from 'vue'
import {
  resolveDomTextureImage,
  useZeroCopyRetouchStage,
} from '../retouch/useZeroCopyRetouchStage.ts'
import { useRetouchHistogramSource } from '../retouch/useRetouchHistogramSource'
import type { ImageToolChromeCommands, ImageToolChromeModel } from './bindings'
import type { ImageToolSession } from './session'
import type { RetouchRenderedPreview } from '../retouch/model.ts'

export function createImageToolChromeAdapter(
  session: ImageToolSession,
  commands: ImageToolChromeCommands,
) {
  const { board, execution } = session.capabilities

  // Zero-copy GPU stage (issues #672, #673): the mounted canvas IS the render
  // target. Texture binding uses the DOM image already on screen directly.
  const retouchStage = useZeroCopyRetouchStage({
    isAdjustMode: session.isImageRetouchMode,
    layers: session.retouchSession.retouchLayers,
    resolveDomImage: () => {
      const nodeId = session.retouchNodeId.value
      const stageElement = nodeId ? board.getImageStageElement(nodeId) : null
      return (
        resolveDomTextureImage(retouchStage.canvasRef.value?.parentElement ?? null) ??
        resolveDomTextureImage(stageElement)
      )
    },
  })

  // Histogram wraps the throttled GPU sample to the shape expected by the
  // histogram source resolver.
  const histogramPreviewSource = computed<RetouchRenderedPreview | null>(() => {
    const sample = retouchStage.histogramSample.value
    if (!sample) return null
    return {
      type: 'rendered-image',
      imageData: sample,
      width: sample.width,
      height: sample.height,
    }
  })
  const { retouchHistogramSource } = useRetouchHistogramSource({
    isAdjustMode: session.isImageRetouchMode,
    layers: session.retouchSession.retouchLayers,
    selectedEffectLayer: session.retouchSession.selectedRetouchEffectLayer,
    previewSource: histogramPreviewSource,
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
        applyingRetouch: session.applyScope.isApplying.value,
      },
    }
  })

  return {
    chromeModel,
    chromeCommands: commands,
    retouchStage,
  }
}
