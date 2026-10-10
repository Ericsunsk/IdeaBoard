import { describe, expect, it, vi } from 'vitest'
import type { RetouchLayerStack } from './layerStack'
import {
  applyLayerCommand,
  canCopySelectedLayer,
  canMergeSelectedLayer,
  canRemoveSelectedLayer,
  createDefaultImageEffectLayer,
  createInitialLayerStack,
  getSelectedEffect,
  isCurvesParam,
  isCurvesParams,
  isHslParam,
  isLevelsParam,
  isLevelsParams,
  isSharpenParam,
  isSharpenParams,
  isThresholdParam,
  isThresholdParams,
  isWholeParamsForKind,
  selectAfterRemove,
  updateEffectLayerParam,
  updateEffectLayerParams,
  updateEffectLayerPatch,
} from './layerStack'

describe('retouchLayerStack', () => {
  it('imports independently without initializing the Vue locale runtime', async () => {
    vi.resetModules()
    vi.doMock('vue-i18n', () => {
      throw new Error('core must not initialize Vue i18n')
    })
    try {
      const core = await import('./layerStack')
      expect(core.createInitialLayerStack().selectedLayerId).toBe('source')
    } finally {
      vi.doUnmock('vue-i18n')
      vi.resetModules()
    }
  })

  it('creates the initial stack with only the source layer', () => {
    const firstStack = createInitialLayerStack()

    expect(firstStack.selectedLayerId).toBe('source')
    expect(firstStack.layers.map((layer) => layer.id)).toEqual(['source'])
  })

  it('creates independent default retouch layer params', () => {
    const firstHslLayer = createDefaultImageEffectLayer('hsl')
    const secondHslLayer = createDefaultImageEffectLayer('hsl')
    firstHslLayer.params.lightness = 45

    expect(secondHslLayer.params).toEqual({ hue: 0, saturation: 0, lightness: 0 })
  })

  it('keeps copy suffix collisions and repeated copies as pure label parts', () => {
    let stack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })
    const originalId = stack.selectedLayerId
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    stack = applyLayerCommand(stack, { type: 'select-layer', layerId: originalId })
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    stack = applyLayerCommand(stack, { type: 'copy-selected' })

    expect(stack.layers.map((layer) => layer.name)).toEqual([
      'canvas.retouch.layerNames.paint 1 canvas.retouch.layerNames.copySuffix',
      'canvas.retouch.layerNames.paint 1 canvas.retouch.layerNames.copySuffix 2 canvas.retouch.layerNames.copySuffix',
      'canvas.retouch.layerNames.paint 1 canvas.retouch.layerNames.copySuffix 2',
      'canvas.retouch.layerNames.paint 1',
      'canvas.retouch.layerNames.source',
    ])
    expect(stack.layers.find((layer) => layer.id === stack.selectedLayerId)?.nameParts).toEqual([
      { labelKey: 'canvas.retouch.layerNames.paint' },
      '1',
      { labelKey: 'canvas.retouch.layerNames.copySuffix' },
      '2',
      { labelKey: 'canvas.retouch.layerNames.copySuffix' },
    ])
  })

  it('retains nested merge and copy names when choosing paint and merge numbers', () => {
    let stack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })
    stack.layers[0]!.name = 'Custom child'
    stack = applyLayerCommand(stack, { type: 'add-paint' })
    stack = applyLayerCommand(stack, { type: 'merge-selected-down' })
    const originalMerge = stack.layers[0]!
    stack = applyLayerCommand(stack, { type: 'copy-selected' })
    const copiedMerge = stack.layers[0]!
    expect(copiedMerge.type).toBe('merged')
    expect(originalMerge.type).toBe('merged')
    if (copiedMerge.type !== 'merged' || originalMerge.type !== 'merged') return
    expect(copiedMerge.children.map((layer) => layer.name)).toEqual([
      'canvas.retouch.layerNames.paint 1',
      'Custom child',
    ])
    expect(copiedMerge.children[0]?.nameParts).toEqual(originalMerge.children[0]?.nameParts)
    expect(copiedMerge.children[0]).not.toBe(originalMerge.children[0])
    stack = applyLayerCommand(stack, { type: 'merge-selected-down' })
    expect(stack.layers[0]?.name).toBe('canvas.retouch.layerNames.merged 2')
    stack = applyLayerCommand(stack, { type: 'add-paint' })
    expect(stack.layers[0]?.name).toBe('canvas.retouch.layerNames.paint 2')
    stack = applyLayerCommand(stack, { type: 'merge-selected-down' })
    expect(stack.layers[0]?.name).toBe('canvas.retouch.layerNames.merged 3')
  })

  it('copies retouch params without sharing nested curve references', () => {
    const stackWithCurves = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'curves',
    })
    const updatedStack = applyLayerCommand(stackWithCurves, {
      type: 'update-selected-retouch-params',
      params: {
        channels: {
          rgb: [{ id: 'midtone', input: 50, output: 60 }],
        },
      },
    })

    const copiedStack = applyLayerCommand(updatedStack, { type: 'copy-selected' })
    const curvesLayers = copiedStack.layers.filter(
      (layer) => layer.type === 'effect' && layer.kind === 'curves',
    )
    const [copyLayer, originalLayer] = curvesLayers
    expect(copyLayer?.type).toBe('effect')
    expect(originalLayer?.type).toBe('effect')
    if (copyLayer?.type !== 'effect' || originalLayer?.type !== 'effect') return
    expect(copyLayer.kind).toBe('curves')
    expect(originalLayer.kind).toBe('curves')
    if (copyLayer.kind !== 'curves' || originalLayer.kind !== 'curves') return

    copyLayer.params.channels.rgb![0]!.output = 10

    expect(originalLayer.params.channels.rgb?.[0]?.output).toBe(60)
  })

  it('selects the nearest editable fallback after removing the selected layer', () => {
    const stack = applyLayerCommand(
      applyLayerCommand(createInitialLayerStack(), {
        type: 'ensure-retouch',
        kind: 'threshold',
      }),
      { type: 'ensure-retouch', kind: 'hsl' },
    )

    const nextStack = applyLayerCommand(stack, { type: 'remove', layerId: 'retouch:hsl' })

    expect(nextStack.layers.map((layer) => layer.id)).toEqual(['retouch:threshold', 'source'])
    expect(nextStack.selectedLayerId).toBe('retouch:threshold')
    expect(selectAfterRemove(nextStack.layers, 0)).toBe('retouch:threshold')
  })

  it('protects the source layer from destructive commands', () => {
    const sourceSelectedStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'select-layer',
      layerId: 'source',
    })

    const copiedStack = applyLayerCommand(sourceSelectedStack, { type: 'copy-selected' })
    const mergedStack = applyLayerCommand(copiedStack, { type: 'merge-selected-down' })
    const removedStack = applyLayerCommand(mergedStack, { type: 'remove', layerId: 'source' })

    expect(removedStack.layers.map((layer) => layer.id)).toEqual(['source'])
    expect(removedStack.selectedLayerId).toBe('source')
  })

  it('creates and selects the requested retouch layer command', () => {
    const nextStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'threshold',
    })

    expect(nextStack.selectedLayerId).toBe('retouch:threshold')
    expect(getSelectedEffect(nextStack)).toMatchObject({
      id: 'retouch:threshold',
      kind: 'threshold',
    })
    expect(nextStack.layers.map((layer) => layer.id)).toEqual(['retouch:threshold', 'source'])
  })

  it('creates sharpen retouch layers with normalized params', () => {
    const sharpenStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'sharpen',
    })
    const updatedStack = applyLayerCommand(sharpenStack, {
      type: 'update-selected-retouch-params',
      params: {
        amount: 240,
        radius: 8,
        threshold: Number.NaN,
        protectColor: false,
      },
    })

    expect(updatedStack.selectedLayerId).toBe('retouch:sharpen')
    expect(getSelectedEffect(updatedStack)).toMatchObject({
      id: 'retouch:sharpen',
      kind: 'sharpen',
      name: 'canvas.retouch.effects.sharpen 1',
      params: {
        amount: 200,
        radius: 5,
        threshold: 4,
        protectColor: false,
      },
    })
    expect(updatedStack.layers.map((layer) => layer.id)).toEqual(['retouch:sharpen', 'source'])
  })

  it('falls back to source selection when the last editable layer is removed', () => {
    const stack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'hsl',
    })
    const nextStack = applyLayerCommand(stack, {
      type: 'remove',
      layerId: 'retouch:hsl',
    })

    expect(nextStack.layers.map((layer) => layer.id)).toEqual(['source'])
    expect(nextStack.selectedLayerId).toBe('source')
    expect(getSelectedEffect(nextStack)).toBeNull()
  })

  it('does not share whole-param update input references with layer state', () => {
    const params = {
      channels: {
        rgb: [{ id: 'midtone', input: 50, output: 60 }],
      },
    }
    const curvesStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'curves',
    })

    const nextStack = applyLayerCommand(curvesStack, {
      type: 'update-selected-retouch-params',
      params,
    })
    params.channels.rgb[0]!.output = 10

    expect(getSelectedEffect(nextStack)).toMatchObject({
      kind: 'curves',
      params: {
        channels: {
          rgb: [{ id: 'midtone', input: 50, output: 60 }],
        },
      },
    })
  })

  it('keeps sharpen params distinct from threshold when both carry a threshold key', () => {
    const sharpenStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'sharpen',
    })
    const updatedStack = applyLayerCommand(sharpenStack, {
      type: 'update-selected-retouch-params',
      params: { amount: 80, radius: 2, threshold: 12, protectColor: true },
    })
    const copiedStack = applyLayerCommand(updatedStack, { type: 'copy-selected' })

    const sharpenLayers = copiedStack.layers.filter(
      (layer) => layer.type === 'effect' && layer.kind === 'sharpen',
    )
    expect(sharpenLayers).toHaveLength(2)
    for (const layer of sharpenLayers) {
      expect(layer).toMatchObject({
        type: 'effect',
        kind: 'sharpen',
        params: { amount: 80, radius: 2, threshold: 12, protectColor: true },
      })
    }

    const thresholdStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'threshold',
    })
    const updatedThresholdStack = applyLayerCommand(thresholdStack, {
      type: 'update-selected-retouch-params',
      params: { threshold: 200, invert: true },
    })
    expect(getSelectedEffect(updatedThresholdStack)).toMatchObject({
      kind: 'threshold',
      params: { threshold: 200, invert: true },
    })
  })

  it('clones hsl params as independent copies through the definition registry', () => {
    const hslStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'hsl',
    })
    const updatedStack = applyLayerCommand(hslStack, {
      type: 'update-selected-param',
      key: 'lightness',
      value: 10,
    })

    const copiedStack = applyLayerCommand(updatedStack, { type: 'copy-selected' })
    const hslLayers = copiedStack.layers.filter(
      (layer) => layer.type === 'effect' && layer.kind === 'hsl',
    )
    expect(hslLayers).toHaveLength(2)
    const [copyLayer, originalLayer] = hslLayers
    if (copyLayer?.type !== 'effect' || originalLayer?.type !== 'effect') return
    if (copyLayer.kind !== 'hsl' || originalLayer.kind !== 'hsl') return
    // The registry clone preserves values while breaking references.
    expect(copyLayer.params).toEqual({ hue: 0, saturation: 0, lightness: 10 })
    copyLayer.params.lightness = 40
    expect(originalLayer.params).toEqual({ hue: 0, saturation: 0, lightness: 10 })
  })

  it('leaves incompatible param commands unchanged', () => {
    const stack: RetouchLayerStack = createInitialLayerStack()

    const nextStack = applyLayerCommand(stack, {
      type: 'update-selected-retouch-params',
      params: { threshold: 20, invert: false },
    })

    expect(nextStack.layers).toEqual(stack.layers)
    expect(nextStack.selectedLayerId).toBe(stack.selectedLayerId)
  })
})

describe('retouchLayerStack shape guards', () => {
  it('keeps single-key hsl guards in layerStack', () => {
    expect(isHslParam('hue', 10)).toBe(true)
    expect(isHslParam('saturation', -5)).toBe(true)
    expect(isHslParam('lightness', 0)).toBe(true)
    expect(isHslParam('hue', Number.NaN)).toBe(true)
    expect(isHslParam('invert', true)).toBe(false)
    expect(isHslParam('threshold', 10)).toBe(false)
  })

  it('keeps single-key guards per kind with value-type checks', () => {
    expect(isThresholdParam('threshold', 10)).toBe(true)
    expect(isThresholdParam('invert', true)).toBe(true)
    expect(isThresholdParam('invert', 1)).toBe(false)
    expect(isThresholdParam('threshold', true)).toBe(false)
    expect(isThresholdParam('hue', 10)).toBe(false)
    expect(isCurvesParam('channels', { rgb: [] })).toBe(true)
    expect(isCurvesParam('channels', null)).toBe(false)
    expect(isCurvesParam('channels', [])).toBe(false)
    expect(isCurvesParam('hue', 10)).toBe(false)
    expect(isLevelsParam('shadows', 10)).toBe(true)
    expect(isLevelsParam('midtones', 128)).toBe(true)
    expect(isLevelsParam('highlights', 255)).toBe(true)
    expect(isLevelsParam('outputBlack', 0)).toBe(true)
    expect(isLevelsParam('outputWhite', 255)).toBe(true)
    expect(isLevelsParam('shadows', true)).toBe(false)
    expect(isLevelsParam('hue', 10)).toBe(false)
    expect(isSharpenParam('amount', 80)).toBe(true)
    expect(isSharpenParam('radius', 1.5)).toBe(true)
    expect(isSharpenParam('threshold', 7)).toBe(true)
    expect(isSharpenParam('protectColor', false)).toBe(true)
    expect(isSharpenParam('protectColor', 1)).toBe(false)
    expect(isSharpenParam('amount', true)).toBe(false)
    expect(isSharpenParam('hue', 10)).toBe(false)
  })

  it('keeps whole-params guards per kind', () => {
    expect(isThresholdParams({ threshold: 10, invert: false })).toBe(true)
    expect(isThresholdParams({ channels: {} })).toBe(false)
    expect(isCurvesParams({ channels: {} })).toBe(true)
    expect(isCurvesParams({ threshold: 10, invert: false })).toBe(false)
    expect(
      isLevelsParams({
        shadows: 0,
        midtones: 128,
        highlights: 255,
        outputBlack: 0,
        outputWhite: 255,
      }),
    ).toBe(true)
    expect(isLevelsParams({ channels: {} })).toBe(false)
    expect(isSharpenParams({ amount: 10, radius: 1, threshold: 4, protectColor: true })).toBe(true)
    expect(isSharpenParams({ threshold: 10, invert: false })).toBe(false)
    expect(isWholeParamsForKind('threshold', { threshold: 1, invert: true })).toBe(true)
    expect(isWholeParamsForKind('curves', { threshold: 1, invert: true })).toBe(false)
  })

  it('returns the same reference on single-key/whole no-ops with a dev trace', () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => undefined)
    try {
      const hslLayer = createDefaultImageEffectLayer('hsl')
      const thresholdLayer = createDefaultImageEffectLayer('threshold')

      expect(updateEffectLayerParam(thresholdLayer, 'hue', 10)).toBe(thresholdLayer)
      expect(updateEffectLayerParams(hslLayer, { threshold: 10, invert: false })).toBe(hslLayer)
      expect(updateEffectLayerParams(thresholdLayer, { channels: {} })).toBe(thresholdLayer)
      expect(debug).toHaveBeenCalled()
    } finally {
      debug.mockRestore()
    }
  })

  it('applies matching single-key and whole updates as new references', () => {
    const hslLayer = createDefaultImageEffectLayer('hsl')
    const nextHsl = updateEffectLayerParam(hslLayer, 'lightness', 12)
    expect(nextHsl).not.toBe(hslLayer)
    expect(nextHsl).toMatchObject({ kind: 'hsl', params: { lightness: 12 } })

    const thresholdLayer = createDefaultImageEffectLayer('threshold')
    const nextThreshold = updateEffectLayerParams(thresholdLayer, {
      threshold: 200,
      invert: true,
    })
    expect(nextThreshold).not.toBe(thresholdLayer)
    expect(nextThreshold).toMatchObject({
      kind: 'threshold',
      params: { threshold: 200, invert: true },
    })
  })

  it('applies full-kind single-key updates with sanitizing clones', () => {
    const thresholdLayer = createDefaultImageEffectLayer('threshold')
    const nextThreshold = updateEffectLayerParam(thresholdLayer, 'threshold', 200)
    expect(nextThreshold).not.toBe(thresholdLayer)
    expect(nextThreshold).toMatchObject({ kind: 'threshold', params: { threshold: 200 } })

    const nextInvert = updateEffectLayerParam(thresholdLayer, 'invert', true)
    expect(nextInvert).not.toBe(thresholdLayer)
    expect(nextInvert).toMatchObject({
      kind: 'threshold',
      params: { threshold: 128, invert: true },
    })

    // Sanitizing clone: NaN falls back to the threshold default, like whole-params.
    expect(updateEffectLayerParam(thresholdLayer, 'threshold', Number.NaN)).toMatchObject({
      kind: 'threshold',
      params: { threshold: 128, invert: false },
    })

    const levelsLayer = createDefaultImageEffectLayer('levels')
    const nextLevels = updateEffectLayerParam(levelsLayer, 'shadows', 10)
    expect(nextLevels).not.toBe(levelsLayer)
    expect(nextLevels).toMatchObject({
      kind: 'levels',
      params: { shadows: 10, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 },
    })

    const sharpenLayer = createDefaultImageEffectLayer('sharpen')
    expect(updateEffectLayerParam(sharpenLayer, 'amount', 80)).toMatchObject({
      kind: 'sharpen',
      params: { amount: 80, radius: 1, threshold: 4, protectColor: true },
    })
    // Sharpen numerics clamp through the same registry clone as whole-params.
    expect(updateEffectLayerParam(sharpenLayer, 'amount', 240)).toMatchObject({
      kind: 'sharpen',
      params: { amount: 200 },
    })
    expect(updateEffectLayerParam(sharpenLayer, 'protectColor', false)).toMatchObject({
      kind: 'sharpen',
      params: { protectColor: false },
    })

    const curvesLayer = createDefaultImageEffectLayer('curves')
    const channels = { rgb: [{ id: 'midtone', input: 50, output: 60 }] }
    const nextCurves = updateEffectLayerParam(curvesLayer, 'channels', channels)
    expect(nextCurves).not.toBe(curvesLayer)
    expect(nextCurves).toMatchObject({
      kind: 'curves',
      params: { channels: { rgb: [{ id: 'midtone', input: 50, output: 60 }] } },
    })
    // No input aliasing: mutating the caller object never leaks into layer state.
    channels.rgb[0]!.output = 10
    expect(nextCurves).toMatchObject({
      kind: 'curves',
      params: { channels: { rgb: [{ id: 'midtone', input: 50, output: 60 }] } },
    })
  })

  it('returns the same reference for cross-kind single-key mismatches', () => {
    const debug = vi.spyOn(console, 'debug').mockImplementation(() => undefined)
    try {
      const thresholdLayer = createDefaultImageEffectLayer('threshold')
      const levelsLayer = createDefaultImageEffectLayer('levels')
      const hslLayer = createDefaultImageEffectLayer('hsl')

      expect(updateEffectLayerParam(levelsLayer, 'invert', true)).toBe(levelsLayer)
      expect(updateEffectLayerParam(hslLayer, 'invert', true)).toBe(hslLayer)
      expect(updateEffectLayerParam(thresholdLayer, 'shadows', 10)).toBe(thresholdLayer)
      expect(debug).toHaveBeenCalled()
    } finally {
      debug.mockRestore()
    }
  })

  it('fans narrow patches out to the dual update paths with the same no-op semantics', () => {
    const hslLayer = createDefaultImageEffectLayer('hsl')
    expect(
      updateEffectLayerPatch(hslLayer, { kind: 'single-param', key: 'lightness', value: 12 }),
    ).toMatchObject({ kind: 'hsl', params: { lightness: 12 } })

    const thresholdLayer = createDefaultImageEffectLayer('threshold')
    expect(
      updateEffectLayerPatch(thresholdLayer, {
        kind: 'whole-params',
        params: { threshold: 200, invert: true },
      }),
    ).toMatchObject({ kind: 'threshold', params: { threshold: 200, invert: true } })

    // Mismatched patches keep the same reference, exactly like the dual APIs.
    expect(
      updateEffectLayerPatch(thresholdLayer, { kind: 'single-param', key: 'lightness', value: 12 }),
    ).toBe(thresholdLayer)
    expect(
      updateEffectLayerPatch(hslLayer, {
        kind: 'whole-params',
        params: { threshold: 200, invert: true },
      }),
    ).toBe(hslLayer)
  })

  it('fans full-kind single-param patches to the sanitizing single-key path', () => {
    const thresholdLayer = createDefaultImageEffectLayer('threshold')
    expect(
      updateEffectLayerPatch(thresholdLayer, { kind: 'single-param', key: 'invert', value: true }),
    ).toMatchObject({ kind: 'threshold', params: { threshold: 128, invert: true } })

    const levelsLayer = createDefaultImageEffectLayer('levels')
    expect(
      updateEffectLayerPatch(levelsLayer, { kind: 'single-param', key: 'shadows', value: 10 }),
    ).toMatchObject({ kind: 'levels', params: { shadows: 10 } })

    const sharpenLayer = createDefaultImageEffectLayer('sharpen')
    expect(
      updateEffectLayerPatch(sharpenLayer, { kind: 'single-param', key: 'amount', value: 80 }),
    ).toMatchObject({ kind: 'sharpen', params: { amount: 80 } })

    const curvesLayer = createDefaultImageEffectLayer('curves')
    expect(
      updateEffectLayerPatch(curvesLayer, {
        kind: 'single-param',
        key: 'channels',
        value: { rgb: [{ id: 'midtone', input: 50, output: 60 }] },
      }),
    ).toMatchObject({
      kind: 'curves',
      params: { channels: { rgb: [{ id: 'midtone', input: 50, output: 60 }] } },
    })
  })

  it('applies full-kind single-key commands through the stack', () => {
    const thresholdStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'threshold',
    })
    const nextStack = applyLayerCommand(thresholdStack, {
      type: 'update-selected-param',
      key: 'invert',
      value: true,
    })
    expect(getSelectedEffect(nextStack)).toMatchObject({
      kind: 'threshold',
      params: { threshold: 128, invert: true },
    })

    const levelsStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'levels',
    })
    const nextLevelsStack = applyLayerCommand(levelsStack, {
      type: 'update-selected-param',
      key: 'shadows',
      value: 10,
    })
    expect(getSelectedEffect(nextLevelsStack)).toMatchObject({
      kind: 'levels',
      params: { shadows: 10 },
    })
  })
})

describe('retouchLayerStack capability matrix', () => {
  it('disables copy/merge/remove on the protected source layer', () => {
    const stack = createInitialLayerStack()

    expect(canCopySelectedLayer(stack)).toBe(false)
    expect(canMergeSelectedLayer(stack)).toBe(false)
    expect(canRemoveSelectedLayer(stack)).toBe(false)
  })

  it('enables copy/remove but not merge for a lone editable above source', () => {
    const hslStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'hsl',
    })

    expect(canCopySelectedLayer(hslStack)).toBe(true)
    expect(canMergeSelectedLayer(hslStack)).toBe(false)
    expect(canRemoveSelectedLayer(hslStack)).toBe(true)

    const paintStack = applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' })

    expect(canCopySelectedLayer(paintStack)).toBe(true)
    expect(canMergeSelectedLayer(paintStack)).toBe(false)
    expect(canRemoveSelectedLayer(paintStack)).toBe(true)
  })

  it('enables merge only when the selected layer has an editable layer below', () => {
    const twoEditable = applyLayerCommand(
      applyLayerCommand(createInitialLayerStack(), { type: 'add-paint' }),
      { type: 'add-paint' },
    )
    expect(canMergeSelectedLayer(twoEditable)).toBe(true)
    expect(canCopySelectedLayer(twoEditable)).toBe(true)
    expect(canRemoveSelectedLayer(twoEditable)).toBe(true)

    const selectedSource = applyLayerCommand(twoEditable, {
      type: 'select-layer',
      layerId: 'source',
    })
    expect(canCopySelectedLayer(selectedSource)).toBe(false)
    expect(canMergeSelectedLayer(selectedSource)).toBe(false)
    expect(canRemoveSelectedLayer(selectedSource)).toBe(false)

    const hslOnly = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-retouch',
      kind: 'hsl',
    })
    const hslSelectedSource = applyLayerCommand(hslOnly, {
      type: 'select-layer',
      layerId: 'source',
    })
    expect(canMergeSelectedLayer(hslSelectedSource)).toBe(false)
  })

  it('matches the copy/merge/remove command no-op behavior', () => {
    const sourceStack = createInitialLayerStack()
    expect(applyLayerCommand(sourceStack, { type: 'copy-selected' })).toBe(sourceStack)
    expect(applyLayerCommand(sourceStack, { type: 'merge-selected-down' })).toBe(sourceStack)
    expect(applyLayerCommand(sourceStack, { type: 'remove', layerId: 'source' })).toBe(sourceStack)
  })
})
