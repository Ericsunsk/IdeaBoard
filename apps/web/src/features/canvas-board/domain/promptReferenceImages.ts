import { getCanvasNodeImageInputLimit } from './canvasNodeRegistry'

export function normalizePromptReferenceImageUrls(
  imageUrls: readonly unknown[] | null | undefined,
): string[] {
  const imageLimit = getCanvasNodeImageInputLimit('prompt')
  return (imageUrls ?? [])
    .map((url) => `${url ?? ''}`.trim())
    .filter(Boolean)
    .slice(0, imageLimit)
}
