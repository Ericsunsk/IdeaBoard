import { describe, expect, it } from 'vitest'
import { useRetouchSession } from './useRetouchSession'
import { flattenVisibleRetouchEffectLayers } from './processing/layerEffects.ts'

describe('useRetouchSession', () => {
  it('keeps retouch selection transient until a parameter changes', () => {
    const session = useRetouchSession()

    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('hsl')
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])

    session.selectImageEffectLayer('threshold')

    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('threshold')
    expect(session.retouchLayers.value).toHaveLength(1)

    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: false })
    expect(session.retouchLayers.value).toHaveLength(1)

    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: true })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold')).toBe(true)
  })

  it('does not leave a layer when switching tools without editing', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 0)
    session.selectImageEffectLayer('curves')
    session.updateSelectedRetouchEffectParams({ channels: {} })
    session.selectImageEffectLayer('levels')
    session.updateSelectedRetouchEffectParams({
      shadows: 0,
      midtones: 128,
      highlights: 255,
      outputBlack: 0,
      outputWhite: 255,
    })
    session.selectImageEffectLayer('sharpen')
    session.updateSelectedRetouchEffectParams({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: true,
    })

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('sharpen')
  })

  it('selects source layers and clears the pending retouch', () => {
    const session = useRetouchSession()

    session.selectRetouchLayer('source')

    expect(session.selectedRetouchEffectLayer.value).toBeNull()
  })

  it('adds paint layers above the selected editable layer', () => {
    const session = useRetouchSession()

    session.addRetouchLayer()
    session.addRetouchLayer()

    const [secondPaintLayer, firstPaintLayer, sourceLayer] = session.retouchLayers.value
    expect(secondPaintLayer?.type).toBe('paint')
    expect(secondPaintLayer?.name).toBe('Layer 2')
    expect(secondPaintLayer?.type === 'paint' ? secondPaintLayer.content : null).toEqual({
      kind: 'empty',
    })
    expect(firstPaintLayer?.type).toBe('paint')
    expect(firstPaintLayer?.name).toBe('Layer 1')
    expect(sourceLayer?.type).toBe('source')
    expect(session.selectedRetouchLayerId.value).toBe(secondPaintLayer?.id)
  })

  it('adds paint layers above the source when source is selected', () => {
    const session = useRetouchSession()

    session.selectRetouchLayer('source')
    session.addRetouchLayer()

    const [paintLayer, sourceLayer] = session.retouchLayers.value
    expect(paintLayer?.type).toBe('paint')
    expect(paintLayer?.name).toBe('Layer 1')
    expect(sourceLayer?.type).toBe('source')
    expect(session.selectedRetouchLayerId.value).toBe(paintLayer?.id)
  })

  it('copies editable layers with unique ids and names', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 10)
    session.copyRetouchLayer()

    const [copyLayer, originalLayer] = session.retouchLayers.value
    expect(copyLayer?.type).toBe('effect')
    expect(copyLayer?.id).not.toBe(originalLayer?.id)
    expect(copyLayer?.name).toBe('Hue/Saturation 1 copy')
    expect(originalLayer?.type).toBe('effect')
    expect(session.selectedRetouchLayerId.value).toBe(copyLayer?.id)
  })

  it('copies paint layers as independent empty raster layers', () => {
    const session = useRetouchSession()

    session.addRetouchLayer()
    session.copyRetouchLayer()

    const [copyLayer, originalLayer] = session.retouchLayers.value
    expect(copyLayer?.type).toBe('paint')
    expect(copyLayer?.name).toBe('Layer 1 copy')
    expect(copyLayer?.id).not.toBe(originalLayer?.id)
    expect(copyLayer?.type === 'paint' ? copyLayer.content : null).toEqual({ kind: 'empty' })
  })

  it('merges the selected editable layer down and preserves visible effects', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 10)
    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: true })
    session.selectImageEffectLayer('hsl')
    session.mergeRetouchLayer()

    const mergedLayer = session.retouchLayers.value[0]
    expect(mergedLayer?.type).toBe('merged')
    expect(mergedLayer?.name).toBe('Merged layer 1')
    expect(session.selectedRetouchLayerId.value).toBe(mergedLayer?.id)
    expect(
      flattenVisibleRetouchEffectLayers(session.retouchLayers.value).map((layer) => layer.kind),
    ).toEqual(['hsl', 'threshold'])
  })

  it('removes selected editable layers and selects the nearest editable fallback', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: true })
    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 10)

    session.removeRetouchLayer('retouch:hsl')

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual([
      'retouch:threshold',
      'source',
    ])
    expect(session.selectedRetouchLayerId.value).toBe('retouch:threshold')
  })

  it('keeps source layers protected from copy, merge, and remove actions', () => {
    const session = useRetouchSession()
    session.selectRetouchLayer('source')

    session.copyRetouchLayer()
    session.mergeRetouchLayer()
    session.removeRetouchLayer('source')

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchLayerId.value).toBe('source')
  })

  it('updates selected retouch layer params through the explicit whole-params API', () => {
    const session = useRetouchSession()
    const curvesParams = {
      channels: {
        rgb: [{ id: 'midtone', input: 50, output: 60 }],
      },
    }

    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchEffectParams({ threshold: Number.NaN, invert: true })

    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'threshold',
      params: { threshold: 128, invert: true },
    })

    session.selectImageEffectLayer('levels')
    session.updateSelectedRetouchEffectParams({
      shadows: 300,
      midtones: Number.NaN,
      highlights: 10,
      outputBlack: 250,
      outputWhite: 100,
    })

    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'levels',
      params: {
        shadows: 9,
        midtones: 10,
        highlights: 10,
        outputBlack: 250,
        outputWhite: 251,
      },
    })

    session.selectImageEffectLayer('curves')
    session.updateSelectedRetouchEffectParams(curvesParams)
    curvesParams.channels.rgb[0]!.output = 10

    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'curves',
      params: {
        channels: {
          rgb: [{ id: 'midtone', input: 50, output: 60 }],
        },
      },
    })

    session.selectImageEffectLayer('sharpen')
    session.updateSelectedRetouchEffectParams({
      amount: 80,
      radius: 1.5,
      threshold: 7,
      protectColor: false,
    })

    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: {
        amount: 80,
        radius: 1.5,
        threshold: 7,
        protectColor: false,
      },
    })
  })

  it('keeps simple retouch params on the field-level API', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 25)
    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'hsl',
      params: { lightness: 25 },
    })
  })
})
