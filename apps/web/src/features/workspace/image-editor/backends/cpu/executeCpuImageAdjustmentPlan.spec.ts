import { describe, expect, it } from 'vitest'
import type { ImageAdjustmentPlan } from '../../utils/imageAdjustmentEngine'
import { executeCpuImageAdjustmentPlanInPlace } from './executeCpuImageAdjustmentPlan'

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

describe('cpu image adjustment backend', () => {
  it('keeps the in-place execution path available for runtime backends', () => {
    const source = makeImageData([10, 20, 30, 77])
    const plan: ImageAdjustmentPlan = [{ kind: 'channel-lut', lut: createInvertLut() }]

    const result = executeCpuImageAdjustmentPlanInPlace(source, plan)

    expect(result).toBe(source)
    expect(pixelValues(source)).toEqual([245, 235, 225, 77])
  })
})

function createInvertLut() {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return lut
}
