import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../types'
import {
  applyImageAdjustments,
  applyImageAdjustmentsInPlace,
  compileImageAdjustmentPlan,
  executeImageAdjustmentPlan,
  executeImageAdjustmentPlanStepRange,
  hasEffectiveImageAdjustments,
  type ImageAdjustmentPlan,
} from './imageAdjustmentEngine'
import { normalizeImageAdjustmentLayer } from './imageAdjustmentDefinitions'

function makeImageData(values: number[]) {
  class TestImageData {
    data: Uint8ClampedArray
    width: number
    height: number

    constructor(data: Uint8ClampedArray, width: number, height: number) {
      this.data = data
      this.width = width
      this.height = height
    }
  }
  return new TestImageData(new Uint8ClampedArray(values), values.length / 4, 1) as ImageData
}

function pixelValues(imageData: ImageData) {
  return Array.from(imageData.data)
}

function makeLayer(
  layer: ImageEditorAdjustmentLayer,
  overrides: Partial<ImageEditorAdjustmentLayer> = {},
): ImageEditorAdjustmentLayer {
  return { ...layer, ...overrides } as ImageEditorAdjustmentLayer
}

const hslLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:hsl',
  type: 'adjustment',
  kind: 'hsl',
  name: '色相/饱和度 1',
  visible: true,
  params: { hue: 0, saturation: 0, lightness: 0 },
}

const thresholdLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:threshold',
  type: 'adjustment',
  kind: 'threshold',
  name: '阈值 1',
  visible: true,
  params: { threshold: 128, invert: false },
}

const curvesLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:curves',
  type: 'adjustment',
  kind: 'curves',
  name: '曲线 1',
  visible: true,
  params: { channels: {} },
}

const levelsLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:levels',
  type: 'adjustment',
  kind: 'levels',
  name: '色阶 1',
  visible: true,
  params: { shadows: 0, midtones: 128, highlights: 255, outputBlack: 0, outputWhite: 255 },
}

const invertLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:invert',
  type: 'adjustment',
  kind: 'invert',
  name: '反相 1',
  visible: true,
  params: { enabled: true },
}

describe('imageAdjustmentEngine', () => {
  it('keeps default adjustment layers visually neutral', () => {
    const source = makeImageData([32, 96, 160, 128])

    const result = applyImageAdjustments(source, [
      hslLayer,
      curvesLayer,
      levelsLayer,
      makeLayer(invertLayer, { params: { enabled: false } }),
    ])

    expect(pixelValues(result)).toEqual([32, 96, 160, 128])
    expect(pixelValues(source)).toEqual([32, 96, 160, 128])
    expect(hasEffectiveImageAdjustments([hslLayer, curvesLayer, levelsLayer])).toBe(false)
  })

  it('skips hidden layers and keeps alpha unchanged', () => {
    const source = makeImageData([10, 20, 30, 77])

    const result = applyImageAdjustments(source, [makeLayer(invertLayer, { visible: false })])

    expect(pixelValues(result)).toEqual([10, 20, 30, 77])
  })

  it('applies invert without touching alpha', () => {
    const source = makeImageData([10, 20, 30, 77])

    const result = applyImageAdjustments(source, [invertLayer])

    expect(pixelValues(result)).toEqual([245, 235, 225, 77])
  })

  it('can execute adjustments in place for renderer memory efficiency', () => {
    const source = makeImageData([10, 20, 30, 77])

    const result = applyImageAdjustmentsInPlace(source, [invertLayer])

    expect(result).toBe(source)
    expect(pixelValues(source)).toEqual([245, 235, 225, 77])
  })

  it('keeps ranged plan-step execution identical to whole-image execution', () => {
    const sourceValues = [10, 20, 30, 77, 120, 80, 40, 88, 200, 210, 220, 99, 32, 96, 160, 111]
    const wholeImage = makeImageData(sourceValues)
    const rangedImage = makeImageData(sourceValues)
    const invertPlanStep = compileImageAdjustmentPlan([invertLayer])[0]
    const invertLuts = invertPlanStep?.kind === 'channel-lut' ? invertPlanStep.luts : failLuts()
    const plan: ImageAdjustmentPlan = [
      { kind: 'channel-lut', luts: invertLuts },
      { kind: 'threshold', threshold: 128, invert: true },
      {
        kind: 'hsl',
        params: {
          hueShift: 120 / 360,
          saturationScale: 0.75,
          lightnessOffset: 0.1,
        },
      },
    ]

    executeImageAdjustmentPlan(wholeImage, plan)
    for (const step of plan) {
      executeImageAdjustmentPlanStepRange(rangedImage.data, step, 0, 8)
      executeImageAdjustmentPlanStepRange(rangedImage.data, step, 8, rangedImage.data.length)
    }

    expect(pixelValues(rangedImage)).toEqual(pixelValues(wholeImage))
  })

  it('applies threshold and threshold invert', () => {
    const source = makeImageData([120, 120, 120, 255, 200, 200, 200, 200])

    const normal = applyImageAdjustments(source, [thresholdLayer])
    const inverted = applyImageAdjustments(source, [
      makeLayer(thresholdLayer, { params: { threshold: 128, invert: true } }),
    ])

    expect(pixelValues(normal)).toEqual([0, 0, 0, 255, 255, 255, 255, 200])
    expect(pixelValues(inverted)).toEqual([255, 255, 255, 255, 0, 0, 0, 200])
  })

  it('applies curves with multiple smooth RGB control points', () => {
    const source = makeImageData([64, 128, 192, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(curvesLayer, {
        params: {
          channels: {
            rgb: [
              { id: 'point:shadow', input: 25, output: 15 },
              { id: 'point:midtone', input: 50, output: 35 },
              { id: 'point:highlight', input: 75, output: 78 },
            ],
          },
        },
      }),
    ])

    expect(pixelValues(result)).toEqual([38, 89, 200, 255])
  })

  it('keeps curves endpoint LUTs monotonic and continuous', () => {
    const source = makeImageData([0, 64, 128, 255, 192, 255, 255, 255])

    const inputAtBlack = applyImageAdjustments(source, [
      makeLayer(curvesLayer, {
        params: {
          channels: {
            rgb: [{ id: 'endpoint:black', input: 0, output: 25 }],
          },
        },
      }),
    ])
    const inputAtWhite = applyImageAdjustments(source, [
      makeLayer(curvesLayer, {
        params: {
          channels: {
            rgb: [{ id: 'endpoint:white', input: 100, output: 75 }],
          },
        },
      }),
    ])

    expect(pixelValues(inputAtBlack)).toEqual([64, 112, 160, 255, 208, 255, 255, 255])
    expect(pixelValues(inputAtWhite)).toEqual([0, 48, 96, 255, 144, 191, 191, 255])
  })

  it('applies per-channel curves after RGB composite curves', () => {
    const source = makeImageData([128, 128, 128, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(curvesLayer, {
        params: {
          channels: {
            rgb: [{ id: 'rgb:mid', input: 50, output: 75 }],
            red: [{ id: 'red:mid', input: 75, output: 25 }],
          },
        },
      }),
    ])

    expect(pixelValues(result)).toEqual([64, 191, 191, 255])
  })

  it('applies levels with black and white points', () => {
    const source = makeImageData([0, 64, 192, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(levelsLayer, {
        params: { shadows: 64, midtones: 128, highlights: 191, outputBlack: 0, outputWhite: 255 },
      }),
    ])

    expect(pixelValues(result)).toEqual([0, 0, 255, 255])
  })

  it('applies hue and lightness adjustments', () => {
    const source = makeImageData([255, 0, 0, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(hslLayer, { params: { hue: 120, saturation: 0, lightness: 10 } }),
    ])

    expect(pixelValues(result)).toEqual([51, 255, 51, 255])
  })

  it('applies non-commutative layers in declared order', () => {
    const source = makeImageData([128, 128, 128, 255])

    const curvesThenThreshold = applyImageAdjustments(source, [
      makeLayer(curvesLayer, {
        params: { channels: { rgb: [{ id: 'point:mid', input: 50, output: 25 }] } },
      }),
      thresholdLayer,
    ])
    const thresholdThenCurves = applyImageAdjustments(source, [
      thresholdLayer,
      makeLayer(curvesLayer, {
        params: { channels: { rgb: [{ id: 'point:mid', input: 50, output: 25 }] } },
      }),
    ])

    expect(pixelValues(curvesThenThreshold)).toEqual([0, 0, 0, 255])
    expect(pixelValues(thresholdThenCurves)).toEqual([255, 255, 255, 255])
  })

  it('compiles neutral layers away and merges continuous channel LUT work', () => {
    const plan = compileImageAdjustmentPlan([
      hslLayer,
      curvesLayer,
      levelsLayer,
      invertLayer,
      makeLayer(invertLayer, { id: 'adjustment:invert:2' }),
    ])

    expect(plan).toEqual([])

    const channelPlan = compileImageAdjustmentPlan([
      makeLayer(curvesLayer, {
        params: { channels: { rgb: [{ id: 'point:mid', input: 50, output: 25 }] } },
      }),
      invertLayer,
    ])

    expect(channelPlan).toHaveLength(1)
    expect(channelPlan[0]?.kind).toBe('channel-lut')
  })

  it('treats non-finite adjustment params as neutral defaults', () => {
    const source = makeImageData([32, 96, 160, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(hslLayer, {
        params: {
          hue: Number.NaN,
          saturation: Number.POSITIVE_INFINITY,
          lightness: Number.NEGATIVE_INFINITY,
        },
      }),
      makeLayer(curvesLayer, {
        params: {
          channels: {
            rgb: [{ id: 'bad', input: Number.NaN, output: Number.NaN }],
          },
        },
      }),
      makeLayer(levelsLayer, {
        params: {
          shadows: Number.NaN,
          midtones: Number.NaN,
          highlights: Number.NaN,
          outputBlack: Number.NaN,
          outputWhite: Number.NaN,
        },
      }),
      makeLayer(thresholdLayer, { params: { threshold: Number.NaN, invert: false } }),
    ])

    expect(pixelValues(result)).toEqual([0, 0, 0, 255])
  })

  it('normalizes adjustment params through centralized definitions', () => {
    const normalizedHsl = normalizeImageAdjustmentLayer(
      makeLayer(hslLayer, {
        params: {
          hue: Number.NaN,
          saturation: Number.POSITIVE_INFINITY,
          lightness: Number.NEGATIVE_INFINITY,
        },
      }),
    )
    const normalizedThreshold = normalizeImageAdjustmentLayer(
      makeLayer(thresholdLayer, { params: { threshold: Number.NaN, invert: true } }),
    )

    expect(normalizedHsl.kind).toBe('hsl')
    if (normalizedHsl.kind !== 'hsl') throw new Error('expected hsl adjustment')
    expect(normalizedHsl.params).toEqual({
      hueShift: 0,
      saturationScale: 1,
      lightnessOffset: 0,
    })
    expect(normalizedThreshold.params).toEqual({ threshold: 128, invert: true })
  })
})

function failLuts(): NonNullable<
  Extract<ImageAdjustmentPlan[number], { kind: 'channel-lut' }>['luts']
> {
  throw new Error('expected compiled LUT step')
}
