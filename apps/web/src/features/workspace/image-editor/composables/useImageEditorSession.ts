import { computed, ref } from 'vue'
import type {
  ImageAdjustmentKind,
  ImageEditorAdjustmentLayer,
  ImageEditorAdjustmentWholeParams,
  ImageEditorCurvesParams,
  ImageEditorEditableLayer,
  ImageEditorHslParams,
  ImageEditorInvertParams,
  ImageEditorLayer,
  ImageEditorLayerByKind,
  ImageEditorLevelsParams,
  ImageEditorMergedLayer,
  ImageEditorPaintLayer,
  ImageEditorParamKey,
  ImageEditorParamValue,
  ImageEditorThresholdParams,
  ImageEditorTool,
} from '../types'
import { getImageAdjustmentDefaultParams } from '../utils/imageAdjustmentDefinitions'
import { normalizeCurvesParams } from '../utils/imageEditorCurves'
import { clampLevelsHandles } from '../utils/imageEditorLevels'
import { normalizeThresholdAdjustment } from '../utils/imageEditorThreshold'

const SOURCE_LAYER_ID = 'source'
let imageEditorLayerIdSeed = 0

const DEFAULT_ADJUSTMENT_LAYER_BY_KIND: {
  [TKind in ImageAdjustmentKind]: ImageEditorLayerByKind<TKind>
} = {
  hsl: {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: '色相/饱和度 1',
    visible: true,
    params: getImageAdjustmentDefaultParams('hsl'),
  },
  threshold: {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值 1',
    visible: true,
    params: getImageAdjustmentDefaultParams('threshold'),
  },
  curves: {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params: getImageAdjustmentDefaultParams('curves'),
  },
  levels: {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: '色阶 1',
    visible: true,
    params: getImageAdjustmentDefaultParams('levels'),
  },
  invert: {
    id: 'adjustment:invert',
    type: 'adjustment',
    kind: 'invert',
    name: '反相 1',
    visible: true,
    params: getImageAdjustmentDefaultParams('invert'),
  },
}

function cloneAdjustmentLayer<TKind extends ImageAdjustmentKind>(
  kind: TKind,
): ImageEditorLayerByKind<TKind> {
  const layer = DEFAULT_ADJUSTMENT_LAYER_BY_KIND[kind]
  return {
    ...layer,
    params: cloneAdjustmentParams(layer.params),
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

function isEditableLayer(layer: ImageEditorLayer): layer is ImageEditorEditableLayer {
  return layer.type !== 'source'
}

function createLayerId(scope: string) {
  imageEditorLayerIdSeed += 1
  return `${scope}:${imageEditorLayerIdSeed}`
}

function collectLayerNames(layers: readonly ImageEditorLayer[]) {
  const names = new Set<string>()
  for (const layer of layers) collectLayerName(layer, names)
  return names
}

function collectLayerName(layer: ImageEditorLayer | ImageEditorEditableLayer, names: Set<string>) {
  names.add(layer.name)
  if (layer.type === 'merged') {
    layer.children.forEach((child) => collectLayerName(child, names))
  }
}

function createUniqueLayerName(layers: readonly ImageEditorLayer[], base: string) {
  const names = collectLayerNames(layers)
  if (!names.has(base)) return base
  let index = 2
  while (names.has(`${base} ${index}`)) index += 1
  return `${base} ${index}`
}

function createNumberedLayerName(layers: readonly ImageEditorLayer[], prefix: string) {
  const names = collectLayerNames(layers)
  let index = 1
  while (names.has(`${prefix} ${index}`)) index += 1
  return `${prefix} ${index}`
}

function createCopyLayerName(layers: readonly ImageEditorLayer[], name: string) {
  return createUniqueLayerName(layers, `${name} 副本`)
}

function createPaintLayer(layers: readonly ImageEditorLayer[]): ImageEditorPaintLayer {
  return {
    id: createLayerId('paint'),
    type: 'paint',
    name: createNumberedLayerName(layers, '图层'),
    visible: true,
    content: { kind: 'empty' },
  }
}

function cloneEditableLayer(
  layer: ImageEditorEditableLayer,
  name = layer.name,
): ImageEditorEditableLayer {
  if (layer.type === 'paint') {
    return {
      ...layer,
      id: createLayerId('paint'),
      name,
      content: { ...layer.content },
    }
  }
  if (layer.type === 'merged') {
    return {
      ...layer,
      id: createLayerId('merged'),
      name,
      children: layer.children.map((child) => cloneEditableLayer(child)),
    }
  }
  return {
    ...layer,
    id: createLayerId(`adjustment:${layer.kind}`),
    name,
    params: cloneAdjustmentParams(layer.params),
  } as ImageEditorAdjustmentLayer
}

function createMergedLayer(
  layers: readonly ImageEditorLayer[],
  children: [ImageEditorEditableLayer, ImageEditorEditableLayer],
): ImageEditorMergedLayer {
  return {
    id: createLayerId('merged'),
    type: 'merged',
    name: createNumberedLayerName(layers, '合并图层'),
    visible: children.some((layer) => layer.visible),
    children: children.map((layer) => cloneEditableLayer(layer)),
  }
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
        (key === 'hue' || key === 'saturation' || key === 'brightness' || key === 'contrast')
      ) {
        return { ...layer, params: { ...layer.params, [key]: value } }
      }
      return layer
    case 'threshold':
    case 'curves':
    case 'levels':
      return layer
    case 'invert':
      if (key === 'enabled' && typeof value === 'boolean') {
        return { ...layer, params: { ...layer.params, enabled: value } }
      }
      return layer
  }
}

function updateAdjustmentLayerParams(
  layer: ImageEditorAdjustmentLayer,
  params: ImageEditorAdjustmentWholeParams,
): ImageEditorAdjustmentLayer {
  switch (layer.kind) {
    case 'hsl':
    case 'invert':
      return layer
    case 'threshold':
      if (isThresholdParams(params)) {
        return { ...layer, params: cloneThresholdParams(params) }
      }
      return layer
    case 'curves':
      if (isCurvesParams(params)) {
        return { ...layer, params: cloneCurvesParams(params) }
      }
      return layer
    case 'levels':
      if (isLevelsParams(params)) {
        return { ...layer, params: cloneLevelsParams(params) }
      }
      return layer
  }
}

function cloneAdjustmentParams(
  params:
    | ImageEditorHslParams
    | ImageEditorThresholdParams
    | ImageEditorCurvesParams
    | ImageEditorLevelsParams
    | ImageEditorInvertParams,
) {
  if ('channels' in params) return cloneCurvesParams(params)
  if ('threshold' in params) return cloneThresholdParams(params)
  if ('shadows' in params) return cloneLevelsParams(params)
  return { ...params }
}

function cloneThresholdParams(params: ImageEditorThresholdParams): ImageEditorThresholdParams {
  return normalizeThresholdAdjustment(params)
}

function cloneCurvesParams(params: ImageEditorCurvesParams): ImageEditorCurvesParams {
  const normalized = normalizeCurvesParams(params)
  const channels: ImageEditorCurvesParams['channels'] = {}
  for (const [channel, points] of Object.entries(normalized.channels)) {
    if (!points) continue
    const denormalizedPoints = points
      .filter((point) => !(point.endpoint && point.input === point.output))
      .map((point) => ({
        id: point.id,
        input: point.input,
        output: point.output,
      }))
    if (denormalizedPoints.length > 0) {
      channels[channel as keyof ImageEditorCurvesParams['channels']] = denormalizedPoints
    }
  }
  return { channels }
}

function cloneLevelsParams(params: ImageEditorLevelsParams): ImageEditorLevelsParams {
  return clampLevelsHandles(params)
}

function isThresholdParams(
  params: ImageEditorAdjustmentWholeParams,
): params is ImageEditorThresholdParams {
  return 'threshold' in params
}

function isCurvesParams(
  params: ImageEditorAdjustmentWholeParams,
): params is ImageEditorCurvesParams {
  return 'channels' in params
}

function isLevelsParams(
  params: ImageEditorAdjustmentWholeParams,
): params is ImageEditorLevelsParams {
  return 'shadows' in params
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

  function resolveLayerInsertionIndex() {
    const selectedIndex = layers.value.findIndex((layer) => layer.id === selectedLayerId.value)
    if (selectedIndex >= 0 && layers.value[selectedIndex]?.type !== 'source') return selectedIndex
    const sourceIndex = layers.value.findIndex((layer) => layer.type === 'source')
    return sourceIndex >= 0 ? sourceIndex : layers.value.length
  }

  function addLayer() {
    const layer = createPaintLayer(layers.value)
    const nextLayers = layers.value.slice()
    nextLayers.splice(resolveLayerInsertionIndex(), 0, layer)
    layers.value = nextLayers
    selectedLayerId.value = layer.id
  }

  function copyLayer() {
    const selectedIndex = layers.value.findIndex((layer) => layer.id === selectedLayerId.value)
    const layer = layers.value[selectedIndex]
    if (selectedIndex < 0 || !layer || !isEditableLayer(layer)) return

    const copy = cloneEditableLayer(layer, createCopyLayerName(layers.value, layer.name))
    const nextLayers = layers.value.slice()
    nextLayers.splice(selectedIndex, 0, copy)
    layers.value = nextLayers
    selectedLayerId.value = copy.id
  }

  function mergeLayer() {
    const selectedIndex = layers.value.findIndex((layer) => layer.id === selectedLayerId.value)
    const layer = layers.value[selectedIndex]
    const target = layers.value[selectedIndex + 1]
    if (
      selectedIndex < 0 ||
      !layer ||
      !target ||
      !isEditableLayer(layer) ||
      !isEditableLayer(target)
    ) {
      return
    }

    const mergedLayer = createMergedLayer(layers.value, [layer, target])
    const nextLayers = layers.value.slice()
    nextLayers.splice(selectedIndex, 2, mergedLayer)
    layers.value = nextLayers
    selectedLayerId.value = mergedLayer.id
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
    const layerIndex = layers.value.findIndex((item) => item.id === layerId)
    const layer = layers.value[layerIndex]
    if (!layer || layer.type === 'source') return
    const nextLayers = layers.value.filter((item) => item.id !== layerId)
    layers.value = nextLayers
    if (selectedLayerId.value !== layerId) return
    const nextSelection =
      nextLayers
        .slice(Math.max(0, Math.min(layerIndex, nextLayers.length - 1)))
        .find((item) => item.type !== 'source') ??
      nextLayers
        .slice(0, Math.max(0, layerIndex))
        .reverse()
        .find((item) => item.type !== 'source') ??
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

  function updateSelectedAdjustmentParams(params: ImageEditorAdjustmentWholeParams) {
    const target = selectedAdjustmentLayer.value
    if (!target) return
    layers.value = layers.value.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'adjustment') return layer
      return updateAdjustmentLayerParams(layer, params)
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
    addImageEditorLayer: addLayer,
    copyImageEditorLayer: copyLayer,
    mergeImageEditorLayer: mergeLayer,
    removeImageEditorLayer: removeLayer,
    updateSelectedImageEditorLayerParam: updateSelectedLayerParam,
    updateSelectedImageEditorAdjustmentParams: updateSelectedAdjustmentParams,
    selectImageAdjustmentLayer: selectAdjustmentLayer,
  }
}
