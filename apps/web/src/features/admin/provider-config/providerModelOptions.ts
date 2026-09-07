import { uniqueTrimmedModelValues } from './providerConfigPayload'

export const MAX_DETECTED_MODELS = 100

export function mergeDetectedModelValues(
  currentValues: string[],
  detectedValues: string[],
  limit = MAX_DETECTED_MODELS,
) {
  return uniqueTrimmedModelValues([...currentValues, ...detectedValues]).slice(0, limit)
}

export function createModelSuggestions(input: {
  detectedValues: string[]
  selectedValues: string[]
  query?: string | undefined
}) {
  const allModels = uniqueTrimmedModelValues([...input.detectedValues, ...input.selectedValues])
  const query = (input.query || '').trim().toLowerCase()
  if (!query) return allModels
  return allModels.filter((model) => model.toLowerCase().includes(query))
}

export function normalizeDetectedModelValues(values: string[], limit = MAX_DETECTED_MODELS) {
  return uniqueTrimmedModelValues(values).slice(0, limit)
}
