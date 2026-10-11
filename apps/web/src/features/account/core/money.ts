export const MIN_IDEA_QUANTITY = 500
export const MAX_IDEA_QUANTITY = 500_000

export type IdeaQuantityParseResult = { ok: true; value: number } | { ok: false; error: string }

export function normalizeIdeaQuantity(rawValue: string): number | null {
  const value = rawValue.trim().replaceAll(',', '')
  if (!value) return null

  const quantity = Number(value)
  if (!Number.isFinite(quantity)) return null

  return Math.min(MAX_IDEA_QUANTITY, Math.max(MIN_IDEA_QUANTITY, Math.floor(quantity)))
}
