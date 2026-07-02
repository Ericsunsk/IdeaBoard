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
} from '../types'
import { getImageAdjustmentDefaultParams } from './imageAdjustmentDefinitions'
import { normalizeCurvesParams } from './imageEditorCurves'
import { clampLevelsHandles } from './imageEditorLevels'
import { normalizeThresholdAdjustment } from './imageEditorThreshold'

export const SOURCE_LAYER_ID = 'source'
export const INITIAL_SELECTED_IMAGE_EDITOR_LAYER_ID = 'adjustment:hsl'

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

export type ImageEditorLayerStack = {
  layers: ImageEditorLayer[]
  selectedLayerId: string
}

export type ImageEditorLayerCommand =
  | { type: 'select-layer'; layerId: string }
  | { type: 'ensure-adjustment'; kind: ImageAdjustmentKind }
  | { type: 'add-paint' }
  | { type: 'copy-selected' }
  | { type: 'merge-selected-down' }
  | { type: 'set-visible'; layerId: string; visible: boolean }
  | { type: 'remove'; layerId: string }
  | { type: 'update-selected-param'; key: ImageEditorParamKey; value: ImageEditorParamValue }
  | { type: 'update-selected-adjustment-params'; params: ImageEditorAdjustmentWholeParams }

export function createInitialLayerStack(): ImageEditorLayerStack {
  return {
    layers: [
      cloneAdjustmentLayer('hsl'),
      {
        id: SOURCE_LAYER_ID,
        type: 'source',
        name: '原图',
        visible: true,
      },
    ],
    selectedLayerId: INITIAL_SELECTED_IMAGE_EDITOR_LAYER_ID,
  }
}

export function applyLayerCommand(
  stack: ImageEditorLayerStack,
  command: ImageEditorLayerCommand,
): ImageEditorLayerStack {
  switch (command.type) {
    case 'select-layer':
      return selectLayer(stack, command.layerId)
    case 'ensure-adjustment':
      return ensureAdjustmentLayer(stack, command.kind)
    case 'add-paint':
      return addPaintLayer(stack)
    case 'copy-selected':
      return copySelectedLayer(stack)
    case 'merge-selected-down':
      return mergeSelectedLayerDown(stack)
    case 'set-visible':
      return setLayerVisible(stack, command.layerId, command.visible)
    case 'remove':
      return removeLayer(stack, command.layerId)
    case 'update-selected-param':
      return updateSelectedLayerParam(stack, command.key, command.value)
    case 'update-selected-adjustment-params':
      return updateSelectedAdjustmentParams(stack, command.params)
  }
}

export function getSelectedAdjustment(
  stack: ImageEditorLayerStack,
): ImageEditorAdjustmentLayer | null {
  const layer = stack.layers.find((item) => item.id === stack.selectedLayerId)
  return layer && isAdjustmentLayer(layer) ? layer : null
}

export function selectAfterRemove(
  layers: readonly ImageEditorLayer[],
  removedLayerIndex: number,
): string {
  const nextSelection =
    layers
      .slice(Math.max(0, Math.min(removedLayerIndex, layers.length - 1)))
      .find((item) => item.type !== 'source') ??
    layers
      .slice(0, Math.max(0, removedLayerIndex))
      .reverse()
      .find((item) => item.type !== 'source') ??
    layers.find((item) => item.type === 'source')
  return nextSelection?.id ?? SOURCE_LAYER_ID
}

function selectLayer(stack: ImageEditorLayerStack, layerId: string): ImageEditorLayerStack {
  const layer = stack.layers.find((item) => item.id === layerId)
  if (!layer) return stack
  return { ...stack, selectedLayerId: layer.id }
}

function ensureAdjustmentLayer(
  stack: ImageEditorLayerStack,
  kind: ImageAdjustmentKind,
): ImageEditorLayerStack {
  const existing = stack.layers.find(
    (layer): layer is ImageEditorAdjustmentLayer =>
      layer.type === 'adjustment' && layer.kind === kind,
  )
  if (existing) return { ...stack, selectedLayerId: existing.id }

  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  const nextLayer = cloneAdjustmentLayer(kind)
  const layers = stack.layers.slice()
  layers.splice(sourceIndex >= 0 ? sourceIndex : layers.length, 0, nextLayer)
  return { layers, selectedLayerId: nextLayer.id }
}

function addPaintLayer(stack: ImageEditorLayerStack): ImageEditorLayerStack {
  const layer = createPaintLayer(stack.layers)
  const layers = stack.layers.slice()
  layers.splice(resolveLayerInsertionIndex(stack), 0, layer)
  return { layers, selectedLayerId: layer.id }
}

function copySelectedLayer(stack: ImageEditorLayerStack): ImageEditorLayerStack {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  const layer = stack.layers[selectedIndex]
  if (selectedIndex < 0 || !layer || !isEditableLayer(layer)) return stack

  const copy = cloneEditableLayer(layer, createCopyLayerName(stack.layers, layer.name))
  const layers = stack.layers.slice()
  layers.splice(selectedIndex, 0, copy)
  return { layers, selectedLayerId: copy.id }
}

function mergeSelectedLayerDown(stack: ImageEditorLayerStack): ImageEditorLayerStack {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  const layer = stack.layers[selectedIndex]
  const target = stack.layers[selectedIndex + 1]
  if (
    selectedIndex < 0 ||
    !layer ||
    !target ||
    !isEditableLayer(layer) ||
    !isEditableLayer(target)
  ) {
    return stack
  }

  const mergedLayer = createMergedLayer(stack.layers, [layer, target])
  const layers = stack.layers.slice()
  layers.splice(selectedIndex, 2, mergedLayer)
  return { layers, selectedLayerId: mergedLayer.id }
}

function setLayerVisible(
  stack: ImageEditorLayerStack,
  layerId: string,
  visible: boolean,
): ImageEditorLayerStack {
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== layerId || layer.type === 'source') return layer
      return { ...layer, visible }
    }),
  }
}

function removeLayer(stack: ImageEditorLayerStack, layerId: string): ImageEditorLayerStack {
  const layerIndex = stack.layers.findIndex((item) => item.id === layerId)
  const layer = stack.layers[layerIndex]
  if (!layer || layer.type === 'source') return stack

  const layers = stack.layers.filter((item) => item.id !== layerId)
  if (stack.selectedLayerId !== layerId) return { ...stack, layers }
  return {
    layers,
    selectedLayerId: selectAfterRemove(layers, layerIndex),
  }
}

function updateSelectedLayerParam(
  stack: ImageEditorLayerStack,
  key: ImageEditorParamKey,
  value: ImageEditorParamValue,
): ImageEditorLayerStack {
  const target = getSelectedAdjustment(stack)
  if (!target) return stack
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'adjustment') return layer
      return updateAdjustmentLayerParam(layer, key, value)
    }),
  }
}

function updateSelectedAdjustmentParams(
  stack: ImageEditorLayerStack,
  params: ImageEditorAdjustmentWholeParams,
): ImageEditorLayerStack {
  const target = getSelectedAdjustment(stack)
  if (!target) return stack
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'adjustment') return layer
      return updateAdjustmentLayerParams(layer, params)
    }),
  }
}

function resolveLayerInsertionIndex(stack: ImageEditorLayerStack) {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  if (selectedIndex >= 0 && stack.layers[selectedIndex]?.type !== 'source') return selectedIndex
  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  return sourceIndex >= 0 ? sourceIndex : stack.layers.length
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
        (key === 'hue' || key === 'saturation' || key === 'lightness')
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
