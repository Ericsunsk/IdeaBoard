import type { RetouchCurveChannel, RetouchCurvePoint, RetouchCurvesParams } from '../types'
import {
  RETOUCH_CHANNEL_MAX,
  RETOUCH_CHANNEL_MIN,
  RETOUCH_PLOT_PERCENT_MAX,
  clampRetouchChannel,
  clampRetouchRoundedPlotPercent,
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from './retouchChannelScale'
import { i18n } from '../../../../shared/i18n'

export const RETOUCH_CURVE_CHANNELS: readonly RetouchCurveChannel[] = [
  'rgb',
  'red',
  'green',
  'blue',
]

export const RETOUCH_CURVE_ENDPOINT_BLACK_ID = 'endpoint:black'
const RETOUCH_CURVE_ENDPOINT_WHITE_ID = 'endpoint:white'
const RETOUCH_CURVE_MAX_USER_POINTS = 16
const RETOUCH_CURVE_MIN_INPUT_SPACING = 1

export type NormalizedCurveChannelPoint = RetouchCurvePoint & {
  inputChannel: number
  outputChannel: number
  endpoint: boolean
}

export type NormalizedCurvesAdjustment = {
  channels: Partial<Record<RetouchCurveChannel, NormalizedCurveChannelPoint[]>>
}

export type RetouchCurvesLuts = {
  red: Uint8ClampedArray
  green: Uint8ClampedArray
  blue: Uint8ClampedArray
}

export type RetouchCurvePreset = {
  id: string
  label: string
  channels: Partial<Record<RetouchCurveChannel, RetouchCurvePoint[]>>
}

export const RETOUCH_CURVE_PRESETS: readonly RetouchCurvePreset[] = [
  {
    id: 'linear',
    label: i18n.global.t('canvas.retouch.curvesPresets.linear'),
    channels: {},
  },
  {
    id: 'brighten',
    label: i18n.global.t('canvas.retouch.curvesPresets.brighten'),
    channels: {
      rgb: [{ id: 'preset:brighten:midtone', input: 50, output: 63 }],
    },
  },
  {
    id: 'darken',
    label: i18n.global.t('canvas.retouch.curvesPresets.darken'),
    channels: {
      rgb: [{ id: 'preset:darken:midtone', input: 50, output: 38 }],
    },
  },
  {
    id: 'soft-contrast',
    label: i18n.global.t('canvas.retouch.curvesPresets.softContrast'),
    channels: {
      rgb: [
        { id: 'preset:soft-contrast:shadow', input: 28, output: 23 },
        { id: 'preset:soft-contrast:highlight', input: 72, output: 78 },
      ],
    },
  },
  {
    id: 'strong-contrast',
    label: i18n.global.t('canvas.retouch.curvesPresets.strongContrast'),
    channels: {
      rgb: [
        { id: 'preset:strong-contrast:shadow', input: 22, output: 14 },
        { id: 'preset:strong-contrast:highlight', input: 78, output: 88 },
      ],
    },
  },
  {
    id: 'matte',
    label: i18n.global.t('canvas.retouch.curvesPresets.matte'),
    channels: {
      rgb: [
        { id: RETOUCH_CURVE_ENDPOINT_BLACK_ID, input: 0, output: 12 },
        { id: 'preset:matte:shadow', input: 32, output: 36 },
        { id: 'preset:matte:highlight', input: 82, output: 88 },
      ],
    },
  },
]

export function normalizeCurvesParams(params: RetouchCurvesParams): NormalizedCurvesAdjustment {
  const channels: NormalizedCurvesAdjustment['channels'] = {}
  const sourceParams = params as Partial<RetouchCurvesParams> | null | undefined
  const sourceChannels = isCurveChannelsRecord(sourceParams?.channels) ? sourceParams.channels : {}
  for (const channel of RETOUCH_CURVE_CHANNELS) {
    const sourcePoints = sourceChannels[channel]
    const normalized = normalizeCurveChannelPoints(Array.isArray(sourcePoints) ? sourcePoints : [])
    if (!isIdentityCurveChannel(normalized)) channels[channel] = normalized
  }
  return { channels }
}

export function normalizeCurveChannelPoints(
  points: readonly RetouchCurvePoint[],
): NormalizedCurveChannelPoint[] {
  const pointByInput = new Map<number, RetouchCurvePoint>()
  let blackEndpointOutput = 0
  let whiteEndpointOutput = RETOUCH_PLOT_PERCENT_MAX

  for (const point of points) {
    if (!isCurvePointRecord(point)) continue
    const input = clampPercent(point.input)
    const output = clampPercent(point.output, input)
    if (input === 0) {
      blackEndpointOutput = output
      continue
    }
    if (input === RETOUCH_PLOT_PERCENT_MAX) {
      whiteEndpointOutput = output
      continue
    }
    if (pointByInput.size >= RETOUCH_CURVE_MAX_USER_POINTS) continue
    pointByInput.set(input, {
      id: normalizePointId(point.id, input),
      input,
      output,
    })
  }

  const userPoints = [...pointByInput.values()]
    .sort((first, second) => first.input - second.input)
    .filter((point, index, sorted) => {
      const previous = sorted[index - 1]
      return !previous || point.input - previous.input >= RETOUCH_CURVE_MIN_INPUT_SPACING
    })

  return [
    createNormalizedPoint(RETOUCH_CURVE_ENDPOINT_BLACK_ID, 0, blackEndpointOutput, true),
    ...userPoints.map((point) => createNormalizedPoint(point.id, point.input, point.output, false)),
    createNormalizedPoint(
      RETOUCH_CURVE_ENDPOINT_WHITE_ID,
      RETOUCH_PLOT_PERCENT_MAX,
      whiteEndpointOutput,
      true,
    ),
  ]
}

function denormalizeCurvesParams(params: NormalizedCurvesAdjustment): RetouchCurvesParams {
  const channels: RetouchCurvesParams['channels'] = {}
  for (const channel of RETOUCH_CURVE_CHANNELS) {
    const points = params.channels[channel]
    if (!points || isIdentityCurveChannel(points)) continue
    const denormalizedPoints = points
      .filter((point) => !isIdentityEndpoint(point))
      .map(({ id, input, output }) => ({ id, input, output }))
    if (denormalizedPoints.length > 0) channels[channel] = denormalizedPoints
  }
  return { channels }
}

export function createCurvesLuts(params: NormalizedCurvesAdjustment): RetouchCurvesLuts {
  const rgbLut = createCurveChannelLut(params.channels.rgb)
  return {
    red: composeCurveChannelLuts(rgbLut, createCurveChannelLut(params.channels.red)),
    green: composeCurveChannelLuts(rgbLut, createCurveChannelLut(params.channels.green)),
    blue: composeCurveChannelLuts(rgbLut, createCurveChannelLut(params.channels.blue)),
  }
}

function createCurveChannelLut(
  points: readonly NormalizedCurveChannelPoint[] | undefined,
): Uint8ClampedArray {
  const transfer = createCurveChannelTransfer(points)
  if (transfer.identity) return createLut((value) => value)

  return createLut((value) =>
    interpolateMonotoneCubic(value, transfer.inputs, transfer.outputs, transfer.slopes),
  )
}

export function sampleCurvePointsPath(points: readonly NormalizedCurveChannelPoint[] | undefined) {
  return createCurveTransferPath(createCurveChannelTransfer(points))
}

function createCurveTransferPath(transfer: ReturnType<typeof createCurveChannelTransfer>) {
  const commands = [
    `M ${formatPathPoint(createTransferPathPoint(transfer.inputs[0], transfer.outputs[0]))}`,
  ]

  for (let index = 0; index < transfer.inputs.length - 1; index += 1) {
    const input = transfer.inputs[index]
    const output = transfer.outputs[index]
    const nextInput = transfer.inputs[index + 1]
    const nextOutput = transfer.outputs[index + 1]
    const inputOffset = (nextInput - input) / 3
    const controlPointA = createTransferPathPoint(
      input + inputOffset,
      output + transfer.slopes[index] * inputOffset,
    )
    const controlPointB = createTransferPathPoint(
      nextInput - inputOffset,
      nextOutput - transfer.slopes[index + 1] * inputOffset,
    )
    const nextPoint = createTransferPathPoint(nextInput, nextOutput)

    commands.push(
      `C ${formatPathPoint(controlPointA)} ${formatPathPoint(controlPointB)} ${formatPathPoint(nextPoint)}`,
    )
  }

  return commands.join(' ')
}

export function applyCurvePreset(presetId: string): RetouchCurvesParams {
  const preset = RETOUCH_CURVE_PRESETS.find((item) => item.id === presetId)
  if (!preset) return { channels: {} }
  return denormalizeCurvesParams(normalizeCurvesParams({ channels: preset.channels }))
}

export function resolveCurvePresetId(params: RetouchCurvesParams) {
  const normalizedParams = serializeNormalizedCurvesParams(normalizeCurvesParams(params))
  for (const preset of RETOUCH_CURVE_PRESETS) {
    const normalizedPreset = serializeNormalizedCurvesParams(
      normalizeCurvesParams({ channels: preset.channels }),
    )
    if (normalizedPreset === normalizedParams) return preset.id
  }
  return 'custom'
}

export function addCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  point: Omit<RetouchCurvePoint, 'id'> & { id?: string },
): RetouchCurvesParams {
  const normalized = normalizeCurveChannelPoints(params.channels[channel] ?? [])
  const input = clampPercent(point.input)
  const nextPoint: RetouchCurvePoint = {
    id: point.id ?? `point:${Date.now().toString(36)}:${input}`,
    input,
    output: clampPercent(point.output, input),
  }
  const nextPoints = normalized.map(({ id, input, output }) => ({ id, input, output }))
  nextPoints.push(nextPoint)
  return setCurveChannelPoints(params, channel, nextPoints)
}

export function moveCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  pointId: string,
  nextValue: Pick<RetouchCurvePoint, 'input' | 'output'>,
): RetouchCurvesParams {
  const normalized = normalizeCurveChannelPoints(params.channels[channel] ?? [])
  const pointIndex = normalized.findIndex((point) => point.id === pointId)
  if (pointIndex < 0) return params

  const point = normalized[pointIndex]
  const previous = normalized[pointIndex - 1]
  const next = normalized[pointIndex + 1]
  const minInput = point.endpoint
    ? point.input
    : (previous?.input ?? 0) + RETOUCH_CURVE_MIN_INPUT_SPACING
  const maxInput = point.endpoint
    ? point.input
    : (next?.input ?? RETOUCH_PLOT_PERCENT_MAX) - RETOUCH_CURVE_MIN_INPUT_SPACING
  const input = clampNumber(nextValue.input, minInput, maxInput, point.input)
  const updated = {
    id: point.id,
    input,
    output: clampPercent(nextValue.output, input),
  }

  const nextPoints = normalized.map((item) =>
    item.id === pointId ? updated : { id: item.id, input: item.input, output: item.output },
  )
  return setCurveChannelPoints(params, channel, nextPoints)
}

export function removeCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  pointId: string,
): RetouchCurvesParams {
  if (pointId === RETOUCH_CURVE_ENDPOINT_BLACK_ID || pointId === RETOUCH_CURVE_ENDPOINT_WHITE_ID) {
    return params
  }
  const nextPoints = normalizeCurveChannelPoints(params.channels[channel] ?? [])
    .filter((point) => point.id !== pointId)
    .map(({ id, input, output }) => ({ id, input, output }))
  return setCurveChannelPoints(params, channel, nextPoints)
}

function isIdentityCurveChannel(points: readonly NormalizedCurveChannelPoint[]) {
  return (
    points.length === 2 &&
    points[0]?.input === 0 &&
    points[0].output === 0 &&
    points[1]?.input === RETOUCH_PLOT_PERCENT_MAX &&
    points[1].output === RETOUCH_PLOT_PERCENT_MAX
  )
}

export function createIdentityCurvePoints() {
  return normalizeCurveChannelPoints([])
}

function isIdentityEndpoint(point: NormalizedCurveChannelPoint) {
  return point.endpoint && point.input === point.output
}

function setCurveChannelPoints(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  points: readonly RetouchCurvePoint[],
): RetouchCurvesParams {
  const normalizedParams = normalizeCurvesParams({
    channels: {
      ...params.channels,
      [channel]: points,
    },
  })
  return denormalizeCurvesParams(normalizedParams)
}

function createNormalizedPoint(
  id: string,
  input: number,
  output: number,
  endpoint: boolean,
): NormalizedCurveChannelPoint {
  return {
    id,
    input,
    output,
    inputChannel: retouchPlotPercentToChannel(input),
    outputChannel: retouchPlotPercentToChannel(output),
    endpoint,
  }
}

function normalizePointId(id: unknown, input: number) {
  const trimmed = typeof id === 'string' ? id.trim() : ''
  if (
    trimmed &&
    trimmed !== RETOUCH_CURVE_ENDPOINT_BLACK_ID &&
    trimmed !== RETOUCH_CURVE_ENDPOINT_WHITE_ID
  ) {
    return trimmed
  }
  return `point:${input}`
}

function clampPercent(value: number, fallback = 0) {
  return clampRetouchRoundedPlotPercent(value, fallback)
}

function isCurveChannelsRecord(
  value: RetouchCurvesParams['channels'] | undefined,
): value is RetouchCurvesParams['channels'] {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isCurvePointRecord(value: unknown): value is Partial<RetouchCurvePoint> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function clampNumber(value: unknown, min: number, max: number, fallback: number) {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback
  return Math.min(max, Math.max(min, value))
}

function composeCurveChannelLuts(first: Uint8ClampedArray, second: Uint8ClampedArray) {
  return createLut((value) => second[first[value]])
}

function createCurveChannelTransfer(points: readonly NormalizedCurveChannelPoint[] | undefined) {
  const normalized = points?.length
    ? normalizeCurveChannelPoints(points)
    : createIdentityCurvePoints()
  const inputs = normalized.map((point) => point.inputChannel)
  const outputs = normalized.map((point) => point.outputChannel)
  return {
    identity: isIdentityCurveChannel(normalized),
    inputs,
    outputs,
    slopes: createMonotoneSlopes(inputs, outputs),
  }
}

function createLut(resolve: (value: number) => number) {
  const lut = new Uint8ClampedArray(256)
  for (let index = RETOUCH_CHANNEL_MIN; index <= RETOUCH_CHANNEL_MAX; index += 1) {
    lut[index] = clampRetouchChannel(resolve(index))
  }
  return lut
}

function createMonotoneSlopes(inputs: readonly number[], outputs: readonly number[]) {
  const pointCount = inputs.length
  const deltas: number[] = []
  const intervals: number[] = []

  for (let index = 0; index < pointCount - 1; index += 1) {
    const interval = Math.max(1, inputs[index + 1] - inputs[index])
    intervals.push(interval)
    deltas.push((outputs[index + 1] - outputs[index]) / interval)
  }

  const slopes = new Array<number>(pointCount).fill(0)
  slopes[0] = deltas[0] ?? 0
  slopes[pointCount - 1] = deltas[deltas.length - 1] ?? 0

  for (let index = 1; index < pointCount - 1; index += 1) {
    const previousDelta = deltas[index - 1]
    const nextDelta = deltas[index]
    if (
      previousDelta === 0 ||
      nextDelta === 0 ||
      Math.sign(previousDelta) !== Math.sign(nextDelta)
    ) {
      slopes[index] = 0
      continue
    }
    const previousInterval = intervals[index - 1]
    const nextInterval = intervals[index]
    const firstWeight = 2 * nextInterval + previousInterval
    const secondWeight = nextInterval + 2 * previousInterval
    slopes[index] =
      (firstWeight + secondWeight) / (firstWeight / previousDelta + secondWeight / nextDelta)
  }

  return slopes
}

function interpolateMonotoneCubic(
  input: number,
  inputs: readonly number[],
  outputs: readonly number[],
  slopes: readonly number[],
) {
  if (input <= inputs[0]) return outputs[0]
  const lastIndex = inputs.length - 1
  if (input >= inputs[lastIndex]) return outputs[lastIndex]

  let segmentIndex = 0
  while (segmentIndex < lastIndex - 1 && input > inputs[segmentIndex + 1]) {
    segmentIndex += 1
  }

  const x0 = inputs[segmentIndex]
  const x1 = inputs[segmentIndex + 1]
  const y0 = outputs[segmentIndex]
  const y1 = outputs[segmentIndex + 1]
  const interval = x1 - x0
  const t = (input - x0) / interval
  const t2 = t * t
  const t3 = t2 * t
  const h00 = 2 * t3 - 3 * t2 + 1
  const h10 = t3 - 2 * t2 + t
  const h01 = -2 * t3 + 3 * t2
  const h11 = t3 - t2
  return (
    h00 * y0 +
    h10 * interval * slopes[segmentIndex] +
    h01 * y1 +
    h11 * interval * slopes[segmentIndex + 1]
  )
}

function serializeNormalizedCurvesParams(params: NormalizedCurvesAdjustment) {
  return JSON.stringify(
    RETOUCH_CURVE_CHANNELS.map((channel) => [
      channel,
      (params.channels[channel] ?? createIdentityCurvePoints()).map((point) => [
        point.input,
        point.output,
      ]),
    ]),
  )
}

function createTransferPathPoint(input: number, output: number) {
  return {
    x: retouchChannelToPlotPercent(input),
    y: RETOUCH_PLOT_PERCENT_MAX - retouchChannelToPlotPercent(output),
  }
}

function formatPathPoint(point: { x: number; y: number }) {
  return `${formatPathNumber(point.x)} ${formatPathNumber(point.y)}`
}

function formatPathNumber(value: number) {
  return Number.isInteger(value)
    ? String(value)
    : value.toFixed(3).replace(/0+$/, '').replace(/\.$/, '')
}
