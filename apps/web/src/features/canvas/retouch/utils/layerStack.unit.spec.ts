import { describe, expect, it } from 'vitest'
import type { RetouchLayerStack } from './layerStack'
import {
  applyLayerCommand,
  createDefaultImageEffectLayer,
  createInitialLayerStack,
  getSelectedEffect,
  selectAfterRemove,
} from './layerStack'

describe('retouchLayerStack', () => {
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
      name: 'Sharpen 1',
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
