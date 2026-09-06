import {
  normalizeTaskImageResult,
  normalizeTaskImageResults,
  type TaskImageResult,
} from '../../../../shared/query/taskResult'
import { i18n } from '../../../../shared/i18n'
import type { ResolvedTaskImageResult } from './resolvedTaskImageResult'
import { buildMediaBrowserURL, type MediaRef } from '../../../../shared/media/MediaRef'

export type ImageSize = {
  width: number
  height: number
}

export type ImageSizeLoader = (url: string) => Promise<ImageSize | null>

export type { ResolvedTaskImageResult }

function normalizePositiveImageSize(width: unknown, height: unknown): ImageSize | null {
  const widthNumber = Number(width)
  const heightNumber = Number(height)
  if (
    !Number.isFinite(widthNumber) ||
    widthNumber <= 0 ||
    !Number.isFinite(heightNumber) ||
    heightNumber <= 0
  ) {
    return null
  }
  const normalizedWidth = Math.max(1, Math.round(widthNumber))
  const normalizedHeight = Math.max(1, Math.round(heightNumber))
  return { width: normalizedWidth, height: normalizedHeight }
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

  let size = normalizePositiveImageSize(normalized.width, normalized.height)
  if (!size) {
    try {
      size = await loadImageSize(normalized.url)
    } catch (error) {
      console.warn('[canvas] failed to probe task image size', error)
    }
  }

  const ratio = size ? size.height / size.width : fallbackRatio > 0 ? fallbackRatio : 1
  const mediaRef: MediaRef | undefined = normalized.assetId
    ? { assetId: normalized.assetId, derivativeTier: 'preview', derivativeFormat: 'webp' }
    : undefined
  return {
    url: mediaRef ? buildMediaBrowserURL(mediaRef) : normalized.url,
    ratio,
    ...(mediaRef ? { mediaRef } : {}),
    ...(size ? { width: size.width, height: size.height } : {}),
    ...(normalized.mimeType ? { mimeType: normalized.mimeType } : {}),
  }
}
