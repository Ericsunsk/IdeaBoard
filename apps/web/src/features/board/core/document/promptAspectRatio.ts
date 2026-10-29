export const PROMPT_ASPECT_RATIO_OPTIONS = ['auto', '1:1', '3:4', '4:3', '9:16', '16:9'] as const

export type PromptAspectRatio = (typeof PROMPT_ASPECT_RATIO_OPTIONS)[number]

export const DEFAULT_PROMPT_ASPECT_RATIO: PromptAspectRatio = 'auto'

export function normalizePromptAspectRatio(value: unknown): PromptAspectRatio {
  if (typeof value !== 'string') return DEFAULT_PROMPT_ASPECT_RATIO
  return (
    PROMPT_ASPECT_RATIO_OPTIONS.find((option) => option === value) ?? DEFAULT_PROMPT_ASPECT_RATIO
  )
}

export function resolvePromptAspectRatioValue(ratio: PromptAspectRatio | string): number | null {
  const parts = ratio.trim().split(':')
  if (parts.length === 2) {
    const w = Number(parts[0])
    const h = Number(parts[1])
    if (Number.isFinite(w) && Number.isFinite(h) && w > 0 && h > 0) {
      return h / w
    }
  }
  return null
}
