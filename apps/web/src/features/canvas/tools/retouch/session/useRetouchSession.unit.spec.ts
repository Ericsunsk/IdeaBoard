import { describe, expect, it } from 'vitest'
import { useRetouchSession } from './useRetouchSession'
import type { ImageRetouchKind } from '../core/model.ts'
import { createDefaultImageEffectLayer } from '../core/layers/layerStack.ts'
import {
  getImageRetouchDefaultParams,
  isNeutralImageRetouch,
  normalizeImageEffectLayer,
} from '../core/definitions.ts'
import { flattenVisibleRetouchEffectLayers } from '../core/layers/layerEffects.ts'

describe('useRetouchSession', () => {
  it('keeps retouch selection transient until a non-neutral parameter change', () => {
    const session = useRetouchSession()

    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('hsl')
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])

    session.selectImageEffectLayer('threshold')

    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('threshold')
    expect(session.retouchLayers.value).toHaveLength(1)

    // Threshold is never neutral (any threshold posterizes), so even its
    // defaults commit on the first edit (see the neutral-gate test below).
    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: false })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold')).toBe(true)

    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: true })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold')).toBe(true)
  })

  it('keeps the pending neutral gate consistent with the definition predicates', () => {
    const defaultEdits: Array<{
      kind: ImageRetouchKind
      edit: (session: ReturnType<typeof useRetouchSession>) => void
      staysPending: boolean
    }> = [
      {
        kind: 'hsl',
        edit: (session) => session.updateSelectedRetouchLayerParam('lightness', 0),
        staysPending: true,
      },
      {
        kind: 'threshold',
        edit: (session) =>
          session.updateSelectedRetouchEffectParams({ threshold: 128, invert: false }),
        // Threshold is never neutral: the definition predicate stays false for
        // defaults, so the first edit commits instead of staying pending.
        staysPending: false,
      },
      {
        kind: 'curves',
        edit: (session) => session.updateSelectedRetouchEffectParams({ channels: {} }),
        staysPending: true,
      },
      {
        kind: 'levels',
        edit: (session) =>
          session.updateSelectedRetouchEffectParams({
            shadows: 0,
            midtones: 128,
            highlights: 255,
            outputBlack: 0,
            outputWhite: 255,
          }),
        staysPending: true,
      },
      {
        kind: 'sharpen',
        edit: (session) =>
          session.updateSelectedRetouchEffectParams({
            amount: 0,
            radius: 1,
            threshold: 4,
            protectColor: true,
          }),
        staysPending: true,
      },
    ]

    for (const { kind, edit, staysPending } of defaultEdits) {
      const predicateNeutral = isNeutralImageRetouch(
        normalizeImageEffectLayer(createDefaultImageEffectLayer(kind)),
      )
      expect(predicateNeutral).toBe(staysPending)

      const session = useRetouchSession()
      session.selectImageEffectLayer(kind)
      expect(session.retouchLayers.value).toHaveLength(1)
      edit(session)
      expect(session.retouchLayers.value).toHaveLength(staysPending ? 1 : 2)
    }

    // A non-neutral first edit always commits, and the stored params keep the
    // definition defaults object untouched (structuredClone writeback).
    const defaultsBefore = getImageRetouchDefaultParams('hsl')
    const hslSession = useRetouchSession()
    hslSession.selectImageEffectLayer('hsl')
    hslSession.updateSelectedRetouchLayerParam('lightness', 10)
    expect(hslSession.retouchLayers.value).toHaveLength(2)
    expect(getImageRetouchDefaultParams('hsl')).toEqual(defaultsBefore)
  })

  it('leaves no layer behind neutral edits, while threshold defaults still commit', () => {
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

    // Threshold is never neutral (any threshold posterizes, matching the
    // engine plan compiler), so even its defaults commit on first edit.
    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: false })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold')).toBe(true)
  })

  it('preserves single-key sharpen draft edits until amount commits the complete draft', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('sharpen')

    session.updateSelectedRetouchLayerParam('radius', 3)
    expect(session.selectedRetouchEffectLayer.value?.params).toMatchObject({ radius: 3 })
    session.updateSelectedRetouchLayerParam('threshold', 12)
    expect(session.selectedRetouchEffectLayer.value?.params).toMatchObject({
      radius: 3,
      threshold: 12,
    })
    session.updateSelectedRetouchLayerParam('protectColor', false)

    const draft = { amount: 0, radius: 3, threshold: 12, protectColor: false }
    expect(session.pendingEffectLayer.value?.params).toEqual(draft)
    expect(session.selectedRetouchEffectLayer.value?.params).toEqual(draft)
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchLayerId.value).toBe('source')

    session.updateSelectedRetouchLayerParam('amount', 80)

    expect(session.pendingEffectLayer.value).toBeNull()
    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      id: 'retouch:sharpen',
      kind: 'sharpen',
      params: { ...draft, amount: 80 },
    })
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual([
      'retouch:sharpen',
      'source',
    ])
    expect(session.selectedRetouchLayerId.value).toBe('retouch:sharpen')
  })

  it('preserves an independent whole-params sharpen draft until a later amount edit commits', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('sharpen')
    const params = { amount: 0, radius: 3, threshold: 12, protectColor: false }

    session.updateSelectedRetouchEffectParams(params)
    params.radius = 1

    const draft = { amount: 0, radius: 3, threshold: 12, protectColor: false }
    expect(session.pendingEffectLayer.value?.params).toEqual(draft)
    expect(session.selectedRetouchEffectLayer.value?.params).toEqual(draft)
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchLayerId.value).toBe('source')

    session.updateSelectedRetouchLayerParam('amount', 80)

    expect(session.pendingEffectLayer.value).toBeNull()
    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: { ...draft, amount: 80 },
    })
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual([
      'retouch:sharpen',
      'source',
    ])
  })

  it.each(['single-key', 'whole-params'] as const)(
    'normalizes invalid neutral sharpen inputs through the %s API while preserving valid values',
    (api) => {
      const session = useRetouchSession()
      session.selectImageEffectLayer('sharpen')

      if (api === 'single-key') {
        session.updateSelectedRetouchLayerParam('amount', -20)
        session.updateSelectedRetouchLayerParam('radius', 8)
        session.updateSelectedRetouchLayerParam('threshold', Number.NaN)
        session.updateSelectedRetouchLayerParam('protectColor', false)
      } else {
        session.updateSelectedRetouchEffectParams({
          amount: -20,
          radius: 8,
          threshold: Number.NaN,
          protectColor: false,
        })
      }

      const draft = { amount: 0, radius: 5, threshold: 4, protectColor: false }
      expect(session.pendingEffectLayer.value?.params).toEqual(draft)
      expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])

      session.updateSelectedRetouchLayerParam('amount', 80)

      expect(session.pendingEffectLayer.value).toBeNull()
      expect(session.selectedRetouchEffectLayer.value?.params).toEqual({ ...draft, amount: 80 })
    },
  )

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
    expect(secondPaintLayer?.name).toBe('canvas.retouch.layerNames.paint 2')
    expect(secondPaintLayer?.type === 'paint' ? secondPaintLayer.content : null).toEqual({
      kind: 'empty',
    })
    expect(firstPaintLayer?.type).toBe('paint')
    expect(firstPaintLayer?.name).toBe('canvas.retouch.layerNames.paint 1')
    expect(sourceLayer?.type).toBe('source')
    expect(session.selectedRetouchLayerId.value).toBe(secondPaintLayer?.id)
  })

  it('adds paint layers above the source when source is selected', () => {
    const session = useRetouchSession()

    session.selectRetouchLayer('source')
    session.addRetouchLayer()

    const [paintLayer, sourceLayer] = session.retouchLayers.value
    expect(paintLayer?.type).toBe('paint')
    expect(paintLayer?.name).toBe('canvas.retouch.layerNames.paint 1')
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
    expect(copyLayer?.name).toBe(
      'canvas.retouch.effects.hsl 1 canvas.retouch.layerNames.copySuffix',
    )
    expect(originalLayer?.type).toBe('effect')
    expect(session.selectedRetouchLayerId.value).toBe(copyLayer?.id)
  })

  it('copies paint layers as independent empty raster layers', () => {
    const session = useRetouchSession()

    session.addRetouchLayer()
    session.copyRetouchLayer()

    const [copyLayer, originalLayer] = session.retouchLayers.value
    expect(copyLayer?.type).toBe('paint')
    expect(copyLayer?.name).toBe(
      'canvas.retouch.layerNames.paint 1 canvas.retouch.layerNames.copySuffix',
    )
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
    expect(mergedLayer?.name).toBe('canvas.retouch.layerNames.merged 1')
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

  it('commits full-kind single-key edits with normalization', () => {
    const thresholdSession = useRetouchSession()
    thresholdSession.selectImageEffectLayer('threshold')
    thresholdSession.updateSelectedRetouchLayerParam('threshold', 200)
    expect(
      thresholdSession.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold'),
    ).toBe(true)
    expect(thresholdSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'threshold',
      params: { threshold: 200, invert: false },
    })

    const invertSession = useRetouchSession()
    invertSession.selectImageEffectLayer('threshold')
    invertSession.updateSelectedRetouchLayerParam('invert', true)
    expect(invertSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'threshold',
      params: { threshold: 128, invert: true },
    })

    const levelsSession = useRetouchSession()
    levelsSession.selectImageEffectLayer('levels')
    levelsSession.updateSelectedRetouchLayerParam('shadows', 10)
    expect(levelsSession.retouchLayers.value.some((layer) => layer.id === 'retouch:levels')).toBe(
      true,
    )
    expect(levelsSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'levels',
      params: { shadows: 10 },
    })

    const sharpenSession = useRetouchSession()
    sharpenSession.selectImageEffectLayer('sharpen')
    sharpenSession.updateSelectedRetouchLayerParam('amount', 80)
    expect(sharpenSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: { amount: 80 },
    })
    sharpenSession.updateSelectedRetouchLayerParam('protectColor', false)
    expect(sharpenSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: { amount: 80, protectColor: false },
    })

    const curvesSession = useRetouchSession()
    curvesSession.selectImageEffectLayer('curves')
    curvesSession.updateSelectedRetouchLayerParam('channels', {
      rgb: [{ id: 'midtone', input: 50, output: 60 }],
    })
    expect(curvesSession.retouchLayers.value.some((layer) => layer.id === 'retouch:curves')).toBe(
      true,
    )
    expect(curvesSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'curves',
      params: { channels: { rgb: [{ id: 'midtone', input: 50, output: 60 }] } },
    })
  })

  it('keeps mismatched single-key and whole-params packets pending without committing', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchLayerParam('lightness', 25)

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('threshold')

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchEffectParams({ threshold: 128, invert: true })

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('hsl')
  })

  it('keeps cross-kind single-key mismatches pending without committing', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('invert', true)

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('hsl')

    session.selectImageEffectLayer('levels')
    session.updateSelectedRetouchLayerParam('invert', true)

    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(session.selectedRetouchEffectLayer.value?.kind).toBe('levels')
  })

  it('commits matching packets through both dual APIs with normalization', () => {
    const session = useRetouchSession()

    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('hue', 10)
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:hsl')).toBe(true)

    const sharpenSession = useRetouchSession()
    sharpenSession.selectImageEffectLayer('sharpen')
    sharpenSession.updateSelectedRetouchEffectParams({
      amount: 240,
      radius: 8,
      threshold: Number.NaN,
      protectColor: false,
    })
    expect(sharpenSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: { amount: 200, radius: 5, threshold: 4, protectColor: false },
    })
    expect(sharpenSession.retouchLayers.value.some((layer) => layer.id === 'retouch:sharpen')).toBe(
      true,
    )
  })

  it('keeps the staged pending id continuous with the committed layer id', () => {
    // The histogram cache keys on the target id across the pending -> committed
    // transition (chromeAdapter stability contract): staging must reuse the
    // deterministic `retouch:<kind>` id that ensure-retouch commits.
    const session = useRetouchSession()

    session.selectImageEffectLayer('threshold')
    const pendingId = session.selectedRetouchEffectLayer.value?.id
    expect(pendingId).toBe('retouch:threshold')

    session.updateSelectedRetouchEffectParams({ threshold: 200, invert: false })

    expect(session.selectedRetouchEffectLayer.value?.id).toBe(pendingId)
    expect(session.retouchLayers.value.map((layer) => layer.id)).toEqual([
      'retouch:threshold',
      'source',
    ])
  })

  it('keeps committed layers uniquely identified through copy and merge snapshots', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchLayerParam('lightness', 10)
    session.selectImageEffectLayer('threshold')
    session.updateSelectedRetouchEffectParams({ threshold: 200, invert: false })

    // Re-selecting the existing kind routes to the committed layer, then copy
    // and merge must keep every layer id unique with counted names.
    session.selectImageEffectLayer('hsl')
    session.copyRetouchLayer()
    expect(session.selectedRetouchLayerId.value).not.toBe('retouch:hsl')
    session.mergeRetouchLayer()

    const layers = session.retouchLayers.value
    const ids = layers.map((layer) => layer.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(layers).toHaveLength(3)
    expect(layers[0]).toMatchObject({ type: 'merged', name: 'canvas.retouch.layerNames.merged 1' })
    expect(session.selectedRetouchLayerId.value).toBe(layers[0]?.id)
    expect(
      flattenVisibleRetouchEffectLayers(session.retouchLayers.value).map((layer) => layer.kind),
    ).toEqual(['hsl', 'hsl', 'threshold'])
  })

  it('routes narrow patches through the same pending/commit transitions as the dual APIs', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('hsl')
    session.updateSelectedRetouchPatch({ kind: 'single-param', key: 'lightness', value: 25 })
    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'hsl',
      params: { lightness: 25 },
    })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:hsl')).toBe(true)

    // Mismatched patches stay pending without committing, like the dual APIs.
    const pendingSession = useRetouchSession()
    pendingSession.selectImageEffectLayer('threshold')
    pendingSession.updateSelectedRetouchPatch({
      kind: 'single-param',
      key: 'lightness',
      value: 25,
    })
    expect(pendingSession.retouchLayers.value.map((layer) => layer.id)).toEqual(['source'])
    expect(pendingSession.selectedRetouchEffectLayer.value?.kind).toBe('threshold')

    pendingSession.updateSelectedRetouchPatch({
      kind: 'whole-params',
      params: { threshold: 200, invert: false },
    })
    expect(
      pendingSession.retouchLayers.value.some((layer) => layer.id === 'retouch:threshold'),
    ).toBe(true)
  })

  it('routes full-kind narrow patches through the same pending/commit transitions', () => {
    const session = useRetouchSession()
    session.selectImageEffectLayer('sharpen')
    session.updateSelectedRetouchPatch({ kind: 'single-param', key: 'amount', value: 80 })
    expect(session.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'sharpen',
      params: { amount: 80 },
    })
    expect(session.retouchLayers.value.some((layer) => layer.id === 'retouch:sharpen')).toBe(true)

    const levelsSession = useRetouchSession()
    levelsSession.selectImageEffectLayer('levels')
    levelsSession.updateSelectedRetouchPatch({ kind: 'single-param', key: 'shadows', value: 10 })
    expect(levelsSession.selectedRetouchEffectLayer.value).toMatchObject({
      kind: 'levels',
      params: { shadows: 10 },
    })
    expect(levelsSession.retouchLayers.value.some((layer) => layer.id === 'retouch:levels')).toBe(
      true,
    )
  })
})
