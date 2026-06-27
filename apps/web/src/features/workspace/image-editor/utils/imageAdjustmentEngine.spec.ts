import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../types'
import { applyImageAdjustments } from './imageAdjustmentEngine'

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
  params: { hue: 0, saturation: 0, brightness: 0, contrast: 0 },
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
  params: { input: 50, output: 50 },
}

const levelsLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:levels',
  type: 'adjustment',
  kind: 'levels',
  name: '色阶 1',
  visible: true,
  params: { shadows: 0, midtones: 50, highlights: 100 },
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

  it('applies threshold and threshold invert', () => {
    const source = makeImageData([120, 120, 120, 255, 200, 200, 200, 200])

    const normal = applyImageAdjustments(source, [thresholdLayer])
    const inverted = applyImageAdjustments(source, [
      makeLayer(thresholdLayer, { params: { threshold: 128, invert: true } }),
    ])

    expect(pixelValues(normal)).toEqual([0, 0, 0, 255, 255, 255, 255, 200])
    expect(pixelValues(inverted)).toEqual([255, 255, 255, 255, 0, 0, 0, 200])
  })

  it('applies curves with a single control point', () => {
    const source = makeImageData([64, 128, 192, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(curvesLayer, { params: { input: 50, output: 25 } }),
    ])

    expect(pixelValues(result)).toEqual([32, 64, 160, 255])
  })

  it('applies levels with black and white points', () => {
    const source = makeImageData([0, 64, 192, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(levelsLayer, { params: { shadows: 25, midtones: 50, highlights: 75 } }),
    ])

    expect(pixelValues(result)).toEqual([0, 0, 255, 255])
  })

  it('applies hue and brightness adjustments', () => {
    const source = makeImageData([255, 0, 0, 255])

    const result = applyImageAdjustments(source, [
      makeLayer(hslLayer, { params: { hue: 120, saturation: 0, brightness: 0, contrast: 0 } }),
    ])

    expect(pixelValues(result)).toEqual([0, 255, 0, 255])
  })
})
