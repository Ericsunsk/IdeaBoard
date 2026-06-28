import { describe, expect, it } from 'vitest'
import {
  createImageEditorHistogram,
  sampleHistogramPath,
} from './imageEditorHistogram'

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

describe('imageEditorHistogram', () => {
  it('creates RGB luminance bins', () => {
    const histogram = createImageEditorHistogram(
      makeImageData([
        255, 0, 0, 255,
        0, 255, 0, 255,
        0, 0, 255, 255,
        255, 255, 255, 128,
      ]),
      'rgb',
    )

    expect(histogram.channel).toBe('rgb')
    expect(histogram.total).toBe(4)
    expect(histogram.maxBin).toBe(1)
    expect(histogram.bins[54]).toBe(1)
    expect(histogram.bins[18]).toBe(1)
    expect(histogram.bins[182]).toBe(1)
    expect(histogram.bins[255]).toBe(1)
    expect(histogram.normalizedBins[54]).toBe(1)
  })

  it('creates red, green, and blue channel bins', () => {
    const imageData = makeImageData([
      10, 20, 30, 255,
      10, 40, 60, 255,
      200, 40, 90, 255,
    ])

    const red = createImageEditorHistogram(imageData, 'red')
    const green = createImageEditorHistogram(imageData, 'green')
    const blue = createImageEditorHistogram(imageData, 'blue')

    expect(red.total).toBe(3)
    expect(red.maxBin).toBe(2)
    expect(red.bins[10]).toBe(2)
    expect(red.bins[200]).toBe(1)
    expect(red.normalizedBins[10]).toBe(1)
    expect(red.normalizedBins[200]).toBeCloseTo(Math.sqrt(1 / 2))

    expect(green.bins[20]).toBe(1)
    expect(green.bins[40]).toBe(2)
    expect(blue.bins[30]).toBe(1)
    expect(blue.bins[60]).toBe(1)
    expect(blue.bins[90]).toBe(1)
  })

  it('falls back to empty histograms for missing or invalid image data', () => {
    const missing = createImageEditorHistogram(null, 'rgb')
    const empty = createImageEditorHistogram(makeImageData([]), 'red')

    expect(missing.total).toBe(0)
    expect(missing.maxBin).toBe(0)
    expect(missing.bins).toHaveLength(256)
    expect(missing.normalizedBins[0]).toBe(0)
    expect(sampleHistogramPath(missing)).toBe('')

    expect(empty.total).toBe(0)
    expect(empty.maxBin).toBe(0)
  })

  it('samples histogram paths across the bin range', () => {
    const histogram = createImageEditorHistogram(
      makeImageData([
        0, 0, 0, 255,
        128, 128, 128, 255,
        255, 255, 255, 255,
      ]),
      'red',
    )

    expect(sampleHistogramPath(histogram, 3)).toBe('M 0 100 L 0 0 L 50.196 0 L 100 0 L 100 100 Z')
  })
})
