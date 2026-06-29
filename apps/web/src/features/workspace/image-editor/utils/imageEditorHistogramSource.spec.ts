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

function createInvertLayer(id = 'adjustment:invert'): ImageEditorAdjustmentLayer {
  return {
    id,
    type: 'adjustment',
    kind: 'invert',
    name: '反相',
    visible: true,
    params: { enabled: true },
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
    const invertLayer = createInvertLayer()
    const curvesLayer = createCurvesLayer()
    const layers = [invertLayer, curvesLayer, createSourceLayer()]

    const resolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers,
      targetLayerId: curvesLayer.id,
    })

    expect(pixelValues(resolution.imageData)).toEqual([245, 235, 225, 255])
    expect(pixelValues(sourceImageData)).toEqual([10, 20, 30, 255])
  })

  it('reuses cached output when the selected layer changes but upstream refs stay the same', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const invertLayer = createInvertLayer()
    const firstCurvesLayer = createCurvesLayer('adjustment:curves', 60)
    const firstLayers = [invertLayer, firstCurvesLayer, createSourceLayer()]
    const cachedResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: firstLayers,
      targetLayerId: firstCurvesLayer.id,
    })
    const nextCurvesLayer = createCurvesLayer('adjustment:curves', 20)
    const nextLayers = [invertLayer, nextCurvesLayer, createSourceLayer()]

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
    const invertLayer = createInvertLayer()
    const curvesLayer = createCurvesLayer()
    const cachedResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: [invertLayer, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
    })

    const nextResolution = resolveImageEditorHistogramSource({
      sourceImageData,
      layers: [{ ...invertLayer, visible: false }, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
      cachedResolution,
    })

    expect(nextResolution).not.toBe(cachedResolution)
    expect(nextResolution.imageData).not.toBe(cachedResolution.imageData)
  })
})
