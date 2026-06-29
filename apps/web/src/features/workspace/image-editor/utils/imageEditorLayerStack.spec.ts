import { describe, expect, it } from 'vitest'
import type { ImageEditorLayerStack } from './imageEditorLayerStack'
import {
  applyLayerCommand,
  createInitialLayerStack,
  getSelectedAdjustment,
  selectAfterRemove,
} from './imageEditorLayerStack'

describe('imageEditorLayerStack', () => {
  it('creates the initial stack with independent default adjustment params', () => {
    const firstStack = createInitialLayerStack()
    const secondStack = createInitialLayerStack()
    const firstHslLayer = firstStack.layers[0]
    const secondHslLayer = secondStack.layers[0]

    expect(firstStack.selectedLayerId).toBe('adjustment:hsl')
    expect(firstStack.layers.map((layer) => layer.id)).toEqual(['adjustment:hsl', 'source'])
    expect(firstHslLayer?.type).toBe('adjustment')
    expect(secondHslLayer?.type).toBe('adjustment')
    if (
      firstHslLayer?.type !== 'adjustment' ||
      firstHslLayer.kind !== 'hsl' ||
      secondHslLayer?.type !== 'adjustment' ||
      secondHslLayer.kind !== 'hsl'
    ) {
      return
    }

    firstHslLayer.params.brightness = 45

    expect(secondHslLayer.params).toEqual({
      hue: 0,
      saturation: 0,
      brightness: 0,
      contrast: 0,
    })
  })

  it('copies adjustment params without sharing nested curve references', () => {
    const stackWithCurves = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-adjustment',
      kind: 'curves',
    })
    const updatedStack = applyLayerCommand(stackWithCurves, {
      type: 'update-selected-adjustment-params',
      params: {
        channels: {
          rgb: [{ id: 'midtone', input: 50, output: 60 }],
        },
      },
    })

    const copiedStack = applyLayerCommand(updatedStack, { type: 'copy-selected' })
    const curvesLayers = copiedStack.layers.filter(
      (layer) => layer.type === 'adjustment' && layer.kind === 'curves',
    )
    const [copyLayer, originalLayer] = curvesLayers
    expect(copyLayer?.type).toBe('adjustment')
    expect(originalLayer?.type).toBe('adjustment')
    if (copyLayer?.type !== 'adjustment' || originalLayer?.type !== 'adjustment') return
    expect(copyLayer.kind).toBe('curves')
    expect(originalLayer.kind).toBe('curves')
    if (copyLayer.kind !== 'curves' || originalLayer.kind !== 'curves') return

    copyLayer.params.channels.rgb![0]!.output = 10

    expect(originalLayer.params.channels.rgb?.[0]?.output).toBe(60)
  })

  it('selects the nearest editable fallback after removing the selected layer', () => {
    const stack = applyLayerCommand(
      applyLayerCommand(createInitialLayerStack(), { type: 'ensure-adjustment', kind: 'threshold' }),
      { type: 'ensure-adjustment', kind: 'hsl' },
    )

    const nextStack = applyLayerCommand(stack, { type: 'remove', layerId: 'adjustment:hsl' })

    expect(nextStack.layers.map((layer) => layer.id)).toEqual(['adjustment:threshold', 'source'])
    expect(nextStack.selectedLayerId).toBe('adjustment:threshold')
    expect(selectAfterRemove(nextStack.layers, 0)).toBe('adjustment:threshold')
  })

  it('protects the source layer from destructive commands', () => {
    const sourceSelectedStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'select-layer',
      layerId: 'source',
    })

    const copiedStack = applyLayerCommand(sourceSelectedStack, { type: 'copy-selected' })
    const mergedStack = applyLayerCommand(copiedStack, { type: 'merge-selected-down' })
    const removedStack = applyLayerCommand(mergedStack, { type: 'remove', layerId: 'source' })

    expect(removedStack.layers.map((layer) => layer.id)).toEqual(['adjustment:hsl', 'source'])
    expect(removedStack.selectedLayerId).toBe('source')
  })

  it('creates and selects the requested adjustment layer command', () => {
    const nextStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-adjustment',
      kind: 'threshold',
    })

    expect(nextStack.selectedLayerId).toBe('adjustment:threshold')
    expect(getSelectedAdjustment(nextStack)).toMatchObject({
      id: 'adjustment:threshold',
      kind: 'threshold',
    })
    expect(nextStack.layers.map((layer) => layer.id)).toEqual([
      'adjustment:hsl',
      'adjustment:threshold',
      'source',
    ])
  })

  it('falls back to source selection when the last editable layer is removed', () => {
    const nextStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'remove',
      layerId: 'adjustment:hsl',
    })

    expect(nextStack.layers.map((layer) => layer.id)).toEqual(['source'])
    expect(nextStack.selectedLayerId).toBe('source')
    expect(getSelectedAdjustment(nextStack)).toBeNull()
  })

  it('does not share whole-param update input references with layer state', () => {
    const params = {
      channels: {
        rgb: [{ id: 'midtone', input: 50, output: 60 }],
      },
    }
    const curvesStack = applyLayerCommand(createInitialLayerStack(), {
      type: 'ensure-adjustment',
      kind: 'curves',
    })

    const nextStack = applyLayerCommand(curvesStack, {
      type: 'update-selected-adjustment-params',
      params,
    })
    params.channels.rgb[0]!.output = 10

    expect(getSelectedAdjustment(nextStack)).toMatchObject({
      kind: 'curves',
      params: {
        channels: {
          rgb: [{ id: 'midtone', input: 50, output: 60 }],
        },
      },
    })
  })

  it('leaves incompatible param commands unchanged', () => {
    const stack: ImageEditorLayerStack = createInitialLayerStack()

    const nextStack = applyLayerCommand(stack, {
      type: 'update-selected-adjustment-params',
      params: { threshold: 20, invert: false },
    })

    expect(nextStack.layers).toEqual(stack.layers)
    expect(nextStack.selectedLayerId).toBe(stack.selectedLayerId)
  })
})
