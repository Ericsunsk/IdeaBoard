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

/**
 * Canonical catalog parser: only 1k/2k/4k are accepted, never a default.
 * Matching API ingress, case and surrounding whitespace are normalized once
 * at this boundary and the canonical lowercase value is returned.
 */
export function parseOutputResolution(value: unknown): OutputResolution | null {
  if (typeof value !== 'string') return null
  const normalized = value.trim().toLowerCase()
  return isOutputResolution(normalized) ? normalized : null
}

export function assertOutputResolution(value: unknown): OutputResolution {
  const parsed = parseOutputResolution(value)
  if (parsed === null) {
    throw new Error('output resolution must be one of 1k, 2k, 4k')
  }
  return parsed
}

/**
 * Snapshot repair normalizer for persisted graph state: a missing, blank,
 * or corrupt resolution reads as the Service policy default 1k (ADR-0012
 * rule 3) so legacy boards stay loadable. It is NOT a catalog validation
 * seam — catalog writes (submit, quote, pool save) and form validation
 * must use parseOutputResolution and surface the error instead of
 * coercing.
 */
export function normalizeOutputResolution(value: unknown): OutputResolution {
  if (value === null || value === undefined) return '1k'
  if (typeof value === 'string' && value.trim() === '') return '1k'
  return parseOutputResolution(value) ?? '1k'
}
