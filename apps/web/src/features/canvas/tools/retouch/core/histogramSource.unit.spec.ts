import { describe, expect, it } from 'vitest'
import type { RetouchEffectLayer, RetouchLayer } from './model.ts'
import { resolveRetouchHistogramSource } from './histogramSource'

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

function createThresholdLayer(id = 'retouch:threshold'): RetouchEffectLayer {
  return {
    id,
    type: 'effect',
    kind: 'threshold',
    name: '阈值',
    visible: true,
    params: { threshold: 20, invert: true },
  }
}

function createCurvesLayer(id = 'retouch:curves', output = 50): RetouchEffectLayer {
  return {
    id,
    type: 'effect',
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

function createSourceLayer(): RetouchLayer {
  return {
    id: 'source',
    type: 'source',
    name: '原图',
    visible: true,
  }
}

describe('resolveRetouchHistogramSource', () => {
  it('renders visible upstream retouch layers for the target retouch layer', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer()
    const layers = [thresholdLayer, curvesLayer, createSourceLayer()]

    const resolution = resolveRetouchHistogramSource({
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
    const firstCurvesLayer = createCurvesLayer('retouch:curves', 60)
    const firstLayers = [thresholdLayer, firstCurvesLayer, createSourceLayer()]
    const cachedResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers: firstLayers,
      targetLayerId: firstCurvesLayer.id,
    })
    const nextCurvesLayer = createCurvesLayer('retouch:curves', 20)
    const nextLayers = [thresholdLayer, nextCurvesLayer, createSourceLayer()]

    const nextResolution = resolveRetouchHistogramSource({
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
    const cachedResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers: [thresholdLayer, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
    })

    const nextResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers: [{ ...thresholdLayer, visible: false }, curvesLayer, createSourceLayer()],
      targetLayerId: curvesLayer.id,
      cachedResolution,
    })

    expect(nextResolution).not.toBe(cachedResolution)
    expect(nextResolution.imageData).not.toBe(cachedResolution.imageData)
  })

  it('resolves an explicit pending target through committed upstream layers', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const thresholdLayer = createThresholdLayer()
    const layers = [thresholdLayer, createSourceLayer()]
    // Staged pending: deterministic `retouch:<kind>` id, not yet in layers.
    const pendingCurves = createCurvesLayer()

    // Legacy id-miss without the explicit slot: raw source, only accidentally
    // correct while the committed stack is empty.
    const legacyResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers,
      targetLayerId: pendingCurves.id,
    })
    expect(pixelValues(legacyResolution.imageData)).toEqual([10, 20, 30, 255])

    const pendingResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers,
      targetLayerId: pendingCurves.id,
      pendingEffectLayer: pendingCurves,
    })
    expect(pixelValues(pendingResolution.imageData)).toEqual([255, 255, 255, 255])

    // Replacing the pending object (same id, edited params) keeps the cached
    // mountain: the input never depends on the pending params themselves.
    const editedPending = createCurvesLayer('retouch:curves', 20)
    const nextResolution = resolveRetouchHistogramSource({
      sourceImageData,
      layers,
      targetLayerId: editedPending.id,
      pendingEffectLayer: editedPending,
      cachedResolution: pendingResolution,
    })
    expect(nextResolution).toBe(pendingResolution)
  })
})
