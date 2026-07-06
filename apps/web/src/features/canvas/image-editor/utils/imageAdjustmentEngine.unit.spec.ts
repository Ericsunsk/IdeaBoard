import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../types'
import {
  applyImageAdjustmentsInPlace,
  compileImageAdjustmentPlan,
  createImageAdjustmentPlanStepExecutionState,
  executeImageAdjustmentPlan,
  executeImageAdjustmentPlanStepRange,
  hasEffectiveImageAdjustments,
  type ImageAdjustmentPlan,
} from './imageAdjustmentEngine'
import { normalizeImageAdjustmentLayer } from './imageAdjustmentDefinitions'

function makeImageData(values: number[], width = values.length / 4, height = 1) {
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
  return new TestImageData(new Uint8ClampedArray(values), width, height) as ImageData
}

function pixelValues(imageData: ImageData) {
  return Array.from(imageData.data)
}

function applyImageAdjustments(
  source: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  const output = makeImageData(pixelValues(source), source.width, source.height)
  applyImageAdjustmentsInPlace(output, layers)
  return output
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

const sharpenLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:sharpen',
  type: 'adjustment',
  kind: 'sharpen',
  name: '锐化 1',
  visible: true,
  params: { amount: 0, radius: 1, threshold: 4, protectColor: true },
}

describe('imageAdjustmentEngine', () => {
  it('keeps default adjustment layers visually neutral', () => {
    const source = makeImageData([32, 96, 160, 128])

    const result = applyImageAdjustments(source, [
      hslLayer,
      curvesLayer,
      levelsLayer,
      sharpenLayer,
    ])

    expect(pixelValues(result)).toEqual([32, 96, 160, 128])
    expect(pixelValues(source)).toEqual([32, 96, 160, 128])
    expect(hasEffectiveImageAdjustments([hslLayer, curvesLayer, levelsLayer, sharpenLayer])).toBe(
      false,
    )
  })

  it('skips hidden layers and keeps alpha unchanged', () => {
    const source = makeImageData([10, 20, 30, 77])

    const result = applyImageAdjustments(source, [makeLayer(thresholdLayer, { visible: false })])

    expect(pixelValues(result)).toEqual([10, 20, 30, 77])
  })

  it('can execute adjustments in place for renderer memory efficiency', () => {
    const source = makeImageData([10, 20, 30, 77])

    const result = applyImageAdjustmentsInPlace(source, [thresholdLayer])

    expect(result).toBe(source)
    expect(pixelValues(source)).toEqual([0, 0, 0, 77])
  })

  it('keeps ranged plan-step execution identical to whole-image execution', () => {
    const sourceValues = [10, 20, 30, 77, 120, 80, 40, 88, 200, 210, 220, 99, 32, 96, 160, 111]
    const wholeImage = makeImageData(sourceValues)
    const rangedImage = makeImageData(sourceValues)
    const plan: ImageAdjustmentPlan = [
      { kind: 'channel-lut', luts: createChannelLiftLutsForTest() },
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

  it('keeps ranged sharpen execution identical to whole-image execution', () => {
    const sourceValues = [
      24, 24, 24, 50, 24, 24, 24, 60, 210, 210, 210, 70, 210, 210, 210, 80, 24, 24, 24, 90,
      24, 24, 24, 100, 210, 210, 210, 110, 210, 210, 210, 120, 24, 24, 24, 130, 24, 24, 24,
      140, 210, 210, 210, 150, 210, 210, 210, 160,
    ]
    const wholeImage = makeImageData(sourceValues, 4, 3)
    const rangedImage = makeImageData(sourceValues, 4, 3)
    const plan = compileImageAdjustmentPlan([
      makeLayer(sharpenLayer, {
        params: { amount: 160, radius: 1.4, threshold: 0, protectColor: true },
      }),
    ])

    executeImageAdjustmentPlan(wholeImage, plan)
    for (const step of plan) {
      const state = createImageAdjustmentPlanStepExecutionState(rangedImage, step)
      executeImageAdjustmentPlanStepRange(rangedImage.data, step, 0, 12, state)
      executeImageAdjustmentPlanStepRange(rangedImage.data, step, 12, 28, state)
      executeImageAdjustmentPlanStepRange(rangedImage.data, step, 28, rangedImage.data.length, state)
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

  it('applies sharpen without touching alpha', () => {
    const source = makeImageData(
      [
        32, 32, 32, 11, 32, 32, 32, 22, 192, 192, 192, 33, 192, 192, 192, 44, 192, 192, 192, 55,
      ],
      5,
      1,
    )

    const result = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 150, radius: 1, threshold: 0, protectColor: false },
      }),
    ])

    expect(pixelValues(result)).not.toEqual(pixelValues(source))
    expect([result.data[3], result.data[7], result.data[11], result.data[15], result.data[19]]).toEqual(
      [11, 22, 33, 44, 55],
    )
  })

  it('uses threshold to suppress low-contrast sharpen noise', () => {
    const source = makeImageData(
      [100, 100, 100, 255, 104, 104, 104, 255, 100, 100, 100, 255],
      3,
      1,
    )

    const suppressed = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 200, radius: 1, threshold: 10, protectColor: false },
      }),
    ])
    const enhanced = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 200, radius: 1, threshold: 0, protectColor: false },
      }),
    ])

    expect(pixelValues(suppressed)).toEqual(pixelValues(source))
    expect(pixelValues(enhanced)).not.toEqual(pixelValues(source))
  })

  it('lets sharpen radius widen the affected area', () => {
    const source = makeImageData(
      [
        20, 20, 20, 255, 20, 20, 20, 255, 20, 20, 20, 255, 220, 220, 220, 255, 220, 220, 220,
        255, 220, 220, 220, 255, 220, 220, 220, 255,
      ],
      7,
      1,
    )

    const narrow = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 160, radius: 0.3, threshold: 0, protectColor: false },
      }),
    ])
    const wide = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 160, radius: 3, threshold: 0, protectColor: false },
      }),
    ])

    expect(pixelValues(wide)).not.toEqual(pixelValues(narrow))
    expect(Math.abs(wide.data[4] - source.data[4])).toBeGreaterThan(
      Math.abs(narrow.data[4] - source.data[4]),
    )
  })

  it('can protect color by injecting sharpened detail through luminance', () => {
    const source = makeImageData(
      [
        96, 80, 70, 255, 96, 80, 70, 255, 170, 95, 82, 255, 170, 95, 82, 255, 170, 95, 82, 255,
      ],
      5,
      1,
    )

    const protectedColor = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 120, radius: 1, threshold: 0, protectColor: true },
      }),
    ])
    const directRgb = applyImageAdjustments(source, [
      makeLayer(sharpenLayer, {
        params: { amount: 120, radius: 1, threshold: 0, protectColor: false },
      }),
    ])
    const protectedDeltas = [
      protectedColor.data[8] - source.data[8],
      protectedColor.data[9] - source.data[9],
      protectedColor.data[10] - source.data[10],
    ]

    expect(pixelValues(protectedColor)).not.toEqual(pixelValues(directRgb))
    expect(new Set(protectedDeltas).size).toBe(1)
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
    ])

    expect(plan).toEqual([])

    const channelPlan = compileImageAdjustmentPlan([
      makeLayer(curvesLayer, {
        params: { channels: { rgb: [{ id: 'point:mid', input: 50, output: 25 }] } },
      }),
      makeLayer(levelsLayer, {
        params: { shadows: 20, midtones: 128, highlights: 235, outputBlack: 0, outputWhite: 255 },
      }),
    ])

    expect(channelPlan).toHaveLength(1)
    expect(channelPlan[0]?.kind).toBe('channel-lut')
  })

  it('compiles non-neutral sharpen as an ordered spatial plan step', () => {
    const plan = compileImageAdjustmentPlan([
      makeLayer(curvesLayer, {
        params: { channels: { rgb: [{ id: 'point:mid', input: 50, output: 25 }] } },
      }),
      makeLayer(sharpenLayer, {
        params: { amount: 120, radius: 1.25, threshold: 5, protectColor: true },
      }),
      makeLayer(levelsLayer, {
        params: { shadows: 20, midtones: 128, highlights: 235, outputBlack: 0, outputWhite: 255 },
      }),
    ])

    expect(plan.map((step) => step.kind)).toEqual(['channel-lut', 'sharpen', 'channel-lut'])
    expect(plan[1]).toMatchObject({
      kind: 'sharpen',
      params: { amount: 120, radius: 1.25, threshold: 5, protectColor: true },
    })
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
    const normalizedSharpen = normalizeImageAdjustmentLayer(
      makeLayer(sharpenLayer, {
        params: {
          amount: Number.POSITIVE_INFINITY,
          radius: Number.NaN,
          threshold: Number.NEGATIVE_INFINITY,
          protectColor: false,
        },
      }),
    )

    expect(normalizedHsl.kind).toBe('hsl')
    if (normalizedHsl.kind !== 'hsl') throw new Error('expected hsl adjustment')
    expect(normalizedHsl.params).toEqual({
      hueShift: 0,
      saturationScale: 1,
      lightnessOffset: 0,
    })
    expect(normalizedThreshold.params).toEqual({ threshold: 128, invert: true })
    expect(normalizedSharpen.params).toEqual({
      amount: 0,
      radius: 1,
      threshold: 4,
      protectColor: false,
    })
  })
})

function createChannelLiftLutsForTest(): NonNullable<
  Extract<ImageAdjustmentPlan[number], { kind: 'channel-lut' }>['luts']
> {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = Math.min(255, index + 10)
  }
  return {
    red: lut,
    green: lut,
    blue: lut,
  }
}
