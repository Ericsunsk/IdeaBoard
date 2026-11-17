import {
  normalizeTaskImageResult,
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../../../core/query/taskResult'
import {
  getWorkspaceMediaAssetMetadata,
  resolveWorkspaceMediaAssetDerivativeURL,
} from '@/api/media/mediaAssetDelivery'
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

/** Shared receipt-derivative budget: event fast-path, hint wait, and poll fallback share one 10s budget. */
export const RECEIPT_DERIVATIVE_BUDGET_MS = 10_000
const RECEIPT_DERIVATIVE_POLL_MS = 500

/** Wait for preview delivery; hints require a metadata read and abort propagates. */
export type ReceiptDerivativeWaiter = (
  workspaceId: string,
  assetId: string,
  signal?: AbortSignal,
) => Promise<void>

/** Poll briefly before probing dimensions; display delivery retries handle timeout. */
export async function awaitReceiptDerivativeReady(
  workspaceId: string,
  assetId: string,
  signal?: AbortSignal,
  budgetMs: number = RECEIPT_DERIVATIVE_BUDGET_MS,
): Promise<void> {
  const startedAt = Date.now()
  while (Date.now() - startedAt < Math.max(0, budgetMs)) {
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
  waitForReceiptDerivative: ReceiptDerivativeWaiter = awaitReceiptDerivativeReady,
): Promise<ResolvedTaskImageResult> {
  const result = pickFirstValidImageResult(results)
  if (!result) throw new Error(noImageMessage())
  const resolved = await resolveTaskImageResultDisplay(
    result,
    fallbackRatio,
    loadImageSize,
    signal,
    waitForReceiptDerivative,
  )
  if (!resolved) throw new Error(noImageMessage())
  return resolved
}

export async function resolveTaskImageResultAtIndexDisplay(
  results: TaskImageResult[],
  resultIndex: number,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
  waitForReceiptDerivative: ReceiptDerivativeWaiter = awaitReceiptDerivativeReady,
): Promise<ResolvedTaskImageResult> {
  const normalized = normalizeTaskImageResults(results)
  const result = normalized.find((candidate) => candidate.resultIndex === resultIndex)
  if (!result) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  const resolved = await resolveTaskImageResultDisplay(
    result,
    fallbackRatio,
    loadImageSize,
    signal,
    waitForReceiptDerivative,
  )
  if (!resolved) throw new Error(i18n.global.t('canvas.execution.taskCompletedNoImageAtIndex'))
  return { ...resolved, resultIndex }
}

async function waitForTaskImagePreview(
  normalized: NonNullable<ReturnType<typeof normalizeTaskImageResult>>,
  waitForReceiptDerivative: ReceiptDerivativeWaiter,
  signal?: AbortSignal,
): Promise<void> {
  if (
    normalized.isPreviewReady === true &&
    normalizeNaturalSize(Number(normalized.width), Number(normalized.height))
  ) {
    return
  }
  try {
    await waitForReceiptDerivative(normalized.workspaceId, normalized.assetId, signal)
  } catch (error) {
    signal?.throwIfAborted()
    console.warn('[canvas] failed to await receipt derivative readiness', {
      assetId: normalized.assetId,
      operation: 'awaitReceiptDerivativeReady',
      error,
    })
  }
}

async function resolveTaskImageSize(
  normalized: NonNullable<ReturnType<typeof normalizeTaskImageResult>>,
  url: string,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
): Promise<{ width: number; height: number } | null> {
  const existing = normalizeNaturalSize(Number(normalized.width), Number(normalized.height))
  if (existing) return existing
  try {
    const metadata = await getWorkspaceMediaAssetMetadata(
      normalized.workspaceId,
      normalized.assetId,
      { signal },
    )
    const originalSize = normalizeNaturalSize(Number(metadata.width), Number(metadata.height))
    if (originalSize) return originalSize
  } catch (error) {
    signal?.throwIfAborted()
    console.warn('[canvas] failed to read task image dimensions', error)
  }
  try {
    return await loadImageSize(url, undefined, signal)
  } catch (error) {
    signal?.throwIfAborted()
    console.warn('[canvas] failed to probe task image size', error)
    return null
  }
}

export async function resolveTaskImageResultDisplay(
  result: TaskImageResult,
  fallbackRatio: number,
  loadImageSize: ImageSizeLoader,
  signal?: AbortSignal,
  waitForReceiptDerivative: ReceiptDerivativeWaiter = awaitReceiptDerivativeReady,
): Promise<ResolvedTaskImageResult | null> {
  signal?.throwIfAborted()
  const normalized = normalizeTaskImageResult(result)
  if (!normalized) return null

  const url = resolveWorkspaceMediaAssetDerivativeURL(
    normalized.workspaceId,
    normalized.assetId,
    'preview',
  )

  await waitForTaskImagePreview(normalized, waitForReceiptDerivative, signal)

  const size = await resolveTaskImageSize(normalized, url, loadImageSize, signal)
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
