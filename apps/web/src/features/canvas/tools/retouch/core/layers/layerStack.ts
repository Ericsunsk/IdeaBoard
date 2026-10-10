import type {
  ImageRetouchKind,
  RetouchCurvesParams,
  RetouchEditableLayer,
  RetouchEffectLayer,
  RetouchEffectParamKey,
  RetouchEffectParamMap,
  RetouchHslParams,
  RetouchLayer,
  RetouchLayerByKind,
  RetouchLayerNamePart,
  RetouchLevelsParams,
  RetouchMergedLayer,
  RetouchNonHslWholeParams,
  RetouchPaintLayer,
  RetouchRasterContent,
  RetouchSharpenParams,
  RetouchThresholdParams,
} from '../model.ts'
import {
  IMAGE_RETOUCH_DEFINITIONS,
  cloneRetouchParamsForKind,
  getImageRetouchDefaultParams,
} from '../definitions'
import { RETOUCH_EFFECT_LAYER_PREFIX, RETOUCH_SOURCE_LAYER_ID, makeRetouchLayerId } from '../ids.ts'
import { RETOUCH_KIND_LABEL_KEYS } from '../kindLabels.ts'

export const SOURCE_LAYER_ID = RETOUCH_SOURCE_LAYER_ID
export { makeRetouchLayerId } from '../ids.ts'

const IMAGE_RETOUCH_KINDS = ['hsl', 'threshold', 'curves', 'levels', 'sharpen'] as const

function makeDefaultEffectLayer<TKind extends ImageRetouchKind>(
  kind: TKind,
): RetouchLayerByKind<TKind> {
  return {
    id: `${RETOUCH_EFFECT_LAYER_PREFIX}${kind}`,
    type: 'effect',
    kind,
    ...createLayerName([{ labelKey: RETOUCH_KIND_LABEL_KEYS[kind] }, '1']),
    visible: true,
    params: getImageRetouchDefaultParams(kind),
  } as unknown as RetouchLayerByKind<TKind>
}

const DEFAULT_EFFECT_LAYER_BY_KIND: {
  [TKind in ImageRetouchKind]: RetouchLayerByKind<TKind>
} = Object.fromEntries(IMAGE_RETOUCH_KINDS.map((kind) => [kind, makeDefaultEffectLayer(kind)])) as {
  [TKind in ImageRetouchKind]: RetouchLayerByKind<TKind>
}

export type RetouchLayerStack = {
  layers: RetouchLayer[]
  selectedLayerId: string
}

type RetouchUpdateSelectedParamCommand = {
  [K in RetouchEffectParamKey]: {
    type: 'update-selected-param'
    key: K
    value: RetouchEffectParamMap[K]
  }
}[RetouchEffectParamKey]

export type RetouchLayerCommand =
  | { type: 'select-layer'; layerId: string }
  | { type: 'ensure-retouch'; kind: ImageRetouchKind }
  | { type: 'add-paint' }
  | { type: 'copy-selected' }
  | { type: 'merge-selected-down' }
  | { type: 'set-visible'; layerId: string; visible: boolean }
  | { type: 'remove'; layerId: string }
  | RetouchUpdateSelectedParamCommand
  | { type: 'update-selected-retouch-params'; params: RetouchNonHslWholeParams }

export function createInitialLayerStack(): RetouchLayerStack {
  return {
    layers: [
      {
        id: SOURCE_LAYER_ID,
        type: 'source',
        ...createLayerName([{ labelKey: 'canvas.retouch.layerNames.source' }]),
        visible: true,
      },
    ],
    selectedLayerId: SOURCE_LAYER_ID,
  }
}

export function createDefaultImageEffectLayer<TKind extends ImageRetouchKind>(
  kind: TKind,
): RetouchLayerByKind<TKind> {
  return cloneEffectLayer(kind)
}

export function applyLayerCommand(
  stack: RetouchLayerStack,
  command: RetouchLayerCommand,
): RetouchLayerStack {
  const handler = LAYER_COMMAND_HANDLERS[command.type] as (
    stack: RetouchLayerStack,
    command: RetouchLayerCommand,
  ) => RetouchLayerStack
  return handler(stack, command)
}

const LAYER_COMMAND_HANDLERS: {
  [K in RetouchLayerCommand['type']]: (
    stack: RetouchLayerStack,
    command: Extract<RetouchLayerCommand, { type: K }>,
  ) => RetouchLayerStack
} = {
  'select-layer': (stack, command) => selectLayer(stack, command.layerId),
  'ensure-retouch': (stack, command) => ensureEffectLayer(stack, command.kind),
  'add-paint': (stack) => addPaintLayer(stack),
  'copy-selected': (stack) => copySelectedLayer(stack),
  'merge-selected-down': (stack) => mergeSelectedLayerDown(stack),
  'set-visible': (stack, command) => setLayerVisible(stack, command.layerId, command.visible),
  remove: (stack, command) => removeLayer(stack, command.layerId),
  'update-selected-param': (stack, command) =>
    updateSelectedLayerParam(stack, command.key, command.value),
  'update-selected-retouch-params': (stack, command) =>
    updateSelectedRetouchParams(stack, command.params),
}

export function getSelectedEffect(stack: RetouchLayerStack): RetouchEffectLayer | null {
  const layer = stack.layers.find((item) => item.id === stack.selectedLayerId)
  return layer && isEffectLayer(layer) ? layer : null
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

export type RetouchLayerStackLike = {
  layers: readonly RetouchLayer[]
  selectedLayerId: string
}

function findSelectedLayer(stack: RetouchLayerStackLike): RetouchLayer | undefined {
  return stack.layers.find((item) => item.id === stack.selectedLayerId)
}

/*
 * Capability predicates for Chrome disabled= bindings. Pure stack reads that
 * mirror the copy/merge/remove command guards below, so the UI never forks
 * the editable-layer rules.
 */
export function canCopySelectedLayer(stack: RetouchLayerStackLike): boolean {
  const layer = findSelectedLayer(stack)
  return layer != null && isEditableLayer(layer)
}

export function canMergeSelectedLayer(stack: RetouchLayerStackLike): boolean {
  const selectedIndex = stack.layers.findIndex((item) => item.id === stack.selectedLayerId)
  const layer = stack.layers[selectedIndex]
  const target = stack.layers[selectedIndex + 1]
  return (
    selectedIndex >= 0 &&
    layer != null &&
    target != null &&
    isEditableLayer(layer) &&
    isEditableLayer(target)
  )
}

export function canRemoveSelectedLayer(stack: RetouchLayerStackLike): boolean {
  // Remove shares the selected-editable guard with copy: source stays protected.
  return canCopySelectedLayer(stack)
}

function selectLayer(stack: RetouchLayerStack, layerId: string): RetouchLayerStack {
  const layer = stack.layers.find((item) => item.id === layerId)
  if (!layer) return stack
  return { ...stack, selectedLayerId: layer.id }
}

function ensureEffectLayer(stack: RetouchLayerStack, kind: ImageRetouchKind): RetouchLayerStack {
  const existing = stack.layers.find(
    (layer): layer is RetouchEffectLayer => layer.type === 'effect' && layer.kind === kind,
  )
  if (existing) return { ...stack, selectedLayerId: existing.id }

  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  const nextLayer = cloneEffectLayer(kind)
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

  const copy = { ...cloneEditableLayer(layer), ...createCopyLayerName(stack.layers, layer) }
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

function updateSelectedLayerParam<TKey extends RetouchEffectParamKey>(
  stack: RetouchLayerStack,
  key: TKey,
  value: RetouchEffectParamMap[TKey],
): RetouchLayerStack {
  const target = getSelectedEffect(stack)
  if (!target) return stack
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'effect') return layer
      return updateEffectLayerParam(layer, key, value)
    }),
  }
}

function updateSelectedRetouchParams(
  stack: RetouchLayerStack,
  params: RetouchNonHslWholeParams,
): RetouchLayerStack {
  const target = getSelectedEffect(stack)
  if (!target) return stack
  return {
    ...stack,
    layers: stack.layers.map((layer) => {
      if (layer.id !== target.id || layer.type !== 'effect') return layer
      return updateEffectLayerParams(layer, params)
    }),
  }
}

function resolveLayerInsertionIndex(stack: RetouchLayerStack) {
  const selectedIndex = stack.layers.findIndex((layer) => layer.id === stack.selectedLayerId)
  if (selectedIndex >= 0 && stack.layers[selectedIndex]?.type !== 'source') return selectedIndex
  const sourceIndex = stack.layers.findIndex((layer) => layer.type === 'source')
  return sourceIndex >= 0 ? sourceIndex : stack.layers.length
}

function cloneEffectLayer<TKind extends ImageRetouchKind>(kind: TKind): RetouchLayerByKind<TKind> {
  const layer = DEFAULT_EFFECT_LAYER_BY_KIND[kind]
  return {
    ...layer,
    params: cloneRetouchParamsForKind(kind, layer.params as never),
  } as RetouchLayerByKind<TKind>
}

function isEffectLayer(layer: RetouchLayer): layer is RetouchEffectLayer {
  return layer.type === 'effect'
}

function isEditableLayer(layer: RetouchLayer): layer is RetouchEditableLayer {
  return layer.type !== 'source'
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

function createLayerName(nameParts: readonly RetouchLayerNamePart[]) {
  return {
    name: nameParts.map((part) => (typeof part === 'string' ? part : part.labelKey)).join(' '),
    nameParts,
  }
}

function createUniqueLayerName(
  layers: readonly RetouchLayer[],
  parts: readonly RetouchLayerNamePart[],
) {
  const names = collectLayerNames(layers)
  const base = createLayerName(parts)
  if (!names.has(base.name)) return base
  let index = 2
  while (names.has(`${base.name} ${index}`)) index += 1
  return createLayerName([...parts, String(index)])
}

function createNumberedLayerName(layers: readonly RetouchLayer[], labelKey: string) {
  const names = collectLayerNames(layers)
  let index = 1
  while (names.has(`${labelKey} ${index}`)) index += 1
  return createLayerName([{ labelKey }, String(index)])
}

function createCopyLayerName(layers: readonly RetouchLayer[], layer: RetouchEditableLayer) {
  // A changed name is a custom literal, even when generated metadata remains.
  const parts =
    layer.nameParts && createLayerName(layer.nameParts).name === layer.name
      ? layer.nameParts
      : [layer.name]
  return createUniqueLayerName(layers, [
    ...parts,
    { labelKey: 'canvas.retouch.layerNames.copySuffix' },
  ])
}

function createPaintLayer(layers: readonly RetouchLayer[]): RetouchPaintLayer {
  return {
    id: makeRetouchLayerId('paint'),
    type: 'paint',
    ...createNumberedLayerName(layers, 'canvas.retouch.layerNames.paint'),
    visible: true,
    content: { kind: 'empty' },
  }
}

function cloneEditableLayer(layer: RetouchEditableLayer): RetouchEditableLayer {
  if (layer.type === 'paint') {
    return {
      ...layer,
      id: makeRetouchLayerId('paint'),
      content: clonePaintContent(layer.content),
    }
  }
  if (layer.type === 'merged') {
    return {
      ...layer,
      id: makeRetouchLayerId('merged'),
      children: layer.children.map((child) => cloneEditableLayer(child)),
    }
  }
  return {
    ...layer,
    id: makeRetouchLayerId(`${RETOUCH_EFFECT_LAYER_PREFIX}${layer.kind}`),
    params: cloneRetouchParamsForKind(layer.kind, layer.params as never),
  } as RetouchEffectLayer
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
    id: makeRetouchLayerId('merged'),
    type: 'merged',
    ...createNumberedLayerName(layers, 'canvas.retouch.layerNames.merged'),
    visible: children.some((layer) => layer.visible),
    children: children.map((layer) => cloneEditableLayer(layer)),
  }
}

export function isHslParam(key: RetouchEffectParamKey, value: unknown): boolean {
  return typeof value === 'number' && (key === 'hue' || key === 'saturation' || key === 'lightness')
}

export function isThresholdParam(key: RetouchEffectParamKey, value: unknown): boolean {
  return (
    (key === 'threshold' && typeof value === 'number') ||
    (key === 'invert' && typeof value === 'boolean')
  )
}

export function isCurvesParam(key: RetouchEffectParamKey, value: unknown): boolean {
  return key === 'channels' && typeof value === 'object' && value !== null && !Array.isArray(value)
}

export function isLevelsParam(key: RetouchEffectParamKey, value: unknown): boolean {
  return (
    typeof value === 'number' &&
    (key === 'shadows' ||
      key === 'midtones' ||
      key === 'highlights' ||
      key === 'outputBlack' ||
      key === 'outputWhite')
  )
}

export function isSharpenParam(key: RetouchEffectParamKey, value: unknown): boolean {
  return (
    ((key === 'amount' || key === 'radius' || key === 'threshold') && typeof value === 'number') ||
    (key === 'protectColor' && typeof value === 'boolean')
  )
}

/*
 * Full-kind narrow-patch vocabulary.
 *
 * Every kind travels as a minimal single-key patch with per-key value
 * correlation (a wrong value type for a key is a compile error); the four
 * non-hsl kinds keep their whole-params packets as well, so the dual APIs
 * stay. Both shapes share this one patch union so the session and the panel
 * fan out through a single function instead of forking per kind. A key that
 * does not belong to the target layer stays a runtime no-op (same
 * reference), exactly like the mismatched whole-params packets.
 */
type RetouchSingleParamPatch = {
  [K in RetouchEffectParamKey]: { kind: 'single-param'; key: K; value: RetouchEffectParamMap[K] }
}[RetouchEffectParamKey]

export type RetouchParamPatch =
  RetouchSingleParamPatch | { kind: 'whole-params'; params: RetouchNonHslWholeParams }

export function updateEffectLayerPatch(
  layer: RetouchEffectLayer,
  patch: RetouchParamPatch,
): RetouchEffectLayer {
  if (patch.kind === 'single-param') {
    return updateEffectLayerParam(layer, patch.key, patch.value)
  }
  return updateEffectLayerParams(layer, patch.params)
}

export function updateEffectLayerParam<TKey extends RetouchEffectParamKey>(
  layer: RetouchEffectLayer,
  key: TKey,
  value: RetouchEffectParamMap[TKey],
): RetouchEffectLayer {
  switch (layer.kind) {
    case 'hsl':
      if (isHslParam(key, value)) {
        return { ...layer, params: { ...layer.params, [key]: value } as RetouchHslParams }
      }
      debugIgnoredSingleParam(layer.kind, key)
      return layer
    case 'threshold': {
      if (!isThresholdParam(key, value)) {
        debugIgnoredSingleParam(layer.kind, key)
        return layer
      }
      const nextParams = { ...layer.params, [key]: value } as RetouchThresholdParams
      return { ...layer, params: cloneRetouchParamsForKind('threshold', nextParams) }
    }
    case 'curves': {
      if (!isCurvesParam(key, value)) {
        debugIgnoredSingleParam(layer.kind, key)
        return layer
      }
      const nextParams = { ...layer.params, [key]: value } as RetouchCurvesParams
      return { ...layer, params: cloneRetouchParamsForKind('curves', nextParams) }
    }
    case 'levels': {
      if (!isLevelsParam(key, value)) {
        debugIgnoredSingleParam(layer.kind, key)
        return layer
      }
      const nextParams = { ...layer.params, [key]: value } as RetouchLevelsParams
      return { ...layer, params: cloneRetouchParamsForKind('levels', nextParams) }
    }
    case 'sharpen': {
      if (!isSharpenParam(key, value)) {
        debugIgnoredSingleParam(layer.kind, key)
        return layer
      }
      const nextParams = { ...layer.params, [key]: value } as RetouchSharpenParams
      return { ...layer, params: cloneRetouchParamsForKind('sharpen', nextParams) }
    }
  }
}

function debugIgnoredSingleParam(kind: ImageRetouchKind, key: RetouchEffectParamKey) {
  if (import.meta.env.DEV) {
    console.debug('[retouch] updateEffectLayerParam ignored', { kind, key })
  }
}

const EFFECT_WHOLE_PARAMS_HANDLERS: {
  [K in Exclude<ImageRetouchKind, 'hsl'>]: {
    requiredKeys: readonly string[]
    clone: (params: never) => RetouchNonHslWholeParams
  }
} = {
  threshold: {
    requiredKeys: ['threshold', 'invert'],
    clone: (params: RetouchThresholdParams) => IMAGE_RETOUCH_DEFINITIONS.threshold.clone(params),
  },
  curves: {
    requiredKeys: ['channels'],
    clone: (params: RetouchCurvesParams) => IMAGE_RETOUCH_DEFINITIONS.curves.clone(params),
  },
  levels: {
    requiredKeys: ['shadows'],
    // UI 约束与入库归一是同一函数: Levels/LevelsFields/LevelsOutputControl
    // markers, constraints, and move* handles all derive from
    // clampLevelsHandles, and the registry clone reuses it here.
    clone: (params: RetouchLevelsParams) => IMAGE_RETOUCH_DEFINITIONS.levels.clone(params),
  },
  sharpen: {
    requiredKeys: ['amount'],
    clone: (params: RetouchSharpenParams) => IMAGE_RETOUCH_DEFINITIONS.sharpen.clone(params),
  },
}

export function isWholeParamsForKind(
  kind: Exclude<ImageRetouchKind, 'hsl'>,
  params: RetouchNonHslWholeParams | RetouchHslParams,
): boolean {
  const handler = EFFECT_WHOLE_PARAMS_HANDLERS[kind]
  if (!handler) return false
  return handler.requiredKeys.every((key) => key in (params as Record<string, unknown>))
}

export function isThresholdParams(
  params: RetouchNonHslWholeParams | RetouchHslParams,
): params is RetouchThresholdParams {
  return isWholeParamsForKind('threshold', params)
}

export function isCurvesParams(
  params: RetouchNonHslWholeParams | RetouchHslParams,
): params is RetouchCurvesParams {
  return isWholeParamsForKind('curves', params)
}

export function isLevelsParams(
  params: RetouchNonHslWholeParams | RetouchHslParams,
): params is RetouchLevelsParams {
  return isWholeParamsForKind('levels', params)
}

export function isSharpenParams(
  params: RetouchNonHslWholeParams | RetouchHslParams,
): params is RetouchSharpenParams {
  return isWholeParamsForKind('sharpen', params)
}

export function updateEffectLayerParams(
  layer: RetouchEffectLayer,
  params: RetouchNonHslWholeParams,
): RetouchEffectLayer {
  if (layer.kind === 'hsl') {
    if (import.meta.env.DEV) {
      console.debug('[retouch] updateEffectLayerParams ignored', { kind: layer.kind })
    }
    return layer
  }
  const kind = layer.kind as Exclude<ImageRetouchKind, 'hsl'>
  const handler = EFFECT_WHOLE_PARAMS_HANDLERS[kind]
  if (!handler || !isWholeParamsForKind(kind, params)) {
    if (import.meta.env.DEV) {
      console.debug('[retouch] updateEffectLayerParams ignored', { kind: layer.kind })
    }
    return layer
  }
  return { ...layer, params: handler.clone(params as never) } as RetouchEffectLayer
}
