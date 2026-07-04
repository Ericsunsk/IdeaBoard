import { computed, ref } from 'vue'
import { describe, expect, it } from 'vitest'
import type { ImageEditorAdjustmentLayer, ImageEditorLayer, ImageEditorRenderedPreview } from '../types'
import { useImageEditorHistogramSource } from './useImageEditorHistogramSource'

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

describe('useImageEditorHistogramSource', () => {
  it('uses the rendered source before the selected adjustment layer', () => {
    const sourceImageData = makeImageData([10, 20, 30, 255])
    const previewSource = computed<ImageEditorRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: sourceImageData,
      width: 1,
      height: 1,
    }))
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer()
    const layers = ref<ImageEditorLayer[]>([thresholdLayer, curvesLayer, createSourceLayer()])
    const selectedAdjustmentLayer = computed(() => curvesLayer)
    const { imageEditorHistogramSource } = useImageEditorHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedAdjustmentLayer,
      previewSource,
    })

    expect(pixelValues(imageEditorHistogramSource.value)).toEqual([255, 255, 255, 255])
    expect(pixelValues(sourceImageData)).toEqual([10, 20, 30, 255])
  })

  it('does not recalculate when only the selected layer params change', () => {
    const previewSource = computed<ImageEditorRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: makeImageData([10, 20, 30, 255]),
      width: 1,
      height: 1,
    }))
    const thresholdLayer = createThresholdLayer()
    const curvesLayer = createCurvesLayer('adjustment:curves', 60)
    const layers = ref<ImageEditorLayer[]>([thresholdLayer, curvesLayer, createSourceLayer()])
    const selectedAdjustmentLayer = computed(() => layers.value[1] as ImageEditorAdjustmentLayer)
    const { imageEditorHistogramSource } = useImageEditorHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedAdjustmentLayer,
      previewSource,
    })

    const firstHistogramSource = imageEditorHistogramSource.value
    layers.value = [
      thresholdLayer,
      createCurvesLayer('adjustment:curves', 20),
      createSourceLayer(),
    ]

    expect(imageEditorHistogramSource.value).toBe(firstHistogramSource)
  })

  it('recalculates when an upstream layer changes or the selected layer changes', () => {
    const previewSource = computed<ImageEditorRenderedPreview>(() => ({
      type: 'rendered-image',
      imageData: makeImageData([10, 20, 30, 255]),
      width: 1,
      height: 1,
    }))
    const firstThresholdLayer = createThresholdLayer('adjustment:threshold:1')
    const firstCurvesLayer = createCurvesLayer('adjustment:curves:1')
    const secondCurvesLayer = createCurvesLayer('adjustment:curves:2')
    const layers = ref<ImageEditorLayer[]>([
      firstThresholdLayer,
      firstCurvesLayer,
      secondCurvesLayer,
      createSourceLayer(),
    ])
    const selectedLayerId = ref(firstCurvesLayer.id)
    const selectedAdjustmentLayer = computed(
      () =>
        layers.value.find(
          (layer): layer is ImageEditorAdjustmentLayer =>
            layer.type === 'adjustment' && layer.id === selectedLayerId.value,
        ) ?? null,
    )
    const { imageEditorHistogramSource } = useImageEditorHistogramSource({
      isAdjustMode: ref(true),
      layers,
      selectedAdjustmentLayer,
      previewSource,
    })

    const firstHistogramSource = imageEditorHistogramSource.value
    layers.value = [
      { ...firstThresholdLayer, visible: false },
      firstCurvesLayer,
      secondCurvesLayer,
      createSourceLayer(),
    ]
    const upstreamChangedHistogramSource = imageEditorHistogramSource.value
    selectedLayerId.value = secondCurvesLayer.id

    expect(upstreamChangedHistogramSource).not.toBe(firstHistogramSource)
    expect(imageEditorHistogramSource.value).not.toBe(upstreamChangedHistogramSource)
  })
})
