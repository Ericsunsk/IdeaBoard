import { computed, ref } from 'vue'
import type { ImageRetouchStack } from '../../../board/domain'
import type {
  ImageRetouchKind,
  RetouchEffectLayer,
  RetouchEffectWholeParams,
  RetouchLayer,
  RetouchParamKey,
  RetouchParamValue,
} from './model.ts'
import {
  applyLayerCommand,
  createDefaultImageEffectLayer,
  createInitialLayerStack,
  getSelectedEffect,
  SOURCE_LAYER_ID,
  type RetouchLayerCommand,
} from './processing/layerStack.ts'
import { deserializeRetouchStack } from './processing/stackPersistence.ts'
import { getImageRetouchDefaultParams } from './processing/definitions.ts'

export function useRetouchSession() {
  const stack = ref(createInitialLayerStack())
  const pendingRetouch = ref<RetouchEffectLayer | null>(createDefaultImageEffectLayer('hsl'))

  const layers = computed<RetouchLayer[]>({
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
  const selectedEffectLayer = computed<RetouchEffectLayer | null>(
    () => getSelectedEffect(stack.value) ?? pendingRetouch.value,
  )
  function applyCommand(command: RetouchLayerCommand) {
    stack.value = applyLayerCommand(stack.value, command)
  }

  function resetRetouchSession() {
    stack.value = createInitialLayerStack()
    pendingRetouch.value = createDefaultImageEffectLayer('hsl')
  }

  function restoreRetouchStack(persisted: ImageRetouchStack | null | undefined) {
    const restored = deserializeRetouchStack(persisted)
    if (!restored || restored.layers.length === 0) {
      resetRetouchSession()
      return false
    }
    const base = createInitialLayerStack()
    stack.value = {
      layers: [...base.layers, ...restored.layers],
      selectedLayerId: restored.selectedLayerId || restored.layers[0]?.id || base.selectedLayerId,
    }
    pendingRetouch.value = null
    return true
  }

  function selectLayer(layerId: string) {
    pendingRetouch.value = null
    applyCommand({ type: 'select-layer', layerId })
  }

  function addLayer() {
    pendingRetouch.value = null
    applyCommand({ type: 'add-paint' })
  }

  function copyLayer() {
    pendingRetouch.value = null
    applyCommand({ type: 'copy-selected' })
  }

  function mergeLayer() {
    pendingRetouch.value = null
    applyCommand({ type: 'merge-selected-down' })
  }

  function selectEffectLayer(kind: ImageRetouchKind) {
    const existing = stack.value.layers.find(
      (layer): layer is RetouchEffectLayer => layer.type === 'effect' && layer.kind === kind,
    )
    if (existing) {
      pendingRetouch.value = null
      applyCommand({ type: 'select-layer', layerId: existing.id })
      return
    }

    pendingRetouch.value = createDefaultImageEffectLayer(kind)
    stack.value = { ...stack.value, selectedLayerId: SOURCE_LAYER_ID }
  }

  function setLayerVisible(layerId: string, visible: boolean) {
    applyCommand({ type: 'set-visible', layerId, visible })
  }

  function removeLayer(layerId: string) {
    pendingRetouch.value = null
    applyCommand({ type: 'remove', layerId })
  }

  function updateSelectedLayerParam(key: RetouchParamKey, value: RetouchParamValue) {
    if (pendingRetouch.value?.kind === 'hsl') {
      const params = { ...pendingRetouch.value.params, [key]: value }
      updatePendingRetouch({ ...pendingRetouch.value, params })
      return
    }
    applyCommand({ type: 'update-selected-param', key, value })
  }

  function updateSelectedRetouchParams(params: RetouchEffectWholeParams) {
    const pending = pendingRetouch.value
    if (pending && pending.kind !== 'hsl') {
      const next = updatePendingWholeParams(pending, params)
      if (next) updatePendingRetouch(next)
      return
    }
    applyCommand({ type: 'update-selected-retouch-params', params })
  }

  function updatePendingRetouch(next: RetouchEffectLayer) {
    const defaults = getImageRetouchDefaultParams(next.kind)
    if (JSON.stringify(next.params) === JSON.stringify(defaults)) {
      pendingRetouch.value = {
        ...next,
        params: structuredClone(defaults),
      } as RetouchEffectLayer
      return
    }

    const kind = next.kind
    pendingRetouch.value = null
    applyCommand({ type: 'ensure-retouch', kind })
    if (kind === 'hsl') {
      for (const [key, value] of Object.entries(next.params)) {
        applyCommand({
          type: 'update-selected-param',
          key: key as RetouchParamKey,
          value: value as RetouchParamValue,
        })
      }
      return
    }
    applyCommand({ type: 'update-selected-retouch-params', params: next.params })
  }

  function updatePendingWholeParams(
    pending: Exclude<RetouchEffectLayer, { kind: 'hsl' }>,
    params: RetouchEffectWholeParams,
  ): RetouchEffectLayer | null {
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
    retouchLayers: layers,
    selectedRetouchLayerId: selectedLayerId,
    selectedRetouchEffectLayer: selectedEffectLayer,
    resetRetouchSession,
    restoreRetouchStack,
    selectRetouchLayer: selectLayer,
    setRetouchLayerVisible: setLayerVisible,
    addRetouchLayer: addLayer,
    copyRetouchLayer: copyLayer,
    mergeRetouchLayer: mergeLayer,
    removeRetouchLayer: removeLayer,
    updateSelectedRetouchLayerParam: updateSelectedLayerParam,
    updateSelectedRetouchEffectParams: updateSelectedRetouchParams,
    selectImageEffectLayer: selectEffectLayer,
  }
}
