import { buildMediaBrowserURL } from '../media/MediaRef'

export type TaskImageResult = {
  url: string
  assetId?: string
  width?: number
  height?: number
  mimeType?: string
  resultIndex?: number
}

type TaskImageResultInput = {
  width?: unknown
  height?: unknown
  mimeType?: unknown
  assetId?: unknown
  asset_id?: unknown
  result_index?: unknown
  resultIndex?: unknown
}

type TaskResultPayloadLike = {
  outputs?: Array<{
    role?: string
    width?: unknown
    height?: unknown
    mime_type?: unknown
    asset_id?: unknown
    result_index?: unknown
  }>
}

function toPositiveNumber(value: unknown): number | undefined {
  const num = Number(value)
  return Number.isFinite(num) && num > 0 ? num : undefined
}

export function normalizeTaskImageResult(result: TaskImageResultInput): TaskImageResult | null {
  const assetId = `${result?.assetId ?? result?.asset_id ?? ''}`.trim()
  if (!assetId) return null
  const url = buildMediaBrowserURL({ assetId, derivativeTier: 'preview', derivativeFormat: 'webp' })
  const width = toPositiveNumber(result?.width)
  const height = toPositiveNumber(result?.height)
  const mimeType = `${result?.mimeType ?? ''}`.trim()
  const resultIndex = Number(result?.result_index ?? result?.resultIndex)
  const normalized: TaskImageResult = {
    url,
    ...(assetId ? { assetId } : {}),
    ...(width ? { width } : {}),
    ...(height ? { height } : {}),
    ...(mimeType ? { mimeType } : {}),
    ...(Number.isInteger(resultIndex) && resultIndex >= 0 ? { resultIndex } : {}),
  }
  return normalized
}

export function normalizeTaskImageResults(results: TaskImageResultInput[]): TaskImageResult[] {
  return results
    .map(normalizeTaskImageResult)
    .filter((result): result is TaskImageResult => result !== null)
}

export function extractImageResultsFromTaskPayload(task: TaskResultPayloadLike): TaskImageResult[] {
  const outputResults = Array.isArray(task.outputs)
    ? task.outputs
        .filter((output) => output?.role === 'image')
        .map((output) =>
          normalizeTaskImageResult({
            width: output.width,
            height: output.height,
            mimeType: output.mime_type,
            asset_id: output.asset_id,
            result_index: output.result_index,
          }),
        )
        .filter((result): result is TaskImageResult => result !== null)
    : []
  if (outputResults.length > 0) return outputResults
  // Raw provider result JSON is intentionally not a browser Media contract.
  return []
}
