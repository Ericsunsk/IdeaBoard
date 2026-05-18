export const PROMPT_ASPECT_RATIO_OPTIONS = [
  'auto',
  '1:1',
  '3:4',
  '4:3',
  '9:16',
  '16:9',
] as const

export type PromptAspectRatio = (typeof PROMPT_ASPECT_RATIO_OPTIONS)[number]

export const DEFAULT_PROMPT_ASPECT_RATIO: PromptAspectRatio = 'auto'

export function normalizePromptAspectRatio(value: unknown): PromptAspectRatio {
  if (typeof value !== 'string') return DEFAULT_PROMPT_ASPECT_RATIO
  return PROMPT_ASPECT_RATIO_OPTIONS.find((option) => option === value) ?? DEFAULT_PROMPT_ASPECT_RATIO
}

export function buildPromptAspectRatioTaskParams(
  ratio: PromptAspectRatio,
): Record<string, string> {
  if (ratio === 'auto') return {}
  return { ratio }
}
