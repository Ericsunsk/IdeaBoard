import { computed, type Ref } from 'vue'
import type {
  ImageEditorFilterPreview,
  ImageEditorHslParams,
  ImageEditorLayer,
} from '../types'
import { createCurvesLut, createLevelsLut } from '../utils/imageAdjustmentEngine'
import { flattenVisibleImageEditorAdjustmentLayers } from '../utils/imageEditorLayerAdjustments'

type UseImageAdjustmentFilterPreviewsOptions = {
  isAdjustMode: Ref<boolean>
  layers: Ref<readonly ImageEditorLayer[]>
}

export function useImageAdjustmentFilterPreviews(options: UseImageAdjustmentFilterPreviewsOptions) {
  const imageAdjustmentFilterPreviews = computed<ImageEditorFilterPreview[]>(() => {
    if (!options.isAdjustMode.value) return []

    return flattenVisibleImageEditorAdjustmentLayers(options.layers.value).reduce<
      ImageEditorFilterPreview[]
    >((previews, layer) => {
      switch (layer.kind) {
        case 'hsl':
          previews.push({
            type: 'css-filter',
            value: createHslFilterValue(layer.params),
          })
          return previews
        case 'threshold':
          previews.push({
            type: 'channel-lut',
            tableValues: createThresholdTableValues(layer.params.threshold, layer.params.invert),
            grayscale: true,
          })
          return previews
        case 'curves':
          previews.push({
            type: 'channel-lut',
            tableValues: createLutTableValues(
              createCurvesLut(layer.params.input, layer.params.output),
            ),
            grayscale: false,
          })
          return previews
        case 'levels':
          previews.push({
            type: 'channel-lut',
            tableValues: createLutTableValues(
              createLevelsLut(
                layer.params.shadows,
                layer.params.midtones,
                layer.params.highlights,
              ),
            ),
            grayscale: false,
          })
          return previews
        case 'invert':
          if (layer.params.enabled) {
            previews.push({
              type: 'css-filter',
              value: 'invert(1)',
            })
          }
          return previews
        default:
          return previews
      }
    }, [])
  })

  return {
    imageAdjustmentFilterPreviews,
  }
}

function createHslFilterValue(params: ImageEditorHslParams) {
  return [
    `hue-rotate(${clampNumber(params.hue, -180, 180)}deg)`,
    `saturate(${Math.max(0, 1 + clampNumber(params.saturation, -100, 100) / 100)})`,
    `brightness(${Math.max(0, 1 + clampNumber(params.brightness, -100, 100) / 100)})`,
    `contrast(${Math.max(0, 1 + clampNumber(params.contrast, -100, 100) / 100)})`,
  ].join(' ')
}

function clampNumber(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}

function createThresholdTableValues(threshold: number, invert: boolean) {
  const thresholdValue = Math.round(clampNumber(threshold, 0, 255))
  const values: string[] = []
  for (let index = 0; index <= 255; index += 1) {
    const value = index >= thresholdValue ? 1 : 0
    values.push(invert ? `${1 - value}` : `${value}`)
  }
  return values.join(' ')
}

function formatLutValue(value: number) {
  return clampNumber(value, 0, 1).toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

function createLutTableValues(lut: Uint8ClampedArray) {
  const values: string[] = []
  for (const value of lut) {
    values.push(formatLutValue(value / 255))
  }
  return values.join(' ')
}
