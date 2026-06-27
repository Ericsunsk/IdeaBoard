import { taskApi } from '../../../shared/api/task'
import {
  createTaskCancelledError,
  waitTaskImageResultsWithSse,
} from '../../../shared/api/taskEvents'
import {
  extractImageResultsFromTaskPayload,
  type TaskImageResult,
} from '../../../shared/query/taskResult'
import { sanitizeApiErrorMessage } from '../../../shared/utils/errorMessages'
import {
  buildExecutionTaskRequest,
  EXECUTION_FEATURE,
  EXECUTION_META,
  EXECUTION_PARTIAL_EDIT_META,
} from '../config/executionContract'
import { buildExecutionSizeParams, extendExecutionImageSize } from '../utils/executionSizeParams'
import { getCanvasNodeImageInputLimit, type PromptAspectRatio } from '../../canvas-board/domain'
import { buildPromptAspectRatioTaskParams } from './promptAspectRatioTaskParams'
import { assertTaskSubmitReady } from './flowTaskExecutorUtils'
import type { ImageDirectActionSource } from '../utils/flowImageDirectActionSource'
import {
  DEFAULT_IMAGE_ACTION_STRENGTH,
  IMAGE_ACTION_REGISTRY,
  IMAGE_CANVAS_EXTEND_ACTION,
  IMAGE_HD_UPSCALE_ACTION,
  requireImageActionExecution,
  type ImageDirectTaskAction,
} from '../config/imageActionRegistry'

type ImageSizeLoader = (url: string) => Promise<{ width: number; height: number } | null>

type CanvasExtendPercent = {
  top: number
  right: number
  bottom: number
  left: number
}

type SubmitCanvasExtendTaskInput = {
  source: ImageDirectActionSource
  loadImageSize: ImageSizeLoader
  extendPercent: CanvasExtendPercent
  prompt?: string
}

type SubmitImageInpaintTaskInput = {
  prompt: string
  croppedImageUrl: string
  croppedMaskUrl: string
  referenceImageUrls?: string[]
  cropSize: { width: number; height: number } | null
}

export function resolveHdClarityStrength(strength: number | undefined): number {
  return Number.isFinite(strength)
    ? Math.max(0, Math.min(100, Math.round(strength as number)))
    : DEFAULT_IMAGE_ACTION_STRENGTH
}

export async function assertDirectActionSubmitReady(action: ImageDirectTaskAction): Promise<void> {
  const actionMeta = IMAGE_ACTION_REGISTRY[action]
  const execution = requireImageActionExecution(action)
  const params =
    'strength' in actionMeta && actionMeta.strength.paramPath === 'variation.strength'
      ? { variation: { strength: actionMeta.strength.defaultValue } }
      : undefined
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: execution.feature,
      params,
    }),
    '提交任务失败',
  )
}

export async function assertPromptImageSubmitReady(): Promise<void> {
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: EXECUTION_PARTIAL_EDIT_META.feature,
    }),
    '提交任务失败',
  )
}

export async function assertCanvasExtendSubmitReady(): Promise<void> {
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: execution.feature,
      params: {
        extend: {
          top_percent: 0,
          right_percent: 1,
          bottom_percent: 0,
          left_percent: 0,
        },
      },
    }),
    '提交任务失败',
  )
}

export async function assertHdUpscaleSubmitReady(): Promise<void> {
  const actionMeta = IMAGE_ACTION_REGISTRY[IMAGE_HD_UPSCALE_ACTION]
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  await assertTaskSubmitReady(
    buildExecutionTaskRequest({
      prompt: 'submit readiness',
      feature: execution.feature,
      params: {
        hd: {
          clarity_strength: actionMeta.strength.defaultValue,
        },
      },
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
    feature: EXECUTION_META[EXECUTION_FEATURE.patternGen].feature,
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
  action: ImageDirectTaskAction,
  loadImageSize: ImageSizeLoader,
  strengthOverride?: number,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const actionMeta = IMAGE_ACTION_REGISTRY[action]
  const execution = requireImageActionExecution(action)
  const strength = Number.isFinite(strengthOverride)
    ? Math.max(0, Math.min(100, Math.round(strengthOverride as number)))
    : DEFAULT_IMAGE_ACTION_STRENGTH
  const request =
    'strength' in actionMeta && actionMeta.strength.paramPath === 'variation.strength'
      ? buildExecutionTaskRequest({
          prompt: '',
          feature: execution.feature,
          imageUrls: [source.url],
          params: {
            ...buildExecutionSizeParams(imageSize),
            variation: {
              strength,
            },
          },
        })
      : buildExecutionTaskRequest({
          prompt: '',
          feature: execution.feature,
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
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
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

export async function submitImageInpaintTask(input: SubmitImageInpaintTaskInput): Promise<string> {
  const referenceImageLimit = getCanvasNodeImageInputLimit('prompt')
  const referenceImageUrls = (input.referenceImageUrls ?? [])
    .map((url) => url.trim())
    .filter(Boolean)
    .slice(0, referenceImageLimit)
  const request = buildExecutionTaskRequest({
    prompt: `${EXECUTION_PARTIAL_EDIT_META.promptPrefix} ${input.prompt}`,
    feature: EXECUTION_PARTIAL_EDIT_META.feature,
    imageUrls: [input.croppedImageUrl],
    params: {
      original_image_url: input.croppedImageUrl,
      mask_image_url: input.croppedMaskUrl,
      reference_image_urls: referenceImageUrls,
      use_mask: true,
      ...buildExecutionSizeParams(input.cropSize),
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交局部重绘任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitCanvasExtendTask(input: SubmitCanvasExtendTaskInput): Promise<string> {
  const imageSize = await input.loadImageSize(input.source.url)
  const prompt = `${input.prompt ?? ''}`.trim()
  const execution = requireImageActionExecution(IMAGE_CANVAS_EXTEND_ACTION)
  const executionPrompt =
    prompt && execution.promptPrefix ? `${execution.promptPrefix} ${prompt}` : prompt
  const request = buildExecutionTaskRequest({
    prompt: executionPrompt,
    feature: execution.feature,
    imageUrls: [input.source.url],
    params: {
      ...buildExecutionSizeParams(
        imageSize,
        extendExecutionImageSize(imageSize, input.extendPercent),
      ),
      extend: {
        top_percent: input.extendPercent.top,
        right_percent: input.extendPercent.right,
        bottom_percent: input.extendPercent.bottom,
        left_percent: input.extendPercent.left,
      },
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交任务失败'))
  }
  return submitRes.data.task_id
}

export async function submitHdUpscaleTask(
  source: ImageDirectActionSource,
  loadImageSize: ImageSizeLoader,
  strength?: number,
): Promise<string> {
  const imageSize = await loadImageSize(source.url)
  const clarityStrength = resolveHdClarityStrength(strength)
  const execution = requireImageActionExecution(IMAGE_HD_UPSCALE_ACTION)
  if (!execution.prompt) {
    throw new Error('HD action execution prompt is not configured')
  }
  const request = buildExecutionTaskRequest({
    prompt: execution.prompt,
    feature: execution.feature,
    imageUrls: [source.url],
    params: {
      ...buildExecutionSizeParams(imageSize, imageSize),
      hd: {
        clarity_strength: clarityStrength,
      },
    },
  })

  const submitRes = await taskApi.submit(request)
  if (submitRes.code !== 0 || !submitRes.data?.task_id) {
    throw new Error(sanitizeApiErrorMessage(submitRes.msg, '提交 HD 任务失败'))
  }
  return submitRes.data.task_id
}

export async function waitExistingOrLiveTaskImageResults(
  taskId: string,
): Promise<TaskImageResult[]> {
  let task:
    | {
        status?: string
        outputs?: Array<{
          role?: string
          url?: string
          width?: number
          height?: number
          mime_type?: string
        }>
        result?: string | null
        error_msg?: string
        cancel_reason?: string
      }
    | undefined
  try {
    const taskRes = await taskApi.get(taskId)
    task = taskRes.data?.task
  } catch (error) {
    const status = Number((error as { status?: unknown; code?: unknown })?.status)
    const code = Number((error as { status?: unknown; code?: unknown })?.code)
    if (status === 404 || code === 404) throw error
    return waitTaskImageResultsWithSse(taskId)
  }
  if (task?.status === 'completed') {
    const results = extractImageResultsFromTaskPayload(task)
    if (results.length > 0) return results
    throw new Error('任务完成但未返回图片')
  }
  if (task?.status === 'failed') {
    throw new Error(sanitizeApiErrorMessage(task.error_msg, '生成失败'))
  }
  if (task?.status === 'cancelled') {
    throw createTaskCancelledError(task)
  }
  return waitTaskImageResultsWithSse(taskId)
}
