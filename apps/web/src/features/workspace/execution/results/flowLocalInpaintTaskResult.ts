import type { TaskImageResult } from '../../../../shared/query/taskResult'
import { composeFlowLocalInpaint, type PreparedFlowLocalInpaintMedia } from '../media/flowLocalInpaint'
import {
  resolveFirstTaskImageResultDisplay,
  resolveTaskImageRatio,
  type ImageSizeLoader,
  type ResolvedTaskImageResult,
} from './flowImageDirectActionTaskResult'

interface ComposeFlowLocalInpaintTaskResultOptions {
  originalImageUrl: string
  croppedTaskResults: TaskImageResult[]
  prepared: PreparedFlowLocalInpaintMedia
  boardId?: string
  fallbackRatio: number
  loadImageSize: ImageSizeLoader
}

export async function composeFlowLocalInpaintTaskResult(
  options: ComposeFlowLocalInpaintTaskResultOptions,
): Promise<ResolvedTaskImageResult> {
  const croppedResult = await resolveFirstTaskImageResultDisplay(
    options.croppedTaskResults,
    options.fallbackRatio,
    options.loadImageSize,
  )
  const finalUrl = await composeFlowLocalInpaint({
    originalImageUrl: options.originalImageUrl,
    croppedResultUrl: croppedResult.url,
    croppedMaskUrl: options.prepared.crop.croppedMaskUrl,
    bbox: options.prepared.crop.bbox,
    boardId: options.boardId,
  })
  const ratio = await resolveTaskImageRatio(finalUrl, options.fallbackRatio, options.loadImageSize)
  return { url: finalUrl, ratio }
}
