import {
  normalizeTaskImageResult,
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../../../core/query/taskResult'
import { getMediaAssetMetadata } from '../../../../../api/mediaAssetDelivery'
import { i18n } from '../../../../../core/i18n'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'
import { buildMediaBrowserURL, type MediaRef } from '../../../../../core/media/mediaRef'
import { normalizeNaturalSize } from '../../../../../core/media/imageDimensions'
import { sleep } from '../../../../../core/utils/polling'

export type ImageSize = {
  width: number
  height: number
}

export type ImageSizeLoader = (url: string) => Promise<ImageSize | null>

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
async function awaitReceiptDerivativeReady(assetId: string): Promise<void> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < RECEIPT_DERIVATIVE_WAIT_MS) {
    try {
      const metadata = await getMediaAssetMetadata(assetId)
      const status = metadata.derivatives.preview?.status ?? metadata.status
      if (status === 'ready' || status === 'failed') return
    } catch (error) {
      console.warn('[canvas] failed to await receipt derivative readiness', {
        assetId,
        operation: 'awaitReceiptDerivativeReady',
        error,
      })
      return
    }
    await sleep(RECEIPT_DERIVATIVE_POLL_MS)
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
): Promise<ResolvedTaskImageResult> {
  const result = pickFirstValidImageResult(results)
  if (!result) throw new Error(noImageMessage())
  const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSize)
  if (!resolved) throw new Error(noImageMessage())
  return resolved
}

export async function resolveTaskImageResultAtIndexDisplay(
  results: TaskImageResult[],
  resultIndex: number,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
): Promise<ResolvedTaskImageResult> {
  const normalized = normalizeTaskImageResults(results)
  const result = normalized.find((candidate) => candidate.resultIndex === resultIndex)
  if (!result) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  const resolved = await resolveTaskImageResultDisplay(result, fallbackRatio, loadImageSize)
  if (!resolved) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  return { ...resolved, resultIndex }
}

export async function resolveTaskImageResultDisplay(
  result: TaskImageResult,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
): Promise<ResolvedTaskImageResult | null> {
  const normalized = normalizeTaskImageResult(result)
  if (!normalized) return null

  if (normalized.assetId) await awaitReceiptDerivativeReady(normalized.assetId)

  let size = normalizeNaturalSize(Number(normalized.width), Number(normalized.height))
  if (!size) {
    try {
      size = await loadImageSize(normalized.url)
    } catch (error) {
      console.warn('[canvas] failed to probe task image size', error)
    }
  }

  const ratio = size ? size.height / size.width : fallbackRatio > 0 ? fallbackRatio : 1
  const mediaRef: MediaRef | undefined = normalized.assetId
    ? { assetId: normalized.assetId, derivativeTier: 'preview' }
    : undefined
  return {
    url: mediaRef ? buildMediaBrowserURL(mediaRef) : normalized.url,
    ratio,
    ...(mediaRef ? { mediaRef } : {}),
    ...(size ? { width: size.width, height: size.height } : {}),
    ...(normalized.mimeType ? { mimeType: normalized.mimeType } : {}),
  }
}
