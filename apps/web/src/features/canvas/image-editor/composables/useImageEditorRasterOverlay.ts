import { computed, onBeforeUnmount, shallowRef, watch, type Ref } from 'vue'
import type { CanvasInteractionKey } from '../../composables/useFlowCanvasInteractionState'
import { usePointerDragSession } from '../../composables/usePointerDragSession'
import type { ImageEditorLayer, ImageEditorRasterContent, ImageEditorTool } from '../types'
import type { ImageEditorModeState } from './useImageEditorMode'
import { createImageSourceCache, readImageSourceImageData } from '../runtime/imageSourceCache'
import { cloneImageData } from '../utils/imageDataUtils'
import {
  compositeImageEditorLayerStack,
  createTransparentImageData,
} from '../utils/imageEditorRasterComposite'
import {
  applyImageEditorRasterStroke,
  canUseImageEditorRasterTool,
  getSelectedImageEditorLayer,
  hasImageEditorRasterEdits,
  isImageEditorRasterLayer,
  isImageEditorRasterTool,
  type ImageEditorRasterStrokePoint,
  type ImageEditorRasterTool,
  type ImageEditorRasterToolSettings,
} from '../utils/imageEditorRasterTools'

const IMAGE_EDITOR_RASTER_INTERACTION_KEY: CanvasInteractionKey = 'image-editor-raster'

type UseImageEditorRasterOverlayOptions = {
  editorState: Ref<ImageEditorModeState | null>
  sourceUrl: Ref<string | null>
  layers: Ref<readonly ImageEditorLayer[]>
  selectedLayerId: Ref<string>
  activeTool: Ref<ImageEditorTool>
  settings: Ref<ImageEditorRasterToolSettings>
  captureHistorySnapshot: () => ImageEditorLayer[]
  commitRasterStroke: (
    layerId: string,
    beforeLayers: readonly ImageEditorLayer[],
    content: ImageEditorRasterContent,
  ) => void
  startCanvasInteraction?: (key: CanvasInteractionKey) => void
  stopCanvasInteraction?: (key: CanvasInteractionKey) => void
}

type PaintingStrokeState = {
  layerId: string
  tool: ImageEditorRasterTool
  beforeLayers: ImageEditorLayer[]
  baseImageData: ImageData
  currentImageData: ImageData
  pointGroups: ImageEditorRasterStrokePoint[][]
  activePointGroup: ImageEditorRasterStrokePoint[] | null
  settings: ImageEditorRasterToolSettings
}

export function useImageEditorRasterOverlay(options: UseImageEditorRasterOverlayOptions) {
  const sourceCache = createImageSourceCache({ maxEntries: 1 })
  const sourceImageData = shallowRef<ImageData | null>(null)
  const sourceRequestToken = shallowRef(0)
  const canvasRefs = new Map<string, HTMLCanvasElement>()
  const rasterImageCache = new Map<string, ImageData>()
  let paintingStroke: PaintingStrokeState | null = null
  let interactionActive = false

  const pointerDragSession = usePointerDragSession<void>({
    onMove: ({ event }) => onPointerMove(event),
    onEnd: finishPainting,
    onCancel: finishPainting,
  })

  const activeNodeId = computed(() => options.editorState.value?.nodeId ?? null)
  const activeRasterTool = computed(() => {
    const tool = options.activeTool.value
    return isImageEditorRasterTool(tool) ? tool : null
  })
  const rasterToolEnabled = computed(
    () =>
      activeRasterTool.value != null &&
      sourceImageData.value != null &&
      canUseImageEditorRasterTool(options.layers.value, options.selectedLayerId.value),
  )
  const hasRasterEdits = computed(() => hasImageEditorRasterEdits(options.layers.value))
  const rasterOverlayStyle = computed<Record<string, string> | null>(() => {
    const state = options.editorState.value
    if (!state || state.mode !== 'adjust') return null
    return {
      left: `${state.imageRect.left}px`,
      top: `${state.imageRect.top}px`,
      width: `${state.imageRect.width}px`,
      height: `${state.imageRect.height}px`,
    }
  })
  const rasterCanvasWidth = computed(() => Math.max(1, sourceImageData.value?.width ?? 1))
  const rasterCanvasHeight = computed(() => Math.max(1, sourceImageData.value?.height ?? 1))

  function startInteraction() {
    if (interactionActive) return
    interactionActive = true
    options.startCanvasInteraction?.(IMAGE_EDITOR_RASTER_INTERACTION_KEY)
  }

  function stopInteraction() {
    if (!interactionActive) return
    interactionActive = false
    options.stopCanvasInteraction?.(IMAGE_EDITOR_RASTER_INTERACTION_KEY)
  }

  async function loadSourceImageData(sourceUrl: string | null) {
    const token = sourceRequestToken.value + 1
    sourceRequestToken.value = token
    sourceImageData.value = null
    rasterImageCache.clear()
    renderRasterPreview()
    if (!sourceUrl || options.editorState.value?.mode !== 'adjust') return

    try {
      const source = await sourceCache.getFullSizeSource(sourceUrl)
      if (!source) return
      if (sourceRequestToken.value !== token) {
        sourceCache.releaseSource(source)
        return
      }
      sourceImageData.value = readImageSourceImageData(source)
      sourceCache.releaseSource(source)
      renderRasterPreview()
    } catch (error) {
      if (sourceRequestToken.value === token) {
        console.warn('[image-editor] failed to load raster source', error)
      }
    }
  }

  function setRasterCanvasRef(nodeId: string, element: Element | null) {
    const canvas = element instanceof HTMLCanvasElement ? element : null
    if (!canvas) {
      canvasRefs.delete(nodeId)
      if (paintingStroke && activeNodeId.value === nodeId) stopPainting()
      return
    }
    canvasRefs.set(nodeId, canvas)
    renderRasterPreview()
  }

  function getActiveCanvas() {
    const nodeId = activeNodeId.value
    return nodeId ? (canvasRefs.get(nodeId) ?? null) : null
  }

  function renderRasterPreview(overrides?: ReadonlyMap<string, ImageData>) {
    const canvas = getActiveCanvas()
    const source = sourceImageData.value
    if (!canvas || !source) return
    if (canvas.width !== source.width) canvas.width = source.width
    if (canvas.height !== source.height) canvas.height = source.height
    const context = canvas.getContext('2d')
    if (!context) return

    if (!hasRasterEdits.value && !overrides?.size) {
      context.clearRect(0, 0, canvas.width, canvas.height)
      return
    }

    const preview = compositeImageEditorLayerStack(source, options.layers.value, {
      getRasterImageData: (content) => rasterImageCache.get(content.dataUrl) ?? null,
      overrides,
    })
    context.putImageData(preview, 0, 0)
  }

  function onCanvasPointerDown(event: PointerEvent) {
    if (event.pointerType === 'mouse' && event.button !== 0) return
    const canvas = getActiveCanvas()
    const source = sourceImageData.value
    const tool = activeRasterTool.value
    if (!canvas || !source || !tool || !rasterToolEnabled.value) return
    const selectedLayer = getSelectedImageEditorLayer(
      options.layers.value,
      options.selectedLayerId.value,
    )
    if (!isImageEditorRasterLayer(selectedLayer)) return

    event.preventDefault()
    event.stopPropagation()
    stopPainting()

    const point = projectPointerToRasterCanvas(event, canvas)
    if (!point) return
    const baseImageData = createLayerBaseImageData(selectedLayer, source)
    const beforeLayers = options.captureHistorySnapshot()
    const pointGroup = [point]
    paintingStroke = {
      layerId: selectedLayer.id,
      tool,
      beforeLayers,
      baseImageData,
      currentImageData: cloneImageData(baseImageData),
      pointGroups: [pointGroup],
      activePointGroup: pointGroup,
      settings: { ...options.settings.value },
    }

    startInteraction()
    pointerDragSession.beginDrag(event, undefined, { target: canvas })
    redrawPaintingStroke()
  }

  function onPointerMove(event: PointerEvent) {
    const canvas = getActiveCanvas()
    if (!canvas || !paintingStroke) return
    event.preventDefault()
    event.stopPropagation()
    const point = projectPointerToRasterCanvas(event, canvas)
    if (!point) {
      paintingStroke.activePointGroup = null
      return
    }
    if (!paintingStroke.activePointGroup) {
      paintingStroke.activePointGroup = [point]
      paintingStroke.pointGroups.push(paintingStroke.activePointGroup)
    } else {
      paintingStroke.activePointGroup.push(point)
    }
    redrawPaintingStroke()
  }

  function redrawPaintingStroke() {
    const stroke = paintingStroke
    if (!stroke) return
    const nextImageData = cloneImageData(stroke.baseImageData)
    applyImageEditorRasterStroke(nextImageData, {
      tool: stroke.tool,
      points: [],
      pointGroups: stroke.pointGroups,
      size: stroke.settings.size,
      hardness: stroke.settings.hardness,
      opacity:
        stroke.tool === 'brush' ? stroke.settings.brushOpacity : stroke.settings.eraserOpacity,
      color: stroke.settings.brushColor,
    })
    stroke.currentImageData = nextImageData
    renderRasterPreview(new Map([[stroke.layerId, nextImageData]]))
  }

  function finishPainting() {
    const stroke = paintingStroke
    stopInteraction()
    paintingStroke = null
    if (!stroke) return

    const content: ImageEditorRasterContent = {
      kind: 'raster',
      dataUrl: createRasterContentDataUrl(stroke.currentImageData),
      width: stroke.currentImageData.width,
      height: stroke.currentImageData.height,
    }
    rasterImageCache.set(content.dataUrl, cloneImageData(stroke.currentImageData))
    options.commitRasterStroke(stroke.layerId, stroke.beforeLayers, content)
    renderRasterPreview()
  }

  function stopPainting() {
    const stoppedDragSession = pointerDragSession.endDrag()
    if (!stoppedDragSession) finishPainting()
  }

  function createLayerBaseImageData(layer: ImageEditorLayer, source: ImageData) {
    if (layer.type === 'source') {
      return layer.content?.kind === 'raster'
        ? cloneImageData(rasterImageCache.get(layer.content.dataUrl) ?? source)
        : cloneImageData(source)
    }

    if (layer.type === 'paint') {
      return layer.content.kind === 'raster'
        ? cloneImageData(
            rasterImageCache.get(layer.content.dataUrl) ??
              createTransparentImageData(source.width, source.height),
          )
        : createTransparentImageData(source.width, source.height)
    }

    return createTransparentImageData(source.width, source.height)
  }

  watch(
    [() => options.sourceUrl.value, () => options.editorState.value?.mode ?? null],
    ([sourceUrl]) => {
      stopPainting()
      void loadSourceImageData(sourceUrl)
    },
    { immediate: true },
  )

  watch(
    () => options.layers.value,
    () => renderRasterPreview(),
  )

  onBeforeUnmount(() => {
    stopPainting()
    sourceRequestToken.value += 1
    canvasRefs.clear()
    rasterImageCache.clear()
    sourceCache.dispose()
  })

  return {
    rasterOverlayStyle,
    rasterCanvasWidth,
    rasterCanvasHeight,
    rasterTool: activeRasterTool,
    rasterToolEnabled,
    rasterBrushSize: computed(() => options.settings.value.size),
    hasRasterEdits,
    setRasterCanvasRef,
    onRasterCanvasPointerDown: onCanvasPointerDown,
  }
}

function projectPointerToRasterCanvas(event: PointerEvent, canvas: HTMLCanvasElement) {
  const rect = canvas.getBoundingClientRect()
  if (!rect.width || !rect.height) return null
  const localX = event.clientX - rect.left
  const localY = event.clientY - rect.top
  if (localX < 0 || localY < 0 || localX > rect.width || localY > rect.height) return null
  return {
    x: Math.min(canvas.width - 0.001, (localX / rect.width) * canvas.width),
    y: Math.min(canvas.height - 0.001, (localY / rect.height) * canvas.height),
  }
}

function createRasterContentDataUrl(imageData: ImageData) {
  const canvas = document.createElement('canvas')
  canvas.width = imageData.width
  canvas.height = imageData.height
  const context = canvas.getContext('2d')
  if (!context) throw new Error('当前浏览器不支持图像处理')
  context.putImageData(imageData, 0, 0)
  return canvas.toDataURL('image/png')
}
