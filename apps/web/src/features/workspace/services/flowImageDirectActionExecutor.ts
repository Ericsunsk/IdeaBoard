import { taskApi } from '../../../shared/api/task'
import { vectorizeApi } from '../../../shared/api/vectorize'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  buildExecutionTaskRequest,
  EXECUTION_DIRECT_ACTION_META,
  EXECUTION_EDIT_MODE_META,
} from '../config/executionContract'
import { buildExecutionSizeParams } from '../utils/executionSizeParams'
import { buildPromptAspectRatioTaskParams, type PromptAspectRatio } from '../config/promptAspectRatio'
import { extractTaskImageUrls } from '../utils/flowImageDirectActionTaskResult'
import { assertTaskSubmitReady, waitTaskResult } from './flowTaskExecutorUtils'
import type { ImageDirectActionSource } from '../utils/flowImageDirectActionSource'

export type ImageDirectActionTaskKey = 'remove-bg' | 'style-innovate' | 'sketch-to-design'

type ImageSizeLoader = (
  url: string,
) => Promise<{ width: number; height: number } | null>

const STYLE_INNOVATE_DEFAULT_STRENGTH = 50

export async function assertDirectActionSubmitReady(
  action: ImageDirectActionTaskKey,
): Promise<void> {
  const feature =
    action === 'style-innovate'
      ? EXECUTION_DIRECT_ACTION_META.styleInnovate.feature
      : action === 'remove-bg'
        ? EXECUTION_DIRECT_ACTION_META.removeBg.feature
        : EXECUTION_DIRECT_ACTION_META.sketchToDesign.feature
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature,
    }),
    '提交任务失败',
  )
}

export async function assertPromptImageSubmitReady(): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: EXECUTION_EDIT_MODE_META['partial-edit'].feature,
    }),
    '提交任务失败',
  )
}

export async function submitPromptTextGenerationTask(
  prompt: string,
  aspectRatio: PromptAspectRatio,
): Promise<string> {
  const request = buildExecutionTaskRequest({
    prompt,
    params: buildPromptAspectRatioTaskParams(aspectRatio),
  })
  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitDirectActionTask(
  source: ImageDirectActionSource,
  action: ImageDirectActionTaskKey,
  loadImageSize: ImageSizeLoader,
  strengthOverride?: number,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const strength = Number.isFinite(strengthOverride)
    ? Math.max(0, Math.min(100, Math.round(strengthOverride as number)))
    : STYLE_INNOVATE_DEFAULT_STRENGTH
  const request =
    action === 'style-innovate'
      ? buildExecutionTaskRequest({
          prompt: '',
          feature: EXECUTION_DIRECT_ACTION_META.styleInnovate.feature,
          imageUrls: [source.url],
          params: {
            original_image_url: source.url,
            ...buildExecutionSizeParams(imageSize),
            variation_strength: strength,
            strength,
          },
        })
      : buildExecutionTaskRequest({
          prompt: '',
          feature:
            action === 'remove-bg'
              ? EXECUTION_DIRECT_ACTION_META.removeBg.feature
              : EXECUTION_DIRECT_ACTION_META.sketchToDesign.feature,
          imageUrls: [source.url],
          params: buildExecutionSizeParams(imageSize),
        })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitPromptImageGenerationTask(
  source: ImageDirectActionSource,
  prompt: string,
  loadImageSize: ImageSizeLoader,
  aspectRatio: PromptAspectRatio,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_EDIT_MODE_META['partial-edit'].promptPrefix} ${prompt}`,
    feature: EXECUTION_EDIT_MODE_META['partial-edit'].feature,
    imageUrls: [source.url],
    params: {
      original_image_url: source.url,
      ...buildExecutionSizeParams(imageSize),
      ...buildPromptAspectRatioTaskParams(aspectRatio),
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function runToVector(source: ImageDirectActionSource): Promise<string[]> {
  const res = await vectorizeApi.vectorize({
    image_url: source.url,
    threshold: 0.45,
    turd_size: 2,
    alpha_max: 1.0,
    opt_tolerance: 0.2,
    turn_policy: 'minority',
    invert: false,
    color_count: 0,
  })
  if (res.code !== 0 || !res.data?.svg_url) {
    throw new Error(sanitizeApiErrorMessage(res.msg, 'Vectorize failed'))
  }
  return [res.data.svg_url]
}

export async function waitExistingOrLiveTaskResult(taskId: string): Promise<string[]> {
  let task:
    | {
        status?: string
        outputs?: Array<{ role?: string; url?: string }>
        result?: string | null
        error_msg?: string
      }
    | undefined
  try {
    const taskRes = await taskApi.get(taskId)
    task = taskRes.data?.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskResult(taskId)
  }
  if (task?.status === 'completed') {
    const urls = extractTaskImageUrls(task)
    if (urls.length > 0) return urls
    throw new Error('任务完成但未返回图片')
  }
  if (task?.status === 'failed') {
    throw new Error(sanitizeApiErrorMessage(task.error_msg, '生成失败'))
  }
  return waitTaskResult(taskId)
}
