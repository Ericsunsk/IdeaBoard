import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import type { RetouchEffectLayer, RetouchLayer, RetouchRenderedPreview } from './model.ts'
import { useRetouchHistogramSource } from './useRetouchHistogramSource'

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

describe('useRetouchHistogramSource', () => {
  it('uses the rendered source before the selected retouch layer', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const previewSource = computed<RetouchRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: sourceImageData,
      width: 1,
      height: 1,
    }))
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer()
    const layers = ref<RetouchLayer[]>([thresholdLayer, curvesLayer, createSourceLayer()])
    const selectedEffectLayer = computed(() => curvesLayer)
    const { retouchHistogramSource } = useRetouchHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedEffectLayer,
      previewSource,
    })

    expect(pixelValues(retouchHistogramSource.value)).toEqual([255, 255, 255, 255])
    expect(pixelValues(sourceImageData)).toEqual([10, 20, 30, 255])
  })

  it('does not recalculate when only the selected layer params change', () => {
    const previewSource = computed<RetouchRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: makeImageData([10, 20, 30, 255]),
      width: 1,
      height: 1,
    }))
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer('retouch:curves', 60)
    const layers = ref<RetouchLayer[]>([thresholdLayer, curvesLayer, createSourceLayer()])
    const selectedEffectLayer = computed(() => layers.value[1] as RetouchEffectLayer)
    const { retouchHistogramSource } = useRetouchHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedEffectLayer,
      previewSource,
    })

    const firstHistogramSource = retouchHistogramSource.value
    layers.value = [thresholdLayer, createCurvesLayer('retouch:curves', 20), createSourceLayer()]

    expect(retouchHistogramSource.value).toBe(firstHistogramSource)
  })

  it('recalculates when an upstream layer changes or the selected layer changes', () => {
    const previewSource = computed<RetouchRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: makeImageData([10, 20, 30, 255]),
      width: 1,
      height: 1,
    }))
    const firstThresholdLayer = createThresholdLayer('retouch:threshold:1')
    const firstCurvesLayer = createCurvesLayer('retouch:curves:1')
    const secondCurvesLayer = createCurvesLayer('retouch:curves:2')
    const layers = ref<RetouchLayer[]>([
      firstThresholdLayer,
      firstCurvesLayer,
      secondCurvesLayer,
      createSourceLayer(),
    ])
    const selectedLayerId = ref(firstCurvesLayer.id)
    const selectedEffectLayer = computed(
      () =>
        layers.value.find(
          (layer): layer is RetouchEffectLayer =>
            layer.type === 'effect' && layer.id === selectedLayerId.value,
        ) ?? null,
    )
    const { retouchHistogramSource } = useRetouchHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedEffectLayer,
      previewSource,
    })

    const firstHistogramSource = retouchHistogramSource.value
    layers.value = [
      { ...firstThresholdLayer, visible: false },
      firstCurvesLayer,
      secondCurvesLayer,
      createSourceLayer(),
    ]
    const upstreamChangedHistogramSource = retouchHistogramSource.value
    selectedLayerId.value = secondCurvesLayer.id

    expect(upstreamChangedHistogramSource).not.toBe(firstHistogramSource)
    expect(retouchHistogramSource.value).not.toBe(upstreamChangedHistogramSource)
  })
})
