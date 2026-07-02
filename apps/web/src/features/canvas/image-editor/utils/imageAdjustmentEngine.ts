import type { ImageEditorAdjustmentLayer } from '../types'
import {
  CHANNEL_MAX,
  CHANNEL_MIN,
  PERCENT_SCALE,
  isNeutralImageAdjustment,
  normalizeImageAdjustmentLayers,
  type NormalizedHslAdjustment,
  type NormalizedImageAdjustment,
  type NormalizedSharpenAdjustment,
} from './imageAdjustmentDefinitions'
import { createCurvesLuts, type ImageEditorCurvesLuts } from './imageEditorCurves'
import { createLevelsLut } from './imageEditorLevels'
import { cloneImageData } from './imageDataUtils'

export type ImageAdjustmentPlanStep =
  | { kind: 'hsl'; params: NormalizedHslAdjustment }
  | { kind: 'threshold'; threshold: number; invert: boolean }
  | { kind: 'channel-lut'; luts: ImageEditorCurvesLuts }
  | { kind: 'sharpen'; params: NormalizedSharpenAdjustment }

export type ImageAdjustmentPlan = readonly ImageAdjustmentPlanStep[]

export type ImageAdjustmentPlanStepExecutionState =
  | {
      kind: 'sharpen'
      width: number
      height: number
      sourceData: Uint8ClampedArray
      blurredRgb: Float32Array
    }
  | null

const SHARPEN_WORK_FACTOR = 3

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
      case 'sharpen':
        flushPendingChannelLut()
        steps.push({ kind: 'sharpen', params: layer.params })
        break
    }
  }

  flushPendingChannelLut()
  return steps
}

export function executeImageAdjustmentPlan(imageData: ImageData, plan: ImageAdjustmentPlan) {
  executeImageAdjustmentPlanData(imageData, plan)
  return imageData
}

function executeImageAdjustmentPlanData(imageData: ImageData, plan: ImageAdjustmentPlan) {
  for (const step of plan) {
    const state = createImageAdjustmentPlanStepExecutionState(imageData, step)
    executeImageAdjustmentPlanStepRange(imageData.data, step, 0, imageData.data.length, state)
  }
}

export function createImageAdjustmentPlanStepExecutionState(
  imageData: Pick<ImageData, 'data' | 'width' | 'height'>,
  step: ImageAdjustmentPlanStep,
): ImageAdjustmentPlanStepExecutionState {
  if (step.kind !== 'sharpen') return null
  const width = Math.max(0, Math.floor(imageData.width))
  const height = Math.max(0, Math.floor(imageData.height))
  const sourceData = new Uint8ClampedArray(imageData.data)
  return {
    kind: 'sharpen',
    width,
    height,
    sourceData,
    blurredRgb: createBlurredRgbData(sourceData, width, height, step.params.radius),
  }
}

export function estimateImageAdjustmentPlanWorkUnits(
  imageData: Pick<ImageData, 'data'>,
  plan: ImageAdjustmentPlan,
) {
  return plan.reduce(
    (total, step) => total + imageData.data.length * getImageAdjustmentPlanStepWorkFactor(step),
    0,
  )
}

export function estimateImageAdjustmentPlanStepRangeWorkUnits(
  data: Uint8ClampedArray,
  step: ImageAdjustmentPlanStep,
  startByte: number,
  endByte: number,
) {
  const start = alignPixelByteOffset(startByte)
  const end = alignPixelByteOffset(Math.min(endByte, data.length))
  return Math.max(0, end - start) * getImageAdjustmentPlanStepWorkFactor(step)
}

export function executeImageAdjustmentPlanStepRange(
  data: Uint8ClampedArray,
  step: ImageAdjustmentPlanStep,
  startByte: number,
  endByte: number,
  state: ImageAdjustmentPlanStepExecutionState = null,
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
    case 'sharpen':
      applySharpenAdjustment(data, step.params, start, end, state)
      break
  }
}

function getImageAdjustmentPlanStepWorkFactor(step: ImageAdjustmentPlanStep) {
  return step.kind === 'sharpen' ? SHARPEN_WORK_FACTOR : 1
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

    data[index] = clampChannel(nextR * CHANNEL_MAX)
    data[index + 1] = clampChannel(nextG * CHANNEL_MAX)
    data[index + 2] = clampChannel(nextB * CHANNEL_MAX)
  }
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

function applySharpenAdjustment(
  data: Uint8ClampedArray,
  params: NormalizedSharpenAdjustment,
  startByte: number,
  endByte: number,
  state: ImageAdjustmentPlanStepExecutionState,
) {
  if (!state || state.kind !== 'sharpen') {
    throw new Error('Sharpen adjustment requires an image-sized execution state')
  }

  const amountScale = params.amount / PERCENT_SCALE
  for (let index = startByte; index < endByte; index += 4) {
    const pixelIndex = index / 4
    const blurOffset = pixelIndex * 3
    if (blurOffset + 2 >= state.blurredRgb.length) break

    const sourceR = state.sourceData[index]
    const sourceG = state.sourceData[index + 1]
    const sourceB = state.sourceData[index + 2]
    const blurredR = state.blurredRgb[blurOffset]
    const blurredG = state.blurredRgb[blurOffset + 1]
    const blurredB = state.blurredRgb[blurOffset + 2]

    if (params.protectColor) {
      const sourceLuminance = getLuminance(sourceR, sourceG, sourceB)
      const blurredLuminance = getLuminance(blurredR, blurredG, blurredB)
      const detail = sourceLuminance - blurredLuminance
      const delta = Math.abs(detail) >= params.threshold ? detail * amountScale : 0
      data[index] = clampChannel(sourceR + delta)
      data[index + 1] = clampChannel(sourceG + delta)
      data[index + 2] = clampChannel(sourceB + delta)
    } else {
      data[index] = sharpenSingleChannel(sourceR, blurredR, amountScale, params.threshold)
      data[index + 1] = sharpenSingleChannel(sourceG, blurredG, amountScale, params.threshold)
      data[index + 2] = sharpenSingleChannel(sourceB, blurredB, amountScale, params.threshold)
    }

    data[index + 3] = state.sourceData[index + 3]
  }
}

function createBlurredRgbData(
  sourceData: Uint8ClampedArray,
  width: number,
  height: number,
  radius: number,
) {
  const pixelCount = Math.min(width * height, Math.floor(sourceData.length / 4))
  const temp = new Float32Array(pixelCount * 3)
  const output = new Float32Array(pixelCount * 3)
  if (width <= 0 || height <= 0 || pixelCount === 0) return output

  const kernel = createGaussianKernel(radius)
  const kernelRadius = Math.floor(kernel.length / 2)

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const pixelIndex = y * width + x
      if (pixelIndex >= pixelCount) continue

      let red = 0
      let green = 0
      let blue = 0
      for (let kernelIndex = 0; kernelIndex < kernel.length; kernelIndex += 1) {
        const sampleX = clampInteger(x + kernelIndex - kernelRadius, 0, width - 1)
        const sourceOffset = (y * width + sampleX) * 4
        const weight = kernel[kernelIndex] ?? 0
        red += sourceData[sourceOffset] * weight
        green += sourceData[sourceOffset + 1] * weight
        blue += sourceData[sourceOffset + 2] * weight
      }

      const tempOffset = pixelIndex * 3
      temp[tempOffset] = red
      temp[tempOffset + 1] = green
      temp[tempOffset + 2] = blue
    }
  }

  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const pixelIndex = y * width + x
      if (pixelIndex >= pixelCount) continue

      let red = 0
      let green = 0
      let blue = 0
      for (let kernelIndex = 0; kernelIndex < kernel.length; kernelIndex += 1) {
        const sampleY = clampInteger(y + kernelIndex - kernelRadius, 0, height - 1)
        const tempOffset = (sampleY * width + x) * 3
        const weight = kernel[kernelIndex] ?? 0
        red += temp[tempOffset] * weight
        green += temp[tempOffset + 1] * weight
        blue += temp[tempOffset + 2] * weight
      }

      const outputOffset = pixelIndex * 3
      output[outputOffset] = red
      output[outputOffset + 1] = green
      output[outputOffset + 2] = blue
    }
  }

  return output
}

function createGaussianKernel(radius: number) {
  const kernelRadius = Math.max(1, Math.ceil(radius * 2))
  const sigma = Math.max(0.35, radius / 2)
  const weights = new Float32Array(kernelRadius * 2 + 1)
  let total = 0

  for (let index = -kernelRadius; index <= kernelRadius; index += 1) {
    const weight = Math.exp(-(index * index) / (2 * sigma * sigma))
    weights[index + kernelRadius] = weight
    total += weight
  }

  for (let index = 0; index < weights.length; index += 1) {
    weights[index] /= total
  }

  return weights
}

function sharpenSingleChannel(
  source: number,
  blurred: number,
  amountScale: number,
  threshold: number,
) {
  const detail = source - blurred
  if (Math.abs(detail) < threshold) return source
  return clampChannel(source + detail * amountScale)
}

function getLuminance(red: number, green: number, blue: number) {
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue
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

function clampInteger(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, value))
}

function createLut(resolve: (value: number) => number) {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index <= CHANNEL_MAX; index += 1) {
    lut[index] = clampChannel(resolve(index))
  }
  return lut
}

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
