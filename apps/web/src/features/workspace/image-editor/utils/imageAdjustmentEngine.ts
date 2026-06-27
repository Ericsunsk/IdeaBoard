import type { ImageEditorAdjustmentLayer } from '../types'

const CHANNEL_MIN = 0
const CHANNEL_MAX = 255
const PERCENT_SCALE = 100

type Rgb = {
  r: number
  g: number
  b: number
}

type Hsl = {
  h: number
  s: number
  l: number
}

export function applyImageAdjustments(
  source: ImageData,
  layers: readonly ImageEditorAdjustmentLayer[],
): ImageData {
  const output = createImageDataCopy(source)
  const visibleLayers = layers.filter((layer) => layer.visible)
  if (visibleLayers.length === 0) return output

  for (const layer of visibleLayers) {
    applyAdjustmentLayer(output.data, layer)
  }
  return output
}

function createImageDataCopy(source: ImageData) {
  const data = new Uint8ClampedArray(source.data)
  const ImageDataCtor = source.constructor as {
    new (data: Uint8ClampedArray, width: number, height: number): ImageData
  }
  return new ImageDataCtor(data, source.width, source.height)
}

function applyAdjustmentLayer(data: Uint8ClampedArray, layer: ImageEditorAdjustmentLayer) {
  switch (layer.kind) {
    case 'hsl':
      applyHslAdjustment(data, layer.params)
      return
    case 'threshold':
      applyThresholdAdjustment(data, layer.params.threshold, layer.params.invert)
      return
    case 'curves':
      applyChannelLut(data, createCurvesLut(layer.params.input, layer.params.output))
      return
    case 'levels':
      applyChannelLut(
        data,
        createLevelsLut(layer.params.shadows, layer.params.midtones, layer.params.highlights),
      )
      return
    case 'invert':
      if (layer.params.enabled) applyInvertAdjustment(data)
      return
  }
}

function applyInvertAdjustment(data: Uint8ClampedArray) {
  for (let index = 0; index < data.length; index += 4) {
    data[index] = CHANNEL_MAX - data[index]
    data[index + 1] = CHANNEL_MAX - data[index + 1]
    data[index + 2] = CHANNEL_MAX - data[index + 2]
  }
}

function applyThresholdAdjustment(data: Uint8ClampedArray, threshold: number, invert: boolean) {
  const thresholdValue = clampChannel(threshold)
  for (let index = 0; index < data.length; index += 4) {
    const luminance = 0.2126 * data[index] + 0.7152 * data[index + 1] + 0.0722 * data[index + 2]
    const next = luminance >= thresholdValue ? CHANNEL_MAX : CHANNEL_MIN
    const value = invert ? CHANNEL_MAX - next : next
    data[index] = value
    data[index + 1] = value
    data[index + 2] = value
  }
}

function applyHslAdjustment(
  data: Uint8ClampedArray,
  params: { hue: number; saturation: number; brightness: number; contrast: number },
) {
  const hueShift = params.hue / 360
  const saturationScale = Math.max(0, 1 + params.saturation / PERCENT_SCALE)
  const lightnessOffset = params.brightness / PERCENT_SCALE
  const contrastFactor = createContrastFactor(params.contrast)

  for (let index = 0; index < data.length; index += 4) {
    const hsl = rgbToHsl({
      r: data[index] / CHANNEL_MAX,
      g: data[index + 1] / CHANNEL_MAX,
      b: data[index + 2] / CHANNEL_MAX,
    })
    const rgb = hslToRgb({
      h: normalizeHueUnit(hsl.h + hueShift),
      s: clampUnit(hsl.s * saturationScale),
      l: clampUnit(hsl.l + lightnessOffset),
    })

    data[index] = applyContrast(rgb.r * CHANNEL_MAX, contrastFactor)
    data[index + 1] = applyContrast(rgb.g * CHANNEL_MAX, contrastFactor)
    data[index + 2] = applyContrast(rgb.b * CHANNEL_MAX, contrastFactor)
  }
}

function applyContrast(value: number, contrastFactor: number) {
  return clampChannel((value - 128) * contrastFactor + 128)
}

function createContrastFactor(value: number) {
  const normalized = clamp(value, -100, 100) * 2.55
  return (259 * (normalized + 255)) / (255 * (259 - normalized))
}

export function createCurvesLut(input: number, output: number) {
  const inputPoint = Math.round((clamp(input, 0, PERCENT_SCALE) / PERCENT_SCALE) * CHANNEL_MAX)
  const outputPoint = Math.round((clamp(output, 0, PERCENT_SCALE) / PERCENT_SCALE) * CHANNEL_MAX)
  if (inputPoint <= CHANNEL_MIN) {
    return createLut((value) => (value === CHANNEL_MIN ? outputPoint : outputPoint + value))
  }
  if (inputPoint >= CHANNEL_MAX) {
    return createLut((value) => (value === CHANNEL_MAX ? outputPoint : value))
  }
  return createLut((value) => {
    if (value <= inputPoint) return (value / inputPoint) * outputPoint
    return outputPoint + ((value - inputPoint) / (CHANNEL_MAX - inputPoint)) * (CHANNEL_MAX - outputPoint)
  })
}

export function createLevelsLut(shadows: number, midtones: number, highlights: number) {
  const black = Math.round((clamp(shadows, 0, PERCENT_SCALE) / PERCENT_SCALE) * CHANNEL_MAX)
  const white = Math.round((clamp(highlights, 0, PERCENT_SCALE) / PERCENT_SCALE) * CHANNEL_MAX)
  const gamma = createLevelsGamma(midtones)
  const range = Math.max(1, white - black)

  return createLut((value) => {
    const normalized = clampUnit((value - black) / range)
    return Math.pow(normalized, gamma) * CHANNEL_MAX
  })
}

function createLevelsGamma(midtones: number) {
  const normalized = clamp(midtones, 1, 99) / PERCENT_SCALE
  return Math.log(0.5) / Math.log(normalized)
}

function applyChannelLut(data: Uint8ClampedArray, lut: Uint8ClampedArray) {
  for (let index = 0; index < data.length; index += 4) {
    data[index] = lut[data[index]]
    data[index + 1] = lut[data[index + 1]]
    data[index + 2] = lut[data[index + 2]]
  }
}

function createLut(resolve: (value: number) => number) {
  const lut = new Uint8ClampedArray(256)
  for (let index = 0; index <= CHANNEL_MAX; index += 1) {
    lut[index] = clampChannel(resolve(index))
  }
  return lut
}

function rgbToHsl(rgb: Rgb): Hsl {
  const max = Math.max(rgb.r, rgb.g, rgb.b)
  const min = Math.min(rgb.r, rgb.g, rgb.b)
  const l = (max + min) / 2
  if (max === min) return { h: 0, s: 0, l }

  const delta = max - min
  const s = l > 0.5 ? delta / (2 - max - min) : delta / (max + min)
  let h: number
  if (max === rgb.r) h = (rgb.g - rgb.b) / delta + (rgb.g < rgb.b ? 6 : 0)
  else if (max === rgb.g) h = (rgb.b - rgb.r) / delta + 2
  else h = (rgb.r - rgb.g) / delta + 4
  return { h: h / 6, s, l }
}

function hslToRgb(hsl: Hsl): Rgb {
  if (hsl.s === 0) return { r: hsl.l, g: hsl.l, b: hsl.l }
  const q = hsl.l < 0.5 ? hsl.l * (1 + hsl.s) : hsl.l + hsl.s - hsl.l * hsl.s
  const p = 2 * hsl.l - q
  return {
    r: hueToRgb(p, q, hsl.h + 1 / 3),
    g: hueToRgb(p, q, hsl.h),
    b: hueToRgb(p, q, hsl.h - 1 / 3),
  }
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

function clampUnit(value: number) {
  return clamp(value, 0, 1)
}

function clampChannel(value: number) {
  return Math.round(clamp(value, CHANNEL_MIN, CHANNEL_MAX))
}

function clamp(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) return min
  return Math.min(max, Math.max(min, value))
}
