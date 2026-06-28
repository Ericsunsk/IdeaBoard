import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer } from '../../types'
import type { ImageAdjustmentPlan } from '../../utils/imageAdjustmentEngine'
import {
  applyCpuImageAdjustments,
  applyCpuImageAdjustmentsInPlace,
  cpuImageAdjustmentBackend,
  executeCpuImageAdjustmentPlan,
  executeCpuImageAdjustmentPlanInPlace,
} from './executeCpuImageAdjustmentPlan'

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

const invertLayer: ImageEditorAdjustmentLayer = {
  id: 'adjustment:invert',
  type: 'adjustment',
  kind: 'invert',
  name: '反相 1',
  visible: true,
  params: { enabled: true },
}

describe('cpu image adjustment backend', () => {
  it('executes a compiled plan against a cloned ImageData output', () => {
    const source = makeImageData([10, 20, 30, 77])
    const plan: ImageAdjustmentPlan = [{ kind: 'channel-lut', lut: createInvertLut() }]

    const result = executeCpuImageAdjustmentPlan(source, plan)

    expect(result).not.toBe(source)
    expect(pixelValues(result)).toEqual([245, 235, 225, 77])
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
  })

  it('keeps the in-place execution path available for runtime backends', () => {
    const source = makeImageData([10, 20, 30, 77])
    const plan: ImageAdjustmentPlan = [{ kind: 'channel-lut', lut: createInvertLut() }]

    const result = executeCpuImageAdjustmentPlanInPlace(source, plan)

    expect(result).toBe(source)
    expect(pixelValues(source)).toEqual([245, 235, 225, 77])
  })

  it('wraps the existing layer apply APIs behind a backend object', () => {
    const source = makeImageData([10, 20, 30, 77])
    const inPlaceSource = makeImageData([10, 20, 30, 77])

    const copied = applyCpuImageAdjustments(source, [invertLayer])
    const inPlace = applyCpuImageAdjustmentsInPlace(inPlaceSource, [invertLayer])

    expect(cpuImageAdjustmentBackend.id).toBe('cpu-main')
    expect(pixelValues(copied)).toEqual([245, 235, 225, 77])
    expect(pixelValues(source)).toEqual([10, 20, 30, 77])
    expect(inPlace).toBe(inPlaceSource)
    expect(pixelValues(inPlaceSource)).toEqual([245, 235, 225, 77])
  })
})

function createInvertLut() {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index < lut.length; index += 1) {
    lut[index] = 255 - index
  }
  return lut
}
