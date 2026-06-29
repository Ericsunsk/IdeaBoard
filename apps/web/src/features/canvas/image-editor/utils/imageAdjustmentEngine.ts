import type { ImageEditorAdjustmentLayer } from '../types'
import {
  CHANNEL_MAX,
  CHANNEL_MIN,
  isNeutralImageAdjustment,
  normalizeImageAdjustmentLayers,
  type NormalizedHslAdjustment,
  type NormalizedImageAdjustment,
} from './imageAdjustmentDefinitions'
import { createCurvesLuts, type ImageEditorCurvesLuts } from './imageEditorCurves'
import { createLevelsLut } from './imageEditorLevels'
import { cloneImageData } from './imageDataUtils'

export type ImageAdjustmentPlanStep =
  | { kind: 'hsl'; params: NormalizedHslAdjustment }
  | { kind: 'threshold'; threshold: number; invert: boolean }
  | { kind: 'channel-lut'; luts: ImageEditorCurvesLuts }

export type ImageAdjustmentPlan = readonly ImageAdjustmentPlanStep[]

export function applyImageAdjustments(
  source: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  const output = cloneImageData(source)
  applyImageAdjustmentsInPlace(output, layers)
  return output
}

export function applyImageAdjustmentsInPlace(
  imageData: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  executeImageAdjustmentPlan(imageData, compileImageAdjustmentPlan(layers))
  return imageData
}

export function hasEffectiveImageAdjustments(layers: readonly ImageEditorAdjustmentLayer[]) {
  return compileImageAdjustmentPlan(layers).length > 0
}

export function normalizeImageAdjustmentStack(
  layers: readonly ImageEditorAdjustmentLayer[],
): NormalizedImageAdjustment[] {
  return normalizeImageAdjustmentLayers(layers.filter((layer) => layer.visible))
}

export function compileImageAdjustmentPlan(
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageAdjustmentPlanStep[] {
  return compileNormalizedImageAdjustmentPlan(normalizeImageAdjustmentStack(layers))
}

export function compileNormalizedImageAdjustmentPlan(
  layers: readonly NormalizedImageAdjustment[],
): ImageAdjustmentPlanStep[] {
  const steps: ImageAdjustmentPlanStep[] = []
  let pendingChannelLut: ImageEditorCurvesLuts | null = null

  const flushPendingChannelLut = () => {
    if (!pendingChannelLut) return
    steps.push({ kind: 'channel-lut', luts: pendingChannelLut })
    pendingChannelLut = null
  }

  for (const layer of layers) {
    if (isNeutralImageAdjustment(layer)) continue

    switch (layer.kind) {
      case 'hsl':
        flushPendingChannelLut()
        steps.push({ kind: 'hsl', params: layer.params })
        break
      case 'threshold':
        flushPendingChannelLut()
        steps.push({
          kind: 'threshold',
          threshold: layer.params.threshold,
          invert: layer.params.invert,
        })
        break
      case 'curves':
        pendingChannelLut = appendChannelLut(pendingChannelLut, createCurvesLuts(layer.params))
        break
      case 'levels':
        pendingChannelLut = appendChannelLut(
          pendingChannelLut,
          createRgbChannelLuts(createLevelsLut(layer.params)),
        )
        break
      case 'invert':
        pendingChannelLut = appendChannelLut(pendingChannelLut, createRgbChannelLuts(INVERT_LUT))
        break
    }
  }

  flushPendingChannelLut()
  return steps
}

export function executeImageAdjustmentPlan(imageData: ImageData, plan: ImageAdjustmentPlan) {
  executeImageAdjustmentPlanData(imageData.data, plan)
  return imageData
}

function executeImageAdjustmentPlanData(data: Uint8ClampedArray, plan: ImageAdjustmentPlan) {
  for (const step of plan) {
    executeImageAdjustmentPlanStepRange(data, step, 0, data.length)
  }
}

export function executeImageAdjustmentPlanStepRange(
  data: Uint8ClampedArray,
  step: ImageAdjustmentPlanStep,
  startByte: number,
  endByte: number,
) {
  const start = alignPixelByteOffset(startByte)
  const end = alignPixelByteOffset(Math.min(endByte, data.length))
  if (start >= end) return

  switch (step.kind) {
    case 'hsl':
      applyHslAdjustment(data, step.params, start, end)
      break
    case 'threshold':
      applyThresholdAdjustment(data, step.threshold, step.invert, start, end)
      break
    case 'channel-lut':
      applyChannelLut(data, step.luts, start, end)
      break
  }
}

function appendChannelLut(
  current: ImageEditorCurvesLuts | null,
  next: ImageEditorCurvesLuts,
): ImageEditorCurvesLuts | null {
  const composed = current ? composeChannelLuts(current, next) : next
  return isIdentityLut(composed) ? null : composed
}

function composeChannelLuts(first: ImageEditorCurvesLuts, second: ImageEditorCurvesLuts) {
  return {
    red: composeSingleChannelLuts(first.red, second.red),
    green: composeSingleChannelLuts(first.green, second.green),
    blue: composeSingleChannelLuts(first.blue, second.blue),
  }
}

function composeSingleChannelLuts(first: Uint8ClampedArray, second: Uint8ClampedArray) {
  return createLut((value) => second[first[value]])
}

function isIdentityLut(luts: ImageEditorCurvesLuts) {
  return (
    isIdentitySingleLut(luts.red) &&
    isIdentitySingleLut(luts.green) &&
    isIdentitySingleLut(luts.blue)
  )
}

function isIdentitySingleLut(lut: Uint8ClampedArray) {
  for (let index = CHANNEL_MIN; index <= CHANNEL_MAX; index += 1) {
    if (lut[index] !== index) return false
  }
  return true
}

function applyThresholdAdjustment(
  data: Uint8ClampedArray,
  threshold: number,
  invert: boolean,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    const luminance = 0.2126 * data[index] + 0.7152 * data[index + 1] + 0.0722 * data[index + 2]
    const next = luminance >= threshold ? CHANNEL_MAX : CHANNEL_MIN
    const value = invert ? CHANNEL_MAX - next : next
    data[index] = value
    data[index + 1] = value
    data[index + 2] = value
  }
}

function applyHslAdjustment(
  data: Uint8ClampedArray,
  params: NormalizedHslAdjustment,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    const sourceR = data[index] / CHANNEL_MAX
    const sourceG = data[index + 1] / CHANNEL_MAX
    const sourceB = data[index + 2] / CHANNEL_MAX
    const max = Math.max(sourceR, sourceG, sourceB)
    const min = Math.min(sourceR, sourceG, sourceB)
    const sourceLightness = (max + min) / 2
    let sourceHue = 0
    let sourceSaturation = 0

    if (max !== min) {
      const delta = max - min
      sourceSaturation = sourceLightness > 0.5 ? delta / (2 - max - min) : delta / (max + min)
      if (max === sourceR) {
        sourceHue = (sourceG - sourceB) / delta + (sourceG < sourceB ? 6 : 0)
      } else if (max === sourceG) {
        sourceHue = (sourceB - sourceR) / delta + 2
      } else {
        sourceHue = (sourceR - sourceG) / delta + 4
      }
      sourceHue /= 6
    }

    const nextHue = normalizeHueUnit(sourceHue + params.hueShift)
    const nextSaturation = clampUnit(sourceSaturation * params.saturationScale)
    const nextLightness = clampUnit(sourceLightness + params.lightnessOffset)
    let nextR = nextLightness
    let nextG = nextLightness
    let nextB = nextLightness

    if (nextSaturation !== 0) {
      const q =
        nextLightness < 0.5
          ? nextLightness * (1 + nextSaturation)
          : nextLightness + nextSaturation - nextLightness * nextSaturation
      const p = 2 * nextLightness - q
      nextR = hueToRgb(p, q, nextHue + 1 / 3)
      nextG = hueToRgb(p, q, nextHue)
      nextB = hueToRgb(p, q, nextHue - 1 / 3)
    }

    data[index] = applyContrast(nextR * CHANNEL_MAX, params.contrastFactor)
    data[index + 1] = applyContrast(nextG * CHANNEL_MAX, params.contrastFactor)
    data[index + 2] = applyContrast(nextB * CHANNEL_MAX, params.contrastFactor)
  }
}

function applyContrast(value: number, contrastFactor: number) {
  return clampChannel((value - 128) * contrastFactor + 128)
}

function applyChannelLut(
  data: Uint8ClampedArray,
  luts: ImageEditorCurvesLuts,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    data[index] = luts.red[data[index]]
    data[index + 1] = luts.green[data[index + 1]]
    data[index + 2] = luts.blue[data[index + 2]]
  }
}

function createRgbChannelLuts(lut: Uint8ClampedArray): ImageEditorCurvesLuts {
  return {
    red: lut,
    green: lut,
    blue: lut,
  }
}

function alignPixelByteOffset(offset: number) {
  return Math.max(0, Math.floor(offset / 4) * 4)
}

function createLut(resolve: (value: number) => number) {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index <= CHANNEL_MAX; index += 1) {
    lut[index] = clampChannel(resolve(index))
  }
  return lut
}

const INVERT_LUT = createLut((value) => CHANNEL_MAX - value)

function hueToRgb(p: number, q: number, t: number) {
  let value = t
  if (value < 0) value += 1
  if (value > 1) value -= 1
  if (value < 1 / 6) return p + (q - p) * 6 * value
  if (value < 1 / 2) return q
  if (value < 2 / 3) return p + (q - p) * (2 / 3 - value) * 6
  return p
}

function normalizeHueUnit(value: number) {
  return ((value % 1) + 1) % 1
}

function clampChannel(value: number) {
  return Math.round(clampNumber(value, CHANNEL_MIN, CHANNEL_MAX, CHANNEL_MIN))
}

function clampUnit(value: number) {
  return clampNumber(value, 0, 1, 0)
}

function clampNumber(value: number, min: number, max: number, fallback: number) {
  if (!Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, value))
}
