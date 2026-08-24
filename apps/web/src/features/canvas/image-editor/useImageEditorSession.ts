import { computed, ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
} from './types'
import {
  applyLayerCommand,
  createDefaultImageAdjustmentLayer,
  createInitialLayerStack,
  getSelectedAdjustment,
  SOURCE_LAYER_ID,
  type ImageEditorLayerCommand,
} from './utils/imageEditorLayerStack'
import { getImageAdjustmentDefaultParams } from './utils/imageAdjustmentDefinitions'

export function useImageEditorSession() {
  const stack = ref(createInitialLayerStack())
  const pendingAdjustment = ref<ImageEditorAdjustmentLayer | null>(
    createDefaultImageAdjustmentLayer('hsl'),
  )

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
  const selectedAdjustmentLayer = computed<ImageEditorAdjustmentLayer | null>(
    () => getSelectedAdjustment(stack.value) ?? pendingAdjustment.value,
  )
  function applyCommand(command: ImageEditorLayerCommand) {
    stack.value = applyLayerCommand(stack.value, command)
  }

  function resetImageEditorSession() {
    stack.value = createInitialLayerStack()
    pendingAdjustment.value = createDefaultImageAdjustmentLayer('hsl')
  }

  function selectLayer(layerId: string) {
    pendingAdjustment.value = null
    applyCommand({ type: 'select-layer', layerId })
  }

  function addLayer() {
    pendingAdjustment.value = null
    applyCommand({ type: 'add-paint' })
  }

  function copyLayer() {
    pendingAdjustment.value = null
    applyCommand({ type: 'copy-selected' })
  }

  function mergeLayer() {
    pendingAdjustment.value = null
    applyCommand({ type: 'merge-selected-down' })
  }

  function selectAdjustmentLayer(kind: ImageAdjustmentKind) {
    const existing = stack.value.layers.find(
      (layer): layer is ImageEditorAdjustmentLayer =>
        layer.type === 'adjustment' && layer.kind === kind,
    )
    if (existing) {
      pendingAdjustment.value = null
      applyCommand({ type: 'select-layer', layerId: existing.id })
      return
    }

    pendingAdjustment.value = createDefaultImageAdjustmentLayer(kind)
    stack.value = { ...stack.value, selectedLayerId: SOURCE_LAYER_ID }
  }

  function setLayerVisible(layerId: string, visible: boolean) {
    applyCommand({ type: 'set-visible', layerId, visible })
  }

  function removeLayer(layerId: string) {
    pendingAdjustment.value = null
    applyCommand({ type: 'remove', layerId })
  }

  function updateSelectedLayerParam(key: ImageEditorParamKey, value: ImageEditorParamValue) {
    if (pendingAdjustment.value?.kind === 'hsl') {
      const params = { ...pendingAdjustment.value.params, [key]: value }
      updatePendingAdjustment({ ...pendingAdjustment.value, params })
      return
    }
    applyCommand({ type: 'update-selected-param', key, value })
  }

  function updateSelectedAdjustmentParams(params: ImageEditorAdjustmentWholeParams) {
    const pending = pendingAdjustment.value
    if (pending && pending.kind !== 'hsl') {
      const next = updatePendingWholeParams(pending, params)
      if (next) updatePendingAdjustment(next)
      return
    }
    applyCommand({ type: 'update-selected-adjustment-params', params })
  }

  function updatePendingAdjustment(next: ImageEditorAdjustmentLayer) {
    const defaults = getImageAdjustmentDefaultParams(next.kind)
    if (JSON.stringify(next.params) === JSON.stringify(defaults)) {
      pendingAdjustment.value = {
        ...next,
        params: structuredClone(defaults),
      } as ImageEditorAdjustmentLayer
      return
    }

    const kind = next.kind
    pendingAdjustment.value = null
    applyCommand({ type: 'ensure-adjustment', kind })
    if (kind === 'hsl') {
      for (const [key, value] of Object.entries(next.params)) {
        applyCommand({
          type: 'update-selected-param',
          key: key as ImageEditorParamKey,
          value: value as ImageEditorParamValue,
        })
      }
      return
    }
    applyCommand({ type: 'update-selected-adjustment-params', params: next.params })
  }

  function updatePendingWholeParams(
    pending: Exclude<ImageEditorAdjustmentLayer, { kind: 'hsl' }>,
    params: ImageEditorAdjustmentWholeParams,
  ): ImageEditorAdjustmentLayer | null {
    if (pending.kind === 'threshold' && 'threshold' in params && 'invert' in params) {
      return { ...pending, params }
    }
    if (pending.kind === 'curves' && 'channels' in params) {
      return { ...pending, params }
    }
    if (pending.kind === 'levels' && 'shadows' in params) {
      return { ...pending, params }
    }
    if (pending.kind === 'sharpen' && 'amount' in params) {
      return { ...pending, params }
    }
    return null
  }

  return {
    imageEditorLayers: layers,
    selectedImageEditorLayerId: selectedLayerId,
    selectedImageEditorAdjustmentLayer: selectedAdjustmentLayer,
    resetImageEditorSession,
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
