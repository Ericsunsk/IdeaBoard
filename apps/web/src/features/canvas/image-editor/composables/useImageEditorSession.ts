import { computed, ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorRasterContent,
  ImageEditorTool,
} from '../types'
import {
  applyLayerCommand,
  createInitialLayerStack,
  getSelectedAdjustment,
  type ImageEditorLayerCommand,
} from '../utils/imageEditorLayerStack'
import {
  clampImageEditorRasterHardness,
  clampImageEditorRasterOpacity,
  clampImageEditorRasterSize,
  createDefaultImageEditorRasterToolSettings,
  IMAGE_EDITOR_RASTER_HISTORY_LIMIT,
  normalizeImageEditorRasterColor,
  type ImageEditorRasterToolSettings,
} from '../utils/imageEditorRasterTools'

export function useImageEditorSession() {
  const activeTool = ref<ImageEditorTool>('move')
  const stack = ref(createInitialLayerStack())
  const rasterToolSettings = ref(createDefaultImageEditorRasterToolSettings())
  const rasterUndoStack = ref<ImageEditorLayer[][]>([])
  const rasterRedoStack = ref<ImageEditorLayer[][]>([])

  const layers = computed<ImageEditorLayer[]>({
    get: () => stack.value.layers,
    set: (nextLayers) => {
      stack.value = { ...stack.value, layers: nextLayers }
    },
  })
  const selectedLayerId = computed({
    get: () => stack.value.selectedLayerId,
    set: (nextSelectedLayerId: string) => {
      stack.value = { ...stack.value, selectedLayerId: nextSelectedLayerId }
    },
  })
  const selectedAdjustmentLayer = computed<ImageEditorAdjustmentLayer | null>(() =>
    getSelectedAdjustment(stack.value),
  )
  const canUndoRasterStroke = computed(() => rasterUndoStack.value.length > 0)
  const canRedoRasterStroke = computed(() => rasterRedoStack.value.length > 0)

  function applyCommand(command: ImageEditorLayerCommand) {
    stack.value = applyLayerCommand(stack.value, command)
  }

  function resetImageEditorSession() {
    stack.value = createInitialLayerStack()
    activeTool.value = 'move'
    rasterToolSettings.value = createDefaultImageEditorRasterToolSettings()
    clearRasterHistory()
  }

  function selectLayer(layerId: string) {
    applyCommand({ type: 'select-layer', layerId })
  }

  function addLayer() {
    applyCommand({ type: 'add-paint' })
  }

  function copyLayer() {
    applyCommand({ type: 'copy-selected' })
  }

  function mergeLayer() {
    applyCommand({ type: 'merge-selected-down' })
  }

  function selectTool(tool: ImageEditorTool) {
    activeTool.value = tool
  }

  function selectAdjustmentLayer(kind: ImageAdjustmentKind) {
    applyCommand({ type: 'ensure-adjustment', kind })
  }

  function setLayerVisible(layerId: string, visible: boolean) {
    applyCommand({ type: 'set-visible', layerId, visible })
  }

  function removeLayer(layerId: string) {
    applyCommand({ type: 'remove', layerId })
  }

  function updateSelectedLayerParam(key: ImageEditorParamKey, value: ImageEditorParamValue) {
    applyCommand({ type: 'update-selected-param', key, value })
  }

  function updateSelectedAdjustmentParams(params: ImageEditorAdjustmentWholeParams) {
    applyCommand({ type: 'update-selected-adjustment-params', params })
  }

  function captureRasterHistorySnapshot(): ImageEditorLayer[] {
    return cloneImageEditorLayers(stack.value.layers)
  }

  function commitRasterStroke(
    layerId: string,
    beforeLayers: readonly ImageEditorLayer[],
    content: ImageEditorRasterContent,
  ) {
    const before = cloneImageEditorLayers(beforeLayers)
    applyCommand({ type: 'update-raster-content', layerId, content })
    const after = cloneImageEditorLayers(stack.value.layers)
    if (createRasterHistorySignature(before) === createRasterHistorySignature(after)) return

    rasterUndoStack.value = [...rasterUndoStack.value, before].slice(
      -IMAGE_EDITOR_RASTER_HISTORY_LIMIT,
    )
    rasterRedoStack.value = []
  }

  function undoRasterStroke() {
    const undoStack = rasterUndoStack.value.slice()
    const previousLayers = undoStack.pop()
    if (!previousLayers) return
    rasterRedoStack.value = [
      ...rasterRedoStack.value,
      cloneImageEditorLayers(stack.value.layers),
    ].slice(-IMAGE_EDITOR_RASTER_HISTORY_LIMIT)
    rasterUndoStack.value = undoStack
    stack.value = { ...stack.value, layers: cloneImageEditorLayers(previousLayers) }
  }

  function redoRasterStroke() {
    const redoStack = rasterRedoStack.value.slice()
    const nextLayers = redoStack.pop()
    if (!nextLayers) return
    rasterUndoStack.value = [
      ...rasterUndoStack.value,
      cloneImageEditorLayers(stack.value.layers),
    ].slice(-IMAGE_EDITOR_RASTER_HISTORY_LIMIT)
    rasterRedoStack.value = redoStack
    stack.value = { ...stack.value, layers: cloneImageEditorLayers(nextLayers) }
  }

  function clearRasterHistory() {
    rasterUndoStack.value = []
    rasterRedoStack.value = []
  }

  function clearRasterEdits() {
    stack.value = {
      ...stack.value,
      layers: clearRasterEditsFromLayers(stack.value.layers),
    }
    clearRasterHistory()
  }

  function updateRasterToolSettings(patch: Partial<ImageEditorRasterToolSettings>) {
    rasterToolSettings.value = {
      ...rasterToolSettings.value,
      ...(patch.size == null ? {} : { size: clampImageEditorRasterSize(patch.size) }),
      ...(patch.hardness == null
        ? {}
        : { hardness: clampImageEditorRasterHardness(patch.hardness) }),
      ...(patch.brushOpacity == null
        ? {}
        : { brushOpacity: clampImageEditorRasterOpacity(patch.brushOpacity) }),
      ...(patch.eraserOpacity == null
        ? {}
        : { eraserOpacity: clampImageEditorRasterOpacity(patch.eraserOpacity) }),
      ...(patch.brushColor == null
        ? {}
        : { brushColor: normalizeImageEditorRasterColor(patch.brushColor) }),
    }
  }

  return {
    activeImageEditorTool: activeTool,
    imageEditorLayers: layers,
    selectedImageEditorLayerId: selectedLayerId,
    selectedImageEditorAdjustmentLayer: selectedAdjustmentLayer,
    imageEditorRasterToolSettings: rasterToolSettings,
    canUndoImageEditorRasterStroke: canUndoRasterStroke,
    canRedoImageEditorRasterStroke: canRedoRasterStroke,
    resetImageEditorSession,
    selectImageEditorTool: selectTool,
    selectImageEditorLayer: selectLayer,
    setImageEditorLayerVisible: setLayerVisible,
    addImageEditorLayer: addLayer,
    copyImageEditorLayer: copyLayer,
    mergeImageEditorLayer: mergeLayer,
    removeImageEditorLayer: removeLayer,
    updateSelectedImageEditorLayerParam: updateSelectedLayerParam,
    updateSelectedImageEditorAdjustmentParams: updateSelectedAdjustmentParams,
    selectImageAdjustmentLayer: selectAdjustmentLayer,
    captureImageEditorRasterHistorySnapshot: captureRasterHistorySnapshot,
    commitImageEditorRasterStroke: commitRasterStroke,
    undoImageEditorRasterStroke: undoRasterStroke,
    redoImageEditorRasterStroke: redoRasterStroke,
    clearImageEditorRasterHistory: clearRasterHistory,
    clearImageEditorRasterEdits: clearRasterEdits,
    updateImageEditorRasterToolSettings: updateRasterToolSettings,
  }
}

function cloneImageEditorLayers(layers: readonly ImageEditorLayer[]): ImageEditorLayer[] {
  return structuredClone(layers) as ImageEditorLayer[]
}

function createRasterHistorySignature(layers: readonly ImageEditorLayer[]): string {
  return JSON.stringify(
    layers.map((layer) => {
      if (layer.type === 'source') {
        return [layer.id, layer.type, layer.content ?? null]
      }
      if (layer.type === 'paint') {
        return [layer.id, layer.type, layer.visible, layer.content]
      }
      if (layer.type === 'merged') {
        return [layer.id, layer.type, layer.visible, createRasterHistorySignature(layer.children)]
      }
      return [layer.id, layer.type, layer.visible]
    }),
  )
}

function clearRasterEditsFromLayers(layers: readonly ImageEditorLayer[]): ImageEditorLayer[] {
  return layers.map((layer) => {
    if (layer.type === 'source') {
      const { content: _content, ...rest } = layer
      return rest
    }
    if (layer.type === 'paint') {
      return { ...layer, content: { kind: 'empty' } }
    }
    if (layer.type === 'merged') {
      return {
        ...layer,
        children: clearRasterEditsFromLayers(layer.children) as typeof layer.children,
      }
    }
    return layer
  })
}
