import {
  normalizeTaskImageResult,
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../../../core/query/taskResult'
import {
  getWorkspaceMediaAssetMetadata,
  resolveWorkspaceMediaAssetDerivativeURL,
} from '../../../../../api/mediaAssetDelivery'
import { i18n } from '../../../../../core/i18n'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'
import type { MediaRef } from '../../../../../core/media/mediaRef'
import { normalizeNaturalSize } from '../../../../../core/media/imageDimensions'
import { sleep } from '../../../../../core/utils/polling'

type ImageSize = {
  width: number
  height: number
}

export type ImageSizeLoader = (
  url: string,
  timeoutMs?: number,
  signal?: AbortSignal,
) => Promise<ImageSize | null>

export type { ResolvedTaskImageResult }

const RECEIPT_DERIVATIVE_WAIT_MS = 10_000
const RECEIPT_DERIVATIVE_POLL_MS = 500

/**
 * Receipt/delivery reconciliation (issue #676): a `completed` task receipt
 * can arrive before the preview derivative finishes materializing. Wait a
 * bounded budget for the derivative to become servable before probing
 * dimensions, so result nodes bind a complete MediaRef instead of racing the
 * asset pipeline. Always resolves (never hangs): on timeout the display
 * layer's own delivery retry takes over.
 */
async function awaitReceiptDerivativeReady(
  workspaceId: string,
  assetId: string,
  signal?: AbortSignal,
): Promise<void> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < RECEIPT_DERIVATIVE_WAIT_MS) {
    try {
      signal?.throwIfAborted()
      const metadata = await getWorkspaceMediaAssetMetadata(workspaceId, assetId, { signal })
      const status = metadata.derivatives.preview?.status ?? metadata.status
      if (status === 'ready' || status === 'failed') return
    } catch (error) {
      signal?.throwIfAborted()
      console.warn('[canvas] failed to await receipt derivative readiness', {
        assetId,
        operation: 'awaitReceiptDerivativeReady',
        error,
      })
      return
    }
    await sleep(RECEIPT_DERIVATIVE_POLL_MS, signal)
  }
}

function pickFirstValidImageResult(results: TaskImageResult[]): TaskImageResult | null {
  return normalizeTaskImageResults(results)[0] ?? null
}

function noImageMessage(): string {
  return i18n.global.t('canvas.execution.taskCompletedNoImage')
}

export async function resolveFirstTaskImageResultDisplay(
  results: TaskImageResult[],
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
): Promise<ResolvedTaskImageResult> {
  const result = pickFirstValidImageResult(results)
  if (!result) throw new Error(noImageMessage())
  const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSize, signal)
  if (!resolved) throw new Error(noImageMessage())
  return resolved
}

export async function resolveTaskImageResultAtIndexDisplay(
  results: TaskImageResult[],
  resultIndex: number,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
): Promise<ResolvedTaskImageResult> {
  const normalized = normalizeTaskImageResults(results)
  const result = normalized.find((candidate) => candidate.resultIndex === resultIndex)
  if (!result) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSize, signal)
  if (!resolved) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  return { ...resolved, resultIndex }
}

export async function resolveTaskImageResultDisplay(
  result: TaskImageResult,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
): Promise<ResolvedTaskImageResult | null> {
  signal?.throwIfAborted()
  const normalized = normalizeTaskImageResult(result)
  if (!normalized) return null

  const url = resolveWorkspaceMediaAssetDerivativeURL(
    normalized.workspaceId,
    normalized.assetId,
    'preview',
  )
  await awaitReceiptDerivativeReady(normalized.workspaceId, normalized.assetId, signal)

  let size = normalizeNaturalSize(Number(normalized.width), Number(normalized.height))
  if (!size) {
    try {
      size = await loadImageSize(url, undefined, signal)
    } catch (error) {
      signal?.throwIfAborted()
      console.warn('[canvas] failed to probe task image size', error)
    }
  }

  signal?.throwIfAborted()
  const ratio = size ? size.height / size.width : fallbackRatio > 0 ? fallbackRatio : 1
  const mediaRef: MediaRef = { assetId: normalized.assetId, derivativeTier: 'preview' }
  return {
    url,
    ratio,
    mediaRef,
    ...(size ? { width: size.width, height: size.height } : {}),
    ...(normalized.mimeType ? { mimeType: normalized.mimeType } : {}),
  }
}
