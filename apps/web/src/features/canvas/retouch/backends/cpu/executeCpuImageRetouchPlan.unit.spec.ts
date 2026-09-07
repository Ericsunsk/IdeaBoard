import { describe, expect, it } from 'vitest'
import type { ImageRetouchPlan } from '../../utils/imageRetouchEngine'
import { executeCpuImageRetouchPlanInPlace } from './executeCpuImageRetouchPlan'

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

describe('cpu image retouch backend', () => {
  it('keeps the in-place execution path available for runtime backends', () => {
    const source = makeImageData([10, 20, 30, 77])
    const plan: ImageRetouchPlan = [
      { kind: 'channel-lut', luts: createRgbChannelLutsForTest(createChannelLiftLut()) },
    ]

    const result = executeCpuImageRetouchPlanInPlace(source, plan)

    expect(result).toBe(source)
    expect(pixelValues(source)).toEqual([20, 30, 40, 77])
  })
})

function createChannelLiftLut() {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = Math.min(255, index + 10)
  }
  return lut
}

function createRgbChannelLutsForTest(lut: Uint8ClampedArray) {
  return {
    red: lut,
    green: lut,
    blue: lut,
  }
}
