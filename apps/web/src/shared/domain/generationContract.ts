export const GENERATION_PRIMITIVE = {
  textToImage: 'text-to-image',
  imageToImage: 'image-to-image',
} as const

export type GenerationPrimitive = (typeof GENERATION_PRIMITIVE)[keyof typeof GENERATION_PRIMITIVE]

export const GENERATION_PRIMITIVE_OPTIONS = Object.values(
  GENERATION_PRIMITIVE,
) as readonly GenerationPrimitive[]

export function isGenerationPrimitive(value: unknown): value is GenerationPrimitive {
  return (
    typeof value === 'string' && GENERATION_PRIMITIVE_OPTIONS.includes(value as GenerationPrimitive)
  )
}

export function parseGenerationPrimitive(value: unknown): GenerationPrimitive | null {
  return isGenerationPrimitive(value) ? value : null
}

export const MIN_REQUESTED_OUTPUT_COUNT = 1
export const MAX_REQUESTED_OUTPUT_COUNT = 4
export const REQUESTED_OUTPUT_COUNT_OPTIONS = [1, 2, 3, 4] as const

export type RequestedOutputCount = (typeof REQUESTED_OUTPUT_COUNT_OPTIONS)[number]

export function isRequestedOutputCount(value: unknown): value is RequestedOutputCount {
  return (
    typeof value === 'number' &&
    Number.isInteger(value) &&
    value >= MIN_REQUESTED_OUTPUT_COUNT &&
    value <= MAX_REQUESTED_OUTPUT_COUNT
  )
}

/** Strict boundary parser. Invalid values are rejected instead of clamped. */
export function parseRequestedOutputCount(value: unknown): RequestedOutputCount | null {
  return isRequestedOutputCount(value) ? value : null
}

export function assertRequestedOutputCount(value: unknown): RequestedOutputCount {
  const parsed = parseRequestedOutputCount(value)
  if (parsed === null) {
    throw new Error('requested output count must be an integer between 1 and 4')
  }
  return parsed
}

export const OUTPUT_RESOLUTION_OPTIONS = ['1k', '2k', '4k'] as const
export type OutputResolution = (typeof OUTPUT_RESOLUTION_OPTIONS)[number]

export function isOutputResolution(value: unknown): value is OutputResolution {
  return typeof value === 'string' && OUTPUT_RESOLUTION_OPTIONS.includes(value as OutputResolution)
}

export function normalizeOutputResolution(value: unknown): OutputResolution {
  return isOutputResolution(value) ? value : '1k'
}
