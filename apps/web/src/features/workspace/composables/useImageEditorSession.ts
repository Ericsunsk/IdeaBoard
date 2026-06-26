import { computed, ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorLayer,
  ImageEditorLayerByKind,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorTool,
} from '../types/imageEditor'

const SOURCE_LAYER_ID = 'source'

const DEFAULT_ADJUSTMENT_LAYER_BY_KIND: {
  [TKind in ImageAdjustmentKind]: ImageEditorLayerByKind<TKind>
} = {
  hsl: {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: '色相/饱和度 1',
    visible: true,
    params: {
      hue: 0,
      saturation: 0,
      brightness: 0,
      contrast: 0,
    },
  },
  threshold: {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值 1',
    visible: true,
    params: {
      threshold: 128,
      invert: false,
    },
  },
  curves: {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params: {
      input: 50,
      output: 50,
    },
  },
  levels: {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: '色阶 1',
    visible: true,
    params: {
      shadows: 0,
      midtones: 50,
      highlights: 100,
    },
  },
  invert: {
    id: 'adjustment:invert',
    type: 'adjustment',
    kind: 'invert',
    name: '反相 1',
    visible: true,
    params: {
      enabled: true,
    },
  },
}

function cloneAdjustmentLayer<TKind extends ImageAdjustmentKind>(
  kind: TKind,
): ImageEditorLayerByKind<TKind> {
  const layer = DEFAULT_ADJUSTMENT_LAYER_BY_KIND[kind]
  return {
    ...layer,
    params: { ...layer.params },
  } as ImageEditorLayerByKind<TKind>
}

function createInitialLayers(): ImageEditorLayer[] {
  return [
    cloneAdjustmentLayer('hsl'),
    {
      id: SOURCE_LAYER_ID,
      type: 'source',
      name: '原图',
      visible: true,
    },
  ]
}

function isAdjustmentLayer(layer: ImageEditorLayer): layer is ImageEditorAdjustmentLayer {
  return layer.type === 'adjustment'
}

function updateAdjustmentLayerParam(
  layer: ImageEditorAdjustmentLayer,
  key: ImageEditorParamKey,
  value: ImageEditorParamValue,
): ImageEditorAdjustmentLayer {
  switch (layer.kind) {
    case 'hsl':
      if (
        typeof value === 'number' &&
        (key === 'hue' ||
          key === 'saturation' ||
          key === 'brightness' ||
          key === 'contrast')
      ) {
        return { ...layer, params: { ...layer.params, [key]: value } }
      }
      return layer
    case 'threshold':
      if (key === 'threshold' && typeof value === 'number') {
        return { ...layer, params: { ...layer.params, threshold: value } }
      }
      if (key === 'invert' && typeof value === 'boolean') {
        return { ...layer, params: { ...layer.params, invert: value } }
      }
      return layer
    case 'curves':
      if (typeof value === 'number' && (key === 'input' || key === 'output')) {
        return { ...layer, params: { ...layer.params, [key]: value } }
      }
      return layer
    case 'levels':
      if (
        typeof value === 'number' &&
        (key === 'shadows' || key === 'midtones' || key === 'highlights')
      ) {
        return { ...layer, params: { ...layer.params, [key]: value } }
      }
      return layer
    case 'invert':
      if (key === 'enabled' && typeof value === 'boolean') {
        return { ...layer, params: { ...layer.params, enabled: value } }
      }
      return layer
  }
}

export function useImageEditorSession() {
  const activeTool = ref<ImageEditorTool>('move')
  const layers = ref<ImageEditorLayer[]>(createInitialLayers())
  const selectedLayerId = ref('adjustment:hsl')

  const selectedLayer = computed<ImageEditorLayer | null>(
    () => layers.value.find((layer) => layer.id === selectedLayerId.value) ?? null,
  )

  const selectedAdjustmentLayer = computed<ImageEditorAdjustmentLayer | null>(() => {
    const layer = selectedLayer.value
    return layer && isAdjustmentLayer(layer) ? layer : null
  })

  function resetImageEditorSession() {
    layers.value = createInitialLayers()
    activeTool.value = 'move'
    selectedLayerId.value = 'adjustment:hsl'
  }

  function selectLayer(layerId: string) {
    const layer = layers.value.find((item) => item.id === layerId)
    if (!layer) return
    selectedLayerId.value = layer.id
  }

  function ensureAdjustmentLayer(kind: ImageAdjustmentKind) {
    const existing = layers.value.find(
      (layer): layer is ImageEditorAdjustmentLayer =>
        layer.type === 'adjustment' && layer.kind === kind,
    )
    if (existing) return existing

    const sourceIndex = layers.value.findIndex((layer) => layer.type === 'source')
    const nextLayer = cloneAdjustmentLayer(kind)
    const nextLayers = layers.value.slice()
    nextLayers.splice(sourceIndex >= 0 ? sourceIndex : nextLayers.length, 0, nextLayer)
    layers.value = nextLayers
    return nextLayer
  }

  function selectTool(tool: ImageEditorTool) {
    activeTool.value = tool
  }

  function selectAdjustmentLayer(kind: ImageAdjustmentKind) {
    const layer = ensureAdjustmentLayer(kind)
    selectedLayerId.value = layer.id
  }

  function setLayerVisible(layerId: string, visible: boolean) {
    layers.value = layers.value.map((layer) => {
      if (layer.id !== layerId || layer.type === 'source') return layer
      return { ...layer, visible }
    })
  }

  function removeLayer(layerId: string) {
    const layer = layers.value.find((item) => item.id === layerId)
    if (!layer || layer.type === 'source') return
    const nextLayers = layers.value.filter((item) => item.id !== layerId)
    layers.value = nextLayers
    if (selectedLayerId.value !== layerId) return
    const nextSelection =
      nextLayers.find((item) => item.type === 'adjustment') ??
      nextLayers.find((item) => item.type === 'source')
    selectedLayerId.value = nextSelection?.id ?? SOURCE_LAYER_ID
  }

  function updateSelectedLayerParam(key: ImageEditorParamKey, value: ImageEditorParamValue) {
    const target = selectedAdjustmentLayer.value
    if (!target) return
    layers.value = layers.value.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'adjustment') return layer
      return updateAdjustmentLayerParam(layer, key, value)
    })
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
    removeImageEditorLayer: removeLayer,
    updateSelectedImageEditorLayerParam: updateSelectedLayerParam,
    selectImageAdjustmentLayer: selectAdjustmentLayer,
  }
}
