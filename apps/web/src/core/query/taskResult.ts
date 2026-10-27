type TaskAssetImageResult = {
  workspaceId: string
  assetId: string
  url?: never
  width?: number
  height?: number
  mimeType?: string
  resultIndex?: number
}

// Remote refs remain transport facts; Canvas materializes managed assets only.
export type TaskImageResult =
  TaskAssetImageResult | { url: string; workspaceId?: never; assetId?: never; resultIndex?: number }

type TaskImageResultInput = {
  workspaceId?: unknown
  assetId?: unknown
  url?: unknown
  width?: unknown
  height?: unknown
  mimeType?: unknown
  resultIndex?: unknown
}

function toPositiveNumber(value: unknown): number | undefined {
  const num = Number(value)
  return Number.isFinite(num) && num > 0 ? num : undefined
}

/** Normalize asset facts without reading Workspace selection or deriving URLs. */
export function normalizeTaskImageResult(
  result: TaskImageResultInput,
): TaskAssetImageResult | null {
  const workspaceId = typeof result.workspaceId === 'string' ? result.workspaceId.trim() : ''
  const assetId = typeof result.assetId === 'string' ? result.assetId.trim() : ''
  if (!workspaceId || !assetId) return null
  const width = toPositiveNumber(result.width)
  const height = toPositiveNumber(result.height)
  const mimeType = typeof result.mimeType === 'string' ? result.mimeType.trim() : ''
  const resultIndex = Number(result.resultIndex)
  return {
    workspaceId,
    assetId,
    ...(width ? { width } : {}),
    ...(height ? { height } : {}),
    ...(mimeType ? { mimeType } : {}),
    ...(Number.isInteger(resultIndex) && resultIndex >= 0 ? { resultIndex } : {}),
  }
}

export function normalizeTaskImageResults(results: TaskImageResultInput[]): TaskAssetImageResult[] {
  return results
    .map(normalizeTaskImageResult)
    .filter((result): result is TaskAssetImageResult => result !== null)
}
