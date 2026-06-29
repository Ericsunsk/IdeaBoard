import { computed, ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorTool,
} from '../types'
import {
  applyLayerCommand,
  createInitialLayerStack,
  getSelectedAdjustment,
  type ImageEditorLayerCommand,
} from '../utils/imageEditorLayerStack'

export function useImageEditorSession() {
  const activeTool = ref<ImageEditorTool>('move')
  const stack = ref(createInitialLayerStack())

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

  function applyCommand(command: ImageEditorLayerCommand) {
    stack.value = applyLayerCommand(stack.value, command)
  }

  function resetImageEditorSession() {
    stack.value = createInitialLayerStack()
    activeTool.value = 'move'
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

  return {
    activeImageEditorTool: activeTool,
    imageEditorLayers: layers,
    selectedImageEditorLayerId: selectedLayerId,
    selectedImageEditorAdjustmentLayer: selectedAdjustmentLayer,
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
  }
}
