import type {
  ImageAdjustmentKind,
  RetouchAdjustmentLayer,
  RetouchAdjustmentWholeParams,
  RetouchCurvesParams,
  RetouchEditableLayer,
  RetouchHslParams,
  RetouchLayer,
  RetouchLayerByKind,
  RetouchLevelsParams,
  RetouchMergedLayer,
  RetouchPaintLayer,
  RetouchParamKey,
  RetouchParamValue,
  RetouchRasterContent,
  RetouchSharpenParams,
  RetouchThresholdParams,
} from '../types'
import {
  IMAGE_ADJUSTMENT_DEFINITIONS,
  getImageAdjustmentDefaultParams,
} from './imageAdjustmentDefinitions'
import { normalizeCurvesParams } from './retouchCurves'
import { clampLevelsHandles } from './retouchLevels'
import { normalizeThresholdAdjustment } from './retouchThreshold'
import { i18n } from '../../../../shared/i18n'

export const SOURCE_LAYER_ID = 'source'

let retouchLayerIdSeed = 0

const DEFAULT_ADJUSTMENT_LAYER_BY_KIND: {
  [TKind in ImageAdjustmentKind]: RetouchLayerByKind<TKind>
} = {
  hsl: {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: `${i18n.global.t('canvas.retouch.adjustments.hsl')} 1`,
    visible: true,
    params: getImageAdjustmentDefaultParams('hsl'),
  },
  threshold: {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: `${i18n.global.t('canvas.retouch.adjustments.threshold')} 1`,
    visible: true,
    params: getImageAdjustmentDefaultParams('threshold'),
  },
  curves: {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: `${i18n.global.t('canvas.retouch.adjustments.curves')} 1`,
    visible: true,
    params: getImageAdjustmentDefaultParams('curves'),
  },
  levels: {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: `${i18n.global.t('canvas.retouch.adjustments.levels')} 1`,
    visible: true,
    params: getImageAdjustmentDefaultParams('levels'),
  },
  sharpen: {
    id: 'adjustment:sharpen',
    type: 'adjustment',
    kind: 'sharpen',
    name: `${i18n.global.t('canvas.retouch.adjustments.sharpen')} 1`,
    visible: true,
    params: getImageAdjustmentDefaultParams('sharpen'),
  },
}

export type RetouchLayerStack = {
  layers: RetouchLayer[]
  selectedLayerId: string
}

export type RetouchLayerCommand =
  | { type: 'select-layer'; layerId: string }
  | { type: 'ensure-adjustment'; kind: ImageAdjustmentKind }
  | { type: 'add-paint' }
  | { type: 'copy-selected' }
  | { type: 'merge-selected-down' }
  | { type: 'set-visible'; layerId: string; visible: boolean }
  | { type: 'remove'; layerId: string }
  | { type: 'update-selected-param'; key: RetouchParamKey; value: RetouchParamValue }
  | { type: 'update-selected-adjustment-params'; params: RetouchAdjustmentWholeParams }

export function createInitialLayerStack(): RetouchLayerStack {
  return {
    layers: [{ id: SOURCE_LAYER_ID, type: 'source', name: i18n.global.t('canvas.retouch.layerNames.source'), visible: true }],
    selectedLayerId: SOURCE_LAYER_ID,
  }
}

export function createDefaultImageAdjustmentLayer<TKind extends ImageAdjustmentKind>(
  kind: TKind,
): RetouchLayerByKind<TKind> {
  return cloneAdjustmentLayer(kind)
}

export function applyLayerCommand(
  stack: RetouchLayerStack,
  command: RetouchLayerCommand,
): RetouchLayerStack {
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
  stack: RetouchLayerStack,
): RetouchAdjustmentLayer | null {
  const layer = stack.layers.find((item) => item.id === stack.selectedLayerId)
  return layer && isAdjustmentLayer(layer) ? layer : null
}

export function selectAfterRemove(
  layers: readonly RetouchLayer[],
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

function selectLayer(stack: RetouchLayerStack, layerId: string): RetouchLayerStack {
  const layer = stack.layers.find((item) => item.id === layerId)
  if (!layer) return stack
  return { ...stack, selectedLayerId: layer.id }
}

function ensureAdjustmentLayer(
  stack: RetouchLayerStack,
  kind: ImageAdjustmentKind,
): RetouchLayerStack {
  const existing = stack.layers.find(
    (layer): layer is RetouchAdjustmentLayer =>
      layer.type === 'adjustment' && layer.kind === kind,
  )
  if (existing) return { ...stack, selectedLayerId: existing.id }

  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  const nextLayer = cloneAdjustmentLayer(kind)
  const layers = stack.layers.slice()
  layers.splice(sourceIndex >= 0 ? sourceIndex : layers.length, 0, nextLayer)
  return { layers, selectedLayerId: nextLayer.id }
}

function addPaintLayer(stack: RetouchLayerStack): RetouchLayerStack {
  const layer = createPaintLayer(stack.layers)
  const layers = stack.layers.slice()
  layers.splice(resolveLayerInsertionIndex(stack), 0, layer)
  return { layers, selectedLayerId: layer.id }
}

function copySelectedLayer(stack: RetouchLayerStack): RetouchLayerStack {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  const layer = stack.layers[selectedIndex]
  if (selectedIndex < 0 || !layer || !isEditableLayer(layer)) return stack

  const copy = cloneEditableLayer(layer, createCopyLayerName(stack.layers, layer.name))
  const layers = stack.layers.slice()
  layers.splice(selectedIndex, 0, copy)
  return { layers, selectedLayerId: copy.id }
}

function mergeSelectedLayerDown(stack: RetouchLayerStack): RetouchLayerStack {
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
  stack: RetouchLayerStack,
  layerId: string,
  visible: boolean,
): RetouchLayerStack {
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== layerId || layer.type === 'source') return layer
      return { ...layer, visible }
    }),
  }
}

function removeLayer(stack: RetouchLayerStack, layerId: string): RetouchLayerStack {
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
  stack: RetouchLayerStack,
  key: RetouchParamKey,
  value: RetouchParamValue,
): RetouchLayerStack {
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
  stack: RetouchLayerStack,
  params: RetouchAdjustmentWholeParams,
): RetouchLayerStack {
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

function resolveLayerInsertionIndex(stack: RetouchLayerStack) {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  if (selectedIndex >= 0 && stack.layers[selectedIndex]?.type !== 'source') return selectedIndex
  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  return sourceIndex >= 0 ? sourceIndex : stack.layers.length
}

function cloneAdjustmentLayer<TKind extends ImageAdjustmentKind>(
  kind: TKind,
): RetouchLayerByKind<TKind> {
  const layer = DEFAULT_ADJUSTMENT_LAYER_BY_KIND[kind]
  return {
    ...layer,
    params: cloneAdjustmentParams(layer.params),
  } as RetouchLayerByKind<TKind>
}

function isAdjustmentLayer(layer: RetouchLayer): layer is RetouchAdjustmentLayer {
  return layer.type === 'adjustment'
}

function isEditableLayer(layer: RetouchLayer): layer is RetouchEditableLayer {
  return layer.type !== 'source'
}

function createLayerId(scope: string) {
  retouchLayerIdSeed += 1
  return `${scope}:${retouchLayerIdSeed}`
}

function collectLayerNames(layers: readonly RetouchLayer[]) {
  const names = new Set<string>()
  for (const layer of layers) collectLayerName(layer, names)
  return names
}

function collectLayerName(layer: RetouchLayer | RetouchEditableLayer, names: Set<string>) {
  names.add(layer.name)
  if (layer.type === 'merged') {
    layer.children.forEach((child) => collectLayerName(child, names))
  }
}

function createUniqueLayerName(layers: readonly RetouchLayer[], base: string) {
  const names = collectLayerNames(layers)
  if (!names.has(base)) return base
  let index = 2
  while (names.has(`${base} ${index}`)) index += 1
  return `${base} ${index}`
}

function createNumberedLayerName(layers: readonly RetouchLayer[], prefix: string) {
  const names = collectLayerNames(layers)
  let index = 1
  while (names.has(`${prefix} ${index}`)) index += 1
  return `${prefix} ${index}`
}

function createCopyLayerName(layers: readonly RetouchLayer[], name: string) {
  return createUniqueLayerName(layers, `${name} ${i18n.global.t('canvas.retouch.layerNames.copySuffix')}`)
}

function createPaintLayer(layers: readonly RetouchLayer[]): RetouchPaintLayer {
  return {
    id: createLayerId('paint'),
    type: 'paint',
    name: createNumberedLayerName(layers, i18n.global.t('canvas.retouch.layerNames.paint')),
    visible: true,
    content: { kind: 'empty' },
  }
}

function cloneEditableLayer(
  layer: RetouchEditableLayer,
  name = layer.name,
): RetouchEditableLayer {
  if (layer.type === 'paint') {
    return {
      ...layer,
      id: createLayerId('paint'),
      name,
      content: clonePaintContent(layer.content),
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
  } as RetouchAdjustmentLayer
}

function clonePaintContent(content: RetouchPaintLayer['content']) {
  return content.kind === 'raster' ? cloneRasterContent(content) : { ...content }
}

function cloneRasterContent(content: RetouchRasterContent): RetouchRasterContent {
  return { ...content }
}

function createMergedLayer(
  layers: readonly RetouchLayer[],
  children: [RetouchEditableLayer, RetouchEditableLayer],
): RetouchMergedLayer {
  return {
    id: createLayerId('merged'),
    type: 'merged',
    name: createNumberedLayerName(layers, i18n.global.t('canvas.retouch.layerNames.merged')),
    visible: children.some((layer) => layer.visible),
    children: children.map((layer) => cloneEditableLayer(layer)),
  }
}

function updateAdjustmentLayerParam(
  layer: RetouchAdjustmentLayer,
  key: RetouchParamKey,
  value: RetouchParamValue,
): RetouchAdjustmentLayer {
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
    case 'sharpen':
      return layer
  }
}

function updateAdjustmentLayerParams(
  layer: RetouchAdjustmentLayer,
  params: RetouchAdjustmentWholeParams,
): RetouchAdjustmentLayer {
  switch (layer.kind) {
    case 'hsl':
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
    case 'sharpen':
      if (isSharpenParams(params)) {
        return { ...layer, params: cloneSharpenParams(params) }
      }
      return layer
  }
}

function cloneAdjustmentParams(
  params:
    | RetouchHslParams
    | RetouchThresholdParams
    | RetouchCurvesParams
    | RetouchLevelsParams
    | RetouchSharpenParams,
) {
  if ('channels' in params) return cloneCurvesParams(params)
  if ('amount' in params) return cloneSharpenParams(params)
  if ('threshold' in params) return cloneThresholdParams(params)
  if ('shadows' in params) return cloneLevelsParams(params)
  return { ...params }
}

function cloneThresholdParams(params: RetouchThresholdParams): RetouchThresholdParams {
  return normalizeThresholdAdjustment(params)
}

function cloneCurvesParams(params: RetouchCurvesParams): RetouchCurvesParams {
  const normalized = normalizeCurvesParams(params)
  const channels: RetouchCurvesParams['channels'] = {}
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
      channels[channel as keyof RetouchCurvesParams['channels']] = denormalizedPoints
    }
  }
  return { channels }
}

function cloneLevelsParams(params: RetouchLevelsParams): RetouchLevelsParams {
  return clampLevelsHandles(params)
}

function cloneSharpenParams(params: RetouchSharpenParams): RetouchSharpenParams {
  return IMAGE_ADJUSTMENT_DEFINITIONS.sharpen.normalize(params)
}

function isThresholdParams(
  params: RetouchAdjustmentWholeParams,
): params is RetouchThresholdParams {
  return 'threshold' in params && 'invert' in params
}

function isCurvesParams(
  params: RetouchAdjustmentWholeParams,
): params is RetouchCurvesParams {
  return 'channels' in params
}

function isLevelsParams(
  params: RetouchAdjustmentWholeParams,
): params is RetouchLevelsParams {
  return 'shadows' in params
}

function isSharpenParams(
  params: RetouchAdjustmentWholeParams,
): params is RetouchSharpenParams {
  return 'amount' in params
}
