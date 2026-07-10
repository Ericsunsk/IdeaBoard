import type { ImageEditorCurveChannel } from '../types'

const CHANNEL_MAX = 255
const PERCENT_SCALE = 100

export type ImageEditorHistogram = {
  channel: ImageEditorCurveChannel
  bins: Uint32Array
  normalizedBins: Float32Array
  maxBin: number
  total: number
}

export function createImageEditorHistogram(
  imageData: ImageData | null | undefined,
  channel: ImageEditorCurveChannel,
): ImageEditorHistogram {
  const bins = new Uint32Array(256)
  const data = imageData?.data
  if (!data || data.length < 4) return createHistogramResult(channel, bins)

  for (let index = 0; index < data.length; index += 4) {
    const value = resolveHistogramValue(data, index, channel)
    bins[value] += 1
  }

  return createHistogramResult(channel, bins)
}

export function sampleHistogramPath(histogram: ImageEditorHistogram, sampleCount = 128) {
  if (histogram.maxBin <= 0) return ''

  const count = Math.max(2, Math.round(sampleCount))
  const points: string[] = [`M 0 ${PERCENT_SCALE}`]
  for (let index = 0; index < count; index += 1) {
    const binIndex = Math.round((index / (count - 1)) * CHANNEL_MAX)
    const x = (binIndex / CHANNEL_MAX) * PERCENT_SCALE
    const y = PERCENT_SCALE - histogram.normalizedBins[binIndex] * PERCENT_SCALE
    points.push(`L ${formatPathNumber(x)} ${formatPathNumber(y)}`)
  }
  points.push(`L ${PERCENT_SCALE} ${PERCENT_SCALE} Z`)
  return points.join(' ')
}

function createHistogramResult(
  channel: ImageEditorCurveChannel,
  bins: Uint32Array,
): ImageEditorHistogram {
  const normalizedBins = new Float32Array(256)
  let maxBin = 0
  let total = 0
  for (const value of bins) {
    if (value > maxBin) maxBin = value
    total += value
  }

  if (maxBin > 0) {
    for (let index = 0; index < bins.length; index += 1) {
      normalizedBins[index] = Math.sqrt(bins[index] / maxBin)
    }
  }

  return {
    channel,
    bins,
    normalizedBins,
    maxBin,
    total,
  }
}

function resolveHistogramValue(
  data: Uint8ClampedArray,
  index: number,
  channel: ImageEditorCurveChannel,
) {
  switch (channel) {
    case 'red':
      return data[index]
    case 'green':
      return data[index + 1]
    case 'blue':
      return data[index + 2]
    case 'rgb':
      return Math.round(0.2126 * data[index] + 0.7152 * data[index + 1] + 0.0722 * data[index + 2])
  }
}

function formatPathNumber(value: number) {
  return Number.isInteger(value)
    ? String(value)
    : value.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
}
