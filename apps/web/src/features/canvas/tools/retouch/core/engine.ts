import type { RetouchEffectLayer } from './model.ts'
import { clamp, clampWithFallback } from '../../../../../shared/utils/number'
import {
  isNeutralImageRetouch,
  normalizeImageEffectLayers,
  type NormalizedImageRetouch,
} from './definitions'
import type { NormalizedHslEffect } from './kinds/hsl'
import type { NormalizedSharpenEffect } from './kinds/sharpen'
import {
  RETOUCH_CHANNEL_MAX,
  RETOUCH_CHANNEL_MIN,
  RETOUCH_PLOT_PERCENT_MAX,
  composeSingleChannelLut,
} from './channelScale'
import { createCurvesLuts, type RetouchCurvesLuts } from './kinds/curves'
import { createLevelsLut } from './kinds/levels'

const FLUSH_KINDS: ReadonlySet<NormalizedImageRetouch['kind']> = new Set([
  'hsl',
  'threshold',
  'sharpen',
])

const CHANNEL_LUT_FACTORIES: {
  [K in Extract<NormalizedImageRetouch, { kind: 'curves' | 'levels' }>['kind']]: (
    layer: Extract<NormalizedImageRetouch, { kind: K }>,
  ) => RetouchCurvesLuts
} = {
  curves: (layer) => createCurvesLuts(layer.params),
  levels: (layer) => createRgbChannelLuts(createLevelsLut(layer.params)),
}

export type ImageRetouchPlanStep =
  | { kind: 'hsl'; params: NormalizedHslEffect }
  // NOTE: threshold stays flattened (`threshold`/`invert` at the top level)
  // instead of `params: NormalizedThresholdEffect` on purpose: the shape is
  // the durable Board recipe contract (board/domain ImageRetouchRecipeStep)
  // and the WebGL uniform baker reads `step.threshold`/`step.invert`
  // directly. Aligning it to a nested `params` would fork recipe
  // serialization and the GPU path for no runtime gain.
  | { kind: 'threshold'; threshold: number; invert: boolean }
  | { kind: 'channel-lut'; luts: RetouchCurvesLuts }
  | { kind: 'sharpen'; params: NormalizedSharpenEffect }

export type ImageRetouchPlan = readonly ImageRetouchPlanStep[]

export type ImageRetouchPlanStepExecutionState = {
  kind: 'sharpen'
  width: number
  height: number
  sourceData: Uint8ClampedArray
  blurredRgb: Float32Array
} | null

export function applyImageRetouchInPlace(
  imageData: ImageData,
  layers: readonly RetouchEffectLayer[],
): ImageData {
  executeImageRetouchPlan(imageData, compileImageRetouchPlan(layers))
  return imageData
}

export function hasEffectiveImageRetouch(layers: readonly RetouchEffectLayer[]) {
  return compileImageRetouchPlan(layers).length > 0
}

export function normalizeImageRetouchStack(
  layers: readonly RetouchEffectLayer[],
): NormalizedImageRetouch[] {
  return normalizeImageEffectLayers(layers.filter((layer) => layer.visible))
}

export function compileImageRetouchPlan(
  layers: readonly RetouchEffectLayer[],
): ImageRetouchPlanStep[] {
  return compileNormalizedImageRetouchPlan(normalizeImageRetouchStack(layers))
}

export function compileNormalizedImageRetouchPlan(
  layers: readonly NormalizedImageRetouch[],
): ImageRetouchPlanStep[] {
  const steps: ImageRetouchPlanStep[] = []
  let pendingChannelLut: RetouchCurvesLuts | null = null

  const flushPendingChannelLut = () => {
    if (!pendingChannelLut) return
    steps.push({ kind: 'channel-lut', luts: pendingChannelLut })
    pendingChannelLut = null
  }

  for (const layer of layers) {
    if (isNeutralImageRetouch(layer)) continue

    if (layer.kind === 'curves' || layer.kind === 'levels') {
      const channelLuts =
        layer.kind === 'curves'
          ? CHANNEL_LUT_FACTORIES.curves(layer)
          : CHANNEL_LUT_FACTORIES.levels(layer)
      pendingChannelLut = composeOrNull(pendingChannelLut, channelLuts)
      continue
    }

    if (FLUSH_KINDS.has(layer.kind)) flushPendingChannelLut()

    switch (layer.kind) {
      case 'hsl':
        steps.push({ kind: 'hsl', params: layer.params })
        break
      case 'threshold':
        steps.push({
          kind: 'threshold',
          threshold: layer.params.threshold,
          invert: layer.params.invert,
        })
        break
      case 'sharpen':
        steps.push({ kind: 'sharpen', params: layer.params })
        break
    }
  }

  flushPendingChannelLut()
  return steps
}

export function executeImageRetouchPlan(imageData: ImageData, plan: ImageRetouchPlan) {
  for (const step of plan) {
    const state = createImageRetouchPlanStepExecutionState(imageData, step)
    executeImageRetouchPlanStepRange(imageData.data, step, 0, imageData.data.length, state)
  }
  return imageData
}

export function createImageRetouchPlanStepExecutionState(
  imageData: Pick<ImageData, 'data' | 'width' | 'height'>,
  step: ImageRetouchPlanStep,
): ImageRetouchPlanStepExecutionState {
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

export function executeImageRetouchPlanStepRange(
  data: Uint8ClampedArray,
  step: ImageRetouchPlanStep,
  startByte: number,
  endByte: number,
  state: ImageRetouchPlanStepExecutionState = null,
) {
  const start = alignPixelByteOffset(startByte)
  const end = alignPixelByteOffset(Math.min(endByte, data.length))
  if (start >= end) return

  const applier = STEP_APPLIERS[step.kind] as (
    data: Uint8ClampedArray,
    step: ImageRetouchPlanStep,
    startByte: number,
    endByte: number,
    state: ImageRetouchPlanStepExecutionState,
  ) => void
  applier(data, step, start, end, state)
}

const STEP_APPLIERS: {
  [K in ImageRetouchPlanStep['kind']]: (
    data: Uint8ClampedArray,
    step: Extract<ImageRetouchPlanStep, { kind: K }>,
    startByte: number,
    endByte: number,
    state: ImageRetouchPlanStepExecutionState,
  ) => void
} = {
  hsl: (data, step, startByte, endByte) => applyHslRetouch(data, step.params, startByte, endByte),
  threshold: (data, step, startByte, endByte) =>
    applyThresholdRetouch(data, step.threshold, step.invert, startByte, endByte),
  'channel-lut': (data, step, startByte, endByte) =>
    applyChannelLut(data, step.luts, startByte, endByte),
  sharpen: (data, step, startByte, endByte, state) =>
    applySharpenRetouch(data, step.params, startByte, endByte, state),
}

function composeOrNull(
  current: RetouchCurvesLuts | null,
  next: RetouchCurvesLuts,
): RetouchCurvesLuts | null {
  const composed = current
    ? {
        red: composeSingleChannelLut(current.red, next.red),
        green: composeSingleChannelLut(current.green, next.green),
        blue: composeSingleChannelLut(current.blue, next.blue),
      }
    : next
  return isIdentityLut(composed) ? null : composed
}

function isIdentityLut(luts: RetouchCurvesLuts) {
  return [luts.red, luts.green, luts.blue].every(isIdentitySingleLut)
}

function isIdentitySingleLut(lut: Uint8ClampedArray) {
  for (let index = RETOUCH_CHANNEL_MIN; index <= RETOUCH_CHANNEL_MAX; index += 1) {
    if ((lut[index] ?? -1) !== index) return false
  }
  return true
}

function applyThresholdRetouch(
  data: Uint8ClampedArray,
  threshold: number,
  invert: boolean,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    const luminance =
      0.2126 * (data[index] ?? 0) +
      0.7152 * (data[index + 1] ?? 0) +
      0.0722 * (data[index + 2] ?? 0)
    const next = luminance >= threshold ? RETOUCH_CHANNEL_MAX : RETOUCH_CHANNEL_MIN
    const value = invert ? RETOUCH_CHANNEL_MAX - next : next
    data[index] = value
    data[index + 1] = value
    data[index + 2] = value
  }
}

function applyHslRetouch(
  data: Uint8ClampedArray,
  params: NormalizedHslEffect,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    const sourceR = (data[index] ?? 0) / RETOUCH_CHANNEL_MAX
    const sourceG = (data[index + 1] ?? 0) / RETOUCH_CHANNEL_MAX
    const sourceB = (data[index + 2] ?? 0) / RETOUCH_CHANNEL_MAX
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

    data[index] = clampChannel(nextR * RETOUCH_CHANNEL_MAX)
    data[index + 1] = clampChannel(nextG * RETOUCH_CHANNEL_MAX)
    data[index + 2] = clampChannel(nextB * RETOUCH_CHANNEL_MAX)
  }
}

function applyChannelLut(
  data: Uint8ClampedArray,
  luts: RetouchCurvesLuts,
  startByte: number,
  endByte: number,
) {
  for (let index = startByte; index < endByte; index += 4) {
    data[index] = luts.red[data[index] ?? 0] ?? 0
    data[index + 1] = luts.green[data[index + 1] ?? 0] ?? 0
    data[index + 2] = luts.blue[data[index + 2] ?? 0] ?? 0
  }
}

function applySharpenRetouch(
  data: Uint8ClampedArray,
  params: NormalizedSharpenEffect,
  startByte: number,
  endByte: number,
  state: ImageRetouchPlanStepExecutionState,
) {
  if (!state || state.kind !== 'sharpen') {
    throw new Error('Sharpen retouch requires an image-sized execution state')
  }

  const amountScale = params.amount / RETOUCH_PLOT_PERCENT_MAX
  for (let index = startByte; index < endByte; index += 4) {
    const pixelIndex = index / 4
    const blurOffset = pixelIndex * 3
    if (blurOffset + 2 >= state.blurredRgb.length) break

    const sourceR = state.sourceData[index] ?? 0
    const sourceG = state.sourceData[index + 1] ?? 0
    const sourceB = state.sourceData[index + 2] ?? 0
    const blurredR = state.blurredRgb[blurOffset] ?? 0
    const blurredG = state.blurredRgb[blurOffset + 1] ?? 0
    const blurredB = state.blurredRgb[blurOffset + 2] ?? 0

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

    data[index + 3] = state.sourceData[index + 3] ?? 0
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

  blurImageRowsHorizontally(sourceData, width, height, pixelCount, temp, kernel, kernelRadius)
  blurImageColumnsVertically(temp, width, height, pixelCount, output, kernel, kernelRadius)

  return output
}

function blurImageRowsHorizontally(
  sourceData: Uint8ClampedArray,
  width: number,
  height: number,
  pixelCount: number,
  temp: Float32Array,
  kernel: Float32Array,
  kernelRadius: number,
) {
  blurImageAxis(
    sourceData,
    temp,
    width,
    height,
    pixelCount,
    kernel,
    kernelRadius,
    4,
    (x, y, kernelIndex) => {
      const sampleX = clampInteger(x + kernelIndex - kernelRadius, 0, width - 1)
      return y * width + sampleX
    },
  )
}

function blurImageColumnsVertically(
  temp: Float32Array,
  width: number,
  height: number,
  pixelCount: number,
  output: Float32Array,
  kernel: Float32Array,
  kernelRadius: number,
) {
  blurImageAxis(
    temp,
    output,
    width,
    height,
    pixelCount,
    kernel,
    kernelRadius,
    3,
    (x, y, kernelIndex) => {
      const sampleY = clampInteger(y + kernelIndex - kernelRadius, 0, height - 1)
      return sampleY * width + x
    },
  )
}

function blurImageAxis(
  source: Uint8ClampedArray | Float32Array,
  dest: Float32Array,
  width: number,
  height: number,
  pixelCount: number,
  kernel: Float32Array,
  kernelRadius: number,
  sourceStride: number,
  resolveSamplePixelIndex: (x: number, y: number, kernelIndex: number) => number,
) {
  for (let y = 0; y < height; y += 1) {
    for (let x = 0; x < width; x += 1) {
      const pixelIndex = y * width + x
      if (pixelIndex >= pixelCount) continue

      let red = 0
      let green = 0
      let blue = 0
      for (let kernelIndex = 0; kernelIndex < kernel.length; kernelIndex += 1) {
        const sourceOffset = resolveSamplePixelIndex(x, y, kernelIndex) * sourceStride
        const weight = kernel[kernelIndex] ?? 0
        red += (source[sourceOffset] ?? 0) * weight
        green += (source[sourceOffset + 1] ?? 0) * weight
        blue += (source[sourceOffset + 2] ?? 0) * weight
      }

      const destOffset = pixelIndex * 3
      dest[destOffset] = red
      dest[destOffset + 1] = green
      dest[destOffset + 2] = blue
    }
  }
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
    weights[index] = (weights[index] ?? 0) / total
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

function createRgbChannelLuts(lut: Uint8ClampedArray): RetouchCurvesLuts {
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
  return clamp(value, min, max)
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
  return Math.round(
    clampWithFallback(value, RETOUCH_CHANNEL_MIN, RETOUCH_CHANNEL_MAX, RETOUCH_CHANNEL_MIN),
  )
}

function clampUnit(value: number) {
  return clampWithFallback(value, 0, 1, 0)
}
