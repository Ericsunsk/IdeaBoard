import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer, ImageEditorLayer } from '../types'
import { resolveImageEditorHistogramSource } from './imageEditorHistogramSource'

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

function pixelValues(imageData: ImageData | null) {
  return imageData ? Array.from(imageData.data) : []
}

function createThresholdLayer(id = 'adjustment:threshold'): ImageEditorAdjustmentLayer {
  return {
    id,
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值',
    visible: true,
    params: { threshold: 20, invert: true },
  }
}

function createCurvesLayer(
  id = 'adjustment:curves',
  output = 50,
): ImageEditorAdjustmentLayer {
  return {
    id,
    type: 'adjustment',
    kind: 'curves',
    name: '曲线',
    visible: true,
    params: {
      channels: {
        rgb: [{ id: 'point:mid', input: 50, output }],
      },
    },
  }
}

function createSourceLayer(): ImageEditorLayer {
  return {
    id: 'source',
    type: 'source',
    name: '原图',
    visible: true,
  }
}

describe('resolveImageEditorHistogramSource', () => {
  it('renders visible upstream adjustment layers for the target adjustment layer', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer()
    const layers = [thresholdLayer, curvesLayer, createSourceLayer()]

    const resolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers,
      targetLayerId: curvesLayer.id,
    })

    expect(pixelValues(resolution.imageData)).toEqual([255, 255, 255, 255])
    expect(pixelValues(sourceImageData)).toEqual([10, 20, 30, 255])
  })

  it('reuses cached output when the selected layer changes but upstream refs stay the same', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const thresholdLayer = createThresholdLayer()
    const firstCurvesLayer = createCurvesLayer('adjustment:curves', 60)
    const firstLayers = [thresholdLayer, firstCurvesLayer, createSourceLayer()]
    const cachedResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: firstLayers,
      targetLayerId: firstCurvesLayer.id,
    })
    const nextCurvesLayer = createCurvesLayer('adjustment:curves', 20)
    const nextLayers = [thresholdLayer, nextCurvesLayer, createSourceLayer()]

    const nextResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: nextLayers,
      targetLayerId: nextCurvesLayer.id,
      cachedResolution,
    })

    expect(nextResolution).toBe(cachedResolution)
  })

  it('recomputes when an upstream layer ref changes', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer()
    const cachedResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: [thresholdLayer, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
    })

    const nextResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: [{ ...thresholdLayer, visible: false }, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
      cachedResolution,
    })

    expect(nextResolution).not.toBe(cachedResolution)
    expect(nextResolution.imageData).not.toBe(cachedResolution.imageData)
  })
})
