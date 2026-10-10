import type { RetouchCurveChannel, RetouchCurvePoint, RetouchCurvesParams } from '../model.ts'
import {
  RETOUCH_PLOT_PERCENT_MAX,
  clampFiniteNumber,
  clampRetouchRoundedPlotPercent,
  composeSingleChannelLut,
  createChannelLut,
  formatPathNumber,
  retouchChannelToPlotPercent,
  retouchPlotPercentToChannel,
} from '../channelScale'
import {
  RETOUCH_CURVE_ENDPOINT_BLACK_ID,
  RETOUCH_CURVE_ENDPOINT_WHITE_ID,
  RETOUCH_CURVE_PRESET_PREFIX,
  makeCurvePointFallbackId,
  makeCurvePointId,
} from '../ids.ts'

export { RETOUCH_CURVE_ENDPOINT_BLACK_ID, RETOUCH_CURVE_ENDPOINT_WHITE_ID } from '../ids.ts'
export { RETOUCH_CURVE_POINT_PREFIX, RETOUCH_CURVE_PRESET_PREFIX } from '../ids.ts'

export const RETOUCH_CURVE_CHANNELS: readonly RetouchCurveChannel[] = [
  'rgb',
  'red',
  'green',
  'blue',
]

const RETOUCH_CURVE_MAX_USER_POINTS = 16
const RETOUCH_CURVE_MIN_INPUT_SPACING = 1

export type NormalizedCurveChannelPoint = RetouchCurvePoint & {
  inputChannel: number
  outputChannel: number
  endpoint: boolean
}

export type NormalizedCurvesEffect = {
  channels: Partial<Record<RetouchCurveChannel, NormalizedCurveChannelPoint[]>>
}

export type RetouchCurvesLuts = {
  red: Uint8ClampedArray
  green: Uint8ClampedArray
  blue: Uint8ClampedArray
}

export const RETOUCH_CURVES_DEFAULT_PARAMS: RetouchCurvesParams = { channels: {} }

export type RetouchCurvePreset = {
  id: string
  labelKey: string
  channels: Partial<Record<RetouchCurveChannel, RetouchCurvePoint[]>>
}

type RetouchCurvePresetDefinition = {
  id: string
  labelKey: string
  rgb: ReadonlyArray<readonly [id: string, input: number, output: number]>
}

const RETOUCH_CURVE_PRESET_DEFINITIONS: readonly RetouchCurvePresetDefinition[] = [
  { id: 'linear', labelKey: 'canvas.retouch.curvesPresets.linear', rgb: [] },
  {
    id: 'brighten',
    labelKey: 'canvas.retouch.curvesPresets.brighten',
    rgb: [[`${RETOUCH_CURVE_PRESET_PREFIX}brighten:midtone`, 50, 63]],
  },
  {
    id: 'darken',
    labelKey: 'canvas.retouch.curvesPresets.darken',
    rgb: [[`${RETOUCH_CURVE_PRESET_PREFIX}darken:midtone`, 50, 38]],
  },
  {
    id: 'soft-contrast',
    labelKey: 'canvas.retouch.curvesPresets.softContrast',
    rgb: [
      [`${RETOUCH_CURVE_PRESET_PREFIX}soft-contrast:shadow`, 28, 23],
      [`${RETOUCH_CURVE_PRESET_PREFIX}soft-contrast:highlight`, 72, 78],
    ],
  },
  {
    id: 'strong-contrast',
    labelKey: 'canvas.retouch.curvesPresets.strongContrast',
    rgb: [
      [`${RETOUCH_CURVE_PRESET_PREFIX}strong-contrast:shadow`, 22, 14],
      [`${RETOUCH_CURVE_PRESET_PREFIX}strong-contrast:highlight`, 78, 88],
    ],
  },
  {
    id: 'matte',
    labelKey: 'canvas.retouch.curvesPresets.matte',
    rgb: [
      [RETOUCH_CURVE_ENDPOINT_BLACK_ID, 0, 12],
      [`${RETOUCH_CURVE_PRESET_PREFIX}matte:shadow`, 32, 36],
      [`${RETOUCH_CURVE_PRESET_PREFIX}matte:highlight`, 82, 88],
    ],
  },
]

export const RETOUCH_CURVE_PRESETS: readonly RetouchCurvePreset[] =
  RETOUCH_CURVE_PRESET_DEFINITIONS.map((preset) => ({
    id: preset.id,
    labelKey: preset.labelKey,
    channels:
      preset.rgb.length === 0
        ? {}
        : {
            rgb: preset.rgb.map(([id, input, output]) => ({ id, input, output })),
          },
  }))

const SERIALIZED_CURVE_PRESETS: readonly { id: string; serialized: string }[] =
  RETOUCH_CURVE_PRESETS.map((preset) => ({
    id: preset.id,
    serialized: serializeNormalizedCurvesParams(
      normalizeCurvesParams({ channels: preset.channels }),
    ),
  }))

export function normalizeCurvesParams(params: RetouchCurvesParams): NormalizedCurvesEffect {
  const channels: NormalizedCurvesEffect['channels'] = {}
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

export function denormalizeCurvesParams(params: NormalizedCurvesEffect): RetouchCurvesParams {
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

export function isNeutralCurvesEffect(params: NormalizedCurvesEffect): boolean {
  return Object.keys(params.channels).length === 0
}

export function isCurvesParams(value: unknown): value is RetouchCurvesParams {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false
  const record = value as Record<string, unknown>
  // `channels` is exclusive to curves: no other kind carries it, so the
  // guard is order-independent against threshold/sharpen/levels/hsl.
  return (
    'channels' in record &&
    !('hue' in record) &&
    !('amount' in record) &&
    !('shadows' in record) &&
    !('threshold' in record && 'invert' in record)
  )
}

export function cloneCurvesParams(params: RetouchCurvesParams): RetouchCurvesParams {
  // Sanitizing clone: round-trip through normalize/denormalize so copies are
  // deep and endpoint-locked, matching the previous layerStack clone path.
  return denormalizeCurvesParams(normalizeCurvesParams(params))
}

export function cloneCurvesDefaults(): RetouchCurvesParams {
  return { channels: { ...RETOUCH_CURVES_DEFAULT_PARAMS.channels } }
}

export function createCurvesLuts(params: NormalizedCurvesEffect): RetouchCurvesLuts {
  const rgbLut = createCurveChannelLut(params.channels.rgb)
  return {
    red: composeSingleChannelLut(rgbLut, createCurveChannelLut(params.channels.red)),
    green: composeSingleChannelLut(rgbLut, createCurveChannelLut(params.channels.green)),
    blue: composeSingleChannelLut(rgbLut, createCurveChannelLut(params.channels.blue)),
  }
}

function createCurveChannelLut(
  points: readonly NormalizedCurveChannelPoint[] | undefined,
): Uint8ClampedArray {
  const transfer = createCurveChannelTransfer(points)
  if (transfer.identity) return createChannelLut((value) => value)

  return createChannelLut((value) =>
    interpolateMonotoneCubic(value, transfer.inputs, transfer.outputs, transfer.slopes),
  )
}

export function sampleCurvePointsPath(points: readonly NormalizedCurveChannelPoint[] | undefined) {
  return createCurveTransferPath(createCurveChannelTransfer(points))
}

function createCurveTransferPath(transfer: ReturnType<typeof createCurveChannelTransfer>) {
  const firstInput = transfer.inputs[0] ?? 0
  const firstOutput = transfer.outputs[0] ?? 0
  const commands = [`M ${formatPathPoint(createTransferPathPoint(firstInput, firstOutput))}`]

  for (let index = 0; index < transfer.inputs.length - 1; index += 1) {
    const input = transfer.inputs[index] ?? 0
    const output = transfer.outputs[index] ?? 0
    const nextInput = transfer.inputs[index + 1] ?? 0
    const nextOutput = transfer.outputs[index + 1] ?? 0
    const inputOffset = (nextInput - input) / 3
    const controlPointA = createTransferPathPoint(
      input + inputOffset,
      output + (transfer.slopes[index] ?? 0) * inputOffset,
    )
    const controlPointB = createTransferPathPoint(
      nextInput - inputOffset,
      nextOutput - (transfer.slopes[index + 1] ?? 0) * inputOffset,
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
  for (const preset of SERIALIZED_CURVE_PRESETS) {
    if (preset.serialized === normalizedParams) return preset.id
  }
  return 'custom'
}

export function addCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  point: Omit<RetouchCurvePoint, 'id'> & { id?: string },
): RetouchCurvesParams {
  const input = clampPercent(point.input)
  const nextPoint: RetouchCurvePoint = {
    id: point.id ?? makeCurvePointId(input),
    input,
    output: clampPercent(point.output, input),
  }
  return updateChannelPoints(params, channel, (normalized) => [
    ...normalized.map(toPlainCurvePoint),
    nextPoint,
  ])
}

export function moveCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  pointId: string,
  nextValue: Pick<RetouchCurvePoint, 'input' | 'output'>,
): RetouchCurvesParams {
  return updateChannelPoints(params, channel, (normalized) => {
    const pointIndex = normalized.findIndex((point) => point.id === pointId)
    if (pointIndex < 0) return null
    const point = normalized[pointIndex]
    if (!point) return null
    const { endpoint, input: currentInput, id } = point
    const { input: previousInput = 0 } = normalized[pointIndex - 1] ?? {}
    const { input: nextInput = RETOUCH_PLOT_PERCENT_MAX } = normalized[pointIndex + 1] ?? {}
    const minInput = endpoint ? currentInput : previousInput + RETOUCH_CURVE_MIN_INPUT_SPACING
    const maxInput = endpoint ? currentInput : nextInput - RETOUCH_CURVE_MIN_INPUT_SPACING
    const input = clampFiniteNumber(nextValue.input, minInput, maxInput, currentInput)
    const updated = {
      id,
      input,
      output: clampPercent(nextValue.output, input),
    }
    return normalized.map((item) => (item.id === pointId ? updated : toPlainCurvePoint(item)))
  })
}

export function removeCurvePoint(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  pointId: string,
): RetouchCurvesParams {
  if (pointId === RETOUCH_CURVE_ENDPOINT_BLACK_ID || pointId === RETOUCH_CURVE_ENDPOINT_WHITE_ID) {
    return params
  }
  return updateChannelPoints(params, channel, (normalized) =>
    normalized.filter((point) => point.id !== pointId).map(toPlainCurvePoint),
  )
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

function updateChannelPoints(
  params: RetouchCurvesParams,
  channel: RetouchCurveChannel,
  update: (
    normalized: readonly NormalizedCurveChannelPoint[],
  ) => readonly RetouchCurvePoint[] | null,
): RetouchCurvesParams {
  const normalized = normalizeCurveChannelPoints(params.channels[channel] ?? [])
  const nextPoints = update(normalized)
  if (!nextPoints) return params
  return setCurveChannelPoints(params, channel, nextPoints)
}

function toPlainCurvePoint({
  id,
  input,
  output,
}: Pick<RetouchCurvePoint, 'id' | 'input' | 'output'>): RetouchCurvePoint {
  return { id, input, output }
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
  return makeCurvePointFallbackId(input)
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

function createCurveChannelTransfer(points: readonly NormalizedCurveChannelPoint[] | undefined) {
  const normalized = points?.length ? points : createIdentityCurvePoints()
  const inputs = normalized.map((point) => point.inputChannel)
  const outputs = normalized.map((point) => point.outputChannel)
  return {
    identity: isIdentityCurveChannel(normalized),
    inputs,
    outputs,
    slopes: createMonotoneSlopes(inputs, outputs),
  }
}

function createMonotoneSlopes(inputs: readonly number[], outputs: readonly number[]) {
  const pointCount = inputs.length
  const deltas: number[] = []
  const intervals: number[] = []

  for (let index = 0; index < pointCount - 1; index += 1) {
    const interval = Math.max(1, (inputs[index + 1] ?? 0) - (inputs[index] ?? 0))
    intervals.push(interval)
    deltas.push(((outputs[index + 1] ?? 0) - (outputs[index] ?? 0)) / interval)
  }

  const slopes = new Array<number>(pointCount).fill(0)
  slopes[0] = deltas[0] ?? 0
  slopes[pointCount - 1] = deltas[deltas.length - 1] ?? 0

  for (let index = 1; index < pointCount - 1; index += 1) {
    slopes[index] = resolveMonotoneSlopeAt(deltas, intervals, index)
  }

  return slopes
}

function resolveMonotoneSlopeAt(
  deltas: readonly number[],
  intervals: readonly number[],
  index: number,
): number {
  const previousDelta = deltas[index - 1]
  const nextDelta = deltas[index]
  const previousInterval = intervals[index - 1]
  const nextInterval = intervals[index]
  if (
    previousDelta === undefined ||
    nextDelta === undefined ||
    previousInterval === undefined ||
    nextInterval === undefined ||
    previousDelta === 0 ||
    nextDelta === 0 ||
    Math.sign(previousDelta) !== Math.sign(nextDelta)
  ) {
    return 0
  }
  const firstWeight = 2 * nextInterval + previousInterval
  const secondWeight = nextInterval + 2 * previousInterval
  return (firstWeight + secondWeight) / (firstWeight / previousDelta + secondWeight / nextDelta)
}

function interpolateMonotoneCubic(
  input: number,
  inputs: readonly number[],
  outputs: readonly number[],
  slopes: readonly number[],
) {
  const firstInput = inputs[0]
  const firstOutput = outputs[0]
  if (firstInput === undefined || firstOutput === undefined) return 0
  if (input <= firstInput) return firstOutput
  const lastIndex = inputs.length - 1
  const lastInput = inputs[lastIndex]
  const lastOutput = outputs[lastIndex]
  if (lastInput === undefined || lastOutput === undefined) return 0
  if (input >= lastInput) return lastOutput

  const segmentIndex = findCurveSegmentIndex(inputs, input, lastIndex)
  const x0 = inputs[segmentIndex] ?? 0
  const x1 = inputs[segmentIndex + 1] ?? 0
  const y0 = outputs[segmentIndex] ?? 0
  const y1 = outputs[segmentIndex + 1] ?? 0
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
    h10 * interval * (slopes[segmentIndex] ?? 0) +
    h01 * y1 +
    h11 * interval * (slopes[segmentIndex + 1] ?? 0)
  )
}

function findCurveSegmentIndex(
  inputs: readonly number[],
  input: number,
  lastIndex: number,
): number {
  let segmentIndex = 0
  while (segmentIndex < lastIndex - 1) {
    const nextSegmentInput = inputs[segmentIndex + 1]
    if (nextSegmentInput === undefined || input <= nextSegmentInput) break
    segmentIndex += 1
  }
  return segmentIndex
}

function serializeNormalizedCurvesParams(params: NormalizedCurvesEffect) {
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
