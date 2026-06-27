import { ref } from 'vue'
import { describe, expect, it } from 'vitest'
import type { ImageEditorLayer } from '../types'
import { useImageAdjustmentFilterPreviews } from './useImageAdjustmentFilterPreviews'

function createHslLayer(params = { hue: 0, saturation: 0, brightness: 0, contrast: 0 }) {
  return {
    id: 'adjustment:hsl',
    type: 'adjustment',
    kind: 'hsl',
    name: '色相/饱和度 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createThresholdLayer(params = { threshold: 128, invert: false }) {
  return {
    id: 'adjustment:threshold',
    type: 'adjustment',
    kind: 'threshold',
    name: '阈值 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createCurvesLayer(params = { input: 50, output: 50 }) {
  return {
    id: 'adjustment:curves',
    type: 'adjustment',
    kind: 'curves',
    name: '曲线 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createLevelsLayer(params = { shadows: 0, midtones: 50, highlights: 100 }) {
  return {
    id: 'adjustment:levels',
    type: 'adjustment',
    kind: 'levels',
    name: '色阶 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function createInvertLayer(params = { enabled: true }) {
  return {
    id: 'adjustment:invert',
    type: 'adjustment',
    kind: 'invert',
    name: '反相 1',
    visible: true,
    params,
  } satisfies ImageEditorLayer
}

function lutValues(tableValues: string) {
  return tableValues.split(' ').map(Number)
}

describe('useImageAdjustmentFilterPreviews', () => {
  it('builds native filter previews for visible hsl adjustment layers', () => {
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([
        createHslLayer({ hue: 90, saturation: 25, brightness: -10, contrast: 15 }),
      ]),
    })

    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([
      {
        type: 'css-filter',
        value: 'hue-rotate(90deg) saturate(1.25) brightness(0.9) contrast(1.15)',
      },
    ])
  })

  it('does not preview hidden layers or inactive adjust mode', () => {
    const layers = ref<ImageEditorLayer[]>([{ ...createHslLayer(), visible: false }])
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers,
    })

    expect(preview.imageAdjustmentFilterPreviews.value).toEqual([])
  })

  it('builds threshold lut previews with grayscale conversion', () => {
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([createThresholdLayer({ threshold: 300, invert: true })]),
    })

    expect(preview.imageAdjustmentFilterPreviews.value[0]).toEqual({
      type: 'channel-lut',
      tableValues: expect.any(String),
      grayscale: true,
    })

    const tableValues =
      preview.imageAdjustmentFilterPreviews.value[0]?.type === 'channel-lut'
        ? preview.imageAdjustmentFilterPreviews.value[0].tableValues
        : ''
    const values = lutValues(tableValues)
    expect(values[0]).toBe(1)
    expect(values[255]).toBe(0)
  })

  it('builds curves lut previews from the control point', () => {
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([createCurvesLayer({ input: 50, output: 25 })]),
    })

    const curvesPreview = preview.imageAdjustmentFilterPreviews.value[0]
    expect(curvesPreview?.type).toBe('channel-lut')
    expect(curvesPreview?.type === 'channel-lut' ? curvesPreview.grayscale : true).toBe(false)

    const values = lutValues(curvesPreview?.type === 'channel-lut' ? curvesPreview.tableValues : '')
    expect(values[0]).toBe(0)
    expect(values[128]).toBeCloseTo(64 / 255, 4)
    expect(values[192]).toBeCloseTo(160 / 255, 4)
  })

  it('builds levels lut previews without grayscale conversion', () => {
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([createLevelsLayer({ shadows: 25, midtones: 50, highlights: 75 })]),
    })

    const levelsPreview = preview.imageAdjustmentFilterPreviews.value[0]
    expect(levelsPreview?.type).toBe('channel-lut')
    expect(levelsPreview?.type === 'channel-lut' ? levelsPreview.grayscale : true).toBe(false)

    const values = lutValues(levelsPreview?.type === 'channel-lut' ? levelsPreview.tableValues : '')
    expect(values[0]).toBe(0)
    expect(values[64]).toBe(0)
    expect(values[192]).toBe(1)
  })

  it('builds native invert previews only when enabled', () => {
    const enabledPreview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([createInvertLayer({ enabled: true })]),
    })
    const disabledPreview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([createInvertLayer({ enabled: false })]),
    })

    expect(enabledPreview.imageAdjustmentFilterPreviews.value).toEqual([
      { type: 'css-filter', value: 'invert(1)' },
    ])
    expect(disabledPreview.imageAdjustmentFilterPreviews.value).toEqual([])
  })

  it('keeps filter previews in layer order', () => {
    const preview = useImageAdjustmentFilterPreviews({
      isAdjustMode: ref(true),
      layers: ref([
        createLevelsLayer(),
        createInvertLayer(),
        createCurvesLayer(),
        createHslLayer({ hue: -45, saturation: 0, brightness: 0, contrast: 0 }),
        createThresholdLayer(),
      ]),
    })

    expect(preview.imageAdjustmentFilterPreviews.value.map((item) => item.type)).toEqual([
      'channel-lut',
      'css-filter',
      'channel-lut',
      'css-filter',
      'channel-lut',
    ])
    expect(preview.imageAdjustmentFilterPreviews.value[1]).toEqual({
      type: 'css-filter',
      value: 'invert(1)',
    })
  })
})
